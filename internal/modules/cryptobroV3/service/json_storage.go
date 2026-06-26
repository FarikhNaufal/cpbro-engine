package service

import (
	"cpbro-engine/internal/modules/cryptobroV3/config"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"cpbro-engine/internal/modules/cryptobroV3/entity"
	"cpbro-engine/internal/modules/cryptobroV3/usecase"
)

type JSONStorageService struct {
	mu         sync.RWMutex
	storageDir string
	files      JSONStorageFiles
}

type JSONStorageFiles struct {
	LatestResultFile     string
	SignalHistoryFile    string
	SignalJournalFile    string
	WatchJournalFile     string
	AIAuditCacheFile     string
	EvaluationReportFile string
	DecisionAuditFile    string
}

func NewJSONStorageService(storageDir string) (*JSONStorageService, error) {
	return NewJSONStorageServiceWithFiles(storageDir, JSONStorageFiles{})
}

func NewJSONStorageServiceWithFiles(storageDir string, files JSONStorageFiles) (*JSONStorageService, error) {
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		return nil, err
	}
	return &JSONStorageService{
		storageDir: storageDir,
		files:      normalizeJSONStorageFiles(files),
	}, nil
}

func normalizeJSONStorageFiles(files JSONStorageFiles) JSONStorageFiles {
	if strings.TrimSpace(files.LatestResultFile) == "" {
		files.LatestResultFile = config.DefaultLatestResultFile
	}
	if strings.TrimSpace(files.SignalHistoryFile) == "" {
		files.SignalHistoryFile = config.DefaultSignalHistoryFile
	}
	if strings.TrimSpace(files.SignalJournalFile) == "" {
		files.SignalJournalFile = config.DefaultSignalJournalFile
	}
	if strings.TrimSpace(files.WatchJournalFile) == "" {
		files.WatchJournalFile = config.DefaultWatchJournalFile
	}
	if strings.TrimSpace(files.AIAuditCacheFile) == "" {
		files.AIAuditCacheFile = config.DefaultAIAuditCacheFile
	}
	if strings.TrimSpace(files.EvaluationReportFile) == "" {
		files.EvaluationReportFile = config.DefaultEvaluationReportFile
	}
	if strings.TrimSpace(files.DecisionAuditFile) == "" {
		files.DecisionAuditFile = config.DefaultDecisionAuditFile
	}
	return files
}

func (s *JSONStorageService) readJSON(filename string, dest interface{}) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	path := filepath.Join(s.storageDir, filename)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil // Return empty, caller handles initialization
		}
		return err
	}

	if len(data) == 0 {
		return nil
	}

	return json.Unmarshal(data, dest)
}

func (s *JSONStorageService) writeJSON(filename string, data interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.storageDir, filename)
	tmpPath := path + ".tmp"

	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(tmpPath, bytes, 0644); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	return nil
}

// Implement usecase.StorageRepository interface methods

func (s *JSONStorageService) LoadLatestResult() (*entity.LatestResult, error) {
	var res entity.LatestResult
	if err := s.readJSON(s.files.LatestResultFile, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (s *JSONStorageService) SaveLatestResult(res *entity.LatestResult) error {
	return s.writeJSON(s.files.LatestResultFile, res)
}

func (s *JSONStorageService) LoadSignalHistory() (*entity.SignalHistory, error) {
	var hist entity.SignalHistory
	if err := s.readJSON(s.files.SignalHistoryFile, &hist); err != nil {
		return nil, err
	}
	return &hist, nil
}

func (s *JSONStorageService) SaveSignalHistory(hist *entity.SignalHistory) error {
	return s.writeJSON(s.files.SignalHistoryFile, hist)
}

func (s *JSONStorageService) LoadSignalJournal() ([]usecase.SignalJournal, error) {
	var journal []usecase.SignalJournal
	if err := s.readJSON(s.files.SignalJournalFile, &journal); err != nil {
		return nil, err
	}
	if journal == nil {
		journal = []usecase.SignalJournal{}
	}
	return journal, nil
}

func (s *JSONStorageService) SaveSignalJournal(journal []usecase.SignalJournal) error {
	return s.writeJSON(s.files.SignalJournalFile, journal)
}

func (s *JSONStorageService) LoadWatchJournal() ([]usecase.WatchJournal, error) {
	var journal []usecase.WatchJournal
	if err := s.readJSON(s.files.WatchJournalFile, &journal); err != nil {
		return nil, err
	}
	if journal == nil {
		journal = []usecase.WatchJournal{}
	}
	return journal, nil
}

func (s *JSONStorageService) FindWatchJournalCandidates(probe usecase.WatchJournal) ([]usecase.WatchJournal, error) {
	journal, err := s.LoadWatchJournal()
	if err != nil {
		return nil, err
	}
	return filterMatchingWatchJournalCandidates(journal, probe), nil
}

func (s *JSONStorageService) SaveWatchJournal(journal []usecase.WatchJournal) error {
	return s.writeJSON(s.files.WatchJournalFile, journal)
}

func (s *JSONStorageService) UpdateSignalJournal(update func([]usecase.SignalJournal) ([]usecase.SignalJournal, error)) error {
	return updateJSONSliceFile(s, s.files.SignalJournalFile, update)
}

func (s *JSONStorageService) UpsertSignalJournalEntries(entries []usecase.SignalJournal) error {
	return upsertSignalJournalFile(s, s.files.SignalJournalFile, entries)
}

func (s *JSONStorageService) AppendSignalJournal(entry usecase.SignalJournal) error {
	return appendJSONSliceFile(s, s.files.SignalJournalFile, entry)
}

func (s *JSONStorageService) UpdateWatchJournal(update func([]usecase.WatchJournal) ([]usecase.WatchJournal, error)) error {
	return updateJSONSliceFile(s, s.files.WatchJournalFile, update)
}

func (s *JSONStorageService) UpsertWatchJournalEntries(entries []usecase.WatchJournal) error {
	return upsertWatchJournalFile(s, s.files.WatchJournalFile, entries)
}

func (s *JSONStorageService) AppendWatchJournal(entry usecase.WatchJournal) error {
	return appendJSONSliceFile(s, s.files.WatchJournalFile, entry)
}

func filterMatchingWatchJournalCandidates(journal []usecase.WatchJournal, probe usecase.WatchJournal) []usecase.WatchJournal {
	if len(journal) == 0 {
		return []usecase.WatchJournal{}
	}

	filtered := make([]usecase.WatchJournal, 0, len(journal))
	for i := range journal {
		entry := journal[i]
		if entry.Symbol != probe.Symbol {
			continue
		}
		if entry.Direction != probe.Direction {
			continue
		}
		if entry.Playbook != probe.Playbook {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

func updateJSONSliceFile[T any](s *JSONStorageService, filename string, update func([]T) ([]T, error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.storageDir, filename)

	var journal []T
	data, err := os.ReadFile(path)
	if err == nil && len(data) > 0 {
		if err := json.Unmarshal(data, &journal); err != nil {
			return err
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if journal == nil {
		journal = []T{}
	}

	updated, err := update(journal)
	if err != nil {
		return err
	}
	if updated == nil {
		updated = []T{}
	}

	tmpPath := path + ".tmp"
	bytes, err := json.MarshalIndent(updated, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmpPath, bytes, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func appendJSONSliceFile[T any](s *JSONStorageService, filename string, entry T) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.storageDir, filename)

	var journal []T
	data, err := os.ReadFile(path)
	if err == nil && len(data) > 0 {
		if err := json.Unmarshal(data, &journal); err != nil {
			return err
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	journal = append(journal, entry)

	tmpPath := path + ".tmp"
	bytes, err := json.MarshalIndent(journal, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmpPath, bytes, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func upsertSignalJournalFile(s *JSONStorageService, filename string, entries []usecase.SignalJournal) error {
	if len(entries) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.storageDir, filename)

	var journal []usecase.SignalJournal
	data, err := os.ReadFile(path)
	if err == nil && len(data) > 0 {
		if err := json.Unmarshal(data, &journal); err != nil {
			return err
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if journal == nil {
		journal = []usecase.SignalJournal{}
	}

	for _, entry := range entries {
		replaced := false
		for i := range journal {
			if journalEntriesMatch(journal[i], entry) {
				journal[i] = entry
				replaced = true
				break
			}
		}
		if !replaced {
			journal = append(journal, entry)
		}
	}

	tmpPath := path + ".tmp"
	bytes, err := json.MarshalIndent(journal, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmpPath, bytes, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func upsertWatchJournalFile(s *JSONStorageService, filename string, entries []usecase.WatchJournal) error {
	signalEntries := make([]usecase.SignalJournal, len(entries))
	for i, entry := range entries {
		signalEntries[i] = usecase.SignalJournal(entry)
	}
	return upsertSignalJournalFile(s, filename, signalEntries)
}

func journalEntriesMatch(left, right usecase.SignalJournal) bool {
	leftID := strings.TrimSpace(left.ID)
	rightID := strings.TrimSpace(right.ID)
	if leftID != "" || rightID != "" {
		return leftID != "" && leftID == rightID
	}

	return left.Symbol == right.Symbol &&
		left.Direction == right.Direction &&
		left.Playbook == right.Playbook &&
		left.CreatedAt.Equal(right.CreatedAt)
}

func (s *JSONStorageService) LoadAIAuditCache() (*entity.AIAuditCache, error) {
	var cache entity.AIAuditCache
	if err := s.readJSON(s.files.AIAuditCacheFile, &cache); err != nil {
		return nil, err
	}
	if cache.CacheMap == nil {
		cache.CacheMap = make(map[string]entity.CachedAudit)
	}
	return &cache, nil
}

func (s *JSONStorageService) UpdateAIAuditCache(update func(*entity.AIAuditCache) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	filename := s.files.AIAuditCacheFile
	path := filepath.Join(s.storageDir, filename)

	var cache entity.AIAuditCache
	data, err := os.ReadFile(path)
	if err == nil && len(data) > 0 {
		if err := json.Unmarshal(data, &cache); err != nil {
			return err
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if cache.CacheMap == nil {
		cache.CacheMap = make(map[string]entity.CachedAudit)
	}

	if err := update(&cache); err != nil {
		return err
	}

	tmpPath := path + ".tmp"
	bytes, err := json.MarshalIndent(&cache, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmpPath, bytes, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func (s *JSONStorageService) SaveAIAuditCache(cache *entity.AIAuditCache) error {
	return s.writeJSON(s.files.AIAuditCacheFile, cache)
}

func (s *JSONStorageService) LoadEvaluationReport() (*usecase.EvaluationReport, error) {
	var report usecase.EvaluationReport
	if err := s.readJSON(s.files.EvaluationReportFile, &report); err != nil {
		return nil, err
	}
	return &report, nil
}

func (s *JSONStorageService) SaveEvaluationReport(report *usecase.EvaluationReport) error {
	return s.writeJSON(s.files.EvaluationReportFile, report)
}

func (s *JSONStorageService) LoadDecisionAudits() ([]usecase.DecisionAudit, error) {
	var audits []usecase.DecisionAudit
	if err := s.readJSON(s.files.DecisionAuditFile, &audits); err != nil {
		return nil, err
	}
	if audits == nil {
		audits = []usecase.DecisionAudit{}
	}
	return audits, nil
}

func (s *JSONStorageService) SaveDecisionAudits(audits []usecase.DecisionAudit) error {
	return s.writeJSON(s.files.DecisionAuditFile, audits)
}

func (s *JSONStorageService) AppendDecisionAudits(entries []usecase.DecisionAudit) error {
	if len(entries) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	filename := s.files.DecisionAuditFile
	path := filepath.Join(s.storageDir, filename)

	var audits []usecase.DecisionAudit
	data, err := os.ReadFile(path)
	if err == nil && len(data) > 0 {
		if err := json.Unmarshal(data, &audits); err != nil {
			return err
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	audits = append(audits, entries...)

	tmpPath := path + ".tmp"
	bytes, err := json.MarshalIndent(audits, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmpPath, bytes, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func (s *JSONStorageService) AppendDecisionAudit(entry usecase.DecisionAudit) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	filename := s.files.DecisionAuditFile
	path := filepath.Join(s.storageDir, filename)

	var audits []usecase.DecisionAudit
	data, err := os.ReadFile(path)
	if err == nil && len(data) > 0 {
		if err := json.Unmarshal(data, &audits); err != nil {
			return err
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	audits = append(audits, entry)
	if len(audits) > 1000 {
		audits = audits[len(audits)-1000:]
	}

	tmpPath := path + ".tmp"
	bytes, err := json.MarshalIndent(audits, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmpPath, bytes, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}
