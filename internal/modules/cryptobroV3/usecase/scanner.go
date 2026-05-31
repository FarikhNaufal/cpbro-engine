package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
	"cpbro-engine/internal/modules/cryptobroV3/entity"
)

type ScannerUsecase struct {
	marketDataUsecase          *MarketDataUsecase
	marketPolicyUsecase        *MarketPolicyUsecase
	universeUsecase            *UniverseUsecase
	strategySelectorUsecase    *StrategySelectorUsecase
	playbookEligibilityUsecase *PlaybookEligibilityUsecase
	playbookQuantEngineUsecase *PlaybookQuantEngineUsecase
	scoringUsecase             *ScoringUsecase
	candidateArbiterUsecase    *CandidateArbiterUsecase
	localGateUsecase           *LocalGateUsecase
	aiCandidateSelectorUsecase *AICandidateSelectorUsecase
	aiAuditorUsecase           *AIAuditorUsecase
	planReconciliationUsecase  *PlanReconciliationUsecase
	stalenessUsecase           *StalenessUsecase
	finalGateUsecase           *FinalGateUsecase
	conflictResolverUsecase    *ConflictResolverUsecase
	signalNotificationUsecase  *SignalNotificationUsecase
	opsNotificationUsecase     *OpsNotificationUsecase
	monitoringUsecase          *MonitoringUsecase
	feedbackUsecase            *FeedbackUsecase
	storageUsecase             *StorageUsecase
}

func NewScannerUsecase(
	marketData *MarketDataUsecase,
	marketPolicy *MarketPolicyUsecase,
	universe *UniverseUsecase,
	strategySelector *StrategySelectorUsecase,
	playbookEligibility *PlaybookEligibilityUsecase,
	playbookQuantEngine *PlaybookQuantEngineUsecase,
	scoring *ScoringUsecase,
	candidateArbiter *CandidateArbiterUsecase,
	localGate *LocalGateUsecase,
	aiCandidateSelector *AICandidateSelectorUsecase,
	aiAuditor *AIAuditorUsecase,
	planReconciliation *PlanReconciliationUsecase,
	staleness *StalenessUsecase,
	finalGate *FinalGateUsecase,
	conflictResolver *ConflictResolverUsecase,
	signalNotification *SignalNotificationUsecase,
	opsNotification *OpsNotificationUsecase,
	monitoring *MonitoringUsecase,
	feedback *FeedbackUsecase,
	storage *StorageUsecase,
) *ScannerUsecase {
	return &ScannerUsecase{
		marketDataUsecase:          marketData,
		marketPolicyUsecase:        marketPolicy,
		universeUsecase:            universe,
		strategySelectorUsecase:    strategySelector,
		playbookEligibilityUsecase: playbookEligibility,
		playbookQuantEngineUsecase: playbookQuantEngine,
		scoringUsecase:             scoring,
		candidateArbiterUsecase:    candidateArbiter,
		localGateUsecase:           localGate,
		aiCandidateSelectorUsecase: aiCandidateSelector,
		aiAuditorUsecase:           aiAuditor,
		planReconciliationUsecase:  planReconciliation,
		stalenessUsecase:           staleness,
		finalGateUsecase:           finalGate,
		conflictResolverUsecase:    conflictResolver,
		signalNotificationUsecase:  signalNotification,
		opsNotificationUsecase:     opsNotification,
		monitoringUsecase:          monitoring,
		feedbackUsecase:            feedback,
		storageUsecase:             storage,
	}
}

func (uc *ScannerUsecase) Run(ctx context.Context, req dto.ScanRequest) (dto.ScanResult, error) {
	scanStart := time.Now()
	scanBoundary := req.TriggerTime
	if scanBoundary.IsZero() {
		scanBoundary = scanStart
	}
	scanID := scanBoundary.Format("20060102150405")

	slog.Info("Starting AnalyzeMarketV3 Scan", "scan_id", scanID)

	finalSignals := []dto.SignalResponse{}

	// Load active signals (signal journal) and history signals for final gate evaluation
	activeSignals, _ := uc.storageUsecase.LoadSignalJournal()
	var historySignals []dto.SignalResponse
	if hist, err := uc.storageUsecase.LoadSignalHistory(); err == nil && hist != nil {
		historySignals = hist.Signals
	}

	// Run monitoring on existing virtual positions first
	if err := uc.monitoringUsecase.MonitorVirtualPositions(ctx); err != nil {
		slog.Warn("Monitoring virtual positions failed", "error", err)
	}

	// Fetch tickers & funding rates to feed the macro Policy Engine
	tickers, err := uc.marketDataUsecase.FetchAllFuturesTickers24h(ctx)
	if err != nil {
		slog.Error("Failed to fetch futures tickers", "error", err)
		GetGlobalMetrics().IncrementScanFail()
		GetGlobalMetrics().SetLastScanTime(scanStart)
		GetGlobalMetrics().IncrementMarketDataError()
		return dto.ScanResult{}, fmt.Errorf("binance ticker total fail: %w", err)
	}
	fundingRates, err := uc.marketDataUsecase.FetchPremiumFundingRates(ctx)
	if err != nil {
		slog.Warn("Failed to fetch funding rates; using fallback values", "error", err)
		GetGlobalMetrics().IncrementMarketDataError()
		fundingRates = make(map[string]float64)
	}

	// Map tickers for quick access
	tickerMap := make(map[string]dto.Ticker24h)
	advancing := 0
	totalTickers := 0
	var btcTicker *dto.Ticker24h
	var ethTicker *dto.Ticker24h

	for i := range tickers {
		t := tickers[i]
		tickerMap[t.Symbol] = t
		if t.PriceChangePercent > 0 {
			advancing++
		}
		totalTickers++
		if t.Symbol == "BTCUSDT" {
			btcTicker = &tickers[i]
		}
		if t.Symbol == "ETHUSDT" {
			ethTicker = &tickers[i]
		}
	}

	breadth := 0.5
	if totalTickers > 0 {
		breadth = float64(advancing) / float64(totalTickers)
	}

	ethBtcPerf := 0.0
	if btcTicker != nil && ethTicker != nil {
		ethBtcPerf = (ethTicker.PriceChangePercent - btcTicker.PriceChangePercent) / 100.0
	}

	btcTrend := "SIDEWAYS"
	btcScore := 50.0
	btcChaos := 0.2
	volatility := "NORMAL"

	if btcTicker != nil {
		if btcTicker.PriceChangePercent > 1.5 {
			btcTrend = "BULLISH"
		} else if btcTicker.PriceChangePercent < -1.5 {
			btcTrend = "BEARISH"
		}
		btcScore = 50.0 + (btcTicker.PriceChangePercent * 5.0)
		if btcScore > 100.0 {
			btcScore = 100.0
		} else if btcScore < 0.0 {
			btcScore = 0.0
		}

		absChange := math.Abs(btcTicker.PriceChangePercent)
		if absChange > 5.0 {
			volatility = "HIGH"
			btcChaos = 0.85
		} else if absChange < 0.5 {
			volatility = "LOW"
			btcChaos = 0.1
		}
	}

	// Evaluate global Policy
	policy := uc.marketPolicyUsecase.EvaluatePolicy(ctx, btcTrend, btcScore, ethBtcPerf, btcChaos, volatility, breadth)

	// Filter dynamic universe candidates
	candidates, rejectedCandidatesList := uc.universeUsecase.FilterUniverse(tickers, fundingRates, policy)

	rejectedSummary := []string{}
	for _, rej := range rejectedCandidatesList {
		rejectedSummary = append(rejectedSummary, fmt.Sprintf("%s: %s", rej.Symbol, rej.Reason))
	}

	// Concurrency limited candle fetching
	concurrencyLimit := 5
	if val := os.Getenv("MAX_MARKETDATA_CONCURRENCY"); val != "" {
		if limit, err := strconv.Atoi(val); err == nil && limit > 0 {
			concurrencyLimit = limit
		}
	}

	type candlesCache struct {
		m15 []dto.Candle
		h1  []dto.Candle
		h4  []dto.Candle
		err error
	}
	candlesMap := make(map[string]candlesCache)
	var mapMu sync.Mutex

	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrencyLimit)

	for _, cand := range candidates {
		wg.Add(1)
		go func(pair string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			m15, h1, h4, fetchErr := uc.marketDataUsecase.FetchCandles(ctx, pair)
			if fetchErr != nil {
				GetGlobalMetrics().IncrementMarketDataError()
			}
			mapMu.Lock()
			candlesMap[pair] = candlesCache{
				m15: m15,
				h1:  h1,
				h4:  h4,
				err: fetchErr,
			}
			mapMu.Unlock()
		}(cand.Symbol)
	}
	wg.Wait()

	var allCandidates []QuantResult
	policyRejectedSummary := []string{}
	totalStrategySelected := 0
	totalPlaybookEligible := 0

	type eligibilityFailure struct {
		Symbol       string
		StrategyName string
		Direction    string
		Reason       string
	}
	var eligibilityFailures []eligibilityFailure

	tickerLastPrice := make(map[string]float64, len(tickers))
	for _, t := range tickers {
		tickerLastPrice[t.Symbol] = t.LastPrice
	}

	for _, candidate := range candidates {
		pair := candidate.Symbol
		cache, exists := candlesMap[pair]
		if !exists || cache.err != nil {
			policyRejectedSummary = append(policyRejectedSummary, fmt.Sprintf("%s: failed to fetch market data", pair))
			continue
		}

		if len(cache.m15) == 0 {
			policyRejectedSummary = append(policyRejectedSummary, fmt.Sprintf("%s: m15 candles empty", pair))
			continue
		}

		// Validate staleness of raw candles
		GetGlobalMetrics().AddStalenessChecked(1)
		if !uc.stalenessUsecase.IsFresh(cache.m15) {
			GetGlobalMetrics().AddStalenessCount(1)
			policyRejectedSummary = append(policyRejectedSummary, fmt.Sprintf("%s: raw candles are stale", pair))
			continue
		}

		// Choose strategy playbooks
		latestPrice := tickerLastPrice[pair]
		if latestPrice == 0 && len(cache.m15) > 0 {
			latestPrice = cache.m15[len(cache.m15)-1].Close
		}

		fr := fundingRates[pair]
		priceChange24h := 0.0
		if t, ok := tickerMap[pair]; ok {
			priceChange24h = t.PriceChangePercent
		}
		tech, structure := PopulateSnapshots(cache.m15, cache.h1, cache.h4, fr, latestPrice, priceChange24h, 0.0)
		prelimData := MarketData{
			Symbol:         pair,
			FundingRate:    fr,
			M15Candles:     cache.m15,
			H1Candles:      cache.h1,
			H4Candles:      cache.h4,
			LatestPrice:    latestPrice,
			PriceChange24h: priceChange24h,
		}

		selections := uc.strategySelectorUsecase.SelectPlaybooks(policy, candidate, prelimData, tech, structure)
		totalStrategySelected += len(selections)

		for _, sel := range selections {
			eligibilityRes := uc.playbookEligibilityUsecase.CheckEligibility(sel, policy, prelimData, tech, structure)
			if !eligibilityRes.Eligible {
				eligibilityFailures = append(eligibilityFailures, eligibilityFailure{
					Symbol:       pair,
					StrategyName: sel.StrategyName,
					Direction:    string(sel.Direction),
					Reason:       eligibilityRes.Reason,
				})
				continue
			}
			totalPlaybookEligible++

			playbook := TREND_PULLBACK
			switch sel.StrategyName {
			case string(COMPRESSION_BREAKOUT_RETEST):
				playbook = COMPRESSION_BREAKOUT_RETEST
			case string(LIQUIDITY_SWEEP_REVERSAL):
				playbook = LIQUIDITY_SWEEP_REVERSAL
			case string(RANGE_EDGE_REVERSAL):
				playbook = RANGE_EDGE_REVERSAL
			case string(CROWDED_POSITIONING_SQUEEZE):
				playbook = CROWDED_POSITIONING_SQUEEZE
			}

			quantResult := uc.playbookQuantEngineUsecase.RunEngine(playbook, sel.Direction, prelimData, policy)
			quantResult.Tier = candidate.Tier
			quantResult.RawKlines = cache.m15

			reconciliationDir := uc.conflictResolverUsecase.Resolve(quantResult.Direction, "NEUTRAL")
			_ = uc.scoringUsecase.Calculate(&quantResult, reconciliationDir, policy)

			allCandidates = append(allCandidates, quantResult)
		}
	}

	type rejectKey struct {
		Symbol       string
		StrategyName string
		Reason       string
	}
	rejectGroups := make(map[rejectKey][]string)
	var rejectKeys []rejectKey

	for _, f := range eligibilityFailures {
		key := rejectKey{Symbol: f.Symbol, StrategyName: f.StrategyName, Reason: f.Reason}
		if _, ok := rejectGroups[key]; !ok {
			rejectKeys = append(rejectKeys, key)
		}
		rejectGroups[key] = append(rejectGroups[key], f.Direction)
	}

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

	// Run Candidate Arbiter
	selectedCandidates, arbiterRejected := uc.candidateArbiterUsecase.Arbitrate(allCandidates, policy)
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

	// Evaluate Local Quality Gate
	var localCandidates []QuantResult
	localGateMap := make(map[string]LocalGateResult)
	for _, qResult := range selectedCandidates {
		pair := qResult.Symbol
		cache := candlesMap[pair]
		lgRes := uc.localGateUsecase.Evaluate(qResult, policy, cache.m15)
		localGateMap[pair] = lgRes
		if lgRes.Passed {
			localCandidates = append(localCandidates, qResult)
		}
	}

	// Select AI Candidates based on MaxAICandidates quota limit
	aiCandidates, skippedCandidates := uc.aiCandidateSelectorUsecase.SelectCandidates(localCandidates, policy)
	_ = skippedCandidates

	// Fetch Gemini Audits concurrently
	aiConcurrencyLimit := 3
	if val := os.Getenv("MAX_AI_CONCURRENCY"); val != "" {
		if limit, err := strconv.Atoi(val); err == nil && limit > 0 {
			aiConcurrencyLimit = limit
		}
	}

	type aiAuditResult struct {
		symbol string
		resp   dto.AIAuditResponse
		err    error
	}
	aiAuditsMap := make(map[string]dto.AIAuditResponse)
	var aiMu sync.Mutex

	var aiWg sync.WaitGroup
	aiSem := make(chan struct{}, aiConcurrencyLimit)

	totalAIConfirm := 0
	totalAIWait := 0
	totalAIReject := 0
	totalAIError := 0

	GetGlobalMetrics().AddAICandidateCount(uint64(len(aiCandidates)))

	for _, qResult := range aiCandidates {
		aiWg.Add(1)
		go func(qr QuantResult) {
			defer aiWg.Done()
			aiSem <- struct{}{}
			defer func() { <-aiSem }()

			pair := qr.Symbol
			cache := candlesMap[pair]
			aiStart := time.Now()

			var auditResponse dto.AIAuditResponse
			var auditErr error

			if os.Getenv("AI_AUDIT_ENABLED") == "false" {
				auditResponse = dto.AIAuditResponse{
					Symbol:          pair,
					Decision:        "WAIT",
					IsApproved:      false,
					Sentiment:       "NEUTRAL",
					Confidence:      "LOW",
					ConfidenceScore: 0.3,
					Reasoning:       "AI_AUDIT_DISABLED: AI audit disabled by configuration; forcing non-executable WATCH verdict",
					Reason:          "AI_AUDIT_DISABLED",
					SuggestedAction: "WATCH_ONLY",
				}
			} else {
				auditResponse, auditErr = uc.aiAuditorUsecase.Audit(ctx, qr, policy, cache.m15, cache.h1, cache.h4)
				aiDuration := time.Since(aiStart)
				GetGlobalMetrics().AddAILatency(aiDuration)
			}

			aiMu.Lock()
			if os.Getenv("AI_AUDIT_ENABLED") == "false" {
				totalAIWait++
			} else if auditErr != nil {
				totalAIError++
				if ctx.Err() == context.DeadlineExceeded || strings.Contains(strings.ToLower(auditErr.Error()), "timeout") || strings.Contains(strings.ToLower(auditErr.Error()), "deadline exceeded") {
					GetGlobalMetrics().IncrementAITimeoutCount()
				}
				auditResponse = dto.AIAuditResponse{
					Symbol:     pair,
					IsApproved: false,
					Sentiment:  "NEUTRAL",
					Reasoning:  "AI_ERROR: " + auditErr.Error(),
					Reason:     "AI_ERROR",
				}
			} else {
				if auditResponse.Decision == "CONFIRM" {
					totalAIConfirm++
				} else if auditResponse.Decision == "WAIT" {
					totalAIWait++
				} else {
					totalAIReject++
				}
			}
			aiAuditsMap[pair] = auditResponse
			aiMu.Unlock()
		}(qResult)
	}
	aiWg.Wait()

	// Build context and run Staleness Check and Final Execution Gates for all candidates
	var decisions []FinalDecision
	type candContext struct {
		quantResult     QuantResult
		auditResponse   dto.AIAuditResponse
		stalenessRes    StalenessResult
		localGateResult LocalGateResult
		planReview      PlanReview
		latestPrice     float64
		aiSkipped       bool
	}
	ctxMap := make(map[string]candContext)

	for _, qResult := range selectedCandidates {
		pair := qResult.Symbol
		cache := candlesMap[pair]
		lgRes := localGateMap[pair]

		var auditResponse dto.AIAuditResponse
		aiSkipped := false
		if !lgRes.Passed {
			decision := "REJECT"
			reason := "LOCAL_GATE_FAILED"
			if lgRes.Status == LOCAL_WATCH {
				decision = "WAIT"
				reason = "LOCAL_GATE_WATCH"
			}
			auditResponse = dto.AIAuditResponse{
				Symbol:     pair,
				IsApproved: false,
				Decision:   decision,
				Sentiment:  "NEUTRAL",
				Reasoning:  "Local gate failed: " + lgRes.Reason,
				Reason:     reason,
			}
		} else {
			resp, audited := aiAuditsMap[pair]
			if audited {
				auditResponse = resp
			} else {
				aiSkipped = true
				auditResponse = dto.AIAuditResponse{
					Symbol:     pair,
					IsApproved: false,
					Decision:   "WAIT",
					Sentiment:  "NEUTRAL",
					Reasoning:  "AI_SKIPPED: Exceeded policy MaxAICandidates quota limit",
					Reason:     "AI_SKIPPED",
				}
			}
		}

		planReview := uc.planReconciliationUsecase.Reconcile(qResult, auditResponse)

		latestPrice := 0.0
		if t, exists := tickerMap[pair]; exists {
			latestPrice = t.LastPrice
		}
		if latestPrice == 0 && len(cache.m15) > 0 {
			latestPrice = cache.m15[len(cache.m15)-1].Close
		}

		stalenessRes := uc.stalenessUsecase.Evaluate(qResult, planReview, policy, latestPrice)
		GetGlobalMetrics().AddStalenessChecked(1)
		if stalenessRes.IsStale {
			GetGlobalMetrics().AddStalenessCount(1)
		}

		finalDecision := uc.finalGateUsecase.Evaluate(
			qResult,
			lgRes,
			auditResponse,
			planReview,
			stalenessRes,
			policy,
			latestPrice,
			activeSignals,
			historySignals,
			cache.m15,
		)

		ctxMap[pair] = candContext{
			quantResult:     qResult,
			auditResponse:   auditResponse,
			stalenessRes:    stalenessRes,
			localGateResult: lgRes,
			planReview:      planReview,
			latestPrice:     latestPrice,
			aiSkipped:       aiSkipped,
		}

		decisions = append(decisions, finalDecision)
	}

	// Resolve conflicts and cooldown
	resolvedDecisions, updatedHistory := uc.conflictResolverUsecase.ResolveConflicts(decisions, historySignals, activeSignals, policy)

	// Save signal history
	histState := &entity.SignalHistory{
		Signals: updatedHistory,
	}
	if err := uc.storageUsecase.SaveSignalHistory(histState); err != nil {
		slog.Warn("Failed to save signal history", "error", err)
	}

	// Build beforeDecision lookup by symbol to avoid index mismatch after map iteration in ResolveConflicts
	beforeDecisionBySymbol := make(map[string]FinalDecision, len(decisions))
	for _, d := range decisions {
		beforeDecisionBySymbol[d.Symbol] = d
	}

	// Counters and list builders for final summary
	totalFinalExecute := 0
	totalFinalWatch := 0
	totalFinalReject := 0

	executeSignals := []dto.SignalResponse{}
	watchlistSignals := []dto.SignalResponse{}

	for _, finalDecision := range resolvedDecisions {
		pair := finalDecision.Symbol
		candCtx := ctxMap[pair]
		beforeDecision := beforeDecisionBySymbol[pair]

		// Save decision audit atomically
		conflictReason := ""
		cooldownReason := ""
		if finalDecision.Status != FINAL_EXECUTE && beforeDecision.Status == FINAL_EXECUTE {
			if finalDecision.WatchReason == "ACTIVE_MONITORING_EXISTS" || finalDecision.WatchReason == "OPPOSITE_SIGNAL_CONFLICT" || finalDecision.WatchReason == "LOWER_PRIORITY_CONFLICT" || finalDecision.WatchReason == "BTC_CHAOS_LIMIT" {
				conflictReason = finalDecision.WatchReason
				GetGlobalMetrics().AddConflictDowngrade(1)
			} else {
				cooldownReason = finalDecision.WatchReason
				GetGlobalMetrics().AddCooldownReject(1)
			}
		}

		localGateStatus := "FAILED"
		if candCtx.localGateResult.Passed {
			localGateStatus = "PASSED"
		}

		activeMode := string(policy.LongMode)
		if finalDecision.Direction == SHORT {
			activeMode = string(policy.ShortMode)
		}

		tp1 := 0.0
		tp2 := finalDecision.TakeProfit
		entryPrice := finalDecision.EntryPrice
		if entryPrice > 0 && tp2 > 0 {
			if finalDecision.Direction == LONG {
				tp1 = entryPrice + (tp2-entryPrice)*0.5
			} else {
				tp1 = entryPrice - (entryPrice-tp2)*0.5
			}
		}

		audit := DecisionAudit{
			ScanID:                    scanID,
			ConfigVersion:             GetGlobalConfigRegistry().GetVersion(),
			GeneratedAt:               time.Now(),
			Symbol:                    pair,
			Direction:                 finalDecision.Direction,
			Playbook:                  finalDecision.Playbook,
			SetupType:                 candCtx.quantResult.SetupType,
			Tier:                      finalDecision.Tier,
			Grade:                     getGrade(finalDecision.Score),
			Score:                     finalDecision.Score,
			RR:                        finalDecision.RR,
			RequiredScore:             finalDecision.RequiredScore,
			RequiredRR:                finalDecision.RequiredRR,
			LocalGateStatus:           localGateStatus,
			LocalGateReason:           candCtx.localGateResult.Reason,
			AIDecision:                candCtx.auditResponse.Decision,
			AIConfidence:              finalDecision.AIConfidence,
			AICandleNarrative:         candCtx.auditResponse.CandleNarrative,
			AIEntryTiming:             candCtx.auditResponse.EntryTiming,
			AIConflictWithBot:         candCtx.auditResponse.ConflictWithBot,
			PlanStatus:                string(candCtx.planReview.Status),
			PlanConflict:              candCtx.planReview.Conflicted,
			NeedRetest:                candCtx.planReview.NeedRetest,
			StalenessStatus:           string(candCtx.stalenessRes.Status),
			FinalStatusBeforeConflict: beforeDecision.Status,
			FinalReasonBeforeConflict: beforeDecision.Reason,
			FinalStatusAfterConflict:  finalDecision.Status,
			FinalReasonAfterConflict:  finalDecision.Reason,
			FinalStatus:               finalDecision.Status,
			FinalReason:               finalDecision.Reason,
			ConflictReason:            conflictReason,
			CooldownReason:            cooldownReason,
			WasNotified:               finalDecision.Status == FINAL_EXECUTE && finalDecision.IsExecutable,
			LatestPriceAtDecision:     candCtx.latestPrice,
			EntryPrice:                entryPrice,
			StopLoss:                  finalDecision.StopLoss,
			TakeProfit1:               tp1,
			TakeProfit2:               tp2,
			MarketRegime:              policy.Reason,
			PolicyMode:                activeMode,
			ThresholdProfileSummary:   finalDecision.ThresholdProfileSummary,
			RejectOrWatchReason:       finalDecision.Reason,
			CreatedAt:                 time.Now(),
			HypotheticalEntry:         entryPrice,
		}

		if os.Getenv("DECISION_AUDIT_ENABLED") != "false" {
			if err := uc.storageUsecase.SaveDecisionAudit(audit); err != nil {
				slog.Warn("Failed to save decision audit trail", "symbol", pair, "error", err)
			}
		}

		// Count final statuses
		if finalDecision.Status == FINAL_EXECUTE {
			totalFinalExecute++
			sigRes := dto.SignalResponse{
				Symbol:         pair,
				Direction:      string(finalDecision.Direction),
				Timeframe:      "M15",
				TriggerPrice:   finalDecision.EntryPrice,
				StopLoss:       finalDecision.StopLoss,
				TakeProfit:     finalDecision.TakeProfit,
				Score:          finalDecision.Score,
				Strategy:       string(finalDecision.Playbook),
				AISentiment:    candCtx.auditResponse.Sentiment,
				IsFinalExecute: true,
				ReconciledTime: time.Now(),
				Status:         string(FINAL_EXECUTE),
			}
			executeSignals = append(executeSignals, sigRes)
			finalSignals = append(finalSignals, sigRes)

			// Save to virtual journal
			_ = uc.storageUsecase.SaveSignalToJournal(SignalJournal{
				ID:                      time.Now().Format("20060102150405") + "_" + pair,
				ConfigVersion:           GetGlobalConfigRegistry().GetVersion(),
				Symbol:                  pair,
				Direction:               finalDecision.Direction,
				Playbook:                finalDecision.Playbook,
				EntryPrice:              finalDecision.EntryPrice,
				StopLoss:                finalDecision.StopLoss,
				TP1:                     tp1,
				TP2:                     tp2,
				RR:                      finalDecision.RR,
				QuantScore:              finalDecision.Score,
				AIConfidence:            finalDecision.AIConfidence,
				MarketRegime:            policy.Reason,
				PolicyMode:              activeMode,
				ThresholdProfileSummary: finalDecision.ThresholdProfileSummary,
				CreatedAt:               time.Now(),
				ExpiresAt:               time.Now().Add(120 * time.Minute),
				Status:                  MONITORING,
				MFE:                     0.0,
				MAE:                     0.0,
				TimeToTP1:               "",
				TimeToTP2:               "",
				TimeToSL:                "",
				OutcomeReason:           "",
				EntryTiming:             candCtx.auditResponse.EntryTiming,
				Tier:                    candCtx.quantResult.Tier,
				Timeframe:               "M15",
				LatestPrice:             finalDecision.EntryPrice,
				TakeProfit:              tp2,
				AISentiment:             candCtx.auditResponse.Sentiment,
				AIReasoning:             candCtx.auditResponse.Reasoning,
				UpdatedAt:               time.Now(),
			})
		} else if finalDecision.Status == FINAL_WATCH {
			totalFinalWatch++
			watchlistSignals = append(watchlistSignals, dto.SignalResponse{
				Symbol:         pair,
				Direction:      string(finalDecision.Direction),
				Timeframe:      "M15",
				TriggerPrice:   finalDecision.EntryPrice,
				StopLoss:       finalDecision.StopLoss,
				TakeProfit:     finalDecision.TakeProfit,
				Score:          finalDecision.Score,
				Strategy:       string(finalDecision.Playbook),
				AISentiment:    candCtx.auditResponse.Sentiment,
				IsFinalExecute: false,
				ReconciledTime: time.Now(),
				Status:         string(FINAL_WATCH),
			})
		} else {
			totalFinalReject++
		}
	}

	thresholdProfileSummary := make(map[string]string)
	for _, fd := range resolvedDecisions {
		thresholdProfileSummary[string(fd.Playbook)] = fd.ThresholdProfileSummary
	}

	// Dispatch V3 Notifications
	summary := ScannerSummaryV3{
		TotalScanned:                    len(candidates),
		CandidatesFound:                 len(decisions),
		StartTime:                       scanStart,
		Duration:                        time.Since(scanStart).String(),
		ActiveRegime:                    policy.Reason,
		BtcTrend:                        btcTrend,
		TotalTickers:                    totalTickers,
		TotalUniversePass:               len(candidates),
		TotalUniverseRejected:           totalTickers - len(candidates),
		TotalStrategySelected:           totalStrategySelected,
		TotalPlaybookEligible:           totalPlaybookEligible,
		TotalQuantCandidates:            len(allCandidates),
		TotalArbiterSelected:            len(selectedCandidates),
		TotalLocalAICandidate:           len(localCandidates),
		TotalAIConfirm:                  totalAIConfirm,
		TotalAIWait:                     totalAIWait,
		TotalAIReject:                   totalAIReject,
		TotalAIError:                    totalAIError,
		TotalFinalExecute:               totalFinalExecute,
		TotalFinalWatch:                 totalFinalWatch,
		TotalFinalReject:                totalFinalReject,
		ExecuteSignals:                  executeSignals,
		Watchlist:                       watchlistSignals,
		RejectedSummary:                 rejectedSummary,
		PolicyRejectedSummary:           policyRejectedSummary,
		SelectedThresholdProfileSummary: thresholdProfileSummary,
		EvaluationDataCompletenessHint:  "has_decision_audit: true",
	}

	var notificationReqs []SignalNotificationRequest
	for _, dec := range resolvedDecisions {
		pair := dec.Symbol
		candCtx := ctxMap[pair]
		notificationReqs = append(notificationReqs, SignalNotificationRequest{
			Decision:      dec,
			AuditResponse: candCtx.auditResponse,
		})
	}

	// OPS: optional admin warning for AI_ERROR_REVIEW (must NOT be sent via SIGNAL channel).
	if uc.opsNotificationUsecase != nil {
		for _, dec := range resolvedDecisions {
			if dec.Status == AI_ERROR_REVIEW {
				uc.opsNotificationUsecase.SendAdminWarningAIError(
					ctx,
					scanID,
					dec.Symbol,
					string(dec.Playbook),
					string(dec.Status),
					dec.Reason,
				)
			}
		}
	}

	// SIGNAL: only actionable FINAL_EXECUTE after conflict resolution.
	if uc.signalNotificationUsecase != nil {
		uc.signalNotificationUsecase.SendV3Signals(ctx, notificationReqs, policy, summary)
	}

	arbiterDetails := []entity.ArbiterSelectedDetail{}
	for _, dec := range resolvedDecisions {
		pair := dec.Symbol
		candCtx := ctxMap[pair]

		localGateStatus := "FAILED"
		if candCtx.localGateResult.Passed {
			localGateStatus = "PASSED"
		} else if candCtx.localGateResult.Status == LOCAL_WATCH {
			localGateStatus = "LOCAL_WATCH"
		}

		arbiterDetails = append(arbiterDetails, entity.ArbiterSelectedDetail{
			Symbol:          pair,
			Playbook:        string(dec.Playbook),
			Direction:       string(dec.Direction),
			LocalGateStatus: localGateStatus,
			AIDecision:      candCtx.auditResponse.Decision,
			StalenessStatus: string(candCtx.stalenessRes.Status),
			FinalStatus:     string(dec.Status),
			FinalReason:     dec.Reason,
		})
	}

	// Save latest scan results
	latestResult := &entity.LatestResult{
		GeneratedAt:                     scanStart,
		ConfigVersion:                   GetGlobalConfigRegistry().GetVersion(),
		ScanID:                          scanID,
		MarketPolicy:                    policy.Reason,
		MarketRegime:                    policy.Reason,
		TotalTickers:                    totalTickers,
		TotalUniversePass:               len(candidates),
		TotalUniverseRejected:           totalTickers - len(candidates),
		TotalStrategySelected:           totalStrategySelected,
		TotalPlaybookEligible:           totalPlaybookEligible,
		TotalQuantCandidates:            len(allCandidates),
		TotalArbiterSelected:            len(selectedCandidates),
		TotalLocalAICandidate:           len(localCandidates),
		TotalAIConfirm:                  totalAIConfirm,
		TotalAIWait:                     totalAIWait,
		TotalAIReject:                   totalAIReject,
		TotalAIError:                    totalAIError,
		TotalFinalExecute:               totalFinalExecute,
		TotalFinalWatch:                 totalFinalWatch,
		TotalFinalReject:                totalFinalReject,
		ExecuteSignals:                  executeSignals,
		Watchlist:                       watchlistSignals,
		RejectedSummary:                 rejectedSummary,
		PolicyRejectedSummary:           policyRejectedSummary,
		SelectedThresholdProfileSummary: thresholdProfileSummary,
		EvaluationDataCompletenessHint:  "has_decision_audit: true",
		ArbiterSelectedDetails:          arbiterDetails,

		LastScanTime: scanStart,
		Duration:     time.Since(scanStart).String(),
		Signals:      finalSignals,
	}

	if err := uc.storageUsecase.SaveLatestResult(latestResult); err != nil {
		slog.Error("Failed to save latest scan result to storage", "error", err)
	}

	duration := time.Since(scanStart)
	GetGlobalMetrics().SetLastScanDuration(duration)
	GetGlobalMetrics().SetLastScanTime(scanStart)
	GetGlobalMetrics().IncrementScanSuccess()
	GetGlobalMetrics().SetLastSuccessScan(scanStart)
	GetGlobalMetrics().AddTotalTickers(uint64(totalTickers))
	GetGlobalMetrics().AddUniversePass(uint64(len(candidates)))
	GetGlobalMetrics().AddUniverseReject(uint64(totalTickers - len(candidates)))

	GetGlobalMetrics().AddFinalExecuteCount(uint64(totalFinalExecute))
	GetGlobalMetrics().AddFinalWatchCount(uint64(totalFinalWatch))
	GetGlobalMetrics().AddFinalRejectCount(uint64(totalFinalReject))

	slog.Info("AnalyzeMarketV3 Scan Completed", "scan_id", scanID, "found_signals", len(finalSignals))

	return dto.ScanResult{
		Timestamp: scanStart,
		Duration:  time.Since(scanStart).String(),
		Found:     len(finalSignals),
		Signals:   finalSignals,
	}, nil
}
