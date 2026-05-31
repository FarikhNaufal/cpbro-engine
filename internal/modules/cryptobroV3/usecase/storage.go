package usecase

import (
	"cpbro-engine/internal/modules/cryptobroV3/dto"
	"cpbro-engine/internal/modules/cryptobroV3/entity"
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
