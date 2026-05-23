package usecase

import (
	"fmt"
	"math"
	"strings"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
)

type LocalGateUsecase struct{}

func NewLocalGateUsecase() *LocalGateUsecase {
	return &LocalGateUsecase{}
}

// Passes checks local structural criteria against active MarketPolicy constraints.
func (uc *LocalGateUsecase) Passes(result QuantResult, policy MarketPolicy, playbook Playbook, m15 []dto.Candle) bool {
	evalRes := uc.Evaluate(result, policy, m15)
	return evalRes.Passed
}

// Evaluate evaluates a trade candidate that has passed Candidate Arbiter.
func (uc *LocalGateUsecase) Evaluate(quant QuantResult, policy MarketPolicy, m15 []dto.Candle) LocalGateResult {
	// Guard: ensure IndicatorValues is never nil to prevent panic on map read
	if quant.TechnicalSnapshot.IndicatorValues == nil {
		quant.TechnicalSnapshot.IndicatorValues = make(map[string]float64)
	}

	// Rule 1: Direction validation
	if quant.Direction != LONG && quant.Direction != SHORT {

		return LocalGateResult{
			Passed: false,
			Status: LOCAL_REJECT,
			Reason: fmt.Sprintf("Direction %s is invalid; must be LONG or SHORT", quant.Direction),
		}
	}

	// Score validation (NaN/Inf)
	if math.IsNaN(quant.Score) || math.IsInf(quant.Score, 0) {
		return LocalGateResult{
			Passed: false,
			Status: LOCAL_REJECT,
			Reason: fmt.Sprintf("Invalid score detected: %f", quant.Score),
		}
	}

	// Rule 2 & 3: LONG tapi policy.AllowLong false, SHORT tapi policy.AllowShort false
	if quant.Direction == LONG && !policy.AllowLong {
		return LocalGateResult{
			Passed: false,
			Status: LOCAL_REJECT,
			Reason: "LONG trades disallowed by MarketPolicy AllowLong constraint",
		}
	}
	if quant.Direction == SHORT && !policy.AllowShort {
		return LocalGateResult{
			Passed: false,
			Status: LOCAL_REJECT,
			Reason: "SHORT trades disallowed by MarketPolicy AllowShort constraint",
		}
	}

	// Rule 4: LongMode/ShortMode disabled check
	if quant.Direction == LONG && policy.LongMode == DISABLED {
		return LocalGateResult{
			Passed: false,
			Status: LOCAL_REJECT,
			Reason: "LONG trades disabled by policy.LongMode configuration",
		}
	}
	if quant.Direction == SHORT && policy.ShortMode == DISABLED {
		return LocalGateResult{
			Passed: false,
			Status: LOCAL_REJECT,
			Reason: "SHORT trades disabled by policy.ShortMode configuration",
		}
	}

	// Allowed playbook validation
	playbookAllowed := false
	for _, p := range policy.AllowedPlaybooks {
		if p == quant.Playbook {
			playbookAllowed = true
			break
		}
	}
	if !playbookAllowed {
		return LocalGateResult{
			Passed: false,
			Status: LOCAL_REJECT,
			Reason: fmt.Sprintf("Playbook %s is not permitted in AllowedPlaybooks", quant.Playbook),
		}
	}

	// Tier validation
	tierAllowed := false
	for _, t := range policy.AllowedTiers {
		if t == quant.Tier {
			tierAllowed = true
			break
		}
	}
	if !tierAllowed {
		return LocalGateResult{
			Passed: false,
			Status: LOCAL_REJECT,
			Reason: fmt.Sprintf("Tier %s is not permitted in AllowedTiers", quant.Tier),
		}
	}

	// Tier C under high volatility or chaos check
	isChaos := strings.Contains(strings.ToUpper(policy.Reason), "CHAOS")
	isHighVol := strings.Contains(strings.ToUpper(policy.Reason), "HIGH_VOL")
	if quant.Tier == TierC && (isChaos || isHighVol) {
		return LocalGateResult{
			Passed: false,
			Status: LOCAL_REJECT,
			Reason: "Tier C candidate blocked during high volatility or chaos regime",
		}
	}

	// TradePlan boundary checks (prevent reversed stop loss or take profit)
	entry := quant.TradePlan.EntryPrice
	tp := quant.TradePlan.TakeProfit
	sl := quant.TradePlan.StopLoss

	if entry <= 0 {
		return LocalGateResult{
			Passed: false,
			Status: LOCAL_REJECT,
			Reason: fmt.Sprintf("Invalid Entry Price: %0.2f", entry),
		}
	}

	if quant.Direction == LONG {
		if sl >= entry {
			return LocalGateResult{
				Passed: false,
				Status: LOCAL_REJECT,
				Reason: fmt.Sprintf("LONG TradePlan alignment error: SL %0.2f must be below Entry %0.2f", sl, entry),
			}
		}
		if tp <= entry {
			return LocalGateResult{
				Passed: false,
				Status: LOCAL_REJECT,
				Reason: fmt.Sprintf("LONG TradePlan alignment error: TP %0.2f must be above Entry %0.2f", tp, entry),
			}
		}
	} else if quant.Direction == SHORT {
		if sl <= entry {
			return LocalGateResult{
				Passed: false,
				Status: LOCAL_REJECT,
				Reason: fmt.Sprintf("SHORT TradePlan alignment error: SL %0.2f must be above Entry %0.2f", sl, entry),
			}
		}
		if tp >= entry {
			return LocalGateResult{
				Passed: false,
				Status: LOCAL_REJECT,
				Reason: fmt.Sprintf("SHORT TradePlan alignment error: TP %0.2f must be below Entry %0.2f", tp, entry),
			}
		}
	}

	// Load playbook specific threshold override profile
	profile := GetPlaybookThresholdProfile(quant.Playbook, policy, quant.Tier)

	// RR validation
	rr := uc.calculateRR(quant)
	if rr <= 0 {
		return LocalGateResult{
			Passed: false,
			Status: LOCAL_REJECT,
			Reason: fmt.Sprintf("Invalid Risk-to-Reward ratio: %0.2f", rr),
		}
	}
	minRR := math.Max(policy.MinRRExecute, profile.MinRR)
	if rr < minRR {
		return LocalGateResult{
			Passed: false,
			Status: LOCAL_REJECT,
			Reason: fmt.Sprintf("Risk-to-Reward ratio %0.2f is below requirement %0.2f", rr, minRR),
		}
	}

	// LongMode/ShortMode detailed checks
	if quant.Direction == LONG {
		if policy.LongMode == REVERSAL_ONLY {
			isSweep := quant.Playbook == LIQUIDITY_SWEEP_REVERSAL
			isRangeReversal := quant.Playbook == RANGE_EDGE_REVERSAL
			isSqueeze := quant.Playbook == CROWDED_POSITIONING_SQUEEZE

			if !isSweep && !isRangeReversal && !isSqueeze {
				return LocalGateResult{
					Passed: false,
					Status: LOCAL_REJECT,
					Reason: fmt.Sprintf("LongMode is REVERSAL_ONLY; playbook %s is blocked", quant.Playbook),
				}
			}
		} else if policy.LongMode == PULLBACK_ONLY {
			if quant.Playbook != TREND_PULLBACK {
				isBreakout := quant.Playbook == COMPRESSION_BREAKOUT_RETEST
				isPremiumReversal := (quant.Playbook == LIQUIDITY_SWEEP_REVERSAL || quant.Playbook == RANGE_EDGE_REVERSAL) && quant.Score >= 7.8
				if isBreakout {
					// Proceed
				} else if isPremiumReversal {
					return LocalGateResult{
						Passed: false,
						Status: LOCAL_WATCH,
						Reason: fmt.Sprintf("LongMode is PULLBACK_ONLY; premium reversal playbook %s sent to watch", quant.Playbook),
					}
				} else {
					return LocalGateResult{
						Passed: false,
						Status: LOCAL_REJECT,
						Reason: fmt.Sprintf("LongMode is PULLBACK_ONLY; playbook %s is blocked", quant.Playbook),
					}
				}
			}
		}
	} else if quant.Direction == SHORT {
		if policy.ShortMode == SWEEP_ONLY {
			isSweep := quant.Playbook == LIQUIDITY_SWEEP_REVERSAL
			isFailedBreakout := strings.Contains(strings.ToUpper(quant.SetupType), "FAILED_BREAKOUT")
			isStrongRejection := quant.Playbook == RANGE_EDGE_REVERSAL && quant.TechnicalSnapshot.IndicatorValues["wick_rejection"] == 1.0

			if !isSweep && !isFailedBreakout && !isStrongRejection {
				return LocalGateResult{
					Passed: false,
					Status: LOCAL_REJECT,
					Reason: fmt.Sprintf("ShortMode is SWEEP_ONLY; playbook %s and setup %s blocked", quant.Playbook, quant.SetupType),
				}
			}
		}
	}

	// Score vs MinScoreAI check using profile override
	minScoreAI := math.Max(policy.MinScoreAI, profile.MinScoreAI)
	if quant.Score < minScoreAI {
		if quant.Score < minScoreAI-0.5 {
			return LocalGateResult{
				Passed: false,
				Status: LOCAL_REJECT,
				Reason: fmt.Sprintf("Score %0.1f below minimum AI score limit %0.1f", quant.Score, minScoreAI),
			}
		}
		return LocalGateResult{
			Passed: false,
			Status: LOCAL_WATCH,
			Reason: fmt.Sprintf("Score %0.1f slightly below minimum AI score limit %0.1f", quant.Score, minScoreAI),
		}
	}

	// ADX validation per profile
	adx := quant.TechnicalSnapshot.IndicatorValues["adx"]
	if profile.RequireADX {
		if adx < profile.MinADX {
			return LocalGateResult{
				Passed: false,
				Status: LOCAL_WATCH,
				Reason: fmt.Sprintf("%s rejected: ADX %0.1f is below execution threshold %0.1f", quant.Playbook, adx, profile.MinADX),
			}
		}
	}

	if profile.RejectADXExpansion {
		maxADX := profile.MaxADX
		if maxADX <= 0 {
			maxADX = 30.0
		}
		if adx > maxADX {
			return LocalGateResult{
				Passed: false,
				Status: LOCAL_WATCH,
				Reason: fmt.Sprintf("%s rejected: high trend expansion detected (ADX = %0.1f > %0.1f)", quant.Playbook, adx, maxADX),
			}
		}
	}

	// BTCChaos validation
	if isChaos {
		minScoreExec := math.Max(policy.MinScoreExecute, profile.MinScoreExecute)
		if quant.Score < minScoreExec {
			return LocalGateResult{
				Passed: false,
				Status: LOCAL_WATCH,
				Reason: fmt.Sprintf("BTCChaos: score %0.1f is below execution threshold %0.1f", quant.Score, minScoreExec),
			}
		}
		if quant.Playbook != LIQUIDITY_SWEEP_REVERSAL && quant.Playbook != CROWDED_POSITIONING_SQUEEZE {
			return LocalGateResult{
				Passed: false,
				Status: LOCAL_REJECT,
				Reason: fmt.Sprintf("BTCChaos: playbook %s is not permitted under chaos regime", quant.Playbook),
			}
		}
		chaosMinRR := minRR
		if rr < chaosMinRR {
			return LocalGateResult{
				Passed: false,
				Status: LOCAL_REJECT,
				Reason: fmt.Sprintf("BTCChaos: Risk-to-Reward ratio %0.2f is below chaos requirement %0.2f", rr, chaosMinRR),
			}
		}
	}

	// Volume confirmation check
	if profile.RequireVolumeConfirm {
		minVolRatio := profile.MinVolumeRatio
		if minVolRatio <= 0 {
			minVolRatio = 1.3
		}
		if !ConfirmLiquiditySweep(m15, 10, minVolRatio) {
			return LocalGateResult{
				Passed: false,
				Status: LOCAL_REJECT,
				Reason: fmt.Sprintf("%s lacks required volume spike confirmation", quant.Playbook),
			}
		}
	}

	// Rejection requirement check
	if profile.RequireRejection {
		wickRejection := quant.TechnicalSnapshot.IndicatorValues["wick_rejection"]
		if wickRejection != 1.0 {
			return LocalGateResult{
				Passed: false,
				Status: LOCAL_REJECT,
				Reason: fmt.Sprintf("%s lacks required wick rejection confirmation", quant.Playbook),
			}
		}
	}

	// Confirmation requirement check
	if profile.RequireConfirmation {
		if !quant.IndicatorMet {
			return LocalGateResult{
				Passed: false,
				Status: LOCAL_REJECT,
				Reason: fmt.Sprintf("%s lacks required indicator confirmation", quant.Playbook),
			}
		}
	}

	// Retest requirement / first breakout candle prevention check
	if profile.RequireRetest {
		isFirstCandle := strings.Contains(strings.ToUpper(quant.SetupType), "BREAKOUT") && !strings.Contains(strings.ToUpper(quant.SetupType), "RETEST")
		if isFirstCandle && !profile.AllowBreakoutCandleEntry {
			return LocalGateResult{
				Passed: false,
				Status: LOCAL_WATCH,
				Reason: "WAIT_RETEST_OR_BREAKOUT_FIRST_CANDLE",
			}
		}
	}

	// Crowding evidence check
	if profile.RequireCrowdingEvidence {
		funding := math.Abs(quant.TechnicalSnapshot.IndicatorValues["funding_rate"])
		oiChange := quant.TechnicalSnapshot.IndicatorValues["oi_change"]
		crowdingScore := quant.TechnicalSnapshot.IndicatorValues["crowding_score"]
		if funding == 0 && oiChange == 0 && crowdingScore < profile.MinCrowdingScore {
			return LocalGateResult{
				Passed: false,
				Status: LOCAL_REJECT,
				Reason: fmt.Sprintf("%s lacks required funding/OI crowding evidence", quant.Playbook),
			}
		}
	}

	// Indicator trigger check
	if !quant.IndicatorMet {
		return LocalGateResult{
			Passed: false,
			Status: LOCAL_REJECT,
			Reason: "Technical setup indicators not fully met",
		}
	}

	// Pass all checks
	return LocalGateResult{
		Passed: true,
		Status: AI_CANDIDATE,
		Reason: "All local quality gate criteria met successfully",
	}
}

// calculateRR extracts Risk-to-Reward ratio from TradePlan parameters
func (uc *LocalGateUsecase) calculateRR(cand QuantResult) float64 {
	entry := cand.TradePlan.EntryPrice
	tp := cand.TradePlan.TakeProfit
	sl := cand.TradePlan.StopLoss
	if entry <= 0 || tp <= 0 || sl <= 0 {
		return 0.0
	}
	if cand.Direction == LONG {
		if entry > sl {
			return (tp - entry) / (entry - sl)
		}
	} else if cand.Direction == SHORT {
		if sl > entry {
			return (entry - tp) / (sl - entry)
		}
	}
	return 0.0
}
