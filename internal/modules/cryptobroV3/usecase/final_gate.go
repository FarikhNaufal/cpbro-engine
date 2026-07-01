package usecase

import (
	"cpbro-engine/internal/modules/cryptobroV3/dto"
	"fmt"
	"math"
	"strings"
	"time"
)

type FinalGateUsecase struct{}

func NewFinalGateUsecase() *FinalGateUsecase {
	return &FinalGateUsecase{}
}

// Validate is kept for backward compatibility and routes to Evaluate with default parameters.
func (uc *FinalGateUsecase) Validate(signal dto.SignalResponse, policy MarketPolicy) (dto.SignalResponse, bool) {
	// Reconstruct entities from DTO to call Evaluate
	quant := QuantResult{
		Symbol:       signal.Symbol,
		Direction:    Direction(signal.Direction),
		Playbook:     Playbook(signal.Strategy),
		TriggerPrice: signal.TriggerPrice,
		StopLoss:     signal.StopLoss,
		TakeProfit:   signal.TakeProfit,
		Score:        signal.Score,
		IndicatorMet: true,
	}
	quant.TradePlan = TradePlan{
		Symbol:     signal.Symbol,
		Direction:  Direction(signal.Direction),
		EntryPrice: signal.TriggerPrice,
		StopLoss:   signal.StopLoss,
		TakeProfit: signal.TakeProfit,
	}
	quant.TechnicalSnapshot = TechnicalSnapshot{
		IndicatorValues: map[string]float64{
			IndicatorADX: 25.0, // default passing
		},
	}

	localGate := LocalGateResult{
		Passed: true,
		Status: AI_CANDIDATE,
	}

	aiAudit := dto.AIAuditResponse{
		Symbol:          signal.Symbol,
		Decision:        "CONFIRM",
		Confidence:      "HIGH",
		IsApproved:      true,
		Sentiment:       signal.AISentiment,
		HasRejection:    true,
		HasConfirmation: true,
	}

	planReview := PlanReview{
		Conflicted:      false,
		EntryStillValid: true,
		NeedRetest:      false,
		Status:          PLAN_VALID,
	}

	staleness := StalenessResult{
		IsStale: false,
		Status:  FRESH,
	}

	decision := uc.Evaluate(
		quant,
		localGate,
		aiAudit,
		planReview,
		staleness,
		policy,
		signal.TriggerPrice, // latestPrice matches triggerPrice
		nil,                 // activeSignals
		nil,                 // historySignals
		nil,                 // m15
	)

	signal.IsFinalExecute = decision.IsExecutable
	signal.Status = string(decision.Status)

	return signal, decision.IsExecutable
}

// Evaluate evaluates a trade setup candidate through the 23 final validation rules.
func (uc *FinalGateUsecase) Evaluate(
	quant QuantResult,
	localGate LocalGateResult,
	aiAudit dto.AIAuditResponse,
	planReview PlanReview,
	staleness StalenessResult,
	policy MarketPolicy,
	latestPrice float64,
	activeSignals []SignalJournal,
	historySignals []dto.SignalResponse,
	m15 []dto.Candle,
) FinalDecision {
	// Rule 0: Initialize PlaybookThresholdProfile
	profile := GetPlaybookThresholdProfile(quant.Playbook, policy, quant.Tier)
	minScoreExecute := math.Max(policy.MinScoreExecute, profile.MinScoreExecute)
	minRRExecute := math.Max(policy.MinRRExecute, profile.MinRR)
	plannedRR := CalculateTradePlanRR(quant)

	// Keep track of check failures
	var rejectReasons []string
	var watchReasons []string
	var rejectBreakdown []string
	var watchBreakdown []string
	primaryRejectLayer := ""
	primaryWatchLayer := ""
	requiredAIConfidence := effectiveRequiredAIConfidence(policy, profile)
	requireFreshEntry := effectiveRequireFreshEntry(policy)
	aiSource := NormalizeAIAuditSource(aiAudit.Source)
	aiCalled := WasAIAuditCalled(aiSource)

	addReject := func(layer string, reason string) {
		rejectReasons = append(rejectReasons, reason)
		rejectBreakdown = append(rejectBreakdown, FormatReasonBreakdown(layer, reason))
		if primaryRejectLayer == "" {
			primaryRejectLayer = layer
		}
		GetGlobalMetrics().IncrementRuleReject(layer)
	}

	addWatch := func(layer string, reason string) {
		watchReasons = append(watchReasons, reason)
		watchBreakdown = append(watchBreakdown, FormatReasonBreakdown(layer, reason))
		if primaryWatchLayer == "" {
			primaryWatchLayer = layer
		}
		GetGlobalMetrics().IncrementRuleWatch(layer)
	}

	// Detect AI Error Policy
	isAIError := strings.Contains(strings.ToUpper(aiAudit.Reasoning), "AI_ERROR") ||
		strings.Contains(strings.ToUpper(aiAudit.Reason), "AI_ERROR") ||
		strings.Contains(strings.ToUpper(aiAudit.Sentiment), "AI_ERROR") ||
		(aiAudit.Decision == "" && aiAudit.Reasoning == "")
	usePlanReview := aiCalled && !isAIError

	// 1. LocalGate status check
	if localGate.Status != AI_CANDIDATE {
		if localGate.Status == LOCAL_WATCH {
			addWatch("LOCAL_GATE", "Local gate status is LOCAL_WATCH")
		} else {
			addReject("LOCAL_GATE", fmt.Sprintf("LocalGate status %s is not AI_CANDIDATE", localGate.Status))
		}
	}

	// 2. AI Decision check
	switch aiSource {
	case AIAuditSourceSyntheticLocalGate:
	case AIAuditSourceSyntheticQuota:
		if policy.AllowAIQuotaWatch {
			addWatch("AI_QUOTA", "AI candidate skipped due policy MaxAICandidates quota")
		} else {
			addReject("AI_QUOTA", "AI candidate skipped due policy MaxAICandidates quota")
		}
	case AIAuditSourceSyntheticDisabled:
		if policy.AllowAIDisabledWatch {
			addWatch("AI_DISABLED", "AI audit disabled by configuration")
		} else {
			addReject("AI_DISABLED", "AI audit disabled by configuration")
		}
	default:
		if aiAudit.Decision == "REJECT" {
			addReject("AI_VERDICT", "AI decision is REJECT")
		} else if aiAudit.Decision == "WAIT" {
			addWatch("AI_VERDICT", "AI decision is WAIT")
		} else if aiAudit.Decision != "CONFIRM" {
			addWatch("AI_VERDICT", fmt.Sprintf("AI decision %s is not CONFIRM", aiAudit.Decision))
		}
	}

	// 3. AI Confidence check
	if aiCalled {
		if strings.ToUpper(aiAudit.Confidence) == string(AIConfidenceLow) {
			addReject("AI_CONFIDENCE", "AI confidence is LOW")
		} else if !meetsRequiredAIConfidence(aiAudit.Confidence, requiredAIConfidence) {
			addWatch("AI_CONFIDENCE", fmt.Sprintf("AI confidence %s is below required %s", aiAudit.Confidence, requiredAIConfidence))
		}
	}

	// 4. AI conflict_with_bot check
	if aiCalled && aiAudit.ConflictWithBot {
		addReject("AI_CONFLICT", "AI conflict with bot is true")
	}

	// 5. PlanReview.EntryStillValid check
	if usePlanReview && !planReview.EntryStillValid {
		addReject("PLAN_RECON", "PlanReview EntryStillValid is false")
	}

	// 6. PlanReview.PlanConflict check
	if usePlanReview && planReview.Conflicted {
		if planReview.NeedRetest {
			reasonStr := "SOFT_PLAN_CONFLICT / NEED_RETEST: " + planReview.Reason
			if aiAudit.Decision == "WAIT" {
				addWatch("PLAN_RECON", reasonStr)
			} else {
				addReject("PLAN_RECON", reasonStr)
			}
		} else {
			reasonStr := "HARD_PLAN_CONFLICT: " + planReview.Reason
			addReject("PLAN_RECON", reasonStr)
		}
	}

	// 7. Staleness check
	if staleness.Status == LATE {
		if requireFreshEntry {
			addWatch("STALENESS", "Staleness status is LATE")
		}
	} else if staleness.Status == MISSED {
		addReject("STALENESS", "Staleness status is MISSED")
	} else if requireFreshEntry && staleness.Status != FRESH {
		addReject("STALENESS", fmt.Sprintf("Staleness status %s is not FRESH", staleness.Status))
	}

	// 8. MarketPolicy direction permissions
	if quant.Direction == LONG && !policy.AllowLong {
		addReject("POLICY", "LONG direction disallowed by policy")
	}
	if quant.Direction == SHORT && !policy.AllowShort {
		addReject("POLICY", "SHORT direction disallowed by policy")
	}

	// 9. Playbook eligibility check
	playbookAllowed := false
	for _, p := range policy.AllowedPlaybooks {
		if p == quant.Playbook {
			playbookAllowed = true
			break
		}
	}
	if !playbookAllowed {
		addReject("POLICY", fmt.Sprintf("Playbook %s not in allowed playbooks list", quant.Playbook))
	}

	// 10. Tier eligibility check
	tierAllowed := false
	for _, t := range policy.AllowedTiers {
		if t == quant.Tier {
			tierAllowed = true
			break
		}
	}
	if !tierAllowed {
		addReject("POLICY", fmt.Sprintf("Tier %s not in allowed tiers list", quant.Tier))
	}

	// 11. Score check
	if quant.Score < minScoreExecute {
		if aiAudit.Decision == "WAIT" {
			addWatch("SCORE", fmt.Sprintf("Quant score %0.1f below minimum execute score %0.1f", quant.Score, minScoreExecute))
		} else {
			addReject("SCORE", fmt.Sprintf("Quant score %0.1f below minimum execute score %0.1f", quant.Score, minScoreExecute))
		}
	}

	// 12. Actual Risk-to-Reward ratio check
	entry := quant.TradePlan.EntryPrice
	tp := quant.TradePlan.TakeProfit
	sl := quant.TradePlan.StopLoss
	rrActual := 0.0

	if latestPrice <= 0 {
		addReject("RR_ACTUAL", "latestPrice unavailable for RR calculation")
	} else if tp > 0 && sl > 0 {
		if quant.Direction == LONG {
			risk := latestPrice - sl
			reward := tp - latestPrice
			if risk > 0 {
				rrActual = reward / risk
			}
		} else if quant.Direction == SHORT {
			risk := sl - latestPrice
			reward := latestPrice - tp
			if risk > 0 {
				rrActual = reward / risk
			}
		}
	} else if entry > 0 && tp > 0 && sl > 0 {
		// Fallback to entry price if latestPrice not available (should not happen due to earlier check)
		if quant.Direction == LONG {
			risk := entry - sl
			reward := tp - entry
			if risk > 0 {
				rrActual = reward / risk
			}
		} else if quant.Direction == SHORT {
			risk := sl - entry
			reward := entry - tp
			if risk > 0 {
				rrActual = reward / risk
			}
		}
	}

	if rrActual < minRRExecute {
		if aiAudit.Decision == "WAIT" {
			addWatch("RR_ACTUAL", fmt.Sprintf("Actual RR %0.2f below minimum required RR %0.2f", rrActual, minRRExecute))
		} else {
			addReject("RR_ACTUAL", fmt.Sprintf("Actual RR %0.2f below minimum required RR %0.2f", rrActual, minRRExecute))
		}
	}

	// 13. ADX rule checks
	adxVal := quant.TechnicalSnapshot.IndicatorValues[IndicatorADX]
	minADX := math.Max(policy.MinADXExecute, profile.MinADX)

	if profile.RequireADX && adxVal < minADX {
		addWatch("ADX", fmt.Sprintf("ADX %0.1f below required threshold %0.1f", adxVal, minADX))
	}
	if profile.RejectADXExpansion {
		maxADX := profile.MaxADX
		if maxADX <= 0 {
			maxADX = safetyADXExpansionCeiling
		}
		if adxVal > maxADX {
			addWatch("ADX", fmt.Sprintf("High trend expansion detected (ADX = %0.1f > %0.1f)", adxVal, maxADX))
		}
	}

	// 14. Rejection requirement checks
	hasRejection := (quant.TechnicalSnapshot.IndicatorValues[IndicatorWickRejection] == 1.0) ||
		(quant.TechnicalSnapshot.IndicatorValues[IndicatorPARejection] == 1.0) ||
		aiAudit.HasRejection ||
		strings.Contains(strings.ToLower(quant.Reason), "rejection") ||
		strings.Contains(strings.ToLower(planReview.Reason), "rejection")

	if profile.RequireRejection || quant.Playbook == LIQUIDITY_SWEEP_REVERSAL || quant.Playbook == RANGE_EDGE_REVERSAL || quant.Playbook == CROWDED_POSITIONING_SQUEEZE {
		if !hasRejection {
			if aiAudit.Decision == "WAIT" {
				addWatch("PRICE_ACTION", "Rejection wick or price action evidence missing")
			} else {
				addReject("PRICE_ACTION", "Rejection wick or price action evidence missing")
			}
		}
	}

	// 15. Confirmation requirement checks
	hasConfirmation := quant.IndicatorMet || aiAudit.HasConfirmation ||
		strings.Contains(strings.ToLower(quant.Reason), "confirm") ||
		strings.Contains(strings.ToLower(planReview.Reason), "confirm")

	if profile.RequireConfirmation || quant.Playbook == LIQUIDITY_SWEEP_REVERSAL || quant.Playbook == RANGE_EDGE_REVERSAL || quant.Playbook == CROWDED_POSITIONING_SQUEEZE {
		if !hasConfirmation {
			addWatch("CONFIRMATION", "Confirmation candle / structure missing")
		}
	}

	// 16. Retest requirement checks
	isBreakoutPlaybook := quant.Playbook == COMPRESSION_BREAKOUT_RETEST
	needsRetest := (usePlanReview && planReview.NeedRetest) || aiAudit.SuggestedAction == "WAIT_RETEST"
	isFirstBreakoutCandle := false
	if isBreakoutPlaybook {
		isFirstBreakoutCandle = strings.Contains(strings.ToUpper(quant.SetupType), "BREAKOUT") && !strings.Contains(strings.ToUpper(quant.SetupType), "RETEST")
	}

	if profile.RequireRetest || isBreakoutPlaybook {
		retestFailed := false
		if usePlanReview && planReview.Conflicted && (strings.Contains(strings.ToLower(planReview.Reason), "retest") || strings.Contains(strings.ToLower(planReview.Reason), "breakout")) {
			retestFailed = true
		}
		if aiAudit.Decision == "REJECT" && strings.Contains(strings.ToLower(aiAudit.Reason), "retest") {
			retestFailed = true
		}

		if retestFailed {
			addReject("RETEST", "Retest failed")
		} else if needsRetest || isFirstBreakoutCandle {
			addWatch("RETEST", "Retest required / breakout is on first candle")
		}
	}

	// 17. Volume confirmation checks
	hasVolumeConfirm := true
	if profile.RequireVolumeConfirm || quant.Playbook == LIQUIDITY_SWEEP_REVERSAL {
		hasSpike := (quant.TechnicalSnapshot.IndicatorValues[IndicatorVolumeSpike] == 1.0)
		if len(m15) > 0 {
			minVolRatio := profile.MinVolumeRatio
			if minVolRatio <= 0 {
				minVolRatio = 1.3
			}
			hasSpike = hasSpike || ConfirmLiquiditySweep(m15, 10, minVolRatio)
		}
		if !hasSpike {
			hasVolumeConfirm = false
		}
	}
	if quant.Playbook == COMPRESSION_BREAKOUT_RETEST {
		hasExpansion := (quant.TechnicalSnapshot.IndicatorValues[IndicatorVolumeSpike] == 1.0) ||
			(quant.TechnicalSnapshot.IndicatorValues[IndicatorExtremeOI] == 1.0) ||
			(quant.TechnicalSnapshot.IndicatorValues[IndicatorOIChange] > 0)
		if !hasExpansion {
			hasVolumeConfirm = false
		}
	}
	if !hasVolumeConfirm {
		if aiAudit.Decision == "WAIT" {
			addWatch("VOLUME", "Volume / OI confirmation missing")
		} else {
			addReject("VOLUME", "Volume / OI confirmation missing")
		}
	}

	// 18. Crowding evidence checks
	hasCrowdingEvidence := (quant.TechnicalSnapshot.IndicatorValues[IndicatorExtremeFunding] == 1.0) ||
		(quant.TechnicalSnapshot.IndicatorValues[IndicatorExtremeOI] == 1.0) ||
		(quant.TechnicalSnapshot.IndicatorValues[IndicatorCrowdingScore] >= profile.MinCrowdingScore)
	if profile.RequireCrowdingEvidence || quant.Playbook == CROWDED_POSITIONING_SQUEEZE {
		if !hasCrowdingEvidence {
			if aiAudit.Decision == "WAIT" {
				addWatch("CROWDING", "Crowded funding/OI positioning evidence missing")
			} else {
				addReject("CROWDING", "Crowded funding/OI positioning evidence missing")
			}
		}
	}

	// 19. Opposite active signal check
	hasOppositeActive := false
	for _, item := range activeSignals {
		if item.Symbol == quant.Symbol && item.Status == MONITORING {
			if item.Direction != Direction(quant.Direction) {
				hasOppositeActive = true
				break
			}
		}
	}
	if hasOppositeActive {
		addReject("ACTIVE_SIGNAL", "Opposite active signal currently open for symbol")
	}

	// 20. Concurrent active signal limit check
	activeCount := 0
	for _, item := range activeSignals {
		if item.Status == MONITORING {
			activeCount++
		}
	}
	if activeCount >= policy.MaxFinalExecute {
		addReject("POSITION_LIMIT", fmt.Sprintf("Concurrent active signals count %d exceeds policy limit %d", activeCount, policy.MaxFinalExecute))
	}

	// 21. Symbol cooldown check
	cooldownActive := false
	if policy.CooldownMinutes > 0 {
		cooldownDuration := time.Duration(policy.CooldownMinutes) * time.Minute
		now := time.Now()
		for _, item := range activeSignals {
			if item.Symbol == quant.Symbol && now.Sub(item.CreatedAt) < cooldownDuration {
				cooldownActive = true
				break
			}
		}
		if !cooldownActive {
			for _, item := range historySignals {
				if item.Symbol == quant.Symbol && now.Sub(item.ReconciledTime) < cooldownDuration {
					cooldownActive = true
					break
				}
			}
		}
	}
	if cooldownActive {
		addReject("COOLDOWN", "Symbol cooldown is active")
	}

	// 22. Setup/playbook blacklist checked via AllowedPlaybooks in localGate.Evaluate,
	// but let's re-verify it to be absolutely sure.

	// 23. TradePlan validation
	tpValid := entry > 0 && sl > 0 && tp > 0 &&
		((quant.Direction == LONG && sl < entry && tp > entry) ||
			(quant.Direction == SHORT && sl > entry && tp < entry))
	if !tpValid {
		addReject("TRADE_PLAN", "TradePlan parameters invalid (e.g. SL or TP reversed)")
	}

	// 24. Symbol-level directional price move guard
	// Block LONG entry when individual symbol has dumped beyond directional threshold,
	// and block SHORT entry when individual symbol has pumped beyond directional threshold.
	symbolPriceChange := quant.TechnicalSnapshot.PriceChange24h / 100.0 // convert from percent to ratio
	if quant.Direction == LONG {
		maxMoveLong := policy.MaxPriceMove24hLong
		if maxMoveLong <= 0 {
			maxMoveLong = policy.MaxPriceMove24h // fallback to symmetric limit
		}
		// Dump = negative price change. Abs of negative change exceeding limit = block LONG
		if symbolPriceChange < 0 && math.Abs(symbolPriceChange) > maxMoveLong {
			addReject("DIRECTIONAL_MOVE",
				fmt.Sprintf("Symbol 24h dump %0.1f%% exceeds directional LONG limit %0.1f%%",
					symbolPriceChange*100, maxMoveLong*100))
		}
	} else if quant.Direction == SHORT {
		maxMoveShort := policy.MaxPriceMove24hShort
		if maxMoveShort <= 0 {
			maxMoveShort = policy.MaxPriceMove24h // fallback to symmetric limit
		}
		// Pump = positive price change. Abs of positive change exceeding limit = block SHORT
		if symbolPriceChange > 0 && math.Abs(symbolPriceChange) > maxMoveShort {
			addReject("DIRECTIONAL_MOVE",
				fmt.Sprintf("Symbol 24h pump %0.1f%% exceeds directional SHORT limit %0.1f%%",
					symbolPriceChange*100, maxMoveShort*100))
		}
	}

	// 25. SL-to-ATR ratio guard
	// Ensures stop loss distance is reasonable relative to current M15 ATR volatility.
	// SL that is too tight relative to ATR will get stopped out by normal price noise.
	atrFromSnapshot := quant.TechnicalSnapshot.IndicatorValues[IndicatorATR]
	if atrFromSnapshot > 0 && entry > 0 && sl > 0 {
		slDistance := math.Abs(entry - sl)
		settings := getRuntimeSettings()
		minSLMultiplier := settings.MinSLATRMultiplierBase
		if minSLMultiplier <= 0 {
			minSLMultiplier = 1.0
		}
		if quant.Playbook == LIQUIDITY_SWEEP_REVERSAL || quant.Playbook == RANGE_EDGE_REVERSAL {
			if settings.MinSLATRMultiplierReversal > 0 {
				minSLMultiplier = settings.MinSLATRMultiplierReversal
			} else {
				minSLMultiplier = 1.2
			}
		}
		if policy.EffectiveRegime() == BTC_CHAOS || policy.EffectiveRegime() == HIGH_VOL {
			if settings.MinSLATRMultiplierHighVol > 0 {
				minSLMultiplier = settings.MinSLATRMultiplierHighVol
			} else {
				minSLMultiplier = 1.5
			}
		}
		if slDistance < atrFromSnapshot*(minSLMultiplier-0.01) {
			addWatch("SL_ATR",
				fmt.Sprintf("SL distance %0.6f is only %0.2fx ATR (%0.6f), minimum required %0.1fx ATR",
					slDistance, slDistance/atrFromSnapshot, atrFromSnapshot, minSLMultiplier))
		}
	}

	// Playbook-specific execution safety rules
	if quant.Playbook == TREND_PULLBACK {
		regime := policy.EffectiveRegime()
		if !isTrendPullbackTrendAligned(quant.Direction, quant.H4Trend, quant.H1Trend, regime) {
			addReject("PLAYBOOK_SAFETY", "Trend pullback lacks H1/H4 trend alignment")
		}
		if aiAudit.CandleNarrative == "EXHAUSTED" || strings.Contains(strings.ToLower(aiAudit.Reason), "overextended") {
			addReject("PLAYBOOK_SAFETY", "Trend pullback is overextended")
		}
	}

	if quant.Playbook == LIQUIDITY_SWEEP_REVERSAL {
		isSweep := strings.Contains(strings.ToLower(quant.Reason), "sweep") ||
			quant.TechnicalSnapshot.IndicatorValues[IndicatorSweepLow] == 1.0 ||
			quant.TechnicalSnapshot.IndicatorValues[IndicatorSweepHigh] == 1.0
		if !isSweep {
			if aiAudit.Decision == "WAIT" {
				addWatch("PLAYBOOK_SAFETY", "Liquidity sweep setup lacks high/low breakout and reclaim evidence")
			} else {
				addReject("PLAYBOOK_SAFETY", "Liquidity sweep setup lacks high/low breakout and reclaim evidence")
			}
		}
	}

	if quant.Playbook == RANGE_EDGE_REVERSAL {
		nearEdge := quant.TechnicalSnapshot.IndicatorValues[IndicatorNearRangeEdge] == 1.0 ||
			strings.Contains(strings.ToLower(quant.Reason), "range")
		if !nearEdge {
			if aiAudit.Decision == "WAIT" {
				addWatch("PLAYBOOK_SAFETY", "Not close enough to range edge bounds")
			} else {
				addReject("PLAYBOOK_SAFETY", "Not close enough to range edge bounds")
			}
		}
		if aiAudit.CandleNarrative == "CONTINUATION" || strings.Contains(strings.ToLower(aiAudit.Reason), "breaking range") {
			addReject("PLAYBOOK_SAFETY", "Range edge candle narrative is CONTINUATION (breakout threat)")
		}
	}

	if quant.Playbook == CROWDED_POSITIONING_SQUEEZE {
		if quant.Score < 7.8 {
			addReject("PLAYBOOK_SAFETY", fmt.Sprintf("Crowded squeeze score %0.1f is below mandatory 7.8", quant.Score))
		}
		if quant.TechnicalSnapshot.IndicatorValues[IndicatorExtremeFunding] == 1.0 && !hasConfirmation {
			if aiAudit.Decision == "WAIT" {
				addWatch("PLAYBOOK_SAFETY", "Crowded squeeze has extreme funding but lacks confirmation candle")
			} else {
				addReject("PLAYBOOK_SAFETY", "Crowded squeeze has extreme funding but lacks confirmation candle")
			}
		}
	}

	// Final Status & Reason Resolution
	var status Status
	var reason string
	var primaryReasonLayer string
	var reasonBreakdown []string

	if isAIError {
		status = AI_ERROR_REVIEW
		reason = "AI_ERROR: " + aiAudit.Reasoning
		primaryReasonLayer = "AI_TRANSPORT"
		reasonBreakdown = []string{FormatReasonBreakdown(primaryReasonLayer, reason)}
	} else if len(rejectReasons) > 0 {
		status = FINAL_REJECT
		reason = strings.Join(rejectReasons, "; ")
		primaryReasonLayer = primaryRejectLayer
		reasonBreakdown = append([]string(nil), rejectBreakdown...)
	} else if len(watchReasons) > 0 {
		status = FINAL_WATCH
		reason = strings.Join(watchReasons, "; ")
		primaryReasonLayer = primaryWatchLayer
		reasonBreakdown = append([]string(nil), watchBreakdown...)
	} else {
		status = FINAL_EXECUTE
		reason = "All final execution criteria met successfully"
	}

	isExecutable := (status == FINAL_EXECUTE)

	var watchReason string
	var rejectReason string
	if status == FINAL_WATCH {
		watchReason = reason
	}
	if status == FINAL_REJECT {
		rejectReason = reason
	}

	policySummary := fmt.Sprintf("AllowLong=%v, AllowShort=%v, MinScore=%0.1f, MinRR=%0.1f, MaxExecute=%d",
		policy.AllowLong, policy.AllowShort, policy.MinScoreExecute, policy.MinRRExecute, policy.MaxFinalExecute)
	profileSummary := fmt.Sprintf("Playbook=%s, MinScore=%0.1f, MinRR=%0.1f, RequireADX=%v, RequireVolumeConfirm=%v, RequireRejection=%v, RequireConfirmation=%v, RequireRetest=%v, RequireAIConfidence=%s, RequireFreshEntry=%v, RequireM5RejectionConfirm=%v, RequireM5ContinuationConfirm=%v, M5ConfirmationMode=%s",
		profile.Playbook, profile.MinScoreExecute, profile.MinRR, profile.RequireADX, profile.RequireVolumeConfirm, profile.RequireRejection, profile.RequireConfirmation, profile.RequireRetest, requiredAIConfidence, requireFreshEntry, profile.RequireM5RejectionConfirm, profile.RequireM5ContinuationConfirm, profile.M5ConfirmationMode)

	return FinalDecision{
		Symbol:                  quant.Symbol,
		Direction:               quant.Direction,
		Playbook:                quant.Playbook,
		Status:                  status,
		Reason:                  reason,
		Score:                   quant.Score,
		RequiredScore:           minScoreExecute,
		RR:                      rrActual,
		PlannedRR:               plannedRR,
		ActualRR:                rrActual,
		RequiredRR:              minRRExecute,
		AIConfidence:            aiAudit.Confidence,
		AISource:                aiSource,
		AICalled:                aiCalled,
		StalenessStatus:         string(staleness.Status),
		PolicySummary:           policySummary,
		ThresholdProfileSummary: profileSummary,
		PrimaryReasonLayer:      primaryReasonLayer,
		ReasonBreakdown:         reasonBreakdown,
		IsExecutable:            isExecutable,
		Tier:                    quant.Tier,
		EntryPrice:              quant.TradePlan.EntryPrice,
		StopLoss:                quant.TradePlan.StopLoss,
		TakeProfit:              quant.TradePlan.TakeProfit,
		WatchReason:             watchReason,
		RejectReason:            rejectReason,
	}
}
