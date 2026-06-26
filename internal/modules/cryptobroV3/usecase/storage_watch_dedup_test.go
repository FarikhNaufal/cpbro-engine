package usecase

import (
	"testing"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/entity"
)

type watchDedupRepo struct {
	watchJournal []WatchJournal
	appendCount  int
	upsertCount  int
	loadCount    int
}

func (r *watchDedupRepo) LoadLatestResult() (*entity.LatestResult, error)    { return nil, nil }
func (r *watchDedupRepo) SaveLatestResult(res *entity.LatestResult) error    { return nil }
func (r *watchDedupRepo) LoadSignalHistory() (*entity.SignalHistory, error)  { return nil, nil }
func (r *watchDedupRepo) SaveSignalHistory(hist *entity.SignalHistory) error { return nil }
func (r *watchDedupRepo) LoadSignalJournal() ([]SignalJournal, error)        { return nil, nil }
func (r *watchDedupRepo) SaveSignalJournal(journal []SignalJournal) error    { return nil }
func (r *watchDedupRepo) AppendSignalJournal(entry SignalJournal) error      { return nil }
func (r *watchDedupRepo) LoadWatchJournal() ([]WatchJournal, error) {
	r.loadCount++
	return append([]WatchJournal(nil), r.watchJournal...), nil
}
func (r *watchDedupRepo) SaveWatchJournal(journal []WatchJournal) error {
	r.watchJournal = append([]WatchJournal(nil), journal...)
	return nil
}
func (r *watchDedupRepo) LoadAIAuditCache() (*entity.AIAuditCache, error)     { return nil, nil }
func (r *watchDedupRepo) SaveAIAuditCache(cache *entity.AIAuditCache) error   { return nil }
func (r *watchDedupRepo) LoadEvaluationReport() (*EvaluationReport, error)    { return nil, nil }
func (r *watchDedupRepo) SaveEvaluationReport(report *EvaluationReport) error { return nil }
func (r *watchDedupRepo) LoadDecisionAudits() ([]DecisionAudit, error)        { return nil, nil }
func (r *watchDedupRepo) SaveDecisionAudits(audits []DecisionAudit) error     { return nil }
func (r *watchDedupRepo) AppendDecisionAudit(entry DecisionAudit) error       { return nil }
func (r *watchDedupRepo) AppendWatchJournal(entry WatchJournal) error {
	r.appendCount++
	r.watchJournal = append(r.watchJournal, entry)
	return nil
}
func (r *watchDedupRepo) UpsertWatchJournalEntries(entries []WatchJournal) error {
	r.upsertCount++
	for _, entry := range entries {
		matched := false
		for i := range r.watchJournal {
			if sameJournalIdentity(r.watchJournal[i], entry) {
				r.watchJournal[i] = entry
				matched = true
				break
			}
		}
		if !matched {
			r.watchJournal = append(r.watchJournal, entry)
		}
	}
	return nil
}

type watchDedupFinderRepo struct {
	*watchDedupRepo
	findCount int
}

func (r *watchDedupFinderRepo) FindWatchJournalCandidates(probe WatchJournal) ([]WatchJournal, error) {
	r.findCount++
	return filterWatchJournalCandidates(r.watchJournal, probe), nil
}

func TestSaveWatchToJournal_SkipsDuplicateActiveWatch(t *testing.T) {
	original := getRuntimeSettings()
	t.Cleanup(func() { SetRuntimeSettings(original) })
	settings := original
	settings.WatchCooldownMinutes = 30
	settings.WatchDedupPriceToleranceBps = 50
	SetRuntimeSettings(settings)

	now := time.Now().UTC()
	repo := &watchDedupRepo{
		watchJournal: []WatchJournal{{
			ID:         "watch_1",
			Symbol:     "SOLUSDT",
			Direction:  LONG,
			Playbook:   LIQUIDITY_SWEEP_REVERSAL,
			EntryPrice: 100,
			StopLoss:   98,
			TP2:        105,
			Status:     WATCH_MONITORING,
			CreatedAt:  now.Add(-10 * time.Minute),
			ExpiresAt:  now.Add(110 * time.Minute),
			UpdatedAt:  now.Add(-5 * time.Minute),
			Reason:     "wait retest",
		}},
	}

	storage := NewStorageUsecase(repo)
	err := storage.SaveWatchToJournal(WatchJournal{
		ID:         "watch_new",
		Symbol:     "SOLUSDT",
		Direction:  LONG,
		Playbook:   LIQUIDITY_SWEEP_REVERSAL,
		EntryPrice: 100.1,
		StopLoss:   98.05,
		TP2:        105.1,
		Status:     WATCH_MONITORING,
		CreatedAt:  now,
		ExpiresAt:  now.Add(2 * time.Hour),
		Reason:     "wait retest",
	})
	if err != nil {
		t.Fatalf("SaveWatchToJournal: %v", err)
	}

	if repo.appendCount != 0 {
		t.Fatalf("expected no append for duplicate active watch, got %d", repo.appendCount)
	}
	if repo.upsertCount != 0 {
		t.Fatalf("expected no upsert for unchanged active watch, got %d", repo.upsertCount)
	}
	if len(repo.watchJournal) != 1 {
		t.Fatalf("expected single watch row after dedup, got %d", len(repo.watchJournal))
	}
}

func TestSaveWatchToJournal_UpdatesExistingActiveWatchOnMaterialChange(t *testing.T) {
	original := getRuntimeSettings()
	t.Cleanup(func() { SetRuntimeSettings(original) })
	settings := original
	settings.WatchCooldownMinutes = 30
	settings.WatchDedupPriceToleranceBps = 50
	SetRuntimeSettings(settings)

	now := time.Now().UTC()
	oldCreatedAt := now.Add(-10 * time.Minute)
	oldExpiresAt := now.Add(110 * time.Minute)
	newExpiresAt := now.Add(2 * time.Hour)
	repo := &watchDedupRepo{
		watchJournal: []WatchJournal{{
			ID:         "watch_1",
			Symbol:     "SOLUSDT",
			Direction:  LONG,
			Playbook:   LIQUIDITY_SWEEP_REVERSAL,
			EntryPrice: 100,
			StopLoss:   98,
			TP2:        105,
			Status:     WATCH_MONITORING,
			CreatedAt:  oldCreatedAt,
			ExpiresAt:  oldExpiresAt,
			Reason:     "old reason",
			QuantScore: 7.2,
		}},
	}

	storage := NewStorageUsecase(repo)
	err := storage.SaveWatchToJournal(WatchJournal{
		ID:         "watch_new",
		Symbol:     "SOLUSDT",
		Direction:  LONG,
		Playbook:   LIQUIDITY_SWEEP_REVERSAL,
		EntryPrice: 100.1,
		StopLoss:   98.05,
		TP2:        105.1,
		Status:     WATCH_MONITORING,
		CreatedAt:  now,
		ExpiresAt:  newExpiresAt,
		Reason:     "updated reason",
		QuantScore: 8.4,
	})
	if err != nil {
		t.Fatalf("SaveWatchToJournal: %v", err)
	}

	if repo.appendCount != 0 {
		t.Fatalf("expected no append for merged watch, got %d", repo.appendCount)
	}
	if repo.upsertCount != 1 {
		t.Fatalf("expected one upsert for changed active watch, got %d", repo.upsertCount)
	}
	if len(repo.watchJournal) != 1 {
		t.Fatalf("expected single watch row after merge, got %d", len(repo.watchJournal))
	}
	if repo.watchJournal[0].ID != "watch_1" {
		t.Fatalf("expected existing ID to be preserved, got %s", repo.watchJournal[0].ID)
	}
	if repo.watchJournal[0].Reason != "updated reason" {
		t.Fatalf("expected updated reason to be persisted, got %s", repo.watchJournal[0].Reason)
	}
	if !repo.watchJournal[0].CreatedAt.Equal(oldCreatedAt) {
		t.Fatalf("expected original CreatedAt to be preserved")
	}
	if !repo.watchJournal[0].ExpiresAt.Equal(newExpiresAt) {
		t.Fatalf("expected ExpiresAt to be refreshed to latest watch window")
	}
	if repo.watchJournal[0].UpdatedAt.IsZero() {
		t.Fatalf("expected UpdatedAt to be refreshed on merge")
	}
}

func TestSaveWatchToJournal_SkipsRecentlyClosedMatchingWatch(t *testing.T) {
	original := getRuntimeSettings()
	t.Cleanup(func() { SetRuntimeSettings(original) })
	settings := original
	settings.WatchCooldownMinutes = 30
	settings.WatchDedupPriceToleranceBps = 50
	SetRuntimeSettings(settings)

	now := time.Now().UTC()
	repo := &watchDedupRepo{
		watchJournal: []WatchJournal{{
			ID:         "watch_1",
			Symbol:     "SOLUSDT",
			Direction:  LONG,
			Playbook:   LIQUIDITY_SWEEP_REVERSAL,
			EntryPrice: 100,
			StopLoss:   98,
			TP2:        105,
			Status:     VIRTUAL_EXPIRED,
			CreatedAt:  now.Add(-2 * time.Hour),
			UpdatedAt:  now.Add(-10 * time.Minute),
			ClosedAt:   now.Add(-10 * time.Minute),
		}},
	}

	storage := NewStorageUsecase(repo)
	err := storage.SaveWatchToJournal(WatchJournal{
		ID:         "watch_new",
		Symbol:     "SOLUSDT",
		Direction:  LONG,
		Playbook:   LIQUIDITY_SWEEP_REVERSAL,
		EntryPrice: 100.1,
		StopLoss:   98.05,
		TP2:        105.1,
		Status:     WATCH_MONITORING,
		CreatedAt:  now,
		ExpiresAt:  now.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("SaveWatchToJournal: %v", err)
	}

	if repo.appendCount != 0 || repo.upsertCount != 0 {
		t.Fatalf("expected recently closed matching watch to be skipped, append=%d upsert=%d", repo.appendCount, repo.upsertCount)
	}
	if len(repo.watchJournal) != 1 {
		t.Fatalf("expected no new watch row, got %d", len(repo.watchJournal))
	}
}

func TestSaveWatchToJournal_RespectsRuntimeCooldownWindow(t *testing.T) {
	original := getRuntimeSettings()
	t.Cleanup(func() { SetRuntimeSettings(original) })
	settings := original
	settings.WatchCooldownMinutes = 5
	settings.WatchDedupPriceToleranceBps = 50
	SetRuntimeSettings(settings)

	now := time.Now().UTC()
	repo := &watchDedupRepo{
		watchJournal: []WatchJournal{{
			ID:         "watch_1",
			Symbol:     "SOLUSDT",
			Direction:  LONG,
			Playbook:   LIQUIDITY_SWEEP_REVERSAL,
			EntryPrice: 100,
			StopLoss:   98,
			TP2:        105,
			Status:     VIRTUAL_EXPIRED,
			CreatedAt:  now.Add(-2 * time.Hour),
			UpdatedAt:  now.Add(-10 * time.Minute),
			ClosedAt:   now.Add(-10 * time.Minute),
		}},
	}

	storage := NewStorageUsecase(repo)
	err := storage.SaveWatchToJournal(WatchJournal{
		ID:         "watch_new",
		Symbol:     "SOLUSDT",
		Direction:  LONG,
		Playbook:   LIQUIDITY_SWEEP_REVERSAL,
		EntryPrice: 100.05,
		StopLoss:   98.02,
		TP2:        105.02,
		Status:     WATCH_MONITORING,
		CreatedAt:  now,
		ExpiresAt:  now.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("SaveWatchToJournal: %v", err)
	}

	if repo.appendCount != 1 {
		t.Fatalf("expected append after cooldown window elapsed, got %d", repo.appendCount)
	}
	if len(repo.watchJournal) != 2 {
		t.Fatalf("expected second watch row after cooldown expiry, got %d", len(repo.watchJournal))
	}
}

func TestSaveWatchToJournal_RespectsRuntimePriceTolerance(t *testing.T) {
	original := getRuntimeSettings()
	t.Cleanup(func() { SetRuntimeSettings(original) })
	settings := original
	settings.WatchCooldownMinutes = 30
	settings.WatchDedupPriceToleranceBps = 5
	SetRuntimeSettings(settings)

	now := time.Now().UTC()
	repo := &watchDedupRepo{
		watchJournal: []WatchJournal{{
			ID:         "watch_1",
			Symbol:     "SOLUSDT",
			Direction:  LONG,
			Playbook:   LIQUIDITY_SWEEP_REVERSAL,
			EntryPrice: 100,
			StopLoss:   98,
			TP2:        105,
			Status:     WATCH_MONITORING,
			CreatedAt:  now.Add(-10 * time.Minute),
			ExpiresAt:  now.Add(110 * time.Minute),
			UpdatedAt:  now.Add(-5 * time.Minute),
		}},
	}

	storage := NewStorageUsecase(repo)
	err := storage.SaveWatchToJournal(WatchJournal{
		ID:         "watch_new",
		Symbol:     "SOLUSDT",
		Direction:  LONG,
		Playbook:   LIQUIDITY_SWEEP_REVERSAL,
		EntryPrice: 100.20,
		StopLoss:   97.80,
		TP2:        105.30,
		Status:     WATCH_MONITORING,
		CreatedAt:  now,
		ExpiresAt:  now.Add(2 * time.Hour),
		Reason:     "materially changed setup",
	})
	if err != nil {
		t.Fatalf("SaveWatchToJournal: %v", err)
	}

	if repo.appendCount != 1 {
		t.Fatalf("expected append for setup outside runtime tolerance, got %d", repo.appendCount)
	}
	if repo.upsertCount != 0 {
		t.Fatalf("expected no upsert for materially different watch, got %d", repo.upsertCount)
	}
	if len(repo.watchJournal) != 2 {
		t.Fatalf("expected two watch rows for materially different setup, got %d", len(repo.watchJournal))
	}
}

func TestSaveWatchToJournal_ExpiredActiveWatchCreatesNewRow(t *testing.T) {
	now := time.Now().UTC()
	repo := &watchDedupRepo{
		watchJournal: []WatchJournal{{
			ID:         "watch_old",
			Symbol:     "SOLUSDT",
			Direction:  LONG,
			Playbook:   LIQUIDITY_SWEEP_REVERSAL,
			EntryPrice: 100,
			StopLoss:   98,
			TP2:        105,
			Status:     WATCH_MONITORING,
			CreatedAt:  now.Add(-3 * time.Hour),
			ExpiresAt:  now.Add(-30 * time.Minute),
			UpdatedAt:  now.Add(-40 * time.Minute),
		}},
	}

	storage := NewStorageUsecase(repo)
	err := storage.SaveWatchToJournal(WatchJournal{
		ID:         "watch_new",
		Symbol:     "SOLUSDT",
		Direction:  LONG,
		Playbook:   LIQUIDITY_SWEEP_REVERSAL,
		EntryPrice: 100.1,
		StopLoss:   98.05,
		TP2:        105.1,
		Status:     WATCH_MONITORING,
		CreatedAt:  now,
		ExpiresAt:  now.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("SaveWatchToJournal: %v", err)
	}

	if repo.appendCount != 1 {
		t.Fatalf("expected new append after expired active watch, got %d", repo.appendCount)
	}
	if repo.upsertCount != 0 {
		t.Fatalf("expected no upsert for expired active watch, got %d", repo.upsertCount)
	}
	if len(repo.watchJournal) != 2 {
		t.Fatalf("expected new row alongside expired watch, got %d", len(repo.watchJournal))
	}
}

func TestSaveWatchToJournal_UsesTargetedCandidateLookupWhenAvailable(t *testing.T) {
	now := time.Now().UTC()
	base := &watchDedupRepo{
		watchJournal: []WatchJournal{
			{
				ID:         "watch_sol",
				Symbol:     "SOLUSDT",
				Direction:  LONG,
				Playbook:   LIQUIDITY_SWEEP_REVERSAL,
				EntryPrice: 100,
				StopLoss:   98,
				TP2:        105,
				Status:     WATCH_MONITORING,
				CreatedAt:  now.Add(-10 * time.Minute),
				ExpiresAt:  now.Add(110 * time.Minute),
			},
			{
				ID:         "watch_btc",
				Symbol:     "BTCUSDT",
				Direction:  LONG,
				Playbook:   LIQUIDITY_SWEEP_REVERSAL,
				EntryPrice: 70000,
				StopLoss:   69000,
				TP2:        72000,
				Status:     WATCH_MONITORING,
				CreatedAt:  now.Add(-30 * time.Minute),
				ExpiresAt:  now.Add(90 * time.Minute),
			},
		},
	}
	repo := &watchDedupFinderRepo{watchDedupRepo: base}

	storage := NewStorageUsecase(repo)
	err := storage.SaveWatchToJournal(WatchJournal{
		ID:         "watch_new",
		Symbol:     "SOLUSDT",
		Direction:  LONG,
		Playbook:   LIQUIDITY_SWEEP_REVERSAL,
		EntryPrice: 100.05,
		StopLoss:   98.02,
		TP2:        105.04,
		Status:     WATCH_MONITORING,
		CreatedAt:  now,
		ExpiresAt:  now.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("SaveWatchToJournal: %v", err)
	}

	if repo.findCount != 1 {
		t.Fatalf("expected targeted candidate finder to be used once, got %d", repo.findCount)
	}
	if repo.loadCount != 0 {
		t.Fatalf("expected full watch journal load to be skipped, got %d", repo.loadCount)
	}
}

func TestSaveWatchToJournal_UpdatesHotMetadataOnDedupMerge(t *testing.T) {
	now := time.Now().UTC()
	repo := &watchDedupRepo{
		watchJournal: []WatchJournal{{
			ID:                 "watch_1",
			Symbol:             "SOLUSDT",
			Direction:          LONG,
			Playbook:           LIQUIDITY_SWEEP_REVERSAL,
			EntryPrice:         100,
			StopLoss:           98,
			TP2:                105,
			Status:             WATCH_MONITORING,
			CreatedAt:          now.Add(-10 * time.Minute),
			ExpiresAt:          now.Add(110 * time.Minute),
			UpdatedAt:          now.Add(-5 * time.Minute),
			Reason:             "wait retest",
			IsHot:              false,
			HotScore:           0,
			HotSource:          "",
			HotRankType:        0,
			HotOverlaySelected: false,
		}},
	}

	storage := NewStorageUsecase(repo)
	err := storage.SaveWatchToJournal(WatchJournal{
		ID:                 "watch_new",
		Symbol:             "SOLUSDT",
		Direction:          LONG,
		Playbook:           LIQUIDITY_SWEEP_REVERSAL,
		EntryPrice:         100.01,
		StopLoss:           98.01,
		TP2:                105.02,
		Status:             WATCH_MONITORING,
		CreatedAt:          now,
		ExpiresAt:          now.Add(2 * time.Hour),
		Reason:             "wait retest",
		IsHot:              true,
		HotScore:           82,
		HotSource:          "Trending, Social Hype",
		HotRankType:        30,
		HotOverlaySelected: true,
	})
	if err != nil {
		t.Fatalf("SaveWatchToJournal: %v", err)
	}

	if repo.appendCount != 0 {
		t.Fatalf("expected no append for merged watch, got %d", repo.appendCount)
	}
	if repo.upsertCount != 1 {
		t.Fatalf("expected one upsert when hot metadata changes, got %d", repo.upsertCount)
	}
	if len(repo.watchJournal) != 1 {
		t.Fatalf("expected single merged watch row, got %d", len(repo.watchJournal))
	}
	merged := repo.watchJournal[0]
	if !merged.IsHot {
		t.Fatalf("expected merged watch to be marked hot")
	}
	if merged.HotScore != 82 {
		t.Fatalf("expected hot score to be updated, got %v", merged.HotScore)
	}
	if merged.HotSource != "Trending, Social Hype" {
		t.Fatalf("expected hot source to be updated, got %q", merged.HotSource)
	}
	if merged.HotRankType != 30 {
		t.Fatalf("expected hot rank type to be updated, got %d", merged.HotRankType)
	}
	if !merged.HotOverlaySelected {
		t.Fatalf("expected hot overlay flag to be updated")
	}
}
