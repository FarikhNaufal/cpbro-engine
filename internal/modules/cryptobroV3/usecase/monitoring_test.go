package usecase_test

import (
	"context"
	"testing"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
	"cpbro-engine/internal/modules/cryptobroV3/entity"
	"cpbro-engine/internal/modules/cryptobroV3/usecase"
)

type mockStorageRepo struct {
	journal []usecase.SignalJournal
	saved   bool
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
	return nil
}

func (m *mockStorageRepo) AppendSignalJournal(entry usecase.SignalJournal) error {
	m.journal = append(m.journal, entry)
	m.saved = true
	return nil
}

type mockMarketDataProvider struct {
	candles []dto.Candle
	price   float64
}

func (m *mockMarketDataProvider) FetchClosedCandles(ctx context.Context, symbol string, interval string, limit int) ([]dto.Candle, error) {
	return m.candles, nil
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
