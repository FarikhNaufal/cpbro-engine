package usecase_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
	"cpbro-engine/internal/modules/cryptobroV3/entity"
	"cpbro-engine/internal/modules/cryptobroV3/usecase"
)

type mockStorageRepo struct {
	journal   []usecase.SignalJournal
	watch     []usecase.WatchJournal
	saved     bool
	saveCount int
}

func (m *mockStorageRepo) LoadLatestResult() (*entity.LatestResult, error) { return nil, nil }
func (m *mockStorageRepo) SaveLatestResult(res *entity.LatestResult) error { return nil }

func (m *mockStorageRepo) LoadSignalHistory() (*entity.SignalHistory, error)  { return nil, nil }
func (m *mockStorageRepo) SaveSignalHistory(hist *entity.SignalHistory) error { return nil }

func (m *mockStorageRepo) LoadAIAuditCache() (*entity.AIAuditCache, error)   { return nil, nil }
func (m *mockStorageRepo) SaveAIAuditCache(cache *entity.AIAuditCache) error { return nil }

func (m *mockStorageRepo) LoadEvaluationReport() (*usecase.EvaluationReport, error)    { return nil, nil }
func (m *mockStorageRepo) SaveEvaluationReport(report *usecase.EvaluationReport) error { return nil }

func (m *mockStorageRepo) LoadDecisionAudits() ([]usecase.DecisionAudit, error)    { return nil, nil }
func (m *mockStorageRepo) SaveDecisionAudits(audits []usecase.DecisionAudit) error { return nil }
func (m *mockStorageRepo) AppendDecisionAudit(entry usecase.DecisionAudit) error   { return nil }

func (m *mockStorageRepo) LoadSignalJournal() ([]usecase.SignalJournal, error) {
	return m.journal, nil
}

func (m *mockStorageRepo) SaveSignalJournal(journal []usecase.SignalJournal) error {
	m.journal = journal
	m.saved = true
	m.saveCount++
	return nil
}

func (m *mockStorageRepo) AppendSignalJournal(entry usecase.SignalJournal) error {
	m.journal = append(m.journal, entry)
	m.saved = true
	m.saveCount++
	return nil
}

func (m *mockStorageRepo) LoadWatchJournal() ([]usecase.WatchJournal, error) {
	return m.watch, nil
}

func (m *mockStorageRepo) SaveWatchJournal(journal []usecase.WatchJournal) error {
	m.watch = journal
	m.saved = true
	m.saveCount++
	return nil
}

func (m *mockStorageRepo) AppendWatchJournal(entry usecase.WatchJournal) error {
	m.watch = append(m.watch, entry)
	m.saved = true
	m.saveCount++
	return nil
}

func (m *mockStorageRepo) UpsertSignalJournalEntries(entries []usecase.SignalJournal) error {
	indexByID := make(map[string]int, len(m.journal))
	for i, entry := range m.journal {
		indexByID[entry.ID] = i
	}
	for _, entry := range entries {
		if idx, ok := indexByID[entry.ID]; ok {
			m.journal[idx] = entry
			continue
		}
		indexByID[entry.ID] = len(m.journal)
		m.journal = append(m.journal, entry)
	}
	m.saved = true
	m.saveCount++
	return nil
}

func (m *mockStorageRepo) UpsertWatchJournalEntries(entries []usecase.WatchJournal) error {
	indexByID := make(map[string]int, len(m.watch))
	for i, entry := range m.watch {
		indexByID[entry.ID] = i
	}
	for _, entry := range entries {
		if idx, ok := indexByID[entry.ID]; ok {
			m.watch[idx] = entry
			continue
		}
		indexByID[entry.ID] = len(m.watch)
		m.watch = append(m.watch, entry)
	}
	m.saved = true
	m.saveCount++
	return nil
}

type mockMarketDataProvider struct {
	mu               sync.Mutex
	candles          []dto.Candle
	price            float64
	fetchClosedCalls int
}

func (m *mockMarketDataProvider) FetchClosedCandles(ctx context.Context, symbol string, interval string, limit int) ([]dto.Candle, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fetchClosedCalls++
	return append([]dto.Candle(nil), m.candles...), nil
}

func (m *mockMarketDataProvider) FetchLatestPrice(ctx context.Context, symbol string) (float64, error) {
	return m.price, nil
}

func (m *mockMarketDataProvider) FetchAllFuturesTickers24h(ctx context.Context) ([]dto.Ticker24h, error) {
	return nil, nil
}

func (m *mockMarketDataProvider) FetchPremiumFundingRates(ctx context.Context) (map[string]float64, error) {
	return nil, nil
}

func (m *mockMarketDataProvider) FetchOpenInterest(ctx context.Context, symbol string) (float64, error) {
	return 0, nil
}

func (m *mockMarketDataProvider) FetchHistoricalCandles(ctx context.Context, symbol string, interval string, startTime time.Time, endTime time.Time) ([]dto.Candle, error) {
	return m.candles, nil
}

type mockMonitoringLatestPriceFeed struct {
	synced []string
	prices map[string]struct {
		price float64
		at    time.Time
		ok    bool
	}
}

func (m *mockMonitoringLatestPriceFeed) SyncSymbols(symbols []string) error {
	m.synced = append([]string(nil), symbols...)
	return nil
}

func (m *mockMonitoringLatestPriceFeed) GetLatestPrice(symbol string) (float64, time.Time, bool) {
	if m == nil || m.prices == nil {
		return 0, time.Time{}, false
	}
	entry, ok := m.prices[symbol]
	if !ok {
		return 0, time.Time{}, false
	}
	return entry.price, entry.at, entry.ok
}

func TestMonitoring_MonitorVirtualPositions_SLHit(t *testing.T) {
	createdAt := time.Now().Add(-30 * time.Minute)
	expiresAt := createdAt.Add(120 * time.Minute)

	journal := []usecase.SignalJournal{
		{
			ID:         "test_signal_sl",
			Symbol:     "BTCUSDT",
			Direction:  usecase.LONG,
			EntryPrice: 100.0,
			StopLoss:   95.0,
			TP1:        105.0,
			TP2:        110.0,
			CreatedAt:  createdAt,
			ExpiresAt:  expiresAt,
			Status:     usecase.MONITORING,
			MFE:        0.0,
			MAE:        0.0,
		},
	}

	repo := &mockStorageRepo{journal: journal}
	storage := usecase.NewStorageUsecase(repo)

	candles := []dto.Candle{
		// Candle after CreatedAt that hits SL
		{
			Time:  createdAt.Add(15 * time.Minute),
			Open:  100.0,
			High:  101.0,
			Low:   94.5, // Hits SL
			Close: 96.0,
		},
	}

	provider := &mockMarketDataProvider{candles: candles, price: 96.0}
	monitor := usecase.NewMonitoringUsecase(provider, storage)

	err := monitor.MonitorVirtualPositions(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updatedJournal, _ := storage.LoadSignalJournal()
	if len(updatedJournal) != 1 {
		t.Fatalf("expected 1 journal entry, got %d", len(updatedJournal))
	}

	item := updatedJournal[0]
	if item.Status != usecase.SL_HIT {
		t.Errorf("expected SL_HIT status, got %s", item.Status)
	}
	if item.MAE != 5.5 { // (100 - 94.5)/100 * 100
		t.Errorf("expected MAE 5.5%%, got %0.2f%%", item.MAE)
	}
	if item.MFE != 1.0 { // (101 - 100)/100 * 100
		t.Errorf("expected MFE 1.0%%, got %0.2f%%", item.MFE)
	}
}

func TestMonitoring_MonitorVirtualPositions_TP1andTP2Hit(t *testing.T) {
	createdAt := time.Now().Add(-30 * time.Minute)
	expiresAt := createdAt.Add(120 * time.Minute)

	journal := []usecase.SignalJournal{
		{
			ID:         "test_signal_tp",
			Symbol:     "ETHUSDT",
			Direction:  usecase.LONG,
			EntryPrice: 100.0,
			StopLoss:   95.0,
			TP1:        105.0,
			TP2:        110.0,
			CreatedAt:  createdAt,
			ExpiresAt:  expiresAt,
			Status:     usecase.MONITORING,
			MFE:        0.0,
			MAE:        0.0,
		},
	}

	repo := &mockStorageRepo{journal: journal}
	storage := usecase.NewStorageUsecase(repo)

	candles := []dto.Candle{
		// First candle hits TP1
		{
			Time:  createdAt.Add(10 * time.Minute),
			Open:  100.0,
			High:  106.0, // Hits TP1
			Low:   99.0,
			Close: 104.0,
		},
		// Second candle hits TP2
		{
			Time:  createdAt.Add(20 * time.Minute),
			Open:  104.0,
			High:  111.0, // Hits TP2
			Low:   103.0,
			Close: 109.0,
		},
	}

	provider := &mockMarketDataProvider{candles: candles, price: 109.0}
	monitor := usecase.NewMonitoringUsecase(provider, storage)

	err := monitor.MonitorVirtualPositions(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updatedJournal, _ := storage.LoadSignalJournal()
	item := updatedJournal[0]
	if item.Status != usecase.TP2_HIT {
		t.Errorf("expected TP2_HIT status, got %s", item.Status)
	}
	if item.MFE != 11.0 { // (111 - 100)/100 * 100
		t.Errorf("expected MFE 11.0%%, got %0.2f%%", item.MFE)
	}
}

func TestMonitoring_MonitorVirtualPositions_Expired(t *testing.T) {
	// Signal created 2.5 hours ago (past the 120 minutes expiry)
	createdAt := time.Now().Add(-150 * time.Minute)
	expiresAt := createdAt.Add(120 * time.Minute)

	journal := []usecase.SignalJournal{
		{
			ID:         "test_signal_expired",
			Symbol:     "SOLUSDT",
			Direction:  usecase.LONG,
			EntryPrice: 100.0,
			StopLoss:   95.0,
			TP1:        105.0,
			TP2:        110.0,
			CreatedAt:  createdAt,
			ExpiresAt:  expiresAt,
			Status:     usecase.MONITORING,
			MFE:        0.0,
			MAE:        0.0,
		},
	}

	repo := &mockStorageRepo{journal: journal}
	storage := usecase.NewStorageUsecase(repo)

	// Candles do not hit TP or SL
	candles := []dto.Candle{
		{
			Time:  createdAt.Add(15 * time.Minute),
			Open:  100.0,
			High:  102.0,
			Low:   98.0,
			Close: 101.0,
		},
	}

	provider := &mockMarketDataProvider{candles: candles, price: 101.0}
	monitor := usecase.NewMonitoringUsecase(provider, storage)

	err := monitor.MonitorVirtualPositions(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updatedJournal, _ := storage.LoadSignalJournal()
	item := updatedJournal[0]
	if item.Status != usecase.EXPIRED {
		t.Errorf("expected EXPIRED status, got %s", item.Status)
	}
}

func TestMonitoring_MonitorVirtualPositions_ExpiredAfterTP1KeepsPartialPnL(t *testing.T) {
	createdAt := time.Now().Add(-150 * time.Minute)
	expiresAt := createdAt.Add(120 * time.Minute)

	journal := []usecase.SignalJournal{
		{
			ID:         "test_signal_expired_tp1",
			Symbol:     "SOLUSDT",
			Direction:  usecase.LONG,
			EntryPrice: 100.0,
			StopLoss:   95.0,
			TP1:        105.0,
			TP2:        110.0,
			CreatedAt:  createdAt,
			ExpiresAt:  expiresAt,
			Status:     usecase.TP1_HIT,
			TimeToTP1:  "10m",
			MFE:        5.0,
			MAE:        1.0,
		},
	}

	repo := &mockStorageRepo{journal: journal}
	storage := usecase.NewStorageUsecase(repo)
	provider := &mockMarketDataProvider{candles: nil, price: 104.0}
	monitor := usecase.NewMonitoringUsecase(provider, storage)

	err := monitor.MonitorVirtualPositions(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updatedJournal, _ := storage.LoadSignalJournal()
	item := updatedJournal[0]
	if item.Status != usecase.TP1_HIT {
		t.Fatalf("expected TP1_HIT status to remain for partial completion, got %s", item.Status)
	}
	if item.OutcomeReason != "Monitoring period expired (120m elapsed) with TP1 success" {
		t.Fatalf("unexpected outcome reason: %s", item.OutcomeReason)
	}
	if item.PnlPercentage != 4.5 {
		t.Fatalf("expected partial realized pnl 4.5, got %v", item.PnlPercentage)
	}
	if item.ClosedAt.IsZero() {
		t.Fatal("expected ClosedAt to be set for finalized TP1 partial outcome")
	}
}

func TestMonitoring_MonitorVirtualWatchPositions_TP1Hit(t *testing.T) {
	createdAt := time.Now().Add(-20 * time.Minute)
	expiresAt := createdAt.Add(120 * time.Minute)

	repo := &mockStorageRepo{
		watch: []usecase.WatchJournal{
			{
				ID:         "watch_signal_tp1",
				Symbol:     "XRPUSDT",
				Direction:  usecase.LONG,
				EntryPrice: 100.0,
				StopLoss:   95.0,
				TP1:        104.0,
				TP2:        108.0,
				CreatedAt:  createdAt,
				ExpiresAt:  expiresAt,
				Status:     usecase.WATCH_MONITORING,
			},
		},
	}
	storage := usecase.NewStorageUsecase(repo)
	provider := &mockMarketDataProvider{
		candles: []dto.Candle{
			{
				Time:  createdAt.Add(15 * time.Minute),
				Open:  100.0,
				High:  104.5,
				Low:   99.5,
				Close: 103.5,
			},
		},
		price: 103.5,
	}

	monitor := usecase.NewMonitoringUsecase(provider, storage)
	if err := monitor.MonitorVirtualPositions(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	watchJournal, _ := storage.LoadWatchJournal()
	if len(watchJournal) != 1 {
		t.Fatalf("expected 1 watch journal entry, got %d", len(watchJournal))
	}
	if watchJournal[0].Status != usecase.VIRTUAL_TP1_HIT {
		t.Fatalf("expected VIRTUAL_TP1_HIT, got %s", watchJournal[0].Status)
	}

	signalJournal, _ := storage.LoadSignalJournal()
	if len(signalJournal) != 0 {
		t.Fatalf("expected signal journal to remain untouched, got %d rows", len(signalJournal))
	}
}

func TestMonitoring_MonitorVirtualPositions_SyncsActiveSymbolsAndUsesRealtimeFeed(t *testing.T) {
	now := time.Now()
	repo := &mockStorageRepo{
		journal: []usecase.SignalJournal{
			{
				ID:         "active_execute",
				Symbol:     "BTCUSDT",
				Direction:  usecase.LONG,
				EntryPrice: 100.0,
				StopLoss:   95.0,
				TP1:        105.0,
				TP2:        110.0,
				CreatedAt:  now.Add(-10 * time.Minute),
				ExpiresAt:  now.Add(110 * time.Minute),
				Status:     usecase.MONITORING,
			},
			{
				ID:         "inactive_execute",
				Symbol:     "SOLUSDT",
				Direction:  usecase.LONG,
				EntryPrice: 50.0,
				StopLoss:   45.0,
				TP1:        55.0,
				TP2:        60.0,
				CreatedAt:  now.Add(-2 * time.Hour),
				ExpiresAt:  now.Add(-time.Minute),
				Status:     usecase.EXPIRED,
			},
		},
		watch: []usecase.WatchJournal{
			{
				ID:         "active_watch",
				Symbol:     "ETHUSDT",
				Direction:  usecase.SHORT,
				EntryPrice: 200.0,
				StopLoss:   210.0,
				TP1:        190.0,
				TP2:        185.0,
				CreatedAt:  now.Add(-5 * time.Minute),
				ExpiresAt:  now.Add(115 * time.Minute),
				Status:     usecase.WATCH_MONITORING,
			},
		},
	}
	storage := usecase.NewStorageUsecase(repo)
	provider := &mockMarketDataProvider{price: 88.0}
	feed := &mockMonitoringLatestPriceFeed{
		prices: map[string]struct {
			price float64
			at    time.Time
			ok    bool
		}{
			"BTCUSDT": {price: 101.5, at: now, ok: true},
			"ETHUSDT": {price: 198.25, at: now, ok: true},
		},
	}

	monitor := usecase.NewMonitoringUsecase(provider, storage)
	monitor.SetLatestPriceFeed(feed)

	if err := monitor.MonitorVirtualPositions(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(feed.synced) != 2 || feed.synced[0] != "BTCUSDT" || feed.synced[1] != "ETHUSDT" {
		t.Fatalf("unexpected synced symbols: %#v", feed.synced)
	}

	signalJournal, _ := storage.LoadSignalJournal()
	if signalJournal[0].LatestPrice != 101.5 {
		t.Fatalf("expected realtime latest price for execute journal, got %v", signalJournal[0].LatestPrice)
	}

	watchJournal, _ := storage.LoadWatchJournal()
	if watchJournal[0].LatestPrice != 198.25 {
		t.Fatalf("expected realtime latest price for watch journal, got %v", watchJournal[0].LatestPrice)
	}
}

func TestMonitoring_MonitorVirtualPositions_ReusesClosedCandleCacheWithinSameM15Window(t *testing.T) {
	now := time.Now().UTC()
	lastClosedOpen := now.Truncate(15 * time.Minute).Add(-15 * time.Minute)

	repo := &mockStorageRepo{
		journal: []usecase.SignalJournal{
			{
				ID:         "cached_execute",
				Symbol:     "BTCUSDT",
				Direction:  usecase.LONG,
				EntryPrice: 100.0,
				StopLoss:   95.0,
				TP1:        105.0,
				TP2:        110.0,
				CreatedAt:  now.Add(-10 * time.Minute),
				ExpiresAt:  now.Add(110 * time.Minute),
				Status:     usecase.MONITORING,
			},
		},
	}
	storage := usecase.NewStorageUsecase(repo)
	provider := &mockMarketDataProvider{
		price: 100.5,
		candles: []dto.Candle{
			{
				Time:  lastClosedOpen,
				Open:  100.0,
				High:  101.0,
				Low:   99.0,
				Close: 100.5,
			},
		},
	}

	monitor := usecase.NewMonitoringUsecase(provider, storage)
	if err := monitor.MonitorVirtualPositions(context.Background()); err != nil {
		t.Fatalf("unexpected error on first run: %v", err)
	}
	if err := monitor.MonitorVirtualPositions(context.Background()); err != nil {
		t.Fatalf("unexpected error on second run: %v", err)
	}

	provider.mu.Lock()
	defer provider.mu.Unlock()
	if provider.fetchClosedCalls != 1 {
		t.Fatalf("expected closed candles fetched once within same M15 window, got %d", provider.fetchClosedCalls)
	}
}

func TestMonitoring_MonitorVirtualPositions_DoesNotPersistWhenNothingChanged(t *testing.T) {
	now := time.Now().UTC()
	lastClosedOpen := now.Truncate(15 * time.Minute).Add(-15 * time.Minute)

	repo := &mockStorageRepo{
		journal: []usecase.SignalJournal{
			{
				ID:          "stable_execute",
				Symbol:      "BTCUSDT",
				Direction:   usecase.LONG,
				EntryPrice:  100.0,
				StopLoss:    95.0,
				TP1:         105.0,
				TP2:         110.0,
				CreatedAt:   now.Add(-10 * time.Minute),
				ExpiresAt:   now.Add(110 * time.Minute),
				Status:      usecase.MONITORING,
				LatestPrice: 100.0,
			},
		},
	}
	storage := usecase.NewStorageUsecase(repo)
	provider := &mockMarketDataProvider{
		price: 100.0,
		candles: []dto.Candle{
			{
				Time:  lastClosedOpen,
				Open:  100.0,
				High:  100.0,
				Low:   100.0,
				Close: 100.0,
			},
		},
	}
	feed := &mockMonitoringLatestPriceFeed{
		prices: map[string]struct {
			price float64
			at    time.Time
			ok    bool
		}{
			"BTCUSDT": {price: 100.0, at: now, ok: true},
		},
	}

	monitor := usecase.NewMonitoringUsecase(provider, storage)
	monitor.SetLatestPriceFeed(feed)

	if err := monitor.MonitorVirtualPositions(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if repo.saveCount != 0 {
		t.Fatalf("expected no persistence write for unchanged monitoring state, got %d writes", repo.saveCount)
	}
}

// TestMonitoring_LiveTP1AndTP2SameTick_TimestampConsistency verifies that when a live
// price simultaneously crosses both TP1 and TP2 (single tick), both TimeToTP1 and
// TimeToTP2 are set to the SAME elapsed duration — preventing the tp2_before_tp1
// anomaly observed in production (e.g., BNBUSDT 2026-06-07).
func TestMonitoring_LiveTP1AndTP2SameTick_TimestampConsistency(t *testing.T) {
	createdAt := time.Now().Add(-30 * time.Minute)
	expiresAt := createdAt.Add(120 * time.Minute)

	journal := []usecase.SignalJournal{
		{
			ID:         "test_live_tp1tp2_same_tick",
			Symbol:     "BNBUSDT",
			Direction:  usecase.LONG,
			EntryPrice: 600.0,
			StopLoss:   590.0,
			TP1:        615.0,
			TP2:        630.0,
			CreatedAt:  createdAt,
			ExpiresAt:  expiresAt,
			Status:     usecase.MONITORING,
		},
	}

	repo := &mockStorageRepo{journal: journal}
	storage := usecase.NewStorageUsecase(repo)
	// Price 635.0 — above both TP1 (615) and TP2 (630) simultaneously
	provider := &mockMarketDataProvider{candles: nil, price: 635.0}
	monitor := usecase.NewMonitoringUsecase(provider, storage)

	err := monitor.MonitorVirtualPositions(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updated, _ := storage.LoadSignalJournal()
	item := updated[0]

	if item.Status != usecase.TP2_HIT {
		t.Fatalf("expected TP2_HIT, got %s", item.Status)
	}
	if item.TimeToTP1 == "" {
		t.Fatal("expected TimeToTP1 to be set")
	}
	if item.TimeToTP2 == "" {
		t.Fatal("expected TimeToTP2 to be set")
	}

	// Parse durations and verify tp2 >= tp1 (no impossible ordering)
	d1, err1 := time.ParseDuration(item.TimeToTP1)
	d2, err2 := time.ParseDuration(item.TimeToTP2)
	if err1 != nil || err2 != nil {
		t.Fatalf("failed to parse durations: tp1=%q tp2=%q", item.TimeToTP1, item.TimeToTP2)
	}
	if d2 < d1 {
		t.Fatalf("tp2_before_tp1 anomaly: TimeToTP2=%v is less than TimeToTP1=%v", d2, d1)
	}
}
