package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
	"cpbro-engine/internal/modules/cryptobroV3/entity"
)

type watchRecheckSummary struct {
	ScanID          string
	TriggerTime     time.Time
	EligibleWatches int
	Evaluated       int
	Promoted        int
	FinalExecute    int
	FinalWatch      int
	FinalReject     int
}

type watchRecheckCandidate struct {
	entry WatchJournal
}

type watchRecheckEvaluation struct {
	origin   WatchJournal
	decision FinalDecision
	context  scanCandidateContext
}

func (uc *ScannerUsecase) RunWatchRecheck(ctx context.Context, req dto.ScanRequest) (watchRecheckSummary, error) {
	start := time.Now()
	triggerTime := req.TriggerTime
	if triggerTime.IsZero() {
		triggerTime = start
	}
	scanID := "recheck_" + triggerTime.Format("20060102150405")
	summary := watchRecheckSummary{
		ScanID:      scanID,
		TriggerTime: triggerTime,
	}

	metrics := GetGlobalMetrics()
	defer func() {
		metrics.SetLastRecheckDuration(time.Since(start))
		metrics.SetLastRecheckTime(start)
	}()

	watchJournal, err := uc.storageUsecase.LoadWatchJournal()
	if err != nil {
		metrics.IncrementRecheckFail()
		return summary, fmt.Errorf("failed to load watch journal for recheck: %w", err)
	}

	shortlist := selectWatchRecheckCandidates(watchJournal, triggerTime)
	summary.EligibleWatches = len(shortlist)
	if len(shortlist) == 0 {
		slog.Info("Watch recheck skipped: no eligible watch candidates", "scan_id", scanID)
		metrics.IncrementRecheckSuccess()
		return summary, nil
	}

	activeSignals, _ := uc.storageUsecase.LoadSignalJournal()
	historyState, _ := uc.storageUsecase.LoadSignalHistory()
	var historySignals []dto.SignalResponse
	if historyState != nil {
		historySignals = historyState.Signals
	}
	previousLatest, _ := uc.storageUsecase.LoadLatestResult()

	tickers, err := uc.marketDataUsecase.FetchAllFuturesTickers24h(ctx)
	if err != nil {
		metrics.IncrementRecheckFail()
		metrics.IncrementMarketDataError()
		return summary, fmt.Errorf("watch recheck ticker bootstrap failed: %w", err)
	}
	fundingRates, err := uc.marketDataUsecase.FetchPremiumFundingRates(ctx)
	if err != nil {
		metrics.IncrementRecheckFail()
		metrics.IncrementMarketDataError()
		return summary, fmt.Errorf("watch recheck funding bootstrap failed: %w", err)
	}

	tickerMap := make(map[string]dto.Ticker24h, len(tickers))
	var ethTicker *dto.Ticker24h
	for i := range tickers {
		tickerMap[tickers[i].Symbol] = tickers[i]
		if tickers[i].Symbol == "ETHUSDT" {
			ethTicker = &tickers[i]
		}
	}

	macroState := deriveMacroMarketState(tickers)
	ethBtcPerf := 0.0
	if btcTicker, ok := tickerMap["BTCUSDT"]; ok && ethTicker != nil {
		ethBtcPerf = (ethTicker.PriceChangePercent - btcTicker.PriceChangePercent) / 100.0
	}

	policy := uc.marketPolicyUsecase.EvaluatePolicy(ctx, macroState.BTCTrend, macroState.BTCScore, ethBtcPerf, macroState.BTCChaos, macroState.Volatility, macroState.Breadth)
	compressionMacroActive := isCompressionMacroActive(macroState)
	if shouldFallbackCompressionToLowVol(previousLatest, compressionMacroActive) {
		policy = normalizeLowVolPolicy(policy, macroState.BTCTrend, "LOW_VOL fallback active - prior compression scans produced zero eligible setups")
	}

	evaluations := make([]watchRecheckEvaluation, 0, len(shortlist))
	for _, candidate := range shortlist {
		evaluated, ok := uc.evaluateWatchRecheckCandidate(ctx, candidate.entry, policy, fundingRates, tickerMap, activeSignals, historySignals)
		if !ok {
			continue
		}
		evaluations = append(evaluations, evaluated)
	}

	summary.Evaluated = len(evaluations)
	if len(evaluations) == 0 {
		slog.Info("Watch recheck completed with zero evaluable candidates", "scan_id", scanID, "eligible_watch_candidates", summary.EligibleWatches)
		metrics.IncrementRecheckSuccess()
		return summary, nil
	}

	decisions := make([]FinalDecision, 0, len(evaluations))
	ctxMap := make(map[string]scanCandidateContext, len(evaluations))
	originMap := make(map[string]WatchJournal, len(evaluations))
	beforeDecisionBySymbol := make(map[string]FinalDecision, len(evaluations))
	for _, evaluated := range evaluations {
		pair := evaluated.decision.Symbol
		decisions = append(decisions, evaluated.decision)
		ctxMap[pair] = evaluated.context
		originMap[pair] = evaluated.origin
		beforeDecisionBySymbol[pair] = evaluated.decision
	}

	resolvedDecisions, updatedHistory := uc.conflictResolverUsecase.ResolveConflicts(decisions, historySignals, activeSignals, policy)
	if err := uc.storageUsecase.SaveSignalHistory(&entity.SignalHistory{Signals: updatedHistory}); err != nil {
		slog.Warn("Watch recheck failed to save signal history", "scan_id", scanID, "error", err)
	}

	notificationReqs := make([]SignalNotificationRequest, 0, len(resolvedDecisions))
	decisionAudits := make([]DecisionAudit, 0, len(resolvedDecisions))
	now := time.Now()
	maxHoldDuration := getMonitoringMaxHoldDuration()

	for _, finalDecision := range resolvedDecisions {
		pair := finalDecision.Symbol
		candCtx := ctxMap[pair]
		origin := originMap[pair]
		beforeDecision := beforeDecisionBySymbol[pair]

		switch finalDecision.Status {
		case FINAL_EXECUTE:
			summary.FinalExecute++
		case FINAL_WATCH:
			summary.FinalWatch++
		default:
			summary.FinalReject++
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

		conflictReason := ""
		cooldownReason := ""
		if finalDecision.Status != FINAL_EXECUTE && beforeDecision.Status == FINAL_EXECUTE {
			if finalDecision.WatchReason == "ACTIVE_MONITORING_EXISTS" || finalDecision.WatchReason == "OPPOSITE_SIGNAL_CONFLICT" || finalDecision.WatchReason == "LOWER_PRIORITY_CONFLICT" || finalDecision.WatchReason == "BTC_CHAOS_LIMIT" {
				conflictReason = finalDecision.WatchReason
				metrics.AddConflictDowngrade(1)
			} else {
				cooldownReason = finalDecision.WatchReason
				metrics.AddCooldownReject(1)
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

		audit := DecisionAudit{
			ScanID:                    scanID,
			ConfigVersion:             GetGlobalConfigRegistry().GetVersion(),
			GeneratedAt:               now,
			Symbol:                    pair,
			Direction:                 finalDecision.Direction,
			Playbook:                  finalDecision.Playbook,
			SetupType:                 candCtx.quantResult.SetupType,
			Tier:                      finalDecision.Tier,
			Grade:                     getGrade(finalDecision.Score),
			Score:                     finalDecision.Score,
			RR:                        finalDecision.RR,
			RRPlan:                    finalDecision.PlannedRR,
			RRActual:                  finalDecision.ActualRR,
			RequiredScore:             finalDecision.RequiredScore,
			RequiredRR:                finalDecision.RequiredRR,
			LocalGateStatus:           localGateStatus,
			LocalGateReason:           candCtx.localGateResult.Reason,
			EnteredAIBatch:            candCtx.localGateResult.Passed,
			AICalled:                  finalDecision.AICalled,
			AISource:                  finalDecision.AISource,
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
			FinalPrimaryReasonLayer:   finalDecision.PrimaryReasonLayer,
			FinalReasonBreakdown:      append([]string(nil), finalDecision.ReasonBreakdown...),
			ConflictReason:            conflictReason,
			CooldownReason:            cooldownReason,
			WasNotified:               finalDecision.Status == FINAL_EXECUTE && finalDecision.IsExecutable,
			LatestPriceAtDecision:     candCtx.latestPrice,
			EntryPrice:                entryPrice,
			StopLoss:                  finalDecision.StopLoss,
			TakeProfit1:               tp1,
			TakeProfit2:               tp2,
			MarketRegime:              string(policy.Regime),
			PolicyMode:                activeMode,
			ThresholdProfileSummary:   finalDecision.ThresholdProfileSummary,
			BreakoutLevel:             candCtx.quantResult.TechnicalSnapshot.IndicatorValues[IndicatorBreakoutLevel],
			RetestTouches:             candCtx.quantResult.TechnicalSnapshot.IndicatorValues[IndicatorRetestTouches],
			RetestHold:                candCtx.quantResult.TechnicalSnapshot.IndicatorValues[IndicatorRetestHold] == 1.0,
			HasDerivativesEvidence:    candCtx.quantResult.TechnicalSnapshot.IndicatorValues[IndicatorHasCrowdingEvidence] == 1.0,
			RejectOrWatchReason:       fmt.Sprintf("watch_recheck origin=%s; %s", origin.ID, finalDecision.Reason),
			CreatedAt:                 now,
			HypotheticalEntry:         entryPrice,
		}
		if candCtx.localGateResult.M5Summary != nil {
			audit.M5ConfirmationUsed = candCtx.localGateResult.M5Summary.Used
			audit.M5ConfirmationMode = string(candCtx.localGateResult.M5Summary.Mode)
			audit.M5ConfirmationStatus = string(candCtx.localGateResult.M5Summary.Status)
			audit.M5ConfirmationReason = candCtx.localGateResult.M5Summary.Reason
			audit.M5ConfirmationType = candCtx.localGateResult.M5Summary.ConfirmationType
			audit.M5Confirmed = candCtx.localGateResult.M5Summary.Confirmed
			audit.M5EarlyInvalidation = candCtx.localGateResult.M5Summary.EarlyInvalidation
		}
		if getRuntimeSettings().DecisionAuditEnabled {
			decisionAudits = append(decisionAudits, audit)
		}

		if finalDecision.Status != FINAL_EXECUTE || !finalDecision.IsExecutable {
			continue
		}

		summary.Promoted++
		promotionReason := fmt.Sprintf("WATCH_RECHECK_PROMOTION origin_watch_id=%s | %s", origin.ID, finalDecision.Reason)
		notificationReqs = append(notificationReqs, SignalNotificationRequest{
			Decision:      finalDecision,
			AuditResponse: candCtx.auditResponse,
		})

		_ = uc.storageUsecase.SaveSignalToJournal(SignalJournal{
			ID:                      now.Format("20060102150405") + "_recheck_" + pair,
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
			MarketRegime:            string(policy.Regime),
			PolicyMode:              activeMode,
			ThresholdProfileSummary: finalDecision.ThresholdProfileSummary,
			BreakoutLevel:           candCtx.quantResult.TechnicalSnapshot.IndicatorValues[IndicatorBreakoutLevel],
			RetestTouches:           candCtx.quantResult.TechnicalSnapshot.IndicatorValues[IndicatorRetestTouches],
			RetestHold:              candCtx.quantResult.TechnicalSnapshot.IndicatorValues[IndicatorRetestHold] == 1.0,
			HasDerivativesEvidence:  candCtx.quantResult.TechnicalSnapshot.IndicatorValues[IndicatorHasCrowdingEvidence] == 1.0,
			CreatedAt:               now,
			ExpiresAt:               now.Add(maxHoldDuration),
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
			UpdatedAt:               now,
			Reason:                  promotionReason,
			IsHot:                   origin.IsHot,
			HotScore:                origin.HotScore,
			HotSource:               origin.HotSource,
			HotRankType:             origin.HotRankType,
			HotOverlaySelected:      origin.HotOverlaySelected,
			TechnicalSnapshot:       candCtx.quantResult.TechnicalSnapshot,
			StructureSnapshot:       candCtx.quantResult.StructureSnapshot,
		})

		_ = uc.promoteWatchJournalEntry(origin, now, promotionReason)
	}

	if getRuntimeSettings().DecisionAuditEnabled && len(decisionAudits) > 0 {
		if err := uc.storageUsecase.SaveDecisionAuditBatch(decisionAudits); err != nil {
			slog.Warn("Watch recheck failed to save decision audits", "scan_id", scanID, "count", len(decisionAudits), "error", err)
		}
	}

	if len(notificationReqs) > 0 && uc.signalNotificationUsecase != nil {
		uc.signalNotificationUsecase.SendV3Signals(ctx, notificationReqs, policy, ScannerSummaryV3{
			ActiveRegime: string(policy.Regime),
		})
	}

	metrics.IncrementRecheckSuccess()
	metrics.AddRecheckPromotionCount(uint64(summary.Promoted))
	metrics.AddFinalExecuteCount(uint64(summary.FinalExecute))
	metrics.AddFinalWatchCount(uint64(summary.FinalWatch))
	metrics.AddFinalRejectCount(uint64(summary.FinalReject))

	slog.Info(
		"Watch recheck completed",
		"scan_id", scanID,
		"eligible_watch_candidates", summary.EligibleWatches,
		"evaluated_candidates", summary.Evaluated,
		"promoted_execute", summary.Promoted,
		"final_execute", summary.FinalExecute,
		"final_watch", summary.FinalWatch,
		"final_reject", summary.FinalReject,
	)

	return summary, nil
}

func (uc *ScannerUsecase) evaluateWatchRecheckCandidate(
	ctx context.Context,
	entry WatchJournal,
	policy MarketPolicy,
	fundingRates map[string]float64,
	tickerMap map[string]dto.Ticker24h,
	activeSignals []SignalJournal,
	historySignals []dto.SignalResponse,
) (watchRecheckEvaluation, bool) {
	fullData, err := uc.marketDataUsecase.FetchMarketData(ctx, entry.Symbol, fundingRates)
	if err != nil {
		slog.Warn("Watch recheck market data fetch failed", "symbol", entry.Symbol, "error", err)
		GetGlobalMetrics().IncrementMarketDataError()
		return watchRecheckEvaluation{}, false
	}
	if !uc.stalenessUsecase.IsFresh(fullData.M15Candles) {
		return watchRecheckEvaluation{}, false
	}

	latestPrice := 0.0
	if ticker, ok := tickerMap[entry.Symbol]; ok {
		latestPrice = ticker.LastPrice
		fullData.PriceChange24h = ticker.PriceChangePercent
	}
	if latestPrice == 0 && len(fullData.M15Candles) > 0 {
		latestPrice = fullData.M15Candles[len(fullData.M15Candles)-1].Close
	}
	fullData.LatestPrice = latestPrice

	prepared, ok := uc.playbookQuantEngineUsecase.prepareContext(fullData)
	if !ok {
		return watchRecheckEvaluation{}, false
	}
	tech := &prepared.technicalSnapshot
	structure := &prepared.structureSnapshot

	sel := StrategySelection{
		Symbol:       entry.Symbol,
		StrategyName: string(entry.Playbook),
		Direction:    entry.Direction,
		Tier:         entry.Tier,
		Status:       STRATEGY_SELECTED,
		Reason:       "Secondary watch recheck candidate",
	}

	eligibilityRes := uc.playbookEligibilityUsecase.CheckEligibility(sel, policy, fullData, tech, structure)
	if !eligibilityRes.Eligible {
		return watchRecheckEvaluation{}, false
	}

	quantResult := uc.playbookQuantEngineUsecase.RunEngineWithPreparedContext(entry.Playbook, entry.Direction, fullData, policy, prepared)
	quantResult.Tier = entry.Tier
	quantResult.RawKlines = fullData.M15Candles
	if !isArbiterReadyQuantResult(quantResult) {
		return watchRecheckEvaluation{}, false
	}

	reconciliationDir := uc.conflictResolverUsecase.Resolve(quantResult.Direction, "NEUTRAL")
	_ = uc.scoringUsecase.Calculate(&quantResult, reconciliationDir, policy)

	localGateResult := uc.localGateUsecase.EvaluateWithContext(ctx, quantResult, policy, fullData.M15Candles)

	var auditResponse dto.AIAuditResponse
	hasAuditedResponse := false
	if localGateResult.Passed {
		hasAuditedResponse = true
		if !getRuntimeSettings().AIAuditEnabled {
			auditResponse = dto.AIAuditResponse{
				Symbol:          entry.Symbol,
				Decision:        "WAIT",
				IsApproved:      false,
				Sentiment:       "NEUTRAL",
				Confidence:      "LOW",
				ConfidenceScore: 0.3,
				Reasoning:       "AI_AUDIT_DISABLED: AI audit disabled by configuration; forcing non-executable WATCH verdict",
				Reason:          "AI_AUDIT_DISABLED",
				SuggestedAction: "WATCH_ONLY",
				Source:          AIAuditSourceSyntheticDisabled,
			}
		} else {
			resp, auditErr := uc.aiAuditorUsecase.Audit(ctx, quantResult, policy, fullData.M15Candles, fullData.H1Candles, fullData.H4Candles, localGateResult.M5Summary)
			if auditErr != nil {
				if ctx.Err() == context.DeadlineExceeded || strings.Contains(strings.ToLower(auditErr.Error()), "timeout") || strings.Contains(strings.ToLower(auditErr.Error()), "deadline exceeded") {
					GetGlobalMetrics().IncrementAITimeoutCount()
				}
				auditResponse = dto.AIAuditResponse{
					Symbol:     entry.Symbol,
					IsApproved: false,
					Sentiment:  "NEUTRAL",
					Reasoning:  "AI_ERROR: " + auditErr.Error(),
					Reason:     "AI_ERROR",
					Source:     AIAuditSourceRealError,
				}
			} else {
				auditResponse = resp
				auditResponse.Source = NormalizeAIAuditSource(auditResponse.Source)
			}
		}
	}

	evaluated := uc.evaluateSelectedCandidate(
		ctx,
		quantResult,
		policy,
		activeSignals,
		historySignals,
		fullData,
		localGateResult,
		auditResponse,
		hasAuditedResponse,
	)

	return watchRecheckEvaluation{
		origin:   entry,
		decision: evaluated.decision,
		context:  evaluated.context,
	}, true
}

func selectWatchRecheckCandidates(journal []WatchJournal, now time.Time) []watchRecheckCandidate {
	if len(journal) == 0 {
		return nil
	}

	maxAgeMinutes := getRuntimeSettings().WatchRecheckMaxAgeMinutes
	if maxAgeMinutes <= 0 {
		maxAgeMinutes = 12
	}
	maxAge := time.Duration(maxAgeMinutes) * time.Minute
	limit := getRuntimeSettings().WatchRecheckBatchLimit
	if limit <= 0 {
		limit = 6
	}

	bestBySymbol := make(map[string]WatchJournal)
	for _, entry := range journal {
		if !isWatchRecheckEligible(entry, now, maxAge) {
			continue
		}
		current, exists := bestBySymbol[entry.Symbol]
		if !exists || compareWatchRecheckPriority(entry, current) < 0 {
			bestBySymbol[entry.Symbol] = entry
		}
	}

	shortlist := make([]watchRecheckCandidate, 0, len(bestBySymbol))
	for _, entry := range bestBySymbol {
		shortlist = append(shortlist, watchRecheckCandidate{entry: entry})
	}

	sort.Slice(shortlist, func(i, j int) bool {
		left := shortlist[i].entry
		right := shortlist[j].entry
		return compareWatchRecheckPriority(left, right) < 0
	})

	if len(shortlist) > limit {
		shortlist = shortlist[:limit]
	}
	return shortlist
}

func isWatchRecheckEligible(entry WatchJournal, now time.Time, maxAge time.Duration) bool {
	if entry.Status != WATCH_MONITORING {
		return false
	}
	if !entry.ClosedAt.IsZero() {
		return false
	}
	if entry.Playbook != TREND_PULLBACK && entry.Playbook != LIQUIDITY_SWEEP_REVERSAL {
		return false
	}
	if entry.CreatedAt.IsZero() || now.Sub(entry.CreatedAt) > maxAge {
		return false
	}

	reasonBlob := strings.ToUpper(strings.TrimSpace(entry.Reason + " " + entry.AIReasoning))
	if reasonBlob == "" {
		return false
	}
	for _, blocked := range []string{
		"ACTIVE_MONITORING_EXISTS",
		"OPPOSITE_SIGNAL_CONFLICT",
		"LOWER_PRIORITY_CONFLICT",
		"DUPLICATE_SIGNAL_BUCKET",
		"SYMBOL_COOLDOWN_ACTIVE",
		"MAX_FINAL_EXECUTE_LIMIT",
		"WAIT_RETEST_OR_BREAKOUT_FIRST_CANDLE",
	} {
		if strings.Contains(reasonBlob, blocked) {
			return false
		}
	}
	if strings.Contains(reasonBlob, "M5 ") {
		return true
	}
	for _, recheckSafe := range []string{
		"AI DECISION IS WAIT",
		"AI_SKIPPED",
		"WATCH_ONLY",
		"WAIT_RETEST",
		"AI CONFIDENCE",
		"LOCAL_GATE_WATCH",
	} {
		if strings.Contains(reasonBlob, recheckSafe) {
			return true
		}
	}
	return false
}

func compareWatchRecheckPriority(left, right WatchJournal) int {
	playbookPriority := func(playbook Playbook) int {
		switch playbook {
		case LIQUIDITY_SWEEP_REVERSAL:
			return 0
		case TREND_PULLBACK:
			return 1
		default:
			return 2
		}
	}

	if lp, rp := playbookPriority(left.Playbook), playbookPriority(right.Playbook); lp != rp {
		if lp < rp {
			return -1
		}
		return 1
	}
	if left.CreatedAt.After(right.CreatedAt) {
		return -1
	}
	if left.CreatedAt.Before(right.CreatedAt) {
		return 1
	}
	if left.QuantScore > right.QuantScore {
		return -1
	}
	if left.QuantScore < right.QuantScore {
		return 1
	}
	return strings.Compare(left.Symbol, right.Symbol)
}

func (uc *ScannerUsecase) promoteWatchJournalEntry(origin WatchJournal, now time.Time, promotionReason string) error {
	return uc.storageUsecase.UpdateWatchJournal(func(current []WatchJournal) ([]WatchJournal, error) {
		updated := append([]WatchJournal(nil), current...)
		for i := range updated {
			if updated[i].ID != origin.ID {
				continue
			}
			updated[i].Status = WATCH_PROMOTED
			updated[i].ClosedAt = now
			updated[i].UpdatedAt = now
			updated[i].OutcomeReason = "Promoted to FINAL_EXECUTE via secondary watch recheck"
			updated[i].Reason = promotionReason
			return updated, nil
		}
		return updated, nil
	})
}
