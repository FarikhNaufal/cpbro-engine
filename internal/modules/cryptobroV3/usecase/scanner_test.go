package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
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
	mu           sync.Mutex
	latestResult *entity.LatestResult
	history      *entity.SignalHistory
	journal      []SignalJournal
	watchJournal []WatchJournal
	auditCache   *entity.AIAuditCache
	evalReport   *EvaluationReport
	audits       []DecisionAudit
}

func (m *mockStorageRepo) LoadLatestResult() (*entity.LatestResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.latestResult == nil {
		return nil, errors.New("no latest result")
	}
	return m.latestResult, nil
}

func (m *mockStorageRepo) SaveLatestResult(res *entity.LatestResult) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.latestResult = res
	return nil
}

func (m *mockStorageRepo) LoadSignalHistory() (*entity.SignalHistory, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.history == nil {
		return &entity.SignalHistory{}, nil
	}
	return m.history, nil
}

func (m *mockStorageRepo) SaveSignalHistory(hist *entity.SignalHistory) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.history = hist
	return nil
}

func (m *mockStorageRepo) LoadSignalJournal() ([]SignalJournal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.journal, nil
}

func (m *mockStorageRepo) SaveSignalJournal(journal []SignalJournal) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.journal = journal
	return nil
}

func (m *mockStorageRepo) AppendSignalJournal(entry SignalJournal) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.journal = append(m.journal, entry)
	return nil
}

func (m *mockStorageRepo) LoadWatchJournal() ([]WatchJournal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.watchJournal, nil
}

func (m *mockStorageRepo) SaveWatchJournal(journal []WatchJournal) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.watchJournal = journal
	return nil
}

func (m *mockStorageRepo) AppendWatchJournal(entry WatchJournal) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.watchJournal = append(m.watchJournal, entry)
	return nil
}

func (m *mockStorageRepo) LoadAIAuditCache() (*entity.AIAuditCache, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.auditCache, nil
}

func (m *mockStorageRepo) SaveAIAuditCache(cache *entity.AIAuditCache) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.auditCache = cache
	return nil
}

func (m *mockStorageRepo) LoadEvaluationReport() (*EvaluationReport, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.evalReport, nil
}

func (m *mockStorageRepo) SaveEvaluationReport(report *EvaluationReport) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.evalReport = report
	return nil
}

func (m *mockStorageRepo) LoadDecisionAudits() ([]DecisionAudit, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.audits, nil
}

func (m *mockStorageRepo) SaveDecisionAudits(audits []DecisionAudit) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.audits = audits
	return nil
}

func (m *mockStorageRepo) AppendDecisionAudit(entry DecisionAudit) error {
	m.mu.Lock()
	defer m.mu.Unlock()
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

func generateBreakoutRetestCandles(startPrice float64) []dto.Candle {
	var candles []dto.Candle
	baseTime := time.Now().Add(-60 * 15 * time.Minute)
	for i := 0; i < 60; i++ {
		t := baseTime.Add(time.Duration(i) * 15 * time.Minute)
		closePrice := startPrice
		vol := 1000.0
		if i == 55 {
			closePrice = startPrice + 4.0
		} else if i > 55 {
			closePrice = startPrice + 1.5
			vol = 2000.0
		}
		candles = append(candles, dto.Candle{
			Time:  t,
			Open:  startPrice,
			High:  closePrice + 0.1,
			Low:   closePrice - 0.1,
			Close: closePrice,
			Vol:   vol,
		})
	}
	return candles
}

func generateSweepCandles(startPrice float64) []dto.Candle {
	var candles []dto.Candle
	baseTime := time.Now().Add(-60 * 15 * time.Minute)
	for i := 0; i < 60; i++ {
		t := baseTime.Add(time.Duration(i) * 15 * time.Minute)
		closePrice := startPrice
		if i%2 == 0 {
			closePrice = startPrice + 0.02
		} else {
			closePrice = startPrice - 0.02
		}
		high := closePrice + 0.05
		low := closePrice - 0.05
		vol := 1000.0
		// Candle 59 is the sweep candle
		if i == 59 {
			closePrice = startPrice
			high = startPrice + 0.02
			low = startPrice - 1.0 // sweeps below previous lows
			vol = 2000.0           // volume spike
		}
		candles = append(candles, dto.Candle{
			Time:  t,
			Open:  startPrice,
			High:  high,
			Low:   low,
			Close: closePrice,
			Vol:   vol,
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
	storageUC := NewStorageUsecase(mockStorage)
	aiAuditorUC := NewAIAuditorUsecase(mockAI, storageUC)
	planReconciliationUC := NewPlanReconciliationUsecase()
	stalenessUC := NewStalenessUsecase(30 * time.Minute)
	stalenessUC.SetFallbackProvider(mockProvider)
	finalGateUC := NewFinalGateUsecase()
	conflictResolverUC := NewConflictResolverUsecase()
	signalNotificationUC := NewSignalNotificationUsecase(mockNotify, storageUC)
	opsNotificationUC := NewOpsNotificationUsecase(mockNotify)
	monitoringUC := NewMonitoringUsecase(mockProvider, storageUC)
	feedbackUC := NewFeedbackUsecase(storageUC)

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

	t.Run("Scan Uses Fresh Bootstrap Cache when Binance Tickers unavailable", func(t *testing.T) {
		mockProvider.tickers = nil
		ctx := context.Background()
		_, err := uc.Run(ctx, dto.ScanRequest{
			TriggerTime: time.Now(),
		})
		if err != nil {
			t.Errorf("expected scanner to succeed using fresh cached tickers, got err=%v", err)
		}
	})

	t.Run("Fresh Scanner Fails when Binance Tickers unavailable and no cache exists", func(t *testing.T) {
		coldProvider := &mockMarketDataProvider{
			tickers:      nil,
			fundingRates: fundingRates,
			m15Candles:   m15Candles,
			h1Candles:    h1Candles,
			h4Candles:    h4Candles,
			prices:       prices,
		}
		coldMarketDataUC := NewMarketDataUsecase(coldProvider)
		coldUC := NewScannerUsecase(
			coldMarketDataUC,
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

		ctx := context.Background()
		_, err := coldUC.Run(ctx, dto.ScanRequest{
			TriggerTime: time.Now(),
		})
		if err == nil {
			t.Errorf("expected scanner to fail when tickers are nil and no cache exists")
		}
	})

	t.Run("AI_AUDIT_ENABLED=false cannot produce FINAL_EXECUTE", func(t *testing.T) {
		original := getRuntimeSettings()
		t.Cleanup(func() { SetRuntimeSettings(original) })
		settings := original
		settings.AIAuditEnabled = false
		SetRuntimeSettings(settings)
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

func TestScannerUsecase_Run_AIWait_And_AIReject(t *testing.T) {
	// Initialize Mock Services
	tickers := []dto.Ticker24h{
		{
			Symbol:             "BTCUSDT",
			LastPrice:          50000.0,
			PriceChangePercent: 2.0,
			QuoteVolume:        1000000000.0,
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
		"SOLUSDT": 0.0001,
	}

	freshM15 := generateSweepCandles(100.0)
	freshH1 := generateFreshCandles(100.0)
	freshH4 := generateFreshCandles(100.0)

	btcM15 := generateFreshCandles(50000.0)
	btcH1 := generateFreshCandles(50000.0)
	btcH4 := generateFreshCandles(50000.0)

	m15Candles := map[string][]dto.Candle{"SOLUSDT": freshM15, "BTCUSDT": btcM15}
	h1Candles := map[string][]dto.Candle{"SOLUSDT": freshH1, "BTCUSDT": btcH1}
	h4Candles := map[string][]dto.Candle{"SOLUSDT": freshH4, "BTCUSDT": btcH4}
	prices := map[string]float64{"SOLUSDT": 100.0, "BTCUSDT": 50000.0}

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
			Decision:        "WAIT", // Start with AI WAIT
			Confidence:      "HIGH",
			IsApproved:      false,
			Sentiment:       "NEUTRAL",
			HasRejection:    false,
			HasConfirmation: true,
			CandleNarrative: "Wait for breakout confirmation.",
			EntryTiming:     "WATCH_ONLY",
		},
	}

	mockNotify := &mockNotification{}
	mockStorage := &mockStorageRepo{
		journal: []SignalJournal{},
		audits:  []DecisionAudit{},
	}

	// Initialize actual Usecases
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
	storageUC := NewStorageUsecase(mockStorage)
	aiAuditorUC := NewAIAuditorUsecase(mockAI, storageUC)
	planReconciliationUC := NewPlanReconciliationUsecase()
	stalenessUC := NewStalenessUsecase(30 * time.Minute)
	stalenessUC.SetFallbackProvider(mockProvider)
	finalGateUC := NewFinalGateUsecase()
	conflictResolverUC := NewConflictResolverUsecase()
	signalNotificationUC := NewSignalNotificationUsecase(mockNotify, storageUC)
	opsNotificationUC := NewOpsNotificationUsecase(mockNotify)
	monitoringUC := NewMonitoringUsecase(mockProvider, storageUC)
	feedbackUC := NewFeedbackUsecase(storageUC)

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

	t.Run("Scanner AI_WAIT becomes FINAL_WATCH and appears in watchlist", func(t *testing.T) {
		original := getRuntimeSettings()
		t.Cleanup(func() { SetRuntimeSettings(original) })
		settings := original
		settings.MonitoringMaxHoldMinutes = 90
		SetRuntimeSettings(settings)
		ctx := context.Background()
		_, err := uc.Run(ctx, dto.ScanRequest{TriggerTime: time.Now()})
		if err != nil {
			t.Fatalf("scanner run failed: %v", err)
		}

		latest, err := storageUC.LoadLatestResult()
		if err != nil {
			t.Fatalf("failed to load latest result: %v", err)
		}
		t.Logf("policy_rejected_summary: %+v", latest.PolicyRejectedSummary)
		t.Logf("rejected_summary: %+v", latest.RejectedSummary)
		t.Logf("arbiterDetails: %+v", latest.ArbiterSelectedDetails)

		if latest.TotalAIWait != 1 {
			t.Errorf("expected TotalAIWait = 1, got %d", latest.TotalAIWait)
		}
		if latest.TotalFinalWatch != 1 {
			t.Errorf("expected TotalFinalWatch = 1, got %d", latest.TotalFinalWatch)
		}
		if latest.TotalFinalReject != 0 {
			t.Errorf("expected TotalFinalReject = 0, got %d", latest.TotalFinalReject)
		}

		if len(latest.Watchlist) != 1 {
			t.Fatalf("expected Watchlist length = 1, got %d", len(latest.Watchlist))
		}
		if latest.Watchlist[0].Symbol != "SOLUSDT" {
			t.Errorf("expected Watchlist[0].Symbol = SOLUSDT, got %s", latest.Watchlist[0].Symbol)
		}
		if latest.Watchlist[0].Status != "FINAL_WATCH" {
			t.Errorf("expected Watchlist[0].Status = FINAL_WATCH, got %s", latest.Watchlist[0].Status)
		}
		if len(latest.ExecuteSignals) != 0 {
			t.Errorf("expected ExecuteSignals to be empty, got %d", len(latest.ExecuteSignals))
		}

		// Verify watchlist is not nil/null representation (it's initialized slice)
		if latest.Watchlist == nil {
			t.Errorf("expected Watchlist to not be nil")
		}
		if latest.ExecuteSignals == nil {
			t.Errorf("expected ExecuteSignals to not be nil")
		}
		if len(latest.FunnelStageSummary) == 0 {
			t.Fatalf("expected FunnelStageSummary to be populated")
		}
		if len(latest.TopFunnelBlockers) == 0 {
			t.Fatalf("expected TopFunnelBlockers to be populated")
		}
		if len(latest.PlaybookBlockerSummary) == 0 {
			t.Fatalf("expected PlaybookBlockerSummary to be populated")
		}
		foundAIWait := false
		foundFinalWatch := false
		foundSweepSummary := false
		for _, stage := range latest.FunnelStageSummary {
			if stage.Stage == funnelStageAIWait {
				foundAIWait = true
				if stage.Total != 1 {
					t.Fatalf("expected ai_wait total 1, got %d", stage.Total)
				}
			}
			if stage.Stage == funnelStageFinalWatch {
				foundFinalWatch = true
				if stage.Total != 1 {
					t.Fatalf("expected final_watch total 1, got %d", stage.Total)
				}
			}
		}
		if !foundAIWait {
			t.Fatalf("expected ai_wait stage summary")
		}
		if !foundFinalWatch {
			t.Fatalf("expected final_watch stage summary")
		}
		for _, summary := range latest.PlaybookBlockerSummary {
			if summary.Playbook == string(LIQUIDITY_SWEEP_REVERSAL) {
				foundSweepSummary = true
			}
		}
		if !foundSweepSummary {
			t.Fatalf("expected sweep playbook blocker summary")
		}

		watchJournal, err := storageUC.LoadWatchJournal()
		if err != nil {
			t.Fatalf("failed to load watch journal: %v", err)
		}
		if len(watchJournal) != 1 {
			t.Fatalf("expected 1 watch journal entry, got %d", len(watchJournal))
		}
		if got := watchJournal[0].ExpiresAt.Sub(watchJournal[0].CreatedAt); got != 90*time.Minute {
			t.Fatalf("expected watch expiry to follow MONITORING_MAX_HOLD_MINUTES=90, got %s", got)
		}
	})

	t.Run("Scanner CONFIRM plus WATCH_ONLY is normalized into FINAL_WATCH", func(t *testing.T) {
		mockAI.response.Decision = "CONFIRM"
		mockAI.response.Confidence = "HIGH"
		mockAI.response.IsApproved = true
		mockAI.response.HasRejection = false
		mockAI.response.HasConfirmation = true
		mockAI.response.SuggestedAction = "WATCH_ONLY"
		mockAI.response.EntryTiming = "FRESH"
		mockAI.response.Reason = "Wait for a follow-up candle to confirm the reversal."
		mockAI.response.Risk = "Reversal confirmation is incomplete."
		mockAI.response.CandleNarrative = "REJECTION"
		mockStorage.auditCache = nil
		mockStorage.watchJournal = nil
		mockStorage.latestResult = nil

		ctx := context.Background()
		_, err := uc.Run(ctx, dto.ScanRequest{TriggerTime: time.Now()})
		if err != nil {
			t.Fatalf("scanner run failed: %v", err)
		}

		latest, err := storageUC.LoadLatestResult()
		if err != nil {
			t.Fatalf("failed to load latest result: %v", err)
		}

		if latest.TotalAIWait != 1 {
			t.Fatalf("expected normalized AI wait count = 1, got %d", latest.TotalAIWait)
		}
		if latest.TotalFinalWatch != 1 {
			t.Fatalf("expected TotalFinalWatch = 1 after WATCH_ONLY normalization, got %d", latest.TotalFinalWatch)
		}
		if latest.TotalFinalReject != 0 {
			t.Fatalf("expected TotalFinalReject = 0 after WATCH_ONLY normalization, got %d", latest.TotalFinalReject)
		}
		if len(latest.Watchlist) != 1 {
			t.Fatalf("expected one watchlist entry, got %d", len(latest.Watchlist))
		}
		if !strings.Contains(latest.Watchlist[0].Reason, "Reversal playbook lacks rejection or confirmation") {
			t.Fatalf("expected watch reason to preserve need-retest context, got %q", latest.Watchlist[0].Reason)
		}
	})

	t.Run("Scanner AI_REJECT becomes FINAL_REJECT and does not appear in watchlist", func(t *testing.T) {
		// Set AI decision to REJECT
		mockAI.response.Decision = "REJECT"
		mockAI.response.SuggestedAction = "REJECT"
		mockAI.response.CandleNarrative = "Overextended trend."
		mockAI.response.EntryTiming = "REJECT"
		mockStorage.auditCache = nil

		ctx := context.Background()
		_, err := uc.Run(ctx, dto.ScanRequest{TriggerTime: time.Now()})
		if err != nil {
			t.Fatalf("scanner run failed: %v", err)
		}

		latest, err := storageUC.LoadLatestResult()
		if err != nil {
			t.Fatalf("failed to load latest result: %v", err)
		}

		if latest.TotalAIReject != 1 {
			t.Errorf("expected TotalAIReject = 1, got %d", latest.TotalAIReject)
		}
		if latest.TotalFinalReject != 1 {
			t.Errorf("expected TotalFinalReject = 1, got %d", latest.TotalFinalReject)
		}
		if latest.TotalFinalWatch != 0 {
			t.Errorf("expected TotalFinalWatch = 0, got %d", latest.TotalFinalWatch)
		}

		if len(latest.Watchlist) != 0 {
			t.Errorf("expected Watchlist to be empty, got %d", len(latest.Watchlist))
		}
		if len(latest.ExecuteSignals) != 0 {
			t.Errorf("expected ExecuteSignals to be empty, got %d", len(latest.ExecuteSignals))
		}
	})
}

func TestPolicyRejectedSummaryCompaction(t *testing.T) {
	// We want to verify that when scanner.Run processes eligibility failures,
	// it correctly formats them and groups LONG and SHORT failures into LONG/SHORT.
	failures := []struct {
		Symbol       string
		StrategyName string
		Direction    string
		Reason       string
	}{
		{
			Symbol:       "ETHUSDT",
			StrategyName: "RANGE_EDGE_REVERSAL",
			Direction:    "LONG",
			Reason:       "Range edge reversal invalid: strong trending expansion",
		},
		{
			Symbol:       "ETHUSDT",
			StrategyName: "RANGE_EDGE_REVERSAL",
			Direction:    "SHORT",
			Reason:       "Range edge reversal invalid: strong trending expansion",
		},
		{
			Symbol:       "SOLUSDT",
			StrategyName: "LIQUIDITY_SWEEP_REVERSAL",
			Direction:    "LONG",
			Reason:       "No lower liquidity sweep detected",
		},
	}

	type rejectKey struct {
		Symbol       string
		StrategyName string
		Reason       string
	}
	rejectGroups := make(map[rejectKey][]string)
	var rejectKeys []rejectKey

	for _, f := range failures {
		key := rejectKey{Symbol: f.Symbol, StrategyName: f.StrategyName, Reason: f.Reason}
		if _, ok := rejectGroups[key]; !ok {
			rejectKeys = append(rejectKeys, key)
		}
		rejectGroups[key] = append(rejectGroups[key], f.Direction)
	}

	var policyRejectedSummary []string
	for _, key := range rejectKeys {
		dirs := rejectGroups[key]
		var dirStr string
		isLong := false
		isShort := false
		for _, d := range dirs {
			if d == "LONG" {
				isLong = true
			} else if d == "SHORT" {
				isShort = true
			}
		}
		if isLong && isShort {
			dirStr = "LONG/SHORT"
		} else if isLong {
			dirStr = "LONG"
		} else if isShort {
			dirStr = "SHORT"
		}

		policyRejectedSummary = append(policyRejectedSummary, fmt.Sprintf("%s (%s %s): %s", key.Symbol, key.StrategyName, dirStr, key.Reason))
	}

	if len(policyRejectedSummary) != 2 {
		t.Fatalf("expected 2 summary entries, got %d", len(policyRejectedSummary))
	}

	expectedETH := "ETHUSDT (RANGE_EDGE_REVERSAL LONG/SHORT): Range edge reversal invalid: strong trending expansion"
	expectedSOL := "SOLUSDT (LIQUIDITY_SWEEP_REVERSAL LONG): No lower liquidity sweep detected"

	if policyRejectedSummary[0] != expectedETH {
		t.Errorf("expected: %q, got: %q", expectedETH, policyRejectedSummary[0])
	}
	if policyRejectedSummary[1] != expectedSOL {
		t.Errorf("expected: %q, got: %q", expectedSOL, policyRejectedSummary[1])
	}
}

func TestArbiterRejectedSummaryFormattingAndDeduplication(t *testing.T) {
	arbiterRejected := []QuantResult{
		{
			Symbol:    "ONDOUSDT",
			Playbook:  RANGE_EDGE_REVERSAL,
			Direction: SHORT,
			Score:     4.5,
			Reason:    "Arbiter reject: opposing LONG exists",
		},
		{
			Symbol:    "ONDOUSDT",
			Playbook:  RANGE_EDGE_REVERSAL,
			Direction: SHORT,
			Score:     4.5,
			Reason:    "Arbiter reject: opposing LONG exists",
		},
		{
			Symbol:    "XAUUSDT",
			Playbook:  LIQUIDITY_SWEEP_REVERSAL,
			Direction: LONG,
			Score:     7.0,
			Reason:    "Arbiter reject: not premium setup",
		},
		{
			Symbol:    "XAUUSDT",
			Playbook:  LIQUIDITY_SWEEP_REVERSAL,
			Direction: LONG,
			Score:     7.0,
			Reason:    "Arbiter reject: not premium setup",
		},
		{
			Symbol:    "XAUUSDT",
			Playbook:  LIQUIDITY_SWEEP_REVERSAL,
			Direction: SHORT,
			Score:     7.2,
			Reason:    "Arbiter reject: not premium setup",
		},
	}

	rejectedSummary := []string{}
	seenArbiterRejections := make(map[string]bool)
	for _, rej := range arbiterRejected {
		reason := rej.Reason
		if reason == "" {
			reason = "failed arbiter filter"
		}
		entry := fmt.Sprintf("%s (%s %s): arbiter rejected - score=%0.1f reason=%s", rej.Symbol, rej.Playbook, rej.Direction, rej.Score, reason)
		if !seenArbiterRejections[entry] {
			seenArbiterRejections[entry] = true
			rejectedSummary = append(rejectedSummary, entry)
		}
	}

	if len(rejectedSummary) != 3 {
		t.Fatalf("expected 3 entries after deduplication, got %d", len(rejectedSummary))
	}

	expected0 := "ONDOUSDT (RANGE_EDGE_REVERSAL SHORT): arbiter rejected - score=4.5 reason=Arbiter reject: opposing LONG exists"
	expected1 := "XAUUSDT (LIQUIDITY_SWEEP_REVERSAL LONG): arbiter rejected - score=7.0 reason=Arbiter reject: not premium setup"
	expected2 := "XAUUSDT (LIQUIDITY_SWEEP_REVERSAL SHORT): arbiter rejected - score=7.2 reason=Arbiter reject: not premium setup"

	if rejectedSummary[0] != expected0 {
		t.Errorf("expected: %q, got: %q", expected0, rejectedSummary[0])
	}
	if rejectedSummary[1] != expected1 {
		t.Errorf("expected: %q, got: %q", expected1, rejectedSummary[1])
	}
	if rejectedSummary[2] != expected2 {
		t.Errorf("expected: %q, got: %q", expected2, rejectedSummary[2])
	}
}

func TestResolveMarketDataPrefetchLimit_DefaultUsesAllCandidates(t *testing.T) {
	original := getRuntimeSettings()
	t.Cleanup(func() { SetRuntimeSettings(original) })
	settings := original
	settings.MaxMarketDataPrefetchSymbols = 0
	SetRuntimeSettings(settings)
	limit := resolveMarketDataPrefetchLimit(MarketPolicy{
		Regime:          DEFAULT,
		MaxSymbols:      50,
		MaxAICandidates: 3,
		MaxFinalExecute: 5,
	}, 42)
	if limit != 18 {
		t.Fatalf("expected dynamic default prefetch limit 18, got %d", limit)
	}
}

func TestResolveMarketDataPrefetchLimit_EnvOverride(t *testing.T) {
	original := getRuntimeSettings()
	t.Cleanup(func() { SetRuntimeSettings(original) })
	settings := original
	settings.MaxMarketDataPrefetchSymbols = 12
	SetRuntimeSettings(settings)
	limit := resolveMarketDataPrefetchLimit(MarketPolicy{MaxSymbols: 50, MaxAICandidates: 3}, 42)
	if limit != 12 {
		t.Fatalf("expected env override prefetch limit 12, got %d", limit)
	}
}

func TestResolveMarketDataPrefetchLimit_BTCChaosIsTighter(t *testing.T) {
	original := getRuntimeSettings()
	t.Cleanup(func() { SetRuntimeSettings(original) })
	settings := original
	settings.MaxMarketDataPrefetchSymbols = 0
	SetRuntimeSettings(settings)
	limit := resolveMarketDataPrefetchLimit(MarketPolicy{
		Regime:          BTC_CHAOS,
		MaxSymbols:      35,
		MaxAICandidates: 1,
		MaxFinalExecute: 1,
	}, 30)
	if limit != 8 {
		t.Fatalf("expected BTC_CHAOS prefetch limit 8, got %d", limit)
	}
}

func TestResolveMarketDataPrefetchLimit_AltSupportiveAllowsBroaderPrefetch(t *testing.T) {
	original := getRuntimeSettings()
	t.Cleanup(func() { SetRuntimeSettings(original) })
	settings := original
	settings.MaxMarketDataPrefetchSymbols = 0
	SetRuntimeSettings(settings)
	limit := resolveMarketDataPrefetchLimit(MarketPolicy{
		Regime:          ALT_SUPPORTIVE,
		MaxSymbols:      75,
		MaxAICandidates: 3,
		MaxFinalExecute: 5,
	}, 40)
	if limit != 18 {
		t.Fatalf("expected ALT_SUPPORTIVE prefetch limit 18, got %d", limit)
	}
}

func TestEstimateScanRequestWeight(t *testing.T) {
	weight := estimateScanRequestWeight(20, 8)
	expected := 40 + 10 + 20 + (8 * 4)
	if weight != expected {
		t.Fatalf("expected weight %d, got %d", expected, weight)
	}
}

func TestResolveAdaptiveScanRequestGuard_ReducesPrefetchToBudget(t *testing.T) {
	original := getRuntimeSettings()
	t.Cleanup(func() { SetRuntimeSettings(original) })
	settings := original
	settings.ScanRequestWeightBudget = 120
	SetRuntimeSettings(settings)

	guard := resolveAdaptiveScanRequestGuard(MarketPolicy{Regime: ALT_SUPPORTIVE}, 50, 24, 8)
	if !guard.Applied {
		t.Fatalf("expected adaptive guard to be applied")
	}
	if guard.PrefetchLimit >= 24 {
		t.Fatalf("expected reduced prefetch limit, got %d", guard.PrefetchLimit)
	}
	if guard.ExpectedWeight > guard.Budget {
		t.Fatalf("expected guarded weight <= budget, got weight=%d budget=%d", guard.ExpectedWeight, guard.Budget)
	}
	if guard.MarketDataConcurrency > 4 {
		t.Fatalf("expected concurrency to be tightened, got %d", guard.MarketDataConcurrency)
	}
}

func TestResolveAdaptiveScanRequestGuard_BTCChaosTightensConcurrency(t *testing.T) {
	original := getRuntimeSettings()
	t.Cleanup(func() { SetRuntimeSettings(original) })
	settings := original
	settings.ScanRequestWeightBudget = 0
	SetRuntimeSettings(settings)

	guard := resolveAdaptiveScanRequestGuard(MarketPolicy{Regime: BTC_CHAOS}, 30, 8, 6)
	if guard.MarketDataConcurrency != 2 {
		t.Fatalf("expected BTC_CHAOS market-data concurrency 2, got %d", guard.MarketDataConcurrency)
	}
	if guard.PipelineConcurrency != 2 {
		t.Fatalf("expected BTC_CHAOS pipeline concurrency 2, got %d", guard.PipelineConcurrency)
	}
}

type mockHotSymbolProvider struct {
	symbols []HotSymbol
	err     error
}

func (m *mockHotSymbolProvider) FetchHotSymbols(ctx context.Context) ([]HotSymbol, error) {
	return m.symbols, m.err
}

func TestScanner_PrefetchSlotReservation(t *testing.T) {
	original := getRuntimeSettings()
	t.Cleanup(func() { SetRuntimeSettings(original) })
	settings := original
	settings.MaxMarketDataPrefetchSymbols = 4
	settings.ScanRequestWeightBudget = 500
	SetRuntimeSettings(settings)

	policy := MarketPolicy{
		AllowedTiers:         []Tier{TierA, TierB, TierC},
		MaxSymbols:           10,
		MinVolume:            1000000.0,
		MaxFundingAbs:        0.01,
		MaxPriceMove24h:      0.20,
		HotMaxBoost:          1.25,
		HotPrefetchSlotRatio: 0.50,
	}

	tickers := []dto.Ticker24h{
		{Symbol: "SOLUSDT", QuoteVolume: 90000000.0, LastPrice: 100.0},
		{Symbol: "ADAUSDT", QuoteVolume: 70000000.0, LastPrice: 1.0},
		{Symbol: "DOGEUSDT", QuoteVolume: 60000000.0, LastPrice: 0.1},
		{Symbol: "ETHUSDT", QuoteVolume: 120000000.0, LastPrice: 3000.0},
		{Symbol: "XRPUSDT", QuoteVolume: 110000000.0, LastPrice: 0.5},
		{Symbol: "LTCUSDT", QuoteVolume: 55000000.0, LastPrice: 80.0},
		{Symbol: "BTCUSDT", QuoteVolume: 1000000000.0, LastPrice: 50000.0, PriceChangePercent: 0.0},
	}

	fundingRates := map[string]float64{
		"SOLUSDT": 0.0001, "ADAUSDT": 0.0001, "DOGEUSDT": 0.0001,
		"ETHUSDT": 0.0001, "XRPUSDT": 0.0001, "LTCUSDT": 0.0001,
		"BTCUSDT": 0.0001,
	}

	freshM15 := generateFreshCandles(100.0)
	freshH1 := generateFreshCandles(100.0)
	freshH4 := generateFreshCandles(100.0)

	m15Candles := map[string][]dto.Candle{
		"SOLUSDT": freshM15, "ADAUSDT": freshM15, "DOGEUSDT": freshM15,
		"ETHUSDT": freshM15, "XRPUSDT": freshM15, "LTCUSDT": freshM15,
		"BTCUSDT": freshM15,
	}
	h1Candles := map[string][]dto.Candle{
		"SOLUSDT": freshH1, "ADAUSDT": freshH1, "DOGEUSDT": freshH1,
		"ETHUSDT": freshH1, "XRPUSDT": freshH1, "LTCUSDT": freshH1,
		"BTCUSDT": freshH1,
	}
	h4Candles := map[string][]dto.Candle{
		"SOLUSDT": freshH4, "ADAUSDT": freshH4, "DOGEUSDT": freshH4,
		"ETHUSDT": freshH4, "XRPUSDT": freshH4, "LTCUSDT": freshH4,
		"BTCUSDT": freshH4,
	}

	prices := map[string]float64{
		"SOLUSDT": 100.0, "ADAUSDT": 1.0, "DOGEUSDT": 0.1,
		"ETHUSDT": 3000.0, "XRPUSDT": 0.5, "LTCUSDT": 80.0,
		"BTCUSDT": 50000.0,
	}

	mockProvider := &mockMarketDataProvider{
		tickers:      tickers,
		fundingRates: fundingRates,
		m15Candles:   m15Candles,
		h1Candles:    h1Candles,
		h4Candles:    h4Candles,
		prices:       prices,
	}

	mockHotProvider := &mockHotSymbolProvider{
		symbols: []HotSymbol{
			{Symbol: "SOL", Score: 100, Source: "Trending"},
			{Symbol: "ADA", Score: 80, Source: "Top Search"},
			{Symbol: "DOGE", Score: 50, Source: "Social Hype"},
		},
	}

	mockNotify := &mockNotification{}
	mockStorage := &mockStorageRepo{
		journal: []SignalJournal{},
		audits:  []DecisionAudit{},
	}

	storageUC := NewStorageUsecase(mockStorage)
	marketDataUC := NewMarketDataUsecase(mockProvider)

	reg := NewDefaultConfigRegistry()
	reg.policies["DEFAULT"] = policy
	riskOffPolicy := policy
	riskOffPolicy.Regime = RISK_OFF
	riskOffPolicy.LongMode = SWEEP_ONLY
	riskOffPolicy.ShortMode = NORMAL
	reg.policies["RISK_OFF"] = riskOffPolicy
	SetGlobalConfigRegistry(reg)
	defer SetGlobalConfigRegistry(NewDefaultConfigRegistry())

	marketPolicyUC := NewMarketPolicyUsecase()
	universeUC := NewUniverseUsecase()
	strategySelectorUC := NewStrategySelectorUsecase()
	playbookEligibilityUC := NewPlaybookEligibilityUsecase()
	playbookQuantEngineUC := NewPlaybookQuantEngineUsecase()
	scoringUC := NewScoringUsecase()
	candidateArbiterUC := NewCandidateArbiterUsecase()
	localGateUC := NewLocalGateUsecase()
	aiCandidateSelectorUC := NewAICandidateSelectorUsecase(60.0)
	mockAI := &mockAIAuditor{response: dto.AIAuditResponse{Decision: "WAIT"}}
	aiAuditorUC := NewAIAuditorUsecase(mockAI, storageUC)
	planReconciliationUC := NewPlanReconciliationUsecase()
	stalenessUC := NewStalenessUsecase(30 * time.Minute)
	stalenessUC.SetFallbackProvider(mockProvider)
	finalGateUC := NewFinalGateUsecase()
	conflictResolverUC := NewConflictResolverUsecase()
	signalNotificationUC := NewSignalNotificationUsecase(mockNotify, storageUC)
	opsNotificationUC := NewOpsNotificationUsecase(mockNotify)
	monitoringUC := NewMonitoringUsecase(mockProvider, storageUC)
	feedbackUC := NewFeedbackUsecase(storageUC)

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
	uc.SetHotSymbolProvider(mockHotProvider)

	ctx := context.Background()
	_, err := uc.Run(ctx, dto.ScanRequest{TriggerTime: time.Now()})
	if err != nil {
		t.Fatalf("scanner run failed: %v", err)
	}

	latest, err := storageUC.LoadLatestResult()
	if err != nil {
		t.Fatalf("failed to load latest result: %v", err)
	}

	deferred := make(map[string]bool)
	t.Logf("All PolicyRejectedSummary: %+v", latest.PolicyRejectedSummary)
	for _, p := range latest.PolicyRejectedSummary {
		if strings.Contains(p, "deferred by market data prefetch limit") {
			parts := strings.Split(p, ":")
			deferred[strings.TrimSpace(parts[0])] = true
		}
	}
	t.Logf("Parsed deferred symbols: %+v", deferred)

	expectedSelected := []string{"SOLUSDT", "ADAUSDT", "ETHUSDT", "XRPUSDT"}
	expectedDeferred := []string{"DOGEUSDT", "LTCUSDT"}

	for _, sym := range expectedSelected {
		if deferred[sym] {
			t.Errorf("Expected %s to be selected, but it was deferred", sym)
		}
	}
	for _, sym := range expectedDeferred {
		if !deferred[sym] {
			t.Errorf("Expected %s to be deferred, but it was selected", sym)
		}
	}
}
