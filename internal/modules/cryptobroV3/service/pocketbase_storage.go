package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/entity"
	"cpbro-engine/internal/modules/cryptobroV3/usecase"
)

// PocketBaseStorageService stores SignalJournal + EvaluationReport into PocketBase collections:
// - signal_journals
// - evaluation_runs
//
// It delegates all other storage concerns to the fallback repository (typically JSONStorageService).
type PocketBaseStorageService struct {
	fallback usecase.StorageRepository
	client   *PocketBaseClient

	mu sync.Mutex
}

type pbListResponse struct {
	Page       int              `json:"page"`
	PerPage    int              `json:"perPage"`
	TotalItems int              `json:"totalItems"`
	TotalPages int              `json:"totalPages"`
	Items      []map[string]any `json:"items"`
	Raw        json.RawMessage  `json:"-"`
}

func NewPocketBaseStorageService(fallback usecase.StorageRepository, client *PocketBaseClient) (*PocketBaseStorageService, error) {
	if fallback == nil {
		return nil, errors.New("fallback storage repo is nil")
	}
	if client == nil {
		return nil, errors.New("pocketbase client is nil")
	}
	return &PocketBaseStorageService{
		fallback: fallback,
		client:   client,
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

// --- Signal Journal ---

func (s *PocketBaseStorageService) LoadSignalJournal() ([]usecase.SignalJournal, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	items, err := s.listAll(ctx, "signal_journals", url.Values{
		"perPage": []string{"200"},
		"sort":    []string{"-created_at"},
	})
	if err != nil {
		return nil, err
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

	// Ensure deterministic ordering (newest first) if created_at missing.
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *PocketBaseStorageService) SaveSignalJournal(journal []usecase.SignalJournal) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveSignalJournalUnlocked(journal)
}

func (s *PocketBaseStorageService) saveSignalJournalUnlocked(journal []usecase.SignalJournal) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	existing, err := s.mapSignalIDToRecordID(ctx)
	if err != nil {
		return err
	}

	for _, entry := range journal {
		if strings.TrimSpace(entry.ID) == "" {
			continue
		}
		payload := encodeSignalJournal(entry)
		if recID, ok := existing[entry.ID]; ok && strings.TrimSpace(recID) != "" {
			_ = s.client.doJSON(ctx, "PATCH", "/api/collections/signal_journals/records/"+recID, nil, payload, nil)
		} else {
			var created map[string]any
			err := s.client.doJSON(ctx, "POST", "/api/collections/signal_journals/records", nil, payload, &created)
			if err != nil {
				// retry as update if unique constraint hit
				recID, lookupErr := s.findSignalJournalRecordIDBySignalID(ctx, entry.ID)
				if lookupErr == nil && recID != "" {
					_ = s.client.doJSON(ctx, "PATCH", "/api/collections/signal_journals/records/"+recID, nil, payload, nil)
					continue
				}
				return err
			}
			if id, _ := created["id"].(string); id != "" {
				existing[entry.ID] = id
			}
		}
	}

	return nil
}

func (s *PocketBaseStorageService) AppendSignalJournal(entry usecase.SignalJournal) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if strings.TrimSpace(entry.ID) == "" {
		return errors.New("signal journal entry missing signal_id")
	}
	payload := encodeSignalJournal(entry)

	// Fast path: try create.
	if err := s.client.doJSON(ctx, "POST", "/api/collections/signal_journals/records", nil, payload, nil); err == nil {
		return nil
	}

	// If already exists, update.
	recID, err := s.findSignalJournalRecordIDBySignalID(ctx, entry.ID)
	if err != nil || recID == "" {
		return err
	}
	return s.client.doJSON(ctx, "PATCH", "/api/collections/signal_journals/records/"+recID, nil, payload, nil)
}

// UpdateSignalJournal implements atomic read-modify-write semantics needed by MonitoringUsecase.
func (s *PocketBaseStorageService) UpdateSignalJournal(update func([]usecase.SignalJournal) ([]usecase.SignalJournal, error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, err := s.LoadSignalJournal()
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
	return s.saveSignalJournalUnlocked(updated)
}

// --- Evaluation ---

func (s *PocketBaseStorageService) LoadEvaluationReport() (*usecase.EvaluationReport, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var list pbListResponse
	q := url.Values{}
	q.Set("perPage", "1")
	q.Set("page", "1")
	q.Set("sort", "-generated_at")
	if err := s.client.doJSON(ctx, "GET", "/api/collections/evaluation_runs/records", q, nil, &list); err != nil {
		return nil, err
	}
	if len(list.Items) == 0 {
		return nil, nil
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

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	evalID := makeEvaluationID(report.GeneratedAt)
	payload := encodeEvaluationRun(evalID, report)
	return s.client.doJSON(ctx, "POST", "/api/collections/evaluation_runs/records", nil, payload, nil)
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

func cloneValues(v url.Values) url.Values {
	out := url.Values{}
	for k, vals := range v {
		cp := make([]string, len(vals))
		copy(cp, vals)
		out[k] = cp
	}
	return out
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

func (s *PocketBaseStorageService) findSignalJournalRecordIDBySignalID(ctx context.Context, signalID string) (string, error) {
	q := url.Values{}
	q.Set("perPage", "1")
	q.Set("page", "1")
	q.Set("filter", fmt.Sprintf("signal_id='%s'", escapePBFilterValue(signalID)))

	var resp pbListResponse
	if err := s.client.doJSON(ctx, "GET", "/api/collections/signal_journals/records", q, nil, &resp); err != nil {
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
		"schema_version":            e.SchemaVersion,
		"config_version":            e.ConfigVersion,
		"signal_id":                 e.ID,
		"symbol":                    e.Symbol,
		"direction":                 string(e.Direction),
		"playbook":                  string(e.Playbook),
		"entry":                     e.EntryPrice,
		"sl":                        e.StopLoss,
		"tp1":                       e.TP1,
		"tp2":                       e.TP2,
		"rr":                        e.RR,
		"score":                     e.QuantScore,
		"ai_confidence":             e.AIConfidence,
		"market_regime":             e.MarketRegime,
		"policy_mode":               e.PolicyMode,
		"threshold_profile_summary": e.ThresholdProfileSummary,
		"breakout_level":            e.BreakoutLevel,
		"retest_touches":            e.RetestTouches,
		"retest_hold":               e.RetestHold,
		"has_derivatives_evidence":  e.HasDerivativesEvidence,
		"created_at":                formatPBTime(e.CreatedAt),
		"expires_at":                formatPBTime(e.ExpiresAt),
		"status":                    string(e.Status),
		"mfe":                       e.MFE,
		"mae":                       e.MAE,
		"time_to_tp1":               e.TimeToTP1,
		"time_to_tp2":               e.TimeToTP2,
		"time_to_sl":                e.TimeToSL,
		"outcome_reason":            e.OutcomeReason,
		"entry_timing":              e.EntryTiming,
		"tier":                      string(e.Tier),
		"timeframe":                 e.Timeframe,
		"latest_price":              e.LatestPrice,
		"take_profit":               e.TakeProfit,
		"ai_sentiment":              e.AISentiment,
		"ai_reasoning":              e.AIReasoning,
		"pnl_percentage":            e.PnlPercentage,
		"updated_at":                formatPBTime(e.UpdatedAt),
		"closed_at":                 formatPBTime(e.ClosedAt),
		"reason":                    e.Reason,
		"notification_status":       e.NotificationStatus,
		"notification_error":        e.NotificationError,
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
	out.TP1 = toFloat(m["tp1"])
	out.TP2 = toFloat(m["tp2"])
	out.RR = toFloat(m["rr"])
	out.QuantScore = toFloat(m["score"])
	out.AIConfidence, _ = m["ai_confidence"].(string)
	out.MarketRegime, _ = m["market_regime"].(string)
	out.PolicyMode, _ = m["policy_mode"].(string)
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

	if strings.TrimSpace(out.ID) == "" || strings.TrimSpace(out.Symbol) == "" {
		return usecase.SignalJournal{}, errors.New("missing required journal fields")
	}
	return out, nil
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
