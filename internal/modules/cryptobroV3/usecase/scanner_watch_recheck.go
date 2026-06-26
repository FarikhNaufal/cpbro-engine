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
	Invalidated     int
	Expired         int
	Promoted        int
	FinalExecute    int
	FinalWatch      int
	FinalReject     int
}

type watchRecheckCandidate struct {
	entry WatchJournal
}

type watchRecheckEvaluation struct {
	origin      WatchJournal
	decision    FinalDecision
	context     scanCandidateContext
	disposition watchRecheckDisposition
}

type watchRecheckDisposition struct {
	Eligible       bool
	Terminal       bool
	TerminalStatus Status
	Reason         string
	OutcomeReason  string
}

func (uc *ScannerUsecase) RunWatchRecheck(ctx context.Context, req dto.ScanRequest) (watchRecheckSummary, error) {
	start := time.Now()
	triggerTime := req.TriggerTime
	if triggerTime.IsZero() {
		triggerTime = start
	}
	scanID := buildScanID(scanTriggerRecheck, triggerTime)
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

	shortlist, terminalUpdates := selectWatchRecheckCandidates(watchJournal, triggerTime)
	summary.EligibleWatches = len(shortlist)
	for _, update := range terminalUpdates {
		switch update.disposition.TerminalStatus {
		case WATCH_RECHECK_EXPIRED:
			summary.Expired++
		case WATCH_RECHECK_INVALIDATED:
			summary.Invalidated++
		}
	}
	if len(terminalUpdates) > 0 {
		if err := uc.applyWatchRecheckTerminalUpdates(terminalUpdates, triggerTime); err != nil {
			slog.Warn("Watch recheck failed to persist terminal state updates", "scan_id", scanID, "error", err)
		}
	}
	if len(shortlist) == 0 {
		slog.Info("Watch recheck skipped: no eligible watch candidates", "scan_id", scanID, "scan_trigger", scanTriggerRecheck, "expired", summary.Expired, "invalidated", summary.Invalidated)
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

	tickers, tickersMeta, err := uc.marketDataUsecase.FetchAllFuturesTickers24hWithMeta(ctx)
	if err != nil {
		metrics.IncrementRecheckFail()
		metrics.IncrementMarketDataError()
		return summary, fmt.Errorf("watch recheck ticker bootstrap failed: %w", err)
	}
	fundingRates, fundingMeta, err := uc.marketDataUsecase.FetchPremiumFundingRatesWithMeta(ctx)
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
		evaluated := uc.evaluateWatchRecheckCandidate(ctx, candidate.entry, policy, fundingRates, tickerMap, activeSignals, historySignals)
		if evaluated.disposition.Terminal {
			if err := uc.applyWatchRecheckTerminalUpdates([]watchRecheckEvaluation{evaluated}, triggerTime); err != nil {
				slog.Warn("Watch recheck failed to persist evaluated terminal state", "scan_id", scanID, "symbol", candidate.entry.Symbol, "error", err)
			}
			switch evaluated.disposition.TerminalStatus {
			case WATCH_RECHECK_EXPIRED:
				summary.Expired++
			case WATCH_RECHECK_INVALIDATED:
				summary.Invalidated++
			}
			continue
		}
		if !evaluated.disposition.Eligible {
			continue
		}
		evaluations = append(evaluations, evaluated)
	}

	summary.Evaluated = len(evaluations)
	if len(evaluations) == 0 {
		slog.Info("Watch recheck completed with zero evaluable candidates", "scan_id", scanID, "scan_trigger", scanTriggerRecheck, "eligible_watch_candidates", summary.EligibleWatches)
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
			RejectOrWatchReason:       buildWatchRecheckAuditContext(origin.ID, finalDecision.Reason),
			CreatedAt:                 now,
			HypotheticalEntry:         entryPrice,
		}
		applyPolicySnapshotToDecisionAudit(&audit, policy, false)
		applyBootstrapProvenanceToDecisionAudit(&audit, tickersMeta, fundingMeta)
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
		signalID := now.Format("20060102150405") + "_recheck_" + pair
		promotionReason := buildWatchRecheckPromotionReason(origin.ID, signalID, finalDecision.Reason)
		notificationReqs = append(notificationReqs, SignalNotificationRequest{
			Decision:      finalDecision,
			AuditResponse: candCtx.auditResponse,
		})

		journalEntry := SignalJournal{
			ID:                      signalID,
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
		}
		applyPolicySnapshotToSignalJournal(&journalEntry, policy)
		_ = uc.storageUsecase.SaveSignalToJournal(journalEntry)

		_ = uc.promoteWatchJournalEntry(origin, now, signalID, promotionReason)
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
		"recheck_expired", summary.Expired,
		"recheck_invalidated", summary.Invalidated,
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
) watchRecheckEvaluation {
	fullData, err := uc.marketDataUsecase.FetchMarketData(ctx, entry.Symbol, fundingRates)
	if err != nil {
		slog.Warn("Watch recheck market data fetch failed", "symbol", entry.Symbol, "error", err)
		GetGlobalMetrics().IncrementMarketDataError()
		return watchRecheckEvaluation{
			origin: entry,
			disposition: watchRecheckDisposition{
				Eligible: false,
				Reason:   "Watch recheck skipped: transient market data fetch error",
			},
		}
	}
	if !uc.stalenessUsecase.IsFresh(fullData.M15Candles) {
		return watchRecheckEvaluation{
			origin: entry,
			disposition: watchRecheckDisposition{
				Eligible: false,
				Reason:   "Watch recheck skipped: latest M15 context is stale",
			},
		}
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
		return watchRecheckEvaluation{
			origin: entry,
			disposition: watchRecheckDisposition{
				Eligible: false,
				Reason:   "Watch recheck skipped: prepared context is not yet reusable",
			},
		}
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
		return watchRecheckEvaluation{
			origin: entry,
			disposition: watchRecheckDisposition{
				Eligible: false,
				Reason:   "Watch recheck skipped: playbook eligibility not yet satisfied",
			},
		}
	}

	quantResult := uc.playbookQuantEngineUsecase.RunEngineWithPreparedContext(entry.Playbook, entry.Direction, fullData, policy, prepared)
	quantResult.Tier = entry.Tier
	quantResult.RawKlines = fullData.M15Candles
	if !isArbiterReadyQuantResult(quantResult) {
		return watchRecheckEvaluation{
			origin: entry,
			disposition: watchRecheckDisposition{
				Eligible: false,
				Reason:   "Watch recheck skipped: quant path is not yet tradable",
			},
		}
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
		disposition: watchRecheckDisposition{
			Eligible: true,
			Reason:   "Watch recheck candidate re-evaluated successfully",
		},
	}
}

func selectWatchRecheckCandidates(journal []WatchJournal, now time.Time) ([]watchRecheckCandidate, []watchRecheckEvaluation) {
	if len(journal) == 0 {
		return nil, nil
	}

	recheckPolicy := resolveWatchRecheckPolicy()

	bestBySymbol := make(map[string]WatchJournal)
	terminalUpdates := make([]watchRecheckEvaluation, 0)
	for _, entry := range journal {
		disposition := classifyWatchRecheckDisposition(entry, now, recheckPolicy)
		if disposition.Terminal {
			terminalUpdates = append(terminalUpdates, watchRecheckEvaluation{
				origin:      entry,
				disposition: disposition,
			})
			continue
		}
		if !disposition.Eligible {
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

	if len(shortlist) > recheckPolicy.BatchLimit {
		shortlist = shortlist[:recheckPolicy.BatchLimit]
	}
	return shortlist, terminalUpdates
}

func classifyWatchRecheckDisposition(entry WatchJournal, now time.Time, policy watchRecheckPolicy) watchRecheckDisposition {
	if entry.Status != WATCH_MONITORING {
		return watchRecheckDisposition{Eligible: false, Reason: "Watch recheck skipped: watch is no longer in WATCH_MONITORING"}
	}
	if !entry.ClosedAt.IsZero() {
		return watchRecheckDisposition{Eligible: false, Reason: "Watch recheck skipped: watch is already closed"}
	}
	if !isWatchRecheckAllowedPlaybook(entry.Playbook, policy) {
		return watchRecheckDisposition{
			Eligible:       false,
			Terminal:       true,
			TerminalStatus: WATCH_RECHECK_INVALIDATED,
			Reason:         fmt.Sprintf("Watch recheck invalidated: playbook %s is not allowlisted for recheck", entry.Playbook),
			OutcomeReason:  "Recheck invalidated because the playbook is no longer eligible for secondary recheck",
		}
	}
	if entry.CreatedAt.IsZero() {
		return watchRecheckDisposition{
			Eligible:       false,
			Terminal:       true,
			TerminalStatus: WATCH_RECHECK_INVALIDATED,
			Reason:         "Watch recheck invalidated: missing created_at timestamp",
			OutcomeReason:  "Recheck invalidated because watch metadata is incomplete",
		}
	}
	if !entry.ExpiresAt.IsZero() && now.After(entry.ExpiresAt) {
		return watchRecheckDisposition{
			Eligible:       false,
			Terminal:       true,
			TerminalStatus: WATCH_RECHECK_EXPIRED,
			Reason:         "Watch recheck expired: watch horizon elapsed before promotion",
			OutcomeReason:  "Recheck expired because watch horizon elapsed before a valid promotion",
		}
	}
	if now.Sub(entry.CreatedAt) > policy.EffectiveHorizon {
		return watchRecheckDisposition{
			Eligible:       false,
			Terminal:       true,
			TerminalStatus: WATCH_RECHECK_EXPIRED,
			Reason:         "Watch recheck expired: secondary recheck max-age elapsed",
			OutcomeReason:  "Recheck expired because the secondary recheck window elapsed",
		}
	}

	reasonBlob := strings.ToUpper(strings.TrimSpace(entry.Reason + " " + entry.AIReasoning))
	if reasonBlob == "" {
		return watchRecheckDisposition{
			Eligible:       false,
			Terminal:       true,
			TerminalStatus: WATCH_RECHECK_INVALIDATED,
			Reason:         "Watch recheck invalidated: watch has no reusable reason context",
			OutcomeReason:  "Recheck invalidated because the watch no longer has reusable decision context",
		}
	}
	for _, blocked := range policy.BlockedReasonTokens {
		if strings.Contains(reasonBlob, blocked) {
			return watchRecheckDisposition{
				Eligible: false,
				Reason:   fmt.Sprintf("Watch recheck skipped: blocked by reason token %s", blocked),
			}
		}
	}
	if isSafeCompressionRecheck(reasonBlob, entry.Playbook) {
		return watchRecheckDisposition{
			Eligible: true,
			Reason:   "Watch recheck allowed: safe compression retest path",
		}
	}
	for _, allowed := range policy.AllowedReasonTokens {
		if strings.Contains(reasonBlob, allowed) {
			return watchRecheckDisposition{
				Eligible: true,
				Reason:   fmt.Sprintf("Watch recheck allowed: matched reason token %s", allowed),
			}
		}
	}
	return watchRecheckDisposition{
		Eligible: false,
		Reason:   "Watch recheck skipped: no allowlisted recheck reason matched",
	}
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

func (uc *ScannerUsecase) promoteWatchJournalEntry(origin WatchJournal, now time.Time, signalID string, promotionReason string) error {
	return uc.storageUsecase.UpdateWatchJournal(func(current []WatchJournal) ([]WatchJournal, error) {
		updated := append([]WatchJournal(nil), current...)
		for i := range updated {
			if updated[i].ID != origin.ID {
				continue
			}
			updated[i].Status = WATCH_PROMOTED
			updated[i].ClosedAt = now
			updated[i].UpdatedAt = now
			updated[i].OutcomeReason = buildWatchRecheckPromotionOutcome(signalID)
			updated[i].Reason = promotionReason
			return updated, nil
		}
		return updated, nil
	})
}

func (uc *ScannerUsecase) applyWatchRecheckTerminalUpdates(updates []watchRecheckEvaluation, now time.Time) error {
	if len(updates) == 0 {
		return nil
	}
	updateMap := make(map[string]watchRecheckDisposition, len(updates))
	for _, update := range updates {
		if !update.disposition.Terminal || update.origin.ID == "" {
			continue
		}
		updateMap[update.origin.ID] = update.disposition
	}
	if len(updateMap) == 0 {
		return nil
	}

	return uc.storageUsecase.UpdateWatchJournal(func(current []WatchJournal) ([]WatchJournal, error) {
		updated := append([]WatchJournal(nil), current...)
		for i := range updated {
			disposition, ok := updateMap[updated[i].ID]
			if !ok {
				continue
			}
			updated[i].Status = disposition.TerminalStatus
			updated[i].ClosedAt = now
			updated[i].UpdatedAt = now
			updated[i].Reason = disposition.Reason
			updated[i].OutcomeReason = disposition.OutcomeReason
		}
		return updated, nil
	})
}
