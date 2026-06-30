package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/entity"
	"cpbro-engine/internal/modules/cryptobroV3/usecase"
)

const (
	defaultPocketBaseReadTimeout  = 10 * time.Second
	defaultPocketBaseWriteTimeout = 15 * time.Second
	defaultPocketBaseSaveTimeout  = 20 * time.Second
)

// PocketBaseStorageService stores SignalJournal/WatchJournal + EvaluationReport into PocketBase collections:
// - signal_journals
// - watch_journals
// - evaluation_runs
//
// It delegates all other storage concerns to the fallback repository (typically JSONStorageService).
type PocketBaseStorageService struct {
	fallback usecase.StorageRepository
	client   *PocketBaseClient

	mu                sync.Mutex
	journalSourceMode string
}

const (
	journalSourcePocketBaseFirst = "pocketbase_first"
	journalSourceLocalFirst      = "local_first"
)

type pbListResponse struct {
	Page       int              `json:"page"`
	PerPage    int              `json:"perPage"`
	TotalItems int              `json:"totalItems"`
	TotalPages int              `json:"totalPages"`
	Items      []map[string]any `json:"items"`
	Raw        json.RawMessage  `json:"-"`
}

func NewPocketBaseStorageService(fallback usecase.StorageRepository, client *PocketBaseClient, sourceMode ...string) (*PocketBaseStorageService, error) {
	if fallback == nil {
		return nil, errors.New("fallback storage repo is nil")
	}
	if client == nil {
		return nil, errors.New("pocketbase client is nil")
	}
	mode := journalSourcePocketBaseFirst
	if len(sourceMode) > 0 {
		mode = normalizeJournalSourceMode(sourceMode[0])
	}
	return &PocketBaseStorageService{
		fallback:          fallback,
		client:            client,
		journalSourceMode: mode,
	}, nil
}

// --- Delegated methods ---

func (s *PocketBaseStorageService) LoadLatestResult() (*entity.LatestResult, error) {
	return s.fallback.LoadLatestResult()
}
func (s *PocketBaseStorageService) SaveLatestResult(res *entity.LatestResult) error {
	return s.fallback.SaveLatestResult(res)
}
func (s *PocketBaseStorageService) LoadSignalHistory() (*entity.SignalHistory, error) {
	return s.fallback.LoadSignalHistory()
}
func (s *PocketBaseStorageService) SaveSignalHistory(hist *entity.SignalHistory) error {
	return s.fallback.SaveSignalHistory(hist)
}
func (s *PocketBaseStorageService) LoadAIAuditCache() (*entity.AIAuditCache, error) {
	return s.fallback.LoadAIAuditCache()
}
func (s *PocketBaseStorageService) SaveAIAuditCache(cache *entity.AIAuditCache) error {
	return s.fallback.SaveAIAuditCache(cache)
}
func (s *PocketBaseStorageService) LoadDecisionAudits() ([]usecase.DecisionAudit, error) {
	return s.fallback.LoadDecisionAudits()
}
func (s *PocketBaseStorageService) SaveDecisionAudits(a []usecase.DecisionAudit) error {
	return s.fallback.SaveDecisionAudits(a)
}
func (s *PocketBaseStorageService) AppendDecisionAudit(e usecase.DecisionAudit) error {
	return s.fallback.AppendDecisionAudit(e)
}

func (s *PocketBaseStorageService) AppendDecisionAudits(entries []usecase.DecisionAudit) error {
	if appender, ok := s.fallback.(interface {
		AppendDecisionAudits([]usecase.DecisionAudit) error
	}); ok {
		return appender.AppendDecisionAudits(entries)
	}
	for _, entry := range entries {
		if err := s.fallback.AppendDecisionAudit(entry); err != nil {
			return err
		}
	}
	return nil
}

// --- Signal Journal ---

func (s *PocketBaseStorageService) LoadSignalJournal() ([]usecase.SignalJournal, error) {
	localJournal, localErr := s.fallback.LoadSignalJournal()
	if s.journalSourceMode == journalSourceLocalFirst && localErr == nil && len(localJournal) > 0 {
		return localJournal, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultPocketBaseReadTimeout)
	defer cancel()

	items, err := s.listAll(ctx, "signal_journals", url.Values{
		"perPage": []string{"200"},
		"sort":    []string{"-created_at"},
	})
	if err != nil {
		slog.Warn("PocketBase signal_journals read failed; falling back to JSON storage", "error", err)
		return s.fallback.LoadSignalJournal()
	}

	out := make([]usecase.SignalJournal, 0, len(items))
	for _, m := range items {
		j, err := decodeSignalJournal(m)
		if err != nil {
			// skip malformed rows instead of failing the entire read
			continue
		}
		out = append(out, j)
	}

	merged := mergeSignalJournalSources(out, localJournal)
	sort.Slice(merged, func(i, j int) bool { return merged[i].CreatedAt.After(merged[j].CreatedAt) })
	if len(merged) > 0 {
		if err := s.fallback.SaveSignalJournal(merged); err != nil {
			slog.Warn("JSON fallback signal_journals sync failed after PocketBase load", "error", err)
		}
	}
	if len(merged) == 0 {
		return []usecase.SignalJournal{}, nil
	}
	return merged, nil
}

func (s *PocketBaseStorageService) SaveSignalJournal(journal []usecase.SignalJournal) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.saveSignalJournalUnlocked(journal); err != nil {
		slog.Warn("PocketBase signal_journals save failed; writing JSON fallback", "error", err)
		return s.fallback.SaveSignalJournal(journal)
	}
	if err := s.fallback.SaveSignalJournal(journal); err != nil {
		slog.Warn("JSON fallback signal_journals mirror failed after PocketBase save", "error", err)
	}
	return nil
}

func (s *PocketBaseStorageService) saveSignalJournalUnlocked(journal []usecase.SignalJournal) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultPocketBaseSaveTimeout)
	defer cancel()

	existing, err := s.mapSignalIDToRecords(ctx)
	if err != nil {
		return err
	}

	for _, entry := range journal {
		if strings.TrimSpace(entry.ID) == "" {
			continue
		}
		payload := encodeSignalJournal(entry)

		var recID string
		var shouldUpdate = true
		if extRecord, ok := existing[entry.ID]; ok {
			recID, _ = extRecord["id"].(string)
			shouldUpdate = false

			// Compare critical fields
			if extStatus, _ := extRecord["status"].(string); extStatus != string(entry.Status) {
				shouldUpdate = true
			}
			if toFloat(extRecord["latest_price"]) != entry.LatestPrice {
				shouldUpdate = true
			}
			if toFloat(extRecord["pnl_percentage"]) != entry.PnlPercentage {
				shouldUpdate = true
			}
			if toFloat(extRecord["mfe"]) != entry.MFE || toFloat(extRecord["mae"]) != entry.MAE {
				shouldUpdate = true
			}
			if !parsePBTime(extRecord["expires_at"]).Equal(entry.ExpiresAt) {
				shouldUpdate = true
			}
			if !parsePBTime(extRecord["closed_at"]).Equal(entry.ClosedAt) {
				shouldUpdate = true
			}
			if extReason, _ := extRecord["outcome_reason"].(string); extReason != entry.OutcomeReason {
				shouldUpdate = true
			}
			if extT1, _ := extRecord["time_to_tp1"].(string); extT1 != entry.TimeToTP1 {
				shouldUpdate = true
			}
			if extT2, _ := extRecord["time_to_tp2"].(string); extT2 != entry.TimeToTP2 {
				shouldUpdate = true
			}
			if extTsl, _ := extRecord["time_to_sl"].(string); extTsl != entry.TimeToSL {
				shouldUpdate = true
			}
			if extPolicyLongMode, _ := extRecord["policy_long_mode"].(string); extPolicyLongMode != entry.PolicyLongMode {
				shouldUpdate = true
			}
			if extPolicyShortMode, _ := extRecord["policy_short_mode"].(string); extPolicyShortMode != entry.PolicyShortMode {
				shouldUpdate = true
			}
			if extPolicyAIConfidence, _ := extRecord["policy_require_ai_confidence"].(string); extPolicyAIConfidence != entry.PolicyRequireAIConfidence {
				shouldUpdate = true
			}
			if toBool(extRecord["policy_require_fresh_entry"]) != entry.PolicyRequireFreshEntry {
				shouldUpdate = true
			}
			if !sameStringSlice(toStringSlice(extRecord["policy_allowed_playbooks"]), entry.PolicyAllowedPlaybooks) {
				shouldUpdate = true
			}
			if extPolicyReason, _ := extRecord["policy_reason"].(string); extPolicyReason != entry.PolicyReason {
				shouldUpdate = true
			}
		}

		if !shouldUpdate {
			continue
		}

		if recID != "" {
			if err := s.client.doJSON(ctx, "PATCH", "/api/collections/signal_journals/records/"+recID, nil, payload, nil); err != nil {
				return err
			}
		} else {
			var created map[string]any
			err := s.client.doJSON(ctx, "POST", "/api/collections/signal_journals/records", nil, payload, &created)
			if err != nil {
				// retry as update if unique constraint hit
				recID, lookupErr := s.findSignalJournalRecordIDBySignalID(ctx, entry.ID)
				if lookupErr == nil && recID != "" {
					if patchErr := s.client.doJSON(ctx, "PATCH", "/api/collections/signal_journals/records/"+recID, nil, payload, nil); patchErr != nil {
						return patchErr
					}
					continue
				}
				return err
			}
			if id, _ := created["id"].(string); id != "" {
				if existing[entry.ID] == nil {
					existing[entry.ID] = make(map[string]any)
				}
				existing[entry.ID]["id"] = id
			}
		}
	}

	return nil
}

func (s *PocketBaseStorageService) AppendSignalJournal(entry usecase.SignalJournal) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), defaultPocketBaseWriteTimeout)
	defer cancel()

	if strings.TrimSpace(entry.ID) == "" {
		return errors.New("signal journal entry missing signal_id")
	}
	payload := encodeSignalJournal(entry)

	// Fast path: try create.
	if err := s.client.doJSON(ctx, "POST", "/api/collections/signal_journals/records", nil, payload, nil); err == nil {
		if err := s.upsertFallbackSignalJournal(entry); err != nil {
			slog.Warn("JSON fallback signal_journals mirror failed after PocketBase append", "error", err)
		}
		return nil
	} else {
		slog.Warn("PocketBase signal_journals append create failed; trying update or fallback", "error", err)
	}

	// If already exists, update.
	recID, err := s.findSignalJournalRecordIDBySignalID(ctx, entry.ID)
	if err != nil || recID == "" {
		slog.Warn("PocketBase signal_journals append lookup failed; writing JSON fallback", "error", err)
		return s.upsertFallbackSignalJournal(entry)
	}
	if err := s.client.doJSON(ctx, "PATCH", "/api/collections/signal_journals/records/"+recID, nil, payload, nil); err != nil {
		slog.Warn("PocketBase signal_journals append update failed; writing JSON fallback", "error", err)
		return s.upsertFallbackSignalJournal(entry)
	}
	if err := s.upsertFallbackSignalJournal(entry); err != nil {
		slog.Warn("JSON fallback signal_journals mirror failed after PocketBase append update", "error", err)
	}
	return nil
}

// UpdateSignalJournal implements atomic read-modify-write semantics needed by MonitoringUsecase.
func (s *PocketBaseStorageService) UpdateSignalJournal(update func([]usecase.SignalJournal) ([]usecase.SignalJournal, error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, err := s.LoadSignalJournal()
	if err != nil {
		slog.Warn("PocketBase signal_journals update load failed; using JSON fallback", "error", err)
		return s.updateFallbackSignalJournal(update)
	}
	updated, err := update(current)
	if err != nil {
		return err
	}
	if updated == nil {
		updated = []usecase.SignalJournal{}
	}
	if err := s.saveSignalJournalUnlocked(updated); err != nil {
		slog.Warn("PocketBase signal_journals update save failed; writing JSON fallback", "error", err)
		return s.fallback.SaveSignalJournal(updated)
	}
	if err := s.fallback.SaveSignalJournal(updated); err != nil {
		slog.Warn("JSON fallback signal_journals mirror failed after PocketBase update", "error", err)
	}
	return nil
}

func (s *PocketBaseStorageService) UpsertSignalJournalEntries(entries []usecase.SignalJournal) error {
	if len(entries) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.upsertJournalEntriesUnlocked("signal_journals", entries); err != nil {
		slog.Warn("PocketBase signal_journals partial upsert failed; writing JSON fallback", "error", err)
		return s.upsertFallbackSignalJournalEntries(entries)
	}
	if err := s.upsertFallbackSignalJournalEntries(entries); err != nil {
		slog.Warn("JSON fallback signal_journals partial mirror failed after PocketBase upsert", "error", err)
	}
	return nil
}

// --- Watch Journal ---

func (s *PocketBaseStorageService) LoadWatchJournal() ([]usecase.WatchJournal, error) {
	localJournal, localErr := s.fallback.LoadWatchJournal()
	if s.journalSourceMode == journalSourceLocalFirst && localErr == nil && len(localJournal) > 0 {
		return localJournal, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultPocketBaseReadTimeout)
	defer cancel()

	items, err := s.listAll(ctx, "watch_journals", url.Values{
		"perPage": []string{"200"},
		"sort":    []string{"-created_at"},
	})
	if err != nil {
		slog.Warn("PocketBase watch_journals read failed; falling back to JSON storage", "error", err)
		return s.fallback.LoadWatchJournal()
	}

	out := make([]usecase.WatchJournal, 0, len(items))
	for _, m := range items {
		j, err := decodeSignalJournal(m)
		if err != nil {
			continue
		}
		out = append(out, j)
	}
	if localErr == nil && len(localJournal) > 0 {
		out = mergeWatchJournalSources(out, localJournal)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if len(out) > 0 {
		if err := s.fallback.SaveWatchJournal(out); err != nil {
			slog.Warn("JSON fallback watch_journals sync failed after PocketBase load", "error", err)
		}
	}
	if len(out) == 0 {
		return []usecase.WatchJournal{}, nil
	}
	return out, nil
}

func (s *PocketBaseStorageService) FindWatchJournalCandidates(probe usecase.WatchJournal) ([]usecase.WatchJournal, error) {
	if s.journalSourceMode == journalSourceLocalFirst {
		localJournal, err := s.fallback.LoadWatchJournal()
		if err == nil && len(localJournal) > 0 {
			return filterMatchingWatchJournalCandidates(localJournal, probe), nil
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultPocketBaseReadTimeout)
	defer cancel()

	q := url.Values{}
	q.Set("perPage", "50")
	q.Set("page", "1")
	q.Set("sort", "-updated_at,-created_at")
	q.Set("filter", fmt.Sprintf(
		"symbol='%s' && direction='%s' && playbook='%s'",
		escapePBFilterValue(probe.Symbol),
		escapePBFilterValue(string(probe.Direction)),
		escapePBFilterValue(string(probe.Playbook)),
	))

	var resp pbListResponse
	if err := s.client.doJSON(ctx, "GET", "/api/collections/watch_journals/records", q, nil, &resp); err != nil {
		slog.Warn("PocketBase watch_journals candidate lookup failed; falling back to JSON storage", "error", err)
		localJournal, localErr := s.fallback.LoadWatchJournal()
		if localErr != nil {
			return nil, localErr
		}
		return filterMatchingWatchJournalCandidates(localJournal, probe), nil
	}

	out := make([]usecase.WatchJournal, 0, len(resp.Items))
	for _, m := range resp.Items {
		j, err := decodeSignalJournal(m)
		if err != nil {
			continue
		}
		out = append(out, usecase.WatchJournal(j))
	}
	localJournal, localErr := s.fallback.LoadWatchJournal()
	if localErr == nil && len(localJournal) > 0 {
		out = filterMatchingWatchJournalCandidates(mergeWatchJournalSources(out, localJournal), probe)
	}

	sort.Slice(out, func(i, j int) bool {
		if !out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].UpdatedAt.After(out[j].UpdatedAt)
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})

	return out, nil
}

func (s *PocketBaseStorageService) SaveWatchJournal(journal []usecase.WatchJournal) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.saveJournalUnlocked("watch_journals", journal); err != nil {
		slog.Warn("PocketBase watch_journals save failed; writing JSON fallback", "error", err)
		return s.fallback.SaveWatchJournal(journal)
	}
	if err := s.fallback.SaveWatchJournal(journal); err != nil {
		slog.Warn("JSON fallback watch_journals mirror failed after PocketBase save", "error", err)
	}
	return nil
}

func (s *PocketBaseStorageService) AppendWatchJournal(entry usecase.WatchJournal) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), defaultPocketBaseWriteTimeout)
	defer cancel()

	if strings.TrimSpace(entry.ID) == "" {
		return errors.New("watch journal entry missing signal_id")
	}
	payload := encodeSignalJournal(entry)

	if err := s.client.doJSON(ctx, "POST", "/api/collections/watch_journals/records", nil, payload, nil); err == nil {
		if err := s.upsertFallbackWatchJournal(entry); err != nil {
			slog.Warn("JSON fallback watch_journals mirror failed after PocketBase append", "error", err)
		}
		return nil
	} else {
		slog.Warn("PocketBase watch_journals append create failed; trying update or fallback", "error", err)
	}

	recID, err := s.findJournalRecordIDBySignalID(ctx, "watch_journals", entry.ID)
	if err != nil || recID == "" {
		slog.Warn("PocketBase watch_journals append lookup failed; writing JSON fallback", "error", err)
		return s.upsertFallbackWatchJournal(entry)
	}
	if err := s.client.doJSON(ctx, "PATCH", "/api/collections/watch_journals/records/"+recID, nil, payload, nil); err != nil {
		slog.Warn("PocketBase watch_journals append update failed; writing JSON fallback", "error", err)
		return s.upsertFallbackWatchJournal(entry)
	}
	if err := s.upsertFallbackWatchJournal(entry); err != nil {
		slog.Warn("JSON fallback watch_journals mirror failed after PocketBase append update", "error", err)
	}
	return nil
}

func (s *PocketBaseStorageService) UpdateWatchJournal(update func([]usecase.WatchJournal) ([]usecase.WatchJournal, error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, err := s.LoadWatchJournal()
	if err != nil {
		slog.Warn("PocketBase watch_journals update load failed; using JSON fallback", "error", err)
		return s.updateFallbackWatchJournal(update)
	}
	updated, err := update(current)
	if err != nil {
		return err
	}
	if updated == nil {
		updated = []usecase.WatchJournal{}
	}
	if err := s.saveJournalUnlocked("watch_journals", updated); err != nil {
		slog.Warn("PocketBase watch_journals update save failed; writing JSON fallback", "error", err)
		return s.fallback.SaveWatchJournal(updated)
	}
	if err := s.fallback.SaveWatchJournal(updated); err != nil {
		slog.Warn("JSON fallback watch_journals mirror failed after PocketBase update", "error", err)
	}
	return nil
}

func (s *PocketBaseStorageService) UpsertWatchJournalEntries(entries []usecase.WatchJournal) error {
	if len(entries) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	signalEntries := make([]usecase.SignalJournal, len(entries))
	for i, entry := range entries {
		signalEntries[i] = usecase.SignalJournal(entry)
	}

	if err := s.upsertJournalEntriesUnlocked("watch_journals", signalEntries); err != nil {
		slog.Warn("PocketBase watch_journals partial upsert failed; writing JSON fallback", "error", err)
		return s.upsertFallbackWatchJournalEntries(entries)
	}
	if err := s.upsertFallbackWatchJournalEntries(entries); err != nil {
		slog.Warn("JSON fallback watch_journals partial mirror failed after PocketBase upsert", "error", err)
	}
	return nil
}

// --- Evaluation ---

func (s *PocketBaseStorageService) LoadEvaluationReport() (*usecase.EvaluationReport, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultPocketBaseReadTimeout)
	defer cancel()

	var list pbListResponse
	q := url.Values{}
	q.Set("perPage", "1")
	q.Set("page", "1")
	q.Set("sort", "-generated_at")
	if err := s.client.doJSON(ctx, "GET", "/api/collections/evaluation_runs/records", q, nil, &list); err != nil {
		slog.Warn("PocketBase evaluation_runs read failed; falling back to JSON storage", "error", err)
		return s.fallback.LoadEvaluationReport()
	}
	if len(list.Items) == 0 {
		return s.fallback.LoadEvaluationReport()
	}
	report, err := decodeEvaluationRun(list.Items[0])
	if err != nil {
		return nil, err
	}
	return report, nil
}

// SaveEvaluationReport persists an immutable run record to evaluation_runs (row-per-run is recommended).
func (s *PocketBaseStorageService) SaveEvaluationReport(report *usecase.EvaluationReport) error {
	if report == nil {
		return errors.New("evaluation report is nil")
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultPocketBaseSaveTimeout)
	defer cancel()

	evalID := makeEvaluationID(report.GeneratedAt)
	payload := encodeEvaluationRun(evalID, report)
	if err := s.client.doJSON(ctx, "POST", "/api/collections/evaluation_runs/records", nil, payload, nil); err != nil {
		slog.Warn("PocketBase evaluation_runs save failed; writing JSON fallback", "error", err)
		return s.fallback.SaveEvaluationReport(report)
	}
	if err := s.fallback.SaveEvaluationReport(report); err != nil {
		slog.Warn("JSON fallback evaluation_report mirror failed after PocketBase save", "error", err)
	}
	return nil
}

// --- Helpers ---

func (s *PocketBaseStorageService) listAll(ctx context.Context, collection string, baseQuery url.Values) ([]map[string]any, error) {
	page := 1
	perPage := 200
	if baseQuery == nil {
		baseQuery = url.Values{}
	}
	if v := baseQuery.Get("perPage"); v != "" {
		fmt.Sscanf(v, "%d", &perPage)
	}

	var out []map[string]any
	for {
		q := cloneValues(baseQuery)
		q.Set("page", fmt.Sprintf("%d", page))
		q.Set("perPage", fmt.Sprintf("%d", perPage))

		var resp pbListResponse
		if err := s.client.doJSON(ctx, "GET", "/api/collections/"+collection+"/records", q, nil, &resp); err != nil {
			return nil, err
		}
		out = append(out, resp.Items...)
		if resp.TotalPages <= 0 || page >= resp.TotalPages {
			break
		}
		page++
	}
	return out, nil
}

func normalizeJournalSourceMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case journalSourceLocalFirst, "local_mirror_first":
		return journalSourceLocalFirst
	default:
		return journalSourcePocketBaseFirst
	}
}

func (s *PocketBaseStorageService) updateFallbackSignalJournal(update func([]usecase.SignalJournal) ([]usecase.SignalJournal, error)) error {
	current, err := s.fallback.LoadSignalJournal()
	if err != nil {
		return err
	}
	updated, err := update(current)
	if err != nil {
		return err
	}
	if updated == nil {
		updated = []usecase.SignalJournal{}
	}
	return s.fallback.SaveSignalJournal(updated)
}

func (s *PocketBaseStorageService) updateFallbackWatchJournal(update func([]usecase.WatchJournal) ([]usecase.WatchJournal, error)) error {
	current, err := s.fallback.LoadWatchJournal()
	if err != nil {
		return err
	}
	updated, err := update(current)
	if err != nil {
		return err
	}
	if updated == nil {
		updated = []usecase.WatchJournal{}
	}
	return s.fallback.SaveWatchJournal(updated)
}

func (s *PocketBaseStorageService) upsertFallbackSignalJournal(entry usecase.SignalJournal) error {
	return s.updateFallbackSignalJournal(func(current []usecase.SignalJournal) ([]usecase.SignalJournal, error) {
		for i := range current {
			if strings.TrimSpace(current[i].ID) == strings.TrimSpace(entry.ID) {
				current[i] = entry
				return current, nil
			}
		}
		return append(current, entry), nil
	})
}

func (s *PocketBaseStorageService) upsertFallbackSignalJournalEntries(entries []usecase.SignalJournal) error {
	if len(entries) == 0 {
		return nil
	}
	if upserter, ok := s.fallback.(interface {
		UpsertSignalJournalEntries([]usecase.SignalJournal) error
	}); ok {
		return upserter.UpsertSignalJournalEntries(entries)
	}
	return s.updateFallbackSignalJournal(func(current []usecase.SignalJournal) ([]usecase.SignalJournal, error) {
		for _, entry := range entries {
			replaced := false
			for i := range current {
				if strings.TrimSpace(current[i].ID) == strings.TrimSpace(entry.ID) && strings.TrimSpace(entry.ID) != "" {
					current[i] = entry
					replaced = true
					break
				}
			}
			if !replaced {
				current = append(current, entry)
			}
		}
		return current, nil
	})
}

func (s *PocketBaseStorageService) upsertFallbackWatchJournal(entry usecase.WatchJournal) error {
	return s.updateFallbackWatchJournal(func(current []usecase.WatchJournal) ([]usecase.WatchJournal, error) {
		for i := range current {
			if strings.TrimSpace(current[i].ID) == strings.TrimSpace(entry.ID) {
				current[i] = entry
				return current, nil
			}
		}
		return append(current, entry), nil
	})
}

func (s *PocketBaseStorageService) upsertFallbackWatchJournalEntries(entries []usecase.WatchJournal) error {
	if len(entries) == 0 {
		return nil
	}
	if upserter, ok := s.fallback.(interface {
		UpsertWatchJournalEntries([]usecase.WatchJournal) error
	}); ok {
		return upserter.UpsertWatchJournalEntries(entries)
	}
	return s.updateFallbackWatchJournal(func(current []usecase.WatchJournal) ([]usecase.WatchJournal, error) {
		for _, entry := range entries {
			replaced := false
			for i := range current {
				if strings.TrimSpace(current[i].ID) == strings.TrimSpace(entry.ID) && strings.TrimSpace(entry.ID) != "" {
					current[i] = entry
					replaced = true
					break
				}
			}
			if !replaced {
				current = append(current, entry)
			}
		}
		return current, nil
	})
}

func cloneValues(v url.Values) url.Values {
	out := url.Values{}
	for k, vals := range v {
		cp := make([]string, len(vals))
		copy(cp, vals)
		out[k] = cp
	}
	return out
}

func mergeSignalJournalSources(primary, secondary []usecase.SignalJournal) []usecase.SignalJournal {
	if len(primary) == 0 {
		return append([]usecase.SignalJournal(nil), secondary...)
	}
	if len(secondary) == 0 {
		return append([]usecase.SignalJournal(nil), primary...)
	}

	merged := make(map[string]usecase.SignalJournal, len(primary)+len(secondary))
	for _, entry := range primary {
		merged[signalJournalMergeKey(entry)] = entry
	}
	for _, entry := range secondary {
		key := signalJournalMergeKey(entry)
		existing, ok := merged[key]
		if !ok || shouldPreferLocalSignalJournal(existing, entry) {
			merged[key] = entry
		}
	}

	out := make([]usecase.SignalJournal, 0, len(merged))
	for _, entry := range merged {
		out = append(out, entry)
	}
	return out
}

func signalJournalMergeKey(entry usecase.SignalJournal) string {
	if id := strings.TrimSpace(entry.ID); id != "" {
		return "id:" + id
	}
	return strings.Join([]string{
		strings.TrimSpace(entry.Symbol),
		string(entry.Direction),
		string(entry.Playbook),
		entry.CreatedAt.UTC().Format(time.RFC3339Nano),
	}, "|")
}

func shouldPreferLocalSignalJournal(remote, local usecase.SignalJournal) bool {
	if hasSignalJournalPolicySnapshot(local) && !hasSignalJournalPolicySnapshot(remote) {
		return true
	}
	if isTerminalSignalJournalStatus(local.Status) && !isTerminalSignalJournalStatus(remote.Status) {
		return true
	}
	if local.ClosedAt.After(remote.ClosedAt) {
		return true
	}
	if local.UpdatedAt.After(remote.UpdatedAt) {
		return true
	}
	if local.Status != remote.Status && local.CreatedAt.After(remote.CreatedAt) {
		return true
	}
	return false
}

func hasSignalJournalPolicySnapshot(entry usecase.SignalJournal) bool {
	return strings.TrimSpace(entry.PolicyLongMode) != "" ||
		strings.TrimSpace(entry.PolicyShortMode) != "" ||
		strings.TrimSpace(entry.PolicyRequireAIConfidence) != "" ||
		len(entry.PolicyAllowedPlaybooks) > 0 ||
		strings.TrimSpace(entry.PolicyReason) != ""
}

func isTerminalSignalJournalStatus(status usecase.Status) bool {
	switch status {
	case usecase.TP1_HIT,
		usecase.TP2_HIT,
		usecase.SL_HIT,
		usecase.EXPIRED:
		return true
	default:
		return false
	}
}

func mergeWatchJournalSources(primary, secondary []usecase.WatchJournal) []usecase.WatchJournal {
	if len(primary) == 0 {
		return append([]usecase.WatchJournal(nil), secondary...)
	}
	if len(secondary) == 0 {
		return append([]usecase.WatchJournal(nil), primary...)
	}

	merged := make(map[string]usecase.WatchJournal, len(primary)+len(secondary))
	for _, entry := range primary {
		merged[watchJournalMergeKey(entry)] = entry
	}
	for _, entry := range secondary {
		key := watchJournalMergeKey(entry)
		existing, ok := merged[key]
		if !ok || shouldPreferLocalWatchJournal(existing, entry) {
			merged[key] = entry
		}
	}

	out := make([]usecase.WatchJournal, 0, len(merged))
	for _, entry := range merged {
		out = append(out, entry)
	}
	return out
}

func watchJournalMergeKey(entry usecase.WatchJournal) string {
	if id := strings.TrimSpace(entry.ID); id != "" {
		return "id:" + id
	}
	return strings.Join([]string{
		strings.TrimSpace(entry.Symbol),
		string(entry.Direction),
		string(entry.Playbook),
		entry.CreatedAt.UTC().Format(time.RFC3339Nano),
	}, "|")
}

func shouldPreferLocalWatchJournal(remote, local usecase.WatchJournal) bool {
	if isTerminalWatchJournalStatus(local.Status) && !isTerminalWatchJournalStatus(remote.Status) {
		return true
	}
	if local.ClosedAt.After(remote.ClosedAt) {
		return true
	}
	if local.UpdatedAt.After(remote.UpdatedAt) {
		return true
	}
	if local.Status != remote.Status && local.CreatedAt.After(remote.CreatedAt) {
		return true
	}
	return false
}

func isTerminalWatchJournalStatus(status usecase.Status) bool {
	switch status {
	case usecase.VIRTUAL_TP2_HIT,
		usecase.VIRTUAL_SL_HIT,
		usecase.VIRTUAL_EXPIRED,
		usecase.WATCH_PROMOTED,
		usecase.WATCH_INVALIDATED,
		usecase.WATCH_EXPIRED:
		return true
	default:
		return false
	}
}

func (s *PocketBaseStorageService) saveJournalUnlocked(collection string, journal []usecase.SignalJournal) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultPocketBaseSaveTimeout)
	defer cancel()

	existing, err := s.mapSignalIDToRecordsForCollection(ctx, collection)
	if err != nil {
		return err
	}

	for _, entry := range journal {
		if strings.TrimSpace(entry.ID) == "" {
			continue
		}
		payload := encodeSignalJournal(entry)

		var recID string
		var shouldUpdate = true
		if extRecord, ok := existing[entry.ID]; ok {
			recID, _ = extRecord["id"].(string)
			shouldUpdate = false

			if extStatus, _ := extRecord["status"].(string); extStatus != string(entry.Status) {
				shouldUpdate = true
			}
			if toFloat(extRecord["latest_price"]) != entry.LatestPrice {
				shouldUpdate = true
			}
			if toFloat(extRecord["pnl_percentage"]) != entry.PnlPercentage {
				shouldUpdate = true
			}
			if toFloat(extRecord["mfe"]) != entry.MFE || toFloat(extRecord["mae"]) != entry.MAE {
				shouldUpdate = true
			}
			if !parsePBTime(extRecord["expires_at"]).Equal(entry.ExpiresAt) {
				shouldUpdate = true
			}
			if !parsePBTime(extRecord["closed_at"]).Equal(entry.ClosedAt) {
				shouldUpdate = true
			}
			if extReason, _ := extRecord["outcome_reason"].(string); extReason != entry.OutcomeReason {
				shouldUpdate = true
			}
			if extT1, _ := extRecord["time_to_tp1"].(string); extT1 != entry.TimeToTP1 {
				shouldUpdate = true
			}
			if extT2, _ := extRecord["time_to_tp2"].(string); extT2 != entry.TimeToTP2 {
				shouldUpdate = true
			}
			if extTsl, _ := extRecord["time_to_sl"].(string); extTsl != entry.TimeToSL {
				shouldUpdate = true
			}
			if extPolicyLongMode, _ := extRecord["policy_long_mode"].(string); extPolicyLongMode != entry.PolicyLongMode {
				shouldUpdate = true
			}
			if extPolicyShortMode, _ := extRecord["policy_short_mode"].(string); extPolicyShortMode != entry.PolicyShortMode {
				shouldUpdate = true
			}
			if extPolicyAIConfidence, _ := extRecord["policy_require_ai_confidence"].(string); extPolicyAIConfidence != entry.PolicyRequireAIConfidence {
				shouldUpdate = true
			}
			if toBool(extRecord["policy_require_fresh_entry"]) != entry.PolicyRequireFreshEntry {
				shouldUpdate = true
			}
			if !sameStringSlice(toStringSlice(extRecord["policy_allowed_playbooks"]), entry.PolicyAllowedPlaybooks) {
				shouldUpdate = true
			}
			if extPolicyReason, _ := extRecord["policy_reason"].(string); extPolicyReason != entry.PolicyReason {
				shouldUpdate = true
			}
		}

		if !shouldUpdate {
			continue
		}

		if recID != "" {
			if err := s.client.doJSON(ctx, "PATCH", "/api/collections/"+collection+"/records/"+recID, nil, payload, nil); err != nil {
				return err
			}
		} else {
			var created map[string]any
			err := s.client.doJSON(ctx, "POST", "/api/collections/"+collection+"/records", nil, payload, &created)
			if err != nil {
				recID, lookupErr := s.findJournalRecordIDBySignalID(ctx, collection, entry.ID)
				if lookupErr == nil && recID != "" {
					if patchErr := s.client.doJSON(ctx, "PATCH", "/api/collections/"+collection+"/records/"+recID, nil, payload, nil); patchErr != nil {
						return patchErr
					}
					continue
				}
				return err
			}
			if id, _ := created["id"].(string); id != "" {
				if existing[entry.ID] == nil {
					existing[entry.ID] = make(map[string]any)
				}
				existing[entry.ID]["id"] = id
			}
		}
	}

	return nil
}

func (s *PocketBaseStorageService) upsertJournalEntriesUnlocked(collection string, entries []usecase.SignalJournal) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultPocketBaseSaveTimeout)
	defer cancel()

	for _, entry := range entries {
		if strings.TrimSpace(entry.ID) == "" {
			continue
		}
		payload := encodeSignalJournal(entry)
		recID, err := s.findJournalRecordIDBySignalID(ctx, collection, entry.ID)
		if err != nil {
			return err
		}
		if recID != "" {
			if err := s.client.doJSON(ctx, "PATCH", "/api/collections/"+collection+"/records/"+recID, nil, payload, nil); err != nil {
				return err
			}
			continue
		}

		if err := s.client.doJSON(ctx, "POST", "/api/collections/"+collection+"/records", nil, payload, nil); err != nil {
			recID, lookupErr := s.findJournalRecordIDBySignalID(ctx, collection, entry.ID)
			if lookupErr == nil && recID != "" {
				if patchErr := s.client.doJSON(ctx, "PATCH", "/api/collections/"+collection+"/records/"+recID, nil, payload, nil); patchErr != nil {
					return patchErr
				}
				continue
			}
			return err
		}
	}

	return nil
}

func (s *PocketBaseStorageService) mapSignalIDToRecordID(ctx context.Context) (map[string]string, error) {
	items, err := s.listAll(ctx, "signal_journals", url.Values{
		"perPage": []string{"200"},
	})
	if err != nil {
		return nil, err
	}
	m := make(map[string]string, len(items))
	for _, it := range items {
		sigID, _ := it["signal_id"].(string)
		recID, _ := it["id"].(string)
		if sigID != "" && recID != "" {
			m[sigID] = recID
		}
	}
	return m, nil
}

func (s *PocketBaseStorageService) mapSignalIDToRecordsForCollection(ctx context.Context, collection string) (map[string]map[string]any, error) {
	items, err := s.listAll(ctx, collection, url.Values{
		"perPage": []string{"200"},
	})
	if err != nil {
		return nil, err
	}
	m := make(map[string]map[string]any, len(items))
	for _, it := range items {
		sigID, _ := it["signal_id"].(string)
		if sigID != "" {
			m[sigID] = it
		}
	}
	return m, nil
}

func (s *PocketBaseStorageService) mapSignalIDToRecords(ctx context.Context) (map[string]map[string]any, error) {
	return s.mapSignalIDToRecordsForCollection(ctx, "signal_journals")
}

func (s *PocketBaseStorageService) findSignalJournalRecordIDBySignalID(ctx context.Context, signalID string) (string, error) {
	return s.findJournalRecordIDBySignalID(ctx, "signal_journals", signalID)
}

func (s *PocketBaseStorageService) findJournalRecordIDBySignalID(ctx context.Context, collection string, signalID string) (string, error) {
	q := url.Values{}
	q.Set("perPage", "1")
	q.Set("page", "1")
	q.Set("filter", fmt.Sprintf("signal_id='%s'", escapePBFilterValue(signalID)))

	var resp pbListResponse
	if err := s.client.doJSON(ctx, "GET", "/api/collections/"+collection+"/records", q, nil, &resp); err != nil {
		return "", err
	}
	if len(resp.Items) == 0 {
		return "", nil
	}
	recID, _ := resp.Items[0]["id"].(string)
	return recID, nil
}

func escapePBFilterValue(v string) string {
	return strings.ReplaceAll(v, "'", "\\'")
}

func makeEvaluationID(t time.Time) string {
	ts := t
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	buf := make([]byte, 5)
	_, _ = rand.Read(buf)
	return fmt.Sprintf("eval_%s_%s", ts.UTC().Format("20060102150405"), hex.EncodeToString(buf))
}

func encodeSignalJournal(e usecase.SignalJournal) map[string]any {
	out := map[string]any{
		"schema_version":               e.SchemaVersion,
		"config_version":               e.ConfigVersion,
		"signal_id":                    e.ID,
		"symbol":                       e.Symbol,
		"direction":                    string(e.Direction),
		"playbook":                     string(e.Playbook),
		"entry":                        e.EntryPrice,
		"sl":                           e.StopLoss,
		"original_sl":                  e.OriginalStopLoss,
		"tp1":                          e.TP1,
		"tp2":                          e.TP2,
		"rr":                           e.RR,
		"score":                        e.QuantScore,
		"ai_confidence":                e.AIConfidence,
		"market_regime":                e.MarketRegime,
		"policy_mode":                  e.PolicyMode,
		"policy_long_mode":             e.PolicyLongMode,
		"policy_short_mode":            e.PolicyShortMode,
		"policy_require_ai_confidence": e.PolicyRequireAIConfidence,
		"policy_require_fresh_entry":   e.PolicyRequireFreshEntry,
		"policy_allowed_playbooks":     append([]string(nil), e.PolicyAllowedPlaybooks...),
		"policy_reason":                e.PolicyReason,
		"threshold_profile_summary":    e.ThresholdProfileSummary,
		"breakout_level":               e.BreakoutLevel,
		"retest_touches":               e.RetestTouches,
		"retest_hold":                  e.RetestHold,
		"has_derivatives_evidence":     e.HasDerivativesEvidence,
		"created_at":                   formatPBTime(e.CreatedAt),
		"expires_at":                   formatPBTime(e.ExpiresAt),
		"status":                       string(e.Status),
		"mfe":                          e.MFE,
		"mae":                          e.MAE,
		"time_to_tp1":                  e.TimeToTP1,
		"time_to_tp2":                  e.TimeToTP2,
		"time_to_sl":                   e.TimeToSL,
		"outcome_reason":               e.OutcomeReason,
		"entry_timing":                 e.EntryTiming,
		"tier":                         string(e.Tier),
		"timeframe":                    e.Timeframe,
		"latest_price":                 e.LatestPrice,
		"take_profit":                  e.TakeProfit,
		"ai_sentiment":                 e.AISentiment,
		"ai_reasoning":                 e.AIReasoning,
		"pnl_percentage":               e.PnlPercentage,
		"updated_at":                   formatPBTime(e.UpdatedAt),
		"closed_at":                    formatPBTime(e.ClosedAt),
		"reason":                       e.Reason,
		"notification_status":          e.NotificationStatus,
		"notification_error":           e.NotificationError,
		"technical_snapshot":           e.TechnicalSnapshot,
		"structure_snapshot":           e.StructureSnapshot,
	}
	if e.IsHot {
		out["hot_info"] = map[string]any{
			"is_hot":               true,
			"hot_score":            e.HotScore,
			"hot_source":           e.HotSource,
			"hot_rank_type":        e.HotRankType,
			"hot_overlay_selected": e.HotOverlaySelected,
		}
	} else {
		out["hot_info"] = nil
	}
	// Remove empty string fields for cleanliness (PB accepts empty, but keep payload small).
	for k, v := range out {
		if s, ok := v.(string); ok && strings.TrimSpace(s) == "" {
			delete(out, k)
		}
	}
	return out
}

func decodeSignalJournal(m map[string]any) (usecase.SignalJournal, error) {
	var out usecase.SignalJournal
	out.SchemaVersion, _ = m["schema_version"].(string)
	out.ConfigVersion, _ = m["config_version"].(string)
	out.ID, _ = m["signal_id"].(string)
	out.Symbol, _ = m["symbol"].(string)
	if v, ok := m["direction"].(string); ok {
		out.Direction = usecase.Direction(v)
	}
	if v, ok := m["playbook"].(string); ok {
		out.Playbook = usecase.Playbook(v)
	}
	out.EntryPrice = toFloat(m["entry"])
	out.StopLoss = toFloat(m["sl"])
	out.OriginalStopLoss = toFloat(m["original_sl"])
	out.TP1 = toFloat(m["tp1"])
	out.TP2 = toFloat(m["tp2"])
	out.RR = toFloat(m["rr"])
	out.QuantScore = toFloat(m["score"])
	out.AIConfidence, _ = m["ai_confidence"].(string)
	out.MarketRegime, _ = m["market_regime"].(string)
	out.PolicyMode, _ = m["policy_mode"].(string)
	out.PolicyLongMode, _ = m["policy_long_mode"].(string)
	out.PolicyShortMode, _ = m["policy_short_mode"].(string)
	out.PolicyRequireAIConfidence, _ = m["policy_require_ai_confidence"].(string)
	out.PolicyRequireFreshEntry = toBool(m["policy_require_fresh_entry"])
	if values := toStringSlice(m["policy_allowed_playbooks"]); len(values) > 0 {
		out.PolicyAllowedPlaybooks = values
	}
	out.PolicyReason, _ = m["policy_reason"].(string)
	out.ThresholdProfileSummary, _ = m["threshold_profile_summary"].(string)
	out.BreakoutLevel = toFloat(m["breakout_level"])
	out.RetestTouches = toFloat(m["retest_touches"])
	out.RetestHold = toBool(m["retest_hold"])
	out.HasDerivativesEvidence = toBool(m["has_derivatives_evidence"])
	out.CreatedAt = parsePBTime(m["created_at"])
	out.ExpiresAt = parsePBTime(m["expires_at"])
	if v, ok := m["status"].(string); ok {
		out.Status = usecase.Status(v)
	}
	out.MFE = toFloat(m["mfe"])
	out.MAE = toFloat(m["mae"])
	out.TimeToTP1, _ = m["time_to_tp1"].(string)
	out.TimeToTP2, _ = m["time_to_tp2"].(string)
	out.TimeToSL, _ = m["time_to_sl"].(string)
	out.OutcomeReason, _ = m["outcome_reason"].(string)
	out.EntryTiming, _ = m["entry_timing"].(string)
	if v, ok := m["tier"].(string); ok {
		out.Tier = usecase.Tier(v)
	}

	out.Timeframe, _ = m["timeframe"].(string)
	out.LatestPrice = toFloat(m["latest_price"])
	out.TakeProfit = toFloat(m["take_profit"])
	out.AISentiment, _ = m["ai_sentiment"].(string)
	out.AIReasoning, _ = m["ai_reasoning"].(string)
	out.PnlPercentage = toFloat(m["pnl_percentage"])
	out.UpdatedAt = parsePBTime(m["updated_at"])
	out.ClosedAt = parsePBTime(m["closed_at"])
	out.Reason, _ = m["reason"].(string)
	out.NotificationStatus, _ = m["notification_status"].(string)
	out.NotificationError, _ = m["notification_error"].(string)

	if hotVal, ok := m["hot_info"]; ok && hotVal != nil {
		switch v := hotVal.(type) {
		case map[string]any:
			out.IsHot, _ = v["is_hot"].(bool)
			out.HotScore = toFloat(v["hot_score"])
			out.HotSource, _ = v["hot_source"].(string)
			if rt, ok := v["hot_rank_type"].(float64); ok {
				out.HotRankType = int(rt)
			}
			out.HotOverlaySelected, _ = v["hot_overlay_selected"].(bool)
		case string:
			if strings.TrimSpace(v) != "" {
				var m2 map[string]any
				if err := json.Unmarshal([]byte(v), &m2); err == nil {
					out.IsHot, _ = m2["is_hot"].(bool)
					out.HotScore = toFloat(m2["hot_score"])
					out.HotSource, _ = m2["hot_source"].(string)
					if rt, ok := m2["hot_rank_type"].(float64); ok {
						out.HotRankType = int(rt)
					}
					out.HotOverlaySelected, _ = m2["hot_overlay_selected"].(bool)
				}
			}
		}
	}

	decodeSnapshot(m["technical_snapshot"], &out.TechnicalSnapshot)
	decodeSnapshot(m["structure_snapshot"], &out.StructureSnapshot)

	if strings.TrimSpace(out.ID) == "" || strings.TrimSpace(out.Symbol) == "" {
		return usecase.SignalJournal{}, errors.New("missing required journal fields")
	}
	return out, nil
}

func decodeSnapshot[T any](val any, target *T) {
	if val == nil {
		return
	}
	switch v := val.(type) {
	case string:
		if strings.TrimSpace(v) != "" {
			_ = json.Unmarshal([]byte(v), target)
		}
	default:
		if bytes, err := json.Marshal(v); err == nil {
			_ = json.Unmarshal(bytes, target)
		}
	}
}

func encodeEvaluationRun(evaluationID string, report *usecase.EvaluationReport) map[string]any {
	out := map[string]any{
		"evaluation_id":                              evaluationID,
		"generated_at":                               formatPBTime(report.GeneratedAt),
		"status":                                     string(report.Status),
		"total_signals":                              report.TotalSignals,
		"data_completeness_json":                     report.DataCompleteness,
		"metrics_json":                               report.Metrics,
		"playbook_stats_json":                        report.PlaybookStats,
		"regime_stats_json":                          report.RegimeStats,
		"tier_stats_json":                            report.TierStats,
		"direction_stats_json":                       report.DirectionStats,
		"ai_stats_json":                              report.AIStats,
		"staleness_stats_json":                       report.StalenessStats,
		"conflict_stats_json":                        report.ConflictStats,
		"cooldown_stats_json":                        report.CooldownStats,
		"gate_bug_findings_json":                     report.GateBugFindings,
		"recommendations_json":                       report.Recommendations,
		"best_playbook":                              report.BestPlaybook,
		"worst_playbook":                             report.WorstPlaybook,
		"setup_yang_sering_langsung_sl":              report.SetupYangSeringLangsungSL,
		"setup_yang_sering_expired":                  report.SetupYangSeringExpired,
		"setup_yang_sering_stale":                    report.SetupYangSeringStale,
		"regime_yang_paling_buruk":                   report.RegimeYangPalingBuruk,
		"tier_yang_paling_buruk":                     report.TierYangPalingBuruk,
		"direction_yang_paling_buruk":                report.DirectionYangPalingBuruk,
		"playbook_dengan_mae_terbesar":               report.PlaybookDenganMAETerbesar,
		"playbook_dengan_expired_rate_terbesar":      report.PlaybookDenganExpiredRate,
		"playbook_dengan_tp1_rate_terbaik":           report.PlaybookDenganTP1Terbaik,
		"playbook_dengan_tp2_follow_through_terbaik": report.PlaybookDenganTP2Follow,
		"notes_json": map[string]any{
			"schema_version": report.SchemaVersion,
			"config_version": report.ConfigVersion,
			"notes":          report.Notes,
			"source_files":   report.SourceFilesUsed,
			"diagnostics": map[string]any{
				"long_regime_playbook_stats": report.LongRegimePlaybookStats,
				"weak_long_setups":           report.WeakLongSetups,
			},
		},
	}
	return out
}

func decodeEvaluationRun(m map[string]any) (*usecase.EvaluationReport, error) {
	var report usecase.EvaluationReport
	report.GeneratedAt = parsePBTime(m["generated_at"])
	if v, ok := m["status"].(string); ok {
		report.Status = usecase.Status(v)
	}
	report.TotalSignals = int(toFloat(m["total_signals"]))
	report.DataCompleteness = decodeJSONField[usecase.DataCompleteness](m["data_completeness_json"])
	report.Metrics = decodeJSONField[map[string]float64](m["metrics_json"])
	report.PlaybookStats = decodeJSONField[map[string]usecase.PlaybookStats](m["playbook_stats_json"])
	report.RegimeStats = decodeJSONField[map[string]usecase.RegimeStats](m["regime_stats_json"])
	report.TierStats = decodeJSONField[map[string]usecase.TierStats](m["tier_stats_json"])
	report.DirectionStats = decodeJSONField[map[string]usecase.DirectionStats](m["direction_stats_json"])
	report.AIStats = decodeJSONField[map[string]usecase.AIStats](m["ai_stats_json"])
	report.StalenessStats = decodeJSONField[map[string]usecase.StalenessStats](m["staleness_stats_json"])
	report.ConflictStats = decodeJSONField[map[string]int](m["conflict_stats_json"])
	report.CooldownStats = decodeJSONField[map[string]int](m["cooldown_stats_json"])
	report.GateBugFindings = decodeJSONField[[]string](m["gate_bug_findings_json"])
	report.Recommendations = decodeJSONField[[]usecase.ThresholdRecommendation](m["recommendations_json"])

	report.BestPlaybook, _ = m["best_playbook"].(string)
	report.WorstPlaybook, _ = m["worst_playbook"].(string)
	report.SetupYangSeringLangsungSL, _ = m["setup_yang_sering_langsung_sl"].(string)
	report.SetupYangSeringExpired, _ = m["setup_yang_sering_expired"].(string)
	report.SetupYangSeringStale, _ = m["setup_yang_sering_stale"].(string)
	report.RegimeYangPalingBuruk, _ = m["regime_yang_paling_buruk"].(string)
	report.TierYangPalingBuruk, _ = m["tier_yang_paling_buruk"].(string)
	report.DirectionYangPalingBuruk, _ = m["direction_yang_paling_buruk"].(string)
	report.PlaybookDenganMAETerbesar, _ = m["playbook_dengan_mae_terbesar"].(string)
	report.PlaybookDenganExpiredRate, _ = m["playbook_dengan_expired_rate_terbesar"].(string)
	report.PlaybookDenganTP1Terbaik, _ = m["playbook_dengan_tp1_rate_terbaik"].(string)
	report.PlaybookDenganTP2Follow, _ = m["playbook_dengan_tp2_follow_through_terbaik"].(string)

	if notes, ok := m["notes_json"].(map[string]any); ok {
		report.SchemaVersion, _ = notes["schema_version"].(string)
		report.ConfigVersion, _ = notes["config_version"].(string)
		report.Notes, _ = notes["notes"].(string)
		if sf, ok := notes["source_files"].([]any); ok {
			for _, v := range sf {
				if s, ok := v.(string); ok {
					report.SourceFilesUsed = append(report.SourceFilesUsed, s)
				}
			}
		}
		if diagnostics, ok := notes["diagnostics"].(map[string]any); ok {
			report.LongRegimePlaybookStats = decodeJSONField[[]usecase.SetupDiagnosticStats](diagnostics["long_regime_playbook_stats"])
			report.WeakLongSetups = decodeJSONField[[]usecase.SetupDiagnosticStats](diagnostics["weak_long_setups"])
		}
	}
	return &report, nil
}

func decodeJSONField[T any](v any) T {
	var zero T
	if v == nil {
		return zero
	}
	b, err := json.Marshal(v)
	if err != nil {
		return zero
	}
	_ = json.Unmarshal(b, &zero)
	return zero
}

func formatPBTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func parsePBTime(v any) time.Time {
	s, ok := v.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return time.Time{}
	}
	// PocketBase date format uses space instead of 'T' (e.g. "2026-06-01 12:45:20.385Z").
	// We normalize it to RFC3339 format before parsing.
	s = strings.Replace(s, " ", "T", 1)
	t, err := time.Parse(time.RFC3339Nano, s)
	if err == nil {
		return t
	}
	t, _ = time.Parse(time.RFC3339, s)
	return t
}

func toFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case json.Number:
		f, _ := x.Float64()
		return f
	case string:
		if strings.TrimSpace(x) == "" {
			return 0
		}
		f, _ := json.Number(x).Float64()
		return f
	default:
		return 0
	}
}

func toBool(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return strings.EqualFold(x, "true") || x == "1"
	case float64:
		return x == 1
	default:
		return false
	}
}

func toStringSlice(v any) []string {
	switch x := v.(type) {
	case []string:
		return append([]string(nil), x...)
	case []any:
		out := make([]string, 0, len(x))
		for _, item := range x {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		if strings.TrimSpace(x) == "" {
			return nil
		}
		var out []string
		if err := json.Unmarshal([]byte(x), &out); err == nil {
			filtered := out[:0]
			for _, item := range out {
				if strings.TrimSpace(item) != "" {
					filtered = append(filtered, item)
				}
			}
			return filtered
		}
	}
	return nil
}

func sameStringSlice(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
