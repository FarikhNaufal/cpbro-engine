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

	// Keep track of check failures
	var rejectReasons []string
	var watchReasons []string

	// Detect AI Error Policy
	isAIError := strings.Contains(strings.ToUpper(aiAudit.Reasoning), "AI_ERROR") ||
		strings.Contains(strings.ToUpper(aiAudit.Reason), "AI_ERROR") ||
		strings.Contains(strings.ToUpper(aiAudit.Sentiment), "AI_ERROR") ||
		(aiAudit.Decision == "" && aiAudit.Reasoning == "")

	// 1. LocalGate status check
	if localGate.Status != AI_CANDIDATE {
		if localGate.Status == LOCAL_WATCH {
			watchReasons = append(watchReasons, "Local gate status is LOCAL_WATCH")
		} else {
			rejectReasons = append(rejectReasons, fmt.Sprintf("LocalGate status %s is not AI_CANDIDATE", localGate.Status))
		}
	}

	// 2. AI Decision check
	if aiAudit.Decision == "REJECT" {
		rejectReasons = append(rejectReasons, "AI decision is REJECT")
	} else if aiAudit.Decision == "WAIT" {
		watchReasons = append(watchReasons, "AI decision is WAIT")
	} else if aiAudit.Decision != "CONFIRM" {
		watchReasons = append(watchReasons, fmt.Sprintf("AI decision %s is not CONFIRM", aiAudit.Decision))
	}

	// 3. AI Confidence check
	if aiAudit.Confidence == "LOW" {
		rejectReasons = append(rejectReasons, "AI confidence is LOW")
	} else if aiAudit.Confidence == "MEDIUM" {
		watchReasons = append(watchReasons, "AI confidence is MEDIUM")
	} else if aiAudit.Confidence != "HIGH" {
		// Untuk MVP, semua FINAL_EXECUTE tetap wajib AI HIGH
		watchReasons = append(watchReasons, fmt.Sprintf("AI confidence %s is not HIGH", aiAudit.Confidence))
	}

	// 4. AI conflict_with_bot check
	if aiAudit.ConflictWithBot {
		rejectReasons = append(rejectReasons, "AI conflict with bot is true")
	}

	// 5. PlanReview.EntryStillValid check
	if !planReview.EntryStillValid {
		rejectReasons = append(rejectReasons, "PlanReview EntryStillValid is false")
	}

	// 6. PlanReview.PlanConflict check
	if planReview.Conflicted {
		if planReview.NeedRetest {
			reasonStr := "SOFT_PLAN_CONFLICT / NEED_RETEST: " + planReview.Reason
			if aiAudit.Decision == "WAIT" {
				watchReasons = append(watchReasons, reasonStr)
			} else {
				rejectReasons = append(rejectReasons, reasonStr)
			}
		} else {
			reasonStr := "HARD_PLAN_CONFLICT: " + planReview.Reason
			rejectReasons = append(rejectReasons, reasonStr)
		}
	}

	// 7. Staleness check
	if staleness.Status == LATE {
		watchReasons = append(watchReasons, "Staleness status is LATE")
	} else if staleness.Status == MISSED {
		rejectReasons = append(rejectReasons, "Staleness status is MISSED")
	} else if staleness.Status != FRESH {
		rejectReasons = append(rejectReasons, fmt.Sprintf("Staleness status %s is not FRESH", staleness.Status))
	}

	// 8. MarketPolicy direction permissions
	if quant.Direction == LONG && !policy.AllowLong {
		rejectReasons = append(rejectReasons, "LONG direction disallowed by policy")
	}
	if quant.Direction == SHORT && !policy.AllowShort {
		rejectReasons = append(rejectReasons, "SHORT direction disallowed by policy")
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
		rejectReasons = append(rejectReasons, fmt.Sprintf("Playbook %s not in allowed playbooks list", quant.Playbook))
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
		rejectReasons = append(rejectReasons, fmt.Sprintf("Tier %s not in allowed tiers list", quant.Tier))
	}

	// 11. Score check
	if quant.Score < minScoreExecute {
		if aiAudit.Decision == "WAIT" {
			watchReasons = append(watchReasons, fmt.Sprintf("Quant score %0.1f below minimum execute score %0.1f", quant.Score, minScoreExecute))
		} else {
			rejectReasons = append(rejectReasons, fmt.Sprintf("Quant score %0.1f below minimum execute score %0.1f", quant.Score, minScoreExecute))
		}
	}

	// 12. Actual Risk-to-Reward ratio check
	entry := quant.TradePlan.EntryPrice
	tp := quant.TradePlan.TakeProfit
	sl := quant.TradePlan.StopLoss
	rrActual := 0.0

	if latestPrice > 0 && tp > 0 && sl > 0 {
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
			watchReasons = append(watchReasons, fmt.Sprintf("Actual RR %0.2f below minimum required RR %0.2f", rrActual, minRRExecute))
		} else {
			rejectReasons = append(rejectReasons, fmt.Sprintf("Actual RR %0.2f below minimum required RR %0.2f", rrActual, minRRExecute))
		}
	}

	// 13. ADX rule checks
	adxVal := quant.TechnicalSnapshot.IndicatorValues[IndicatorADX]
	minADX := math.Max(policy.MinADXExecute, profile.MinADX)

	if profile.RequireADX && adxVal < minADX {
		watchReasons = append(watchReasons, fmt.Sprintf("ADX %0.1f below required threshold %0.1f", adxVal, minADX))
	}
	if profile.RejectADXExpansion {
		maxADX := profile.MaxADX
		if maxADX <= 0 {
			maxADX = 30.0
		}
		if adxVal > maxADX {
			watchReasons = append(watchReasons, fmt.Sprintf("High trend expansion detected (ADX = %0.1f > %0.1f)", adxVal, maxADX))
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
				watchReasons = append(watchReasons, "Rejection wick or price action evidence missing")
			} else {
				rejectReasons = append(rejectReasons, "Rejection wick or price action evidence missing")
			}
		}
	}

	// 15. Confirmation requirement checks
	hasConfirmation := quant.IndicatorMet || aiAudit.HasConfirmation ||
		strings.Contains(strings.ToLower(quant.Reason), "confirm") ||
		strings.Contains(strings.ToLower(planReview.Reason), "confirm")

	if profile.RequireConfirmation || quant.Playbook == LIQUIDITY_SWEEP_REVERSAL || quant.Playbook == RANGE_EDGE_REVERSAL || quant.Playbook == CROWDED_POSITIONING_SQUEEZE {
		if !hasConfirmation {
			watchReasons = append(watchReasons, "Confirmation candle / structure missing")
		}
	}

	// 16. Retest requirement checks
	isBreakoutPlaybook := quant.Playbook == COMPRESSION_BREAKOUT_RETEST
	needsRetest := planReview.NeedRetest || aiAudit.SuggestedAction == "WAIT_RETEST"
	isFirstBreakoutCandle := false
	if isBreakoutPlaybook {
		isFirstBreakoutCandle = strings.Contains(strings.ToUpper(quant.SetupType), "BREAKOUT") && !strings.Contains(strings.ToUpper(quant.SetupType), "RETEST")
	}

	if profile.RequireRetest || isBreakoutPlaybook {
		retestFailed := false
		if planReview.Conflicted && (strings.Contains(strings.ToLower(planReview.Reason), "retest") || strings.Contains(strings.ToLower(planReview.Reason), "breakout")) {
			retestFailed = true
		}
		if aiAudit.Decision == "REJECT" && strings.Contains(strings.ToLower(aiAudit.Reason), "retest") {
			retestFailed = true
		}

		if retestFailed {
			rejectReasons = append(rejectReasons, "Retest failed")
		} else if needsRetest || isFirstBreakoutCandle {
			watchReasons = append(watchReasons, "Retest required / breakout is on first candle")
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
			watchReasons = append(watchReasons, "Volume / OI confirmation missing")
		} else {
			rejectReasons = append(rejectReasons, "Volume / OI confirmation missing")
		}
	}

	// 18. Crowding evidence checks
	hasCrowdingEvidence := (quant.TechnicalSnapshot.IndicatorValues[IndicatorExtremeFunding] == 1.0) ||
		(quant.TechnicalSnapshot.IndicatorValues[IndicatorExtremeOI] == 1.0) ||
		(quant.TechnicalSnapshot.IndicatorValues[IndicatorCrowdingScore] >= profile.MinCrowdingScore)
	if profile.RequireCrowdingEvidence || quant.Playbook == CROWDED_POSITIONING_SQUEEZE {
		if !hasCrowdingEvidence {
			if aiAudit.Decision == "WAIT" {
				watchReasons = append(watchReasons, "Crowded funding/OI positioning evidence missing")
			} else {
				rejectReasons = append(rejectReasons, "Crowded funding/OI positioning evidence missing")
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
		rejectReasons = append(rejectReasons, "Opposite active signal currently open for symbol")
	}

	// 20. Concurrent active signal limit check
	activeCount := 0
	for _, item := range activeSignals {
		if item.Status == MONITORING {
			activeCount++
		}
	}
	if activeCount >= policy.MaxFinalExecute {
		rejectReasons = append(rejectReasons, fmt.Sprintf("Concurrent active signals count %d exceeds policy limit %d", activeCount, policy.MaxFinalExecute))
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
		rejectReasons = append(rejectReasons, "Symbol cooldown is active")
	}

	// 22. Setup/playbook blacklist checked via AllowedPlaybooks in localGate.Evaluate,
	// but let's re-verify it to be absolutely sure.

	// 23. TradePlan validation
	tpValid := entry > 0 && sl > 0 && tp > 0 &&
		((quant.Direction == LONG && sl < entry && tp > entry) ||
			(quant.Direction == SHORT && sl > entry && tp < entry))
	if !tpValid {
		rejectReasons = append(rejectReasons, "TradePlan parameters invalid (e.g. SL or TP reversed)")
	}

	// Playbook-specific execution safety rules
	if quant.Playbook == TREND_PULLBACK {
		trendAligned := false
		if quant.Direction == LONG && quant.H4Trend == "BULLISH" && quant.H1Trend == "BULLISH" {
			trendAligned = true
		} else if quant.Direction == SHORT && quant.H4Trend == "BEARISH" && quant.H1Trend == "BEARISH" {
			trendAligned = true
		}
		if !trendAligned {
			rejectReasons = append(rejectReasons, "Trend pullback lacks H1/H4 trend alignment")
		}
		if aiAudit.CandleNarrative == "EXHAUSTED" || strings.Contains(strings.ToLower(aiAudit.Reason), "overextended") {
			rejectReasons = append(rejectReasons, "Trend pullback is overextended")
		}
	}

	if quant.Playbook == LIQUIDITY_SWEEP_REVERSAL {
		isSweep := strings.Contains(strings.ToLower(quant.Reason), "sweep") ||
			quant.TechnicalSnapshot.IndicatorValues[IndicatorSweepLow] == 1.0 ||
			quant.TechnicalSnapshot.IndicatorValues[IndicatorSweepHigh] == 1.0
		if !isSweep {
			if aiAudit.Decision == "WAIT" {
				watchReasons = append(watchReasons, "Liquidity sweep setup lacks high/low breakout and reclaim evidence")
			} else {
				rejectReasons = append(rejectReasons, "Liquidity sweep setup lacks high/low breakout and reclaim evidence")
			}
		}
	}

	if quant.Playbook == RANGE_EDGE_REVERSAL {
		nearEdge := quant.TechnicalSnapshot.IndicatorValues[IndicatorNearRangeEdge] == 1.0 ||
			strings.Contains(strings.ToLower(quant.Reason), "range")
		if !nearEdge {
			if aiAudit.Decision == "WAIT" {
				watchReasons = append(watchReasons, "Not close enough to range edge bounds")
			} else {
				rejectReasons = append(rejectReasons, "Not close enough to range edge bounds")
			}
		}
		if aiAudit.CandleNarrative == "CONTINUATION" || strings.Contains(strings.ToLower(aiAudit.Reason), "breaking range") {
			rejectReasons = append(rejectReasons, "Range edge candle narrative is CONTINUATION (breakout threat)")
		}
	}

	if quant.Playbook == CROWDED_POSITIONING_SQUEEZE {
		if quant.Score < 7.8 {
			rejectReasons = append(rejectReasons, fmt.Sprintf("Crowded squeeze score %0.1f is below mandatory 7.8", quant.Score))
		}
		if quant.TechnicalSnapshot.IndicatorValues[IndicatorExtremeFunding] == 1.0 && !hasConfirmation {
			if aiAudit.Decision == "WAIT" {
				watchReasons = append(watchReasons, "Crowded squeeze has extreme funding but lacks confirmation candle")
			} else {
				rejectReasons = append(rejectReasons, "Crowded squeeze has extreme funding but lacks confirmation candle")
			}
		}
	}

	// Final Status & Reason Resolution
	var status Status
	var reason string

	if isAIError {
		status = AI_ERROR_REVIEW
		reason = "AI_ERROR: " + aiAudit.Reasoning
	} else if len(rejectReasons) > 0 {
		status = FINAL_REJECT
		reason = strings.Join(rejectReasons, "; ")
	} else if len(watchReasons) > 0 {
		status = FINAL_WATCH
		reason = strings.Join(watchReasons, "; ")
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
	profileSummary := fmt.Sprintf("Playbook=%s, MinScore=%0.1f, MinRR=%0.1f, RequireADX=%v, RequireVolumeConfirm=%v, RequireRejection=%v, RequireConfirmation=%v, RequireRetest=%v",
		profile.Playbook, profile.MinScoreExecute, profile.MinRR, profile.RequireADX, profile.RequireVolumeConfirm, profile.RequireRejection, profile.RequireConfirmation, profile.RequireRetest)

	return FinalDecision{
		Symbol:                  quant.Symbol,
		Direction:               quant.Direction,
		Playbook:                quant.Playbook,
		Status:                  status,
		Reason:                  reason,
		Score:                   quant.Score,
		RequiredScore:           minScoreExecute,
		RR:                      rrActual,
		RequiredRR:              minRRExecute,
		AIConfidence:            aiAudit.Confidence,
		StalenessStatus:         string(staleness.Status),
		PolicySummary:           policySummary,
		ThresholdProfileSummary: profileSummary,
		IsExecutable:            isExecutable,
		Tier:                    quant.Tier,
		EntryPrice:              quant.TradePlan.EntryPrice,
		StopLoss:                quant.TradePlan.StopLoss,
		TakeProfit:              quant.TradePlan.TakeProfit,
		WatchReason:             watchReason,
		RejectReason:            rejectReason,
	}
}
