package service

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"cpbro-engine/internal/modules/cryptobroV3/entity"
	"cpbro-engine/internal/modules/cryptobroV3/usecase"
)

type JSONStorageService struct {
	mu         sync.RWMutex
	storageDir string
}

func NewJSONStorageService(storageDir string) (*JSONStorageService, error) {
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		return nil, err
	}
	return &JSONStorageService{
		storageDir: storageDir,
	}, nil
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
	if err := s.readJSON("latest_result.json", &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (s *JSONStorageService) SaveLatestResult(res *entity.LatestResult) error {
	return s.writeJSON("latest_result.json", res)
}

func (s *JSONStorageService) LoadSignalHistory() (*entity.SignalHistory, error) {
	var hist entity.SignalHistory
	if err := s.readJSON("signal_history.json", &hist); err != nil {
		return nil, err
	}
	return &hist, nil
}

func (s *JSONStorageService) SaveSignalHistory(hist *entity.SignalHistory) error {
	return s.writeJSON("signal_history.json", hist)
}

func (s *JSONStorageService) LoadSignalJournal() ([]usecase.SignalJournal, error) {
	var journal []usecase.SignalJournal
	if err := s.readJSON("signal_journal.json", &journal); err != nil {
		return nil, err
	}
	if journal == nil {
		journal = []usecase.SignalJournal{}
	}
	return journal, nil
}

func (s *JSONStorageService) SaveSignalJournal(journal []usecase.SignalJournal) error {
	return s.writeJSON("signal_journal.json", journal)
}

func (s *JSONStorageService) AppendSignalJournal(entry usecase.SignalJournal) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	filename := "signal_journal.json"
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

func (s *JSONStorageService) LoadAIAuditCache() (*entity.AIAuditCache, error) {
	var cache entity.AIAuditCache
	if err := s.readJSON("ai_audit_cache.json", &cache); err != nil {
		return nil, err
	}
	if cache.CacheMap == nil {
		cache.CacheMap = make(map[string]entity.CachedAudit)
	}
	return &cache, nil
}

func (s *JSONStorageService) SaveAIAuditCache(cache *entity.AIAuditCache) error {
	return s.writeJSON("ai_audit_cache.json", cache)
}

func (s *JSONStorageService) LoadEvaluationReport() (*usecase.EvaluationReport, error) {
	var report usecase.EvaluationReport
	if err := s.readJSON("evaluation_report.json", &report); err != nil {
		return nil, err
	}
	return &report, nil
}

func (s *JSONStorageService) SaveEvaluationReport(report *usecase.EvaluationReport) error {
	return s.writeJSON("evaluation_report.json", report)
}

func (s *JSONStorageService) LoadDecisionAudits() ([]usecase.DecisionAudit, error) {
	var audits []usecase.DecisionAudit
	if err := s.readJSON("decision_audit.json", &audits); err != nil {
		return nil, err
	}
	if audits == nil {
		audits = []usecase.DecisionAudit{}
	}
	return audits, nil
}

func (s *JSONStorageService) SaveDecisionAudits(audits []usecase.DecisionAudit) error {
	return s.writeJSON("decision_audit.json", audits)
}

func (s *JSONStorageService) AppendDecisionAudit(entry usecase.DecisionAudit) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	filename := "decision_audit.json"
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
