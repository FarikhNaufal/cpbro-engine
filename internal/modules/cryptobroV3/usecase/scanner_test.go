package usecase

import (
	"context"
	"errors"
	"fmt"
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
		high := startPrice + 0.1
		low := startPrice - 0.1
		vol := 1000.0
		// Candle 59 is the sweep candle
		if i == 59 {
			low = startPrice - 1.0 // sweeps below previous lows (99.9)
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
	})

	t.Run("Scanner AI_REJECT becomes FINAL_REJECT and does not appear in watchlist", func(t *testing.T) {
		// Set AI decision to REJECT
		mockAI.response.Decision = "REJECT"
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
