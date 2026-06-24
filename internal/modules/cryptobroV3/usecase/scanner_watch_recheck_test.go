package usecase

import (
	"context"
	"strings"
	"testing"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
)

func TestSelectWatchRecheckCandidates(t *testing.T) {
	original := getRuntimeSettings()
	t.Cleanup(func() { SetRuntimeSettings(original) })
	settings := original
	settings.WatchRecheckMaxAgeMinutes = 12
	settings.WatchRecheckBatchLimit = 4
	SetRuntimeSettings(settings)

	now := time.Now()
	journal := []WatchJournal{
		{
			ID:          "watch_sol_latest",
			Symbol:      "SOLUSDT",
			Direction:   LONG,
			Playbook:    LIQUIDITY_SWEEP_REVERSAL,
			Status:      WATCH_MONITORING,
			CreatedAt:   now.Add(-5 * time.Minute),
			Reason:      "AI decision is WAIT",
			AIReasoning: "AI decision is WAIT for better confirmation",
			QuantScore:  8.1,
		},
		{
			ID:          "watch_sol_old",
			Symbol:      "SOLUSDT",
			Direction:   LONG,
			Playbook:    TREND_PULLBACK,
			Status:      WATCH_MONITORING,
			CreatedAt:   now.Add(-6 * time.Minute),
			Reason:      "AI decision is WAIT",
			AIReasoning: "AI decision is WAIT",
			QuantScore:  7.8,
		},
		{
			ID:          "watch_btc_too_old",
			Symbol:      "BTCUSDT",
			Direction:   LONG,
			Playbook:    TREND_PULLBACK,
			Status:      WATCH_MONITORING,
			CreatedAt:   now.Add(-20 * time.Minute),
			Reason:      "AI decision is WAIT",
			AIReasoning: "AI decision is WAIT",
		},
		{
			ID:          "watch_eth_bad_reason",
			Symbol:      "ETHUSDT",
			Direction:   LONG,
			Playbook:    TREND_PULLBACK,
			Status:      WATCH_MONITORING,
			CreatedAt:   now.Add(-4 * time.Minute),
			Reason:      "Risk-to-Reward ratio 1.60 is below policy requirement 2.00 but above hard minimum 1.50",
			AIReasoning: "Local gate failed: Risk-to-Reward ratio 1.60 is below policy requirement 2.00 but above hard minimum 1.50",
		},
		{
			ID:          "watch_xrp_wrong_playbook",
			Symbol:      "XRPUSDT",
			Direction:   LONG,
			Playbook:    COMPRESSION_BREAKOUT_RETEST,
			Status:      WATCH_MONITORING,
			CreatedAt:   now.Add(-4 * time.Minute),
			Reason:      "AI decision is WAIT",
			AIReasoning: "AI decision is WAIT",
		},
		{
			ID:          "watch_ada_local_m5",
			Symbol:      "ADAUSDT",
			Direction:   LONG,
			Playbook:    TREND_PULLBACK,
			Status:      WATCH_MONITORING,
			CreatedAt:   now.Add(-3 * time.Minute),
			Reason:      "Local gate status is LOCAL_WATCH",
			AIReasoning: "Local gate failed: M5 continuation confirmation unavailable: failed to fetch M5 candles",
		},
	}

	shortlist := selectWatchRecheckCandidates(journal, now)
	if len(shortlist) != 2 {
		t.Fatalf("expected 2 recheck candidates, got %d", len(shortlist))
	}
	if shortlist[0].entry.Symbol != "SOLUSDT" {
		t.Fatalf("expected SOLUSDT to win duplicate symbol prioritization, got %s", shortlist[0].entry.Symbol)
	}
	if shortlist[1].entry.Symbol != "ADAUSDT" {
		t.Fatalf("expected ADAUSDT second eligible candidate, got %s", shortlist[1].entry.Symbol)
	}
}

func TestScannerUsecase_RunWatchRecheckPromotesEligibleWatch(t *testing.T) {
	tickers := []dto.Ticker24h{
		{Symbol: "BTCUSDT", LastPrice: 50000.0, PriceChangePercent: 2.0, QuoteVolume: 1000000000.0},
		{Symbol: "ETHUSDT", LastPrice: 3000.0, PriceChangePercent: 1.5, QuoteVolume: 500000000.0},
		{Symbol: "SOLUSDT", LastPrice: 100.0, PriceChangePercent: 1.0, QuoteVolume: 200000000.0},
	}
	fundingRates := map[string]float64{
		"BTCUSDT": 0.0001,
		"SOLUSDT": 0.0001,
	}

	freshM15 := generateSweepCandles(100.0)
	freshH1 := generateFreshCandles(100.0)
	freshH4 := generateFreshCandles(100.0)
	freshM5 := []dto.Candle{
		{Time: time.Now().Add(-15 * time.Minute), Open: 100.0, High: 100.1, Low: 99.9, Close: 100.0, Vol: 800.0},
		{Time: time.Now().Add(-10 * time.Minute), Open: 100.0, High: 100.1, Low: 99.8, Close: 100.0, Vol: 850.0},
		{Time: time.Now().Add(-5 * time.Minute), Open: 100.0, High: 100.05, Low: 99.3, Close: 100.0, Vol: 1200.0},
	}
	btcM15 := generateFreshCandles(50000.0)
	btcH1 := generateFreshCandles(50000.0)
	btcH4 := generateFreshCandles(50000.0)

	mockProvider := &mockMarketDataProvider{
		tickers:      tickers,
		fundingRates: fundingRates,
		m5Candles:    map[string][]dto.Candle{"SOLUSDT": freshM5},
		m15Candles:   map[string][]dto.Candle{"SOLUSDT": freshM15, "BTCUSDT": btcM15},
		h1Candles:    map[string][]dto.Candle{"SOLUSDT": freshH1, "BTCUSDT": btcH1},
		h4Candles:    map[string][]dto.Candle{"SOLUSDT": freshH4, "BTCUSDT": btcH4},
		prices:       map[string]float64{"SOLUSDT": 100.0, "BTCUSDT": 50000.0},
	}

	mockAI := &mockAIAuditor{
		response: dto.AIAuditResponse{
			Symbol:          "SOLUSDT",
			Decision:        "CONFIRM",
			Confidence:      "HIGH",
			IsApproved:      true,
			Sentiment:       "BULLISH",
			HasRejection:    true,
			HasConfirmation: true,
			CandleNarrative: "REJECTION",
			EntryTiming:     "FRESH",
			SuggestedAction: "EXECUTE_IF_NOT_STALE",
			Reasoning:       "Fresh lower sweep with rejection confirmed.",
			Reason:          "AI_CONFIRM",
			Source:          AIAuditSourceReal,
		},
	}

	createdAt := time.Now().Add(-4 * time.Minute)
	mockStorage := &mockStorageRepo{
		journal: []SignalJournal{},
		watchJournal: []WatchJournal{
			{
				ID:          "watch_20260624120000_SOLUSDT",
				Symbol:      "SOLUSDT",
				Direction:   LONG,
				Playbook:    LIQUIDITY_SWEEP_REVERSAL,
				Status:      WATCH_MONITORING,
				CreatedAt:   createdAt,
				UpdatedAt:   createdAt,
				ExpiresAt:   createdAt.Add(90 * time.Minute),
				Reason:      "AI decision is WAIT",
				AIReasoning: "AI decision is WAIT pending extra confirmation",
				Tier:        TierA,
				Timeframe:   "M15",
				IsHot:       true,
				HotScore:    1.2,
				HotSource:   "TEST",
			},
		},
		audits: []DecisionAudit{},
	}

	mockNotify := &mockNotification{}

	storageUC := NewStorageUsecase(mockStorage)
	marketDataUC := NewMarketDataUsecase(mockProvider)
	marketPolicyUC := NewMarketPolicyUsecase()
	universeUC := NewUniverseUsecase()
	strategySelectorUC := NewStrategySelectorUsecase()
	playbookEligibilityUC := NewPlaybookEligibilityUsecase()
	playbookQuantEngineUC := NewPlaybookQuantEngineUsecase()
	scoringUC := NewScoringUsecase()
	candidateArbiterUC := NewCandidateArbiterUsecase()
	localGateUC := NewLocalGateUsecase()
	localGateUC.SetMarketData(marketDataUC)
	aiCandidateSelectorUC := NewAICandidateSelectorUsecase(60.0)
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

	original := getRuntimeSettings()
	t.Cleanup(func() { SetRuntimeSettings(original) })
	settings := original
	settings.RequireAIHighForExecute = true
	settings.RequireFreshEntryForExecute = true
	settings.WatchRecheckMaxAgeMinutes = 12
	settings.WatchRecheckBatchLimit = 4
	SetRuntimeSettings(settings)

	summary, err := uc.RunWatchRecheck(context.Background(), dto.ScanRequest{TriggerTime: time.Now()})
	if err != nil {
		t.Fatalf("RunWatchRecheck failed: %v", err)
	}

	if summary.Promoted != 1 || summary.FinalExecute != 1 {
		t.Fatalf("expected one promotion/final execute, got summary=%+v", summary)
	}

	journal, err := storageUC.LoadSignalJournal()
	if err != nil {
		t.Fatalf("failed to load signal journal: %v", err)
	}
	if len(journal) != 1 {
		t.Fatalf("expected 1 promoted signal journal entry, got %d", len(journal))
	}
	if journal[0].Status != MONITORING {
		t.Fatalf("expected promoted journal status MONITORING, got %s", journal[0].Status)
	}
	if !strings.Contains(journal[0].Reason, "WATCH_RECHECK_PROMOTION") {
		t.Fatalf("expected promotion reason in signal journal, got %q", journal[0].Reason)
	}

	watchJournal, err := storageUC.LoadWatchJournal()
	if err != nil {
		t.Fatalf("failed to load watch journal: %v", err)
	}
	if len(watchJournal) != 1 {
		t.Fatalf("expected 1 watch journal entry, got %d", len(watchJournal))
	}
	if watchJournal[0].Status != WATCH_PROMOTED {
		t.Fatalf("expected watch status WATCH_PROMOTED, got %s", watchJournal[0].Status)
	}
	if watchJournal[0].ClosedAt.IsZero() {
		t.Fatal("expected promoted watch to be closed")
	}

	audits, err := storageUC.LoadDecisionAudits()
	if err != nil {
		t.Fatalf("failed to load decision audits: %v", err)
	}
	if len(audits) != 1 {
		t.Fatalf("expected 1 decision audit, got %d", len(audits))
	}
	if audits[0].FinalStatus != FINAL_EXECUTE {
		t.Fatalf("expected recheck decision audit FINAL_EXECUTE, got %s", audits[0].FinalStatus)
	}
	if !strings.HasPrefix(audits[0].ScanID, "recheck_") {
		t.Fatalf("expected recheck scan id prefix, got %s", audits[0].ScanID)
	}

	if len(mockNotify.signalMsgs) != 1 {
		t.Fatalf("expected 1 notification message, got %d", len(mockNotify.signalMsgs))
	}
}
