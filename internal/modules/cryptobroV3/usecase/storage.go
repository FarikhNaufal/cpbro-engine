package usecase

import (
	"cpbro-engine/internal/modules/cryptobroV3/dto"
	"cpbro-engine/internal/modules/cryptobroV3/entity"
	"strings"
)

type StorageUsecase struct {
	repo StorageRepository
}

func NewStorageUsecase(repo StorageRepository) *StorageUsecase {
	return &StorageUsecase{
		repo: repo,
	}
}

type signalJournalAtomicUpdater interface {
	UpdateSignalJournal(update func([]SignalJournal) ([]SignalJournal, error)) error
}

type watchJournalAtomicUpdater interface {
	UpdateWatchJournal(update func([]WatchJournal) ([]WatchJournal, error)) error
}

type signalJournalEntryUpserter interface {
	UpsertSignalJournalEntries(entries []SignalJournal) error
}

type watchJournalEntryUpserter interface {
	UpsertWatchJournalEntries(entries []WatchJournal) error
}

type decisionAuditBatchAppender interface {
	AppendDecisionAudits(entries []DecisionAudit) error
}

type aiAuditCacheAtomicUpdater interface {
	UpdateAIAuditCache(update func(*entity.AIAuditCache) error) error
}

func (uc *StorageUsecase) LoadLatestResult() (*entity.LatestResult, error) {
	return uc.repo.LoadLatestResult()
}

func (uc *StorageUsecase) SaveLatestResult(latest *entity.LatestResult) error {
	err := uc.repo.SaveLatestResult(latest)
	if err != nil {
		GetGlobalMetrics().IncrementStorageWriteFail()
	}
	return err
}

func (uc *StorageUsecase) SaveLatestScanResult(result dto.ScanResult) error {
	latest := &entity.LatestResult{
		LastScanTime: result.Timestamp,
		Duration:     result.Duration,
		Signals:      result.Signals,
	}
	return uc.repo.SaveLatestResult(latest)
}

func (uc *StorageUsecase) LoadSignalHistory() (*entity.SignalHistory, error) {
	return uc.repo.LoadSignalHistory()
}

func (uc *StorageUsecase) SaveSignalHistory(hist *entity.SignalHistory) error {
	err := uc.repo.SaveSignalHistory(hist)
	if err != nil {
		GetGlobalMetrics().IncrementStorageWriteFail()
	}
	return err
}

func (uc *StorageUsecase) LoadSignalJournal() ([]SignalJournal, error) {
	return uc.repo.LoadSignalJournal()
}

func (uc *StorageUsecase) SaveSignalJournal(journal []SignalJournal) error {
	err := uc.repo.SaveSignalJournal(journal)
	if err != nil {
		GetGlobalMetrics().IncrementStorageWriteFail()
	}
	return err
}

func (uc *StorageUsecase) UpdateSignalJournal(update func([]SignalJournal) ([]SignalJournal, error)) error {
	if updater, ok := uc.repo.(signalJournalAtomicUpdater); ok {
		err := updater.UpdateSignalJournal(update)
		if err != nil {
			GetGlobalMetrics().IncrementStorageWriteFail()
		}
		return err
	}

	journal, err := uc.repo.LoadSignalJournal()
	if err != nil {
		return err
	}
	updated, err := update(journal)
	if err != nil {
		return err
	}
	return uc.SaveSignalJournal(updated)
}

func (uc *StorageUsecase) SaveSignalToJournal(sig SignalJournal) error {
	err := uc.repo.AppendSignalJournal(sig)
	if err != nil {
		GetGlobalMetrics().IncrementStorageWriteFail()
	}
	return err
}

func (uc *StorageUsecase) UpsertSignalJournalEntries(entries []SignalJournal) error {
	if len(entries) == 0 {
		return nil
	}

	if upserter, ok := uc.repo.(signalJournalEntryUpserter); ok {
		err := upserter.UpsertSignalJournalEntries(entries)
		if err != nil {
			GetGlobalMetrics().IncrementStorageWriteFail()
		}
		return err
	}

	return uc.UpdateSignalJournal(func(current []SignalJournal) ([]SignalJournal, error) {
		updated := append([]SignalJournal(nil), current...)
		for _, entry := range entries {
			matched := false
			for i := range updated {
				if sameJournalIdentity(updated[i], entry) {
					updated[i] = entry
					matched = true
					break
				}
			}
			if !matched {
				updated = append(updated, entry)
			}
		}
		return updated, nil
	})
}

func (uc *StorageUsecase) LoadWatchJournal() ([]WatchJournal, error) {
	return uc.repo.LoadWatchJournal()
}

func (uc *StorageUsecase) SaveWatchJournal(journal []WatchJournal) error {
	err := uc.repo.SaveWatchJournal(journal)
	if err != nil {
		GetGlobalMetrics().IncrementStorageWriteFail()
	}
	return err
}

func (uc *StorageUsecase) UpdateWatchJournal(update func([]WatchJournal) ([]WatchJournal, error)) error {
	if updater, ok := uc.repo.(watchJournalAtomicUpdater); ok {
		err := updater.UpdateWatchJournal(update)
		if err != nil {
			GetGlobalMetrics().IncrementStorageWriteFail()
		}
		return err
	}

	journal, err := uc.repo.LoadWatchJournal()
	if err != nil {
		return err
	}
	updated, err := update(journal)
	if err != nil {
		return err
	}
	return uc.SaveWatchJournal(updated)
}

func (uc *StorageUsecase) SaveWatchToJournal(sig WatchJournal) error {
	err := uc.repo.AppendWatchJournal(sig)
	if err != nil {
		GetGlobalMetrics().IncrementStorageWriteFail()
	}
	return err
}

func (uc *StorageUsecase) UpsertWatchJournalEntries(entries []WatchJournal) error {
	if len(entries) == 0 {
		return nil
	}

	if upserter, ok := uc.repo.(watchJournalEntryUpserter); ok {
		err := upserter.UpsertWatchJournalEntries(entries)
		if err != nil {
			GetGlobalMetrics().IncrementStorageWriteFail()
		}
		return err
	}

	return uc.UpdateWatchJournal(func(current []WatchJournal) ([]WatchJournal, error) {
		updated := append([]WatchJournal(nil), current...)
		for _, entry := range entries {
			matched := false
			for i := range updated {
				if sameJournalIdentity(updated[i], entry) {
					updated[i] = entry
					matched = true
					break
				}
			}
			if !matched {
				updated = append(updated, entry)
			}
		}
		return updated, nil
	})
}

func (uc *StorageUsecase) LoadAIAuditCache() (*entity.AIAuditCache, error) {
	return uc.repo.LoadAIAuditCache()
}

func (uc *StorageUsecase) SaveAIAuditCache(cache entity.AIAuditCache) error {
	err := uc.repo.SaveAIAuditCache(&cache)
	if err != nil {
		GetGlobalMetrics().IncrementStorageWriteFail()
	}
	return err
}

func (uc *StorageUsecase) UpdateAIAuditCache(update func(*entity.AIAuditCache) error) error {
	if updater, ok := uc.repo.(aiAuditCacheAtomicUpdater); ok {
		err := updater.UpdateAIAuditCache(update)
		if err != nil {
			GetGlobalMetrics().IncrementStorageWriteFail()
		}
		return err
	}

	cache, err := uc.repo.LoadAIAuditCache()
	if err != nil {
		return err
	}
	if cache == nil {
		cache = &entity.AIAuditCache{CacheMap: make(map[string]entity.CachedAudit)}
	}
	if cache.CacheMap == nil {
		cache.CacheMap = make(map[string]entity.CachedAudit)
	}
	if err := update(cache); err != nil {
		return err
	}
	return uc.SaveAIAuditCache(*cache)
}

func (uc *StorageUsecase) LoadEvaluationReport() (*EvaluationReport, error) {
	return uc.repo.LoadEvaluationReport()
}

func (uc *StorageUsecase) SaveEvaluationReport(report EvaluationReport) error {
	err := uc.repo.SaveEvaluationReport(&report)
	if err != nil {
		GetGlobalMetrics().IncrementStorageWriteFail()
	}
	return err
}

func (uc *StorageUsecase) LoadDecisionAudits() ([]DecisionAudit, error) {
	return uc.repo.LoadDecisionAudits()
}

func (uc *StorageUsecase) SaveDecisionAudits(audits []DecisionAudit) error {
	err := uc.repo.SaveDecisionAudits(audits)
	if err != nil {
		GetGlobalMetrics().IncrementStorageWriteFail()
	}
	return err
}

func (uc *StorageUsecase) SaveDecisionAudit(audit DecisionAudit) error {
	err := uc.repo.AppendDecisionAudit(audit)
	if err != nil {
		GetGlobalMetrics().IncrementStorageWriteFail()
	}
	return err
}

func (uc *StorageUsecase) SaveDecisionAuditBatch(audits []DecisionAudit) error {
	if len(audits) == 0 {
		return nil
	}

	if appender, ok := uc.repo.(decisionAuditBatchAppender); ok {
		err := appender.AppendDecisionAudits(audits)
		if err != nil {
			GetGlobalMetrics().IncrementStorageWriteFail()
		}
		return err
	}

	existing, err := uc.repo.LoadDecisionAudits()
	if err != nil {
		return err
	}
	combined := append(existing, audits...)
	return uc.SaveDecisionAudits(combined)
}

func sameJournalIdentity(left, right SignalJournal) bool {
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
