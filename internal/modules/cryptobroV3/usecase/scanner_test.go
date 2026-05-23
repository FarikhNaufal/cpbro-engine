package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
	"cpbro-engine/internal/modules/cryptobroV3/entity"
)

// Mock implementations
type mockMarketDataProvider struct {
	tickers      []dto.Ticker24h
	fundingRates map[string]float64
	m15Candles   map[string][]dto.Candle
	h1Candles    map[string][]dto.Candle
	h4Candles    map[string][]dto.Candle
	prices       map[string]float64
}

func (m *mockMarketDataProvider) FetchClosedCandles(ctx context.Context, symbol string, interval string, limit int) ([]dto.Candle, error) {
	if interval == "15m" {
		if c, ok := m.m15Candles[symbol]; ok {
			return c, nil
		}
	} else if interval == "1h" {
		if c, ok := m.h1Candles[symbol]; ok {
			return c, nil
		}
	} else if interval == "4h" {
		if c, ok := m.h4Candles[symbol]; ok {
			return c, nil
		}
	}
	return nil, errors.New("no candles mock found for symbol: " + symbol + " and interval: " + interval)
}

func (m *mockMarketDataProvider) FetchLatestPrice(ctx context.Context, symbol string) (float64, error) {
	if p, ok := m.prices[symbol]; ok {
		return p, nil
	}
	return 0.0, nil
}

func (m *mockMarketDataProvider) FetchAllFuturesTickers24h(ctx context.Context) ([]dto.Ticker24h, error) {
	if m.tickers == nil {
		return nil, errors.New("tickers unavailable")
	}
	return m.tickers, nil
}

func (m *mockMarketDataProvider) FetchPremiumFundingRates(ctx context.Context) (map[string]float64, error) {
	return m.fundingRates, nil
}

func (m *mockMarketDataProvider) FetchOpenInterest(ctx context.Context, symbol string) (float64, error) {
	return 1000000.0, nil
}

func (m *mockMarketDataProvider) FetchHistoricalCandles(ctx context.Context, symbol string, interval string, startTime time.Time, endTime time.Time) ([]dto.Candle, error) {
	if interval == "15m" {
		return m.m15Candles[symbol], nil
	} else if interval == "1h" {
		return m.h1Candles[symbol], nil
	} else if interval == "4h" {
		return m.h4Candles[symbol], nil
	}
	return nil, nil
}

type mockAIAuditor struct {
	response dto.AIAuditResponse
	err      error
}

func (m *mockAIAuditor) AuditCandidate(ctx context.Context, req dto.AIAuditRequest) (*dto.AIAuditResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &m.response, nil
}

type mockNotification struct {
	signalMsgs []string
	opsMsgs    []string
}

func (m *mockNotification) SendSignalMessage(ctx context.Context, msg string) error {
	m.signalMsgs = append(m.signalMsgs, msg)
	return nil
}

func (m *mockNotification) SendOpsMessage(ctx context.Context, msg string) error {
	m.opsMsgs = append(m.opsMsgs, msg)
	return nil
}

type mockStorageRepo struct {
	latestResult *entity.LatestResult
	history      *entity.SignalHistory
	journal      []SignalJournal
	auditCache   *entity.AIAuditCache
	evalReport   *EvaluationReport
	audits       []DecisionAudit
}

func (m *mockStorageRepo) LoadLatestResult() (*entity.LatestResult, error) {
	if m.latestResult == nil {
		return nil, errors.New("no latest result")
	}
	return m.latestResult, nil
}

func (m *mockStorageRepo) SaveLatestResult(res *entity.LatestResult) error {
	m.latestResult = res
	return nil
}

func (m *mockStorageRepo) LoadSignalHistory() (*entity.SignalHistory, error) {
	if m.history == nil {
		return &entity.SignalHistory{}, nil
	}
	return m.history, nil
}

func (m *mockStorageRepo) SaveSignalHistory(hist *entity.SignalHistory) error {
	m.history = hist
	return nil
}

func (m *mockStorageRepo) LoadSignalJournal() ([]SignalJournal, error) {
	return m.journal, nil
}

func (m *mockStorageRepo) SaveSignalJournal(journal []SignalJournal) error {
	m.journal = journal
	return nil
}

func (m *mockStorageRepo) AppendSignalJournal(entry SignalJournal) error {
	m.journal = append(m.journal, entry)
	return nil
}

func (m *mockStorageRepo) LoadAIAuditCache() (*entity.AIAuditCache, error) {
	return m.auditCache, nil
}

func (m *mockStorageRepo) SaveAIAuditCache(cache *entity.AIAuditCache) error {
	m.auditCache = cache
	return nil
}

func (m *mockStorageRepo) LoadEvaluationReport() (*EvaluationReport, error) {
	return m.evalReport, nil
}

func (m *mockStorageRepo) SaveEvaluationReport(report *EvaluationReport) error {
	m.evalReport = report
	return nil
}

func (m *mockStorageRepo) LoadDecisionAudits() ([]DecisionAudit, error) {
	return m.audits, nil
}

func (m *mockStorageRepo) SaveDecisionAudits(audits []DecisionAudit) error {
	m.audits = audits
	return nil
}

func (m *mockStorageRepo) AppendDecisionAudit(entry DecisionAudit) error {
	m.audits = append(m.audits, entry)
	if len(m.audits) > 1000 {
		m.audits = m.audits[len(m.audits)-1000:]
	}
	return nil
}

func generateFreshCandles(startPrice float64) []dto.Candle {
	var candles []dto.Candle
	baseTime := time.Now().Add(-60 * 15 * time.Minute)
	for i := 0; i < 60; i++ {
		t := baseTime.Add(time.Duration(i) * 15 * time.Minute)
		candles = append(candles, dto.Candle{
			Time:  t,
			Open:  startPrice,
			High:  startPrice + 1.0,
			Low:   startPrice - 1.0,
			Close: startPrice,
			Vol:   1000.0,
		})
	}
	return candles
}

func TestScannerUsecase_Run(t *testing.T) {
	// Initialize Mock Services
	tickers := []dto.Ticker24h{
		{
			Symbol:             "BTCUSDT",
			LastPrice:          50000.0,
			PriceChangePercent: 2.0,
			QuoteVolume:        1000000000.0,
		},
		{
			Symbol:             "ETHUSDT",
			LastPrice:          3000.0,
			PriceChangePercent: 1.5,
			QuoteVolume:        500000000.0,
		},
		{
			Symbol:             "SOLUSDT",
			LastPrice:          100.0,
			PriceChangePercent: 1.0,
			QuoteVolume:        200000000.0,
		},
	}

	fundingRates := map[string]float64{
		"BTCUSDT": 0.0001,
		"ETHUSDT": 0.0001,
		"SOLUSDT": 0.0001,
	}

	freshM15 := generateFreshCandles(100.0)
	freshH1 := generateFreshCandles(100.0)
	freshH4 := generateFreshCandles(100.0)

	// Inject candle mocks for BTC, ETH, and SOL
	m15Candles := map[string][]dto.Candle{
		"SOLUSDT": freshM15,
		"BTCUSDT": freshM15,
		"ETHUSDT": freshM15,
	}
	h1Candles := map[string][]dto.Candle{
		"SOLUSDT": freshH1,
		"BTCUSDT": freshH1,
		"ETHUSDT": freshH1,
	}
	h4Candles := map[string][]dto.Candle{
		"SOLUSDT": freshH4,
		"BTCUSDT": freshH4,
		"ETHUSDT": freshH4,
	}

	prices := map[string]float64{
		"SOLUSDT": 100.0,
		"BTCUSDT": 50000.0,
		"ETHUSDT": 3000.0,
	}

	mockProvider := &mockMarketDataProvider{
		tickers:      tickers,
		fundingRates: fundingRates,
		m15Candles:   m15Candles,
		h1Candles:    h1Candles,
		h4Candles:    h4Candles,
		prices:       prices,
	}

	mockAI := &mockAIAuditor{
		response: dto.AIAuditResponse{
			Symbol:          "SOLUSDT",
			Decision:        "CONFIRM",
			Confidence:      "HIGH",
			IsApproved:      true,
			Sentiment:       "BULLISH",
			HasRejection:    false,
			HasConfirmation: true,
			CandleNarrative: "Very strong upward breakout.",
			EntryTiming:     "NOW",
		},
	}

	mockNotify := &mockNotification{}
	mockStorage := &mockStorageRepo{
		journal: []SignalJournal{},
		audits:  []DecisionAudit{},
	}

	// Initialize actual Usecases with mocks injected
	marketDataUC := NewMarketDataUsecase(mockProvider)
	marketPolicyUC := NewMarketPolicyUsecase()
	universeUC := NewUniverseUsecase()
	strategySelectorUC := NewStrategySelectorUsecase()
	playbookEligibilityUC := NewPlaybookEligibilityUsecase()
	playbookQuantEngineUC := NewPlaybookQuantEngineUsecase()
	scoringUC := NewScoringUsecase()
	candidateArbiterUC := NewCandidateArbiterUsecase()
	localGateUC := NewLocalGateUsecase()
	aiCandidateSelectorUC := NewAICandidateSelectorUsecase(60.0)
	aiAuditorUC := NewAIAuditorUsecase(mockAI, NewStorageUsecase(mockStorage))
	planReconciliationUC := NewPlanReconciliationUsecase()
	stalenessUC := NewStalenessUsecase(30 * time.Minute)
	finalGateUC := NewFinalGateUsecase()
	conflictResolverUC := NewConflictResolverUsecase()
	signalNotificationUC := NewSignalNotificationUsecase(mockNotify)
	opsNotificationUC := NewOpsNotificationUsecase(mockNotify)
	monitoringUC := NewMonitoringUsecase(mockProvider, NewStorageUsecase(mockStorage))
	feedbackUC := NewFeedbackUsecase(NewStorageUsecase(mockStorage))
	storageUC := NewStorageUsecase(mockStorage)

	uc := NewScannerUsecase(
		marketDataUC,
		marketPolicyUC,
		universeUC,
		strategySelectorUC,
		playbookEligibilityUC,
		playbookQuantEngineUC,
		scoringUC,
		candidateArbiterUC,
		localGateUC,
		aiCandidateSelectorUC,
		aiAuditorUC,
		planReconciliationUC,
		stalenessUC,
		finalGateUC,
		conflictResolverUC,
		signalNotificationUC,
		opsNotificationUC,
		monitoringUC,
		feedbackUC,
		storageUC,
	)

	t.Run("Normal Scan Execution Flow", func(t *testing.T) {
		// Mock dynamic selection of SOLUSDT for COMPRESSION_BREAKOUT_RETEST
		for i := 40; i < 50; i++ {
			freshM15[i].Close = 105.0 // rise to look bullish
		}
		freshM15[59].Close = 110.0 // breakout close

		ctx := context.Background()
		res, err := uc.Run(ctx, dto.ScanRequest{
			TriggerTime: time.Now(),
		})
		if err != nil {
			t.Fatalf("scanner run failed: %v", err)
		}

		// Verify that a latest result was saved to storage
		latest, err := storageUC.LoadLatestResult()
		if err != nil {
			t.Fatalf("failed to load latest result: %v", err)
		}
		if latest.ScanID == "" {
			t.Errorf("expected ScanID to be populated")
		}

		// Verify decision audit trail was saved
		audits, err := storageUC.LoadDecisionAudits()
		if err != nil {
			t.Fatalf("failed to load decision audits: %v", err)
		}
		if len(audits) == 0 {
			t.Logf("Warning: no candidates reached local gate to audit")
		} else {
			audit := audits[0]
			if audit.ScanID == "" {
				t.Errorf("expected audit ScanID to be populated")
			}
		}

		_ = res
	})

	t.Run("Scan Fails when Binance Tickers unavailable", func(t *testing.T) {
		mockProvider.tickers = nil
		ctx := context.Background()
		_, err := uc.Run(ctx, dto.ScanRequest{
			TriggerTime: time.Now(),
		})
		if err == nil {
			t.Errorf("expected scanner to fail when tickers are nil")
		}
	})

	t.Run("AI_AUDIT_ENABLED=false cannot produce FINAL_EXECUTE", func(t *testing.T) {
		t.Setenv("AI_AUDIT_ENABLED", "false")
		// Ensure mocks remain populated for the second run.
		mockProvider.tickers = tickers

		ctx := context.Background()
		_, err := uc.Run(ctx, dto.ScanRequest{
			TriggerTime: time.Now(),
		})
		if err != nil {
			t.Fatalf("scanner run failed: %v", err)
		}

		latest, err := storageUC.LoadLatestResult()
		if err != nil {
			t.Fatalf("failed to load latest result: %v", err)
		}
		if latest.TotalFinalExecute != 0 {
			t.Fatalf("expected TotalFinalExecute=0 when AI is disabled, got %d", latest.TotalFinalExecute)
		}
		if latest.TotalAIConfirm != 0 {
			t.Fatalf("expected TotalAIConfirm=0 when AI is disabled, got %d", latest.TotalAIConfirm)
		}
	})
}
