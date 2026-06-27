package usecase

import (
	"fmt"
	"math"
	"strings"
)

func resolveVolatilityRegimePenalty(playbook Playbook, regime MarketRegime) (float64, string) {
	switch regime {
	case BTC_CHAOS:
		switch playbook {
		case TREND_PULLBACK, COMPRESSION_BREAKOUT_RETEST:
			return 8.0, "GLOBAL PENALTY: BTC_CHAOS regime remains too hostile for continuation/breakout setups (-8)"
		case RANGE_EDGE_REVERSAL:
			return 5.0, "GLOBAL PENALTY: BTC_CHAOS regime remains too unstable for range reversal timing (-5)"
		case LIQUIDITY_SWEEP_REVERSAL, CROWDED_POSITIONING_SQUEEZE:
			return 3.0, "GLOBAL PENALTY: BTC_CHAOS regime still adds execution noise even for reversal setups (-3)"
		}
	case HIGH_VOL:
		switch playbook {
		case TREND_PULLBACK, COMPRESSION_BREAKOUT_RETEST:
			return 5.0, "GLOBAL PENALTY: HIGH_VOL regime adds continuation timing noise (-5)"
		case RANGE_EDGE_REVERSAL:
			return 2.0, "GLOBAL PENALTY: HIGH_VOL regime reduces mean-reversion reliability (-2)"
		case LIQUIDITY_SWEEP_REVERSAL, CROWDED_POSITIONING_SQUEEZE:
			return 0.0, ""
		}
	}

	return 0.0, ""
}

type ScoringUsecase struct{}

func NewScoringUsecase() *ScoringUsecase {
	return &ScoringUsecase{}
}

// Calculate computes the final playbook-specific score for a quant candidate.
// The score is returned on a scale of 0.0 to 10.0.
// It mutates the quant.Reason field to explain the score breakdown, grade, and any applied penalties.
func (uc *ScoringUsecase) Calculate(quant *QuantResult, resolvedDirection Direction, policy MarketPolicy) float64 {
	if quant == nil || !quant.IndicatorMet || resolvedDirection == WAIT {
		return 0.0
	}

	// Default base variables
	rawScore := 0.0
	tech := quant.TechnicalSnapshot
	rsi := tech.RSI
	adxVal := tech.IndicatorValues[IndicatorADX]

	// Flags from TechnicalSnapshot
	sweepLow := tech.IndicatorValues[IndicatorSweepLow]
	sweepHigh := tech.IndicatorValues[IndicatorSweepHigh]
	wickRejection := tech.IndicatorValues[IndicatorWickRejection]
	volumeSpike := tech.IndicatorValues[IndicatorVolumeSpike]
	contraction := tech.IndicatorValues[IndicatorContraction]
	nearRangeEdge := tech.IndicatorValues[IndicatorNearRangeEdge]
	extremeFunding := tech.IndicatorValues[IndicatorExtremeFunding]
	extremeOI := tech.IndicatorValues[IndicatorExtremeOI]
	paRejection := tech.IndicatorValues[IndicatorPARejection]
	fundingRate := tech.IndicatorValues[IndicatorFundingRate]
	if fundingRate == 0 {
		fundingRate = tech.FundingRate
	}
	fundingAbs := math.Abs(fundingRate)
	fundingExtreme := extremeFunding == 1.0 || fundingAbs > fundingExtremeThreshold()
	supportsSqueezeDirection := (resolvedDirection == LONG && fundingRate < 0) || (resolvedDirection == SHORT && fundingRate > 0)

	// Calculate RR (Risk-to-Reward)
	rr := 1.0
	if resolvedDirection == LONG && quant.TriggerPrice > quant.StopLoss {
		rr = (quant.TakeProfit - quant.TriggerPrice) / (quant.TriggerPrice - quant.StopLoss)
	} else if resolvedDirection == SHORT && quant.StopLoss > quant.TriggerPrice {
		rr = (quant.TriggerPrice - quant.TakeProfit) / (quant.StopLoss - quant.TriggerPrice)
	}

	var notes []string

	switch quant.Playbook {
	case TREND_PULLBACK:
		// 1. Trend alignment (Max 30)
		trendScore := 0.0
		if resolvedDirection == LONG {
			if quant.H4Trend == "BULLISH" {
				trendScore += 15.0
			}
			if quant.H1Trend == "BULLISH" {
				trendScore += 15.0
			}
		} else if resolvedDirection == SHORT {
			if quant.H4Trend == "BEARISH" {
				trendScore += 15.0
			}
			if quant.H1Trend == "BEARISH" {
				trendScore += 15.0
			}
		}
		rawScore += trendScore
		notes = append(notes, fmt.Sprintf("TrendAlign: +%0.1f", trendScore))

		// 2. Pullback quality (Max 25)
		pullbackScore := 5.0
		if resolvedDirection == LONG {
			if rsi >= 40.0 && rsi <= 55.0 {
				pullbackScore = 25.0
			} else if rsi >= 30.0 && rsi < 40.0 {
				pullbackScore = 15.0
			}
		} else if resolvedDirection == SHORT {
			if rsi >= 45.0 && rsi <= 60.0 {
				pullbackScore = 25.0
			} else if rsi > 60.0 && rsi <= 70.0 {
				pullbackScore = 15.0
			}
		}
		rawScore += pullbackScore
		notes = append(notes, fmt.Sprintf("PullbackQual: +%0.1f", pullbackScore))

		// 3. Momentum (Max 20)
		momentumScore := 5.0
		if resolvedDirection == LONG && tech.MACD > 0 {
			momentumScore = 20.0
		} else if resolvedDirection == SHORT && tech.MACD < 0 {
			momentumScore = 20.0
		}
		rawScore += momentumScore
		notes = append(notes, fmt.Sprintf("Momentum: +%0.1f", momentumScore))

		// 4. RR (Max 15)
		rrScore := 0.0
		if rr >= 2.0 {
			rrScore = 15.0
		} else if rr >= 1.5 {
			rrScore = 10.0
		}
		rawScore += rrScore
		notes = append(notes, fmt.Sprintf("RRScore: +%0.1f", rrScore))

		// 5. Participation (Max 10)
		partScore := 3.0
		if adxVal > 22.0 && volumeSpike == 1.0 {
			partScore = 10.0
		} else if adxVal > 20.0 {
			partScore = 7.0
		}
		rawScore += partScore
		notes = append(notes, fmt.Sprintf("Participation: +%0.1f", partScore))

		// Playbook specific penalties
		if resolvedDirection == LONG && policy.LongMode == REVERSAL_ONLY {
			rawScore -= 5.0
			notes = append(notes, "PENALTY: Policy LongMode is REVERSAL_ONLY (-15)")
		}
		if resolvedDirection == SHORT && (policy.ShortMode == SWEEP_ONLY || policy.ShortMode == DISABLED || policy.ShortMode == REVERSAL_ONLY) {
			rawScore -= 5.0
			notes = append(notes, "PENALTY: Policy ShortMode not supportive of trend pullback (-15)")
		}
		if resolvedDirection == LONG && quant.H4Trend != "BULLISH" {
			rawScore -= 10.0
			notes = append(notes, "PENALTY: LONG trend playbook symbol H4 structure is weak (-10)")
		}
		if resolvedDirection == SHORT && quant.H4Trend != "BEARISH" {
			rawScore -= 10.0
			notes = append(notes, "PENALTY: SHORT trend playbook symbol H4 structure is weak (-10)")
		}

	case LIQUIDITY_SWEEP_REVERSAL:
		// 1. Sweep quality (Max 30)
		sweepScore := 0.0
		if resolvedDirection == LONG && sweepLow == 1.0 {
			sweepScore = 30.0
		} else if resolvedDirection == SHORT && sweepHigh == 1.0 {
			sweepScore = 30.0
		}
		rawScore += sweepScore
		notes = append(notes, fmt.Sprintf("SweepQual: +%0.1f", sweepScore))

		// 2. Rejection quality (Max 25)
		rejScore := 5.0
		if wickRejection == 1.0 {
			rejScore = 25.0
		}
		rawScore += rejScore
		notes = append(notes, fmt.Sprintf("RejectionQual: +%0.1f", rejScore))

		// 3. Confirmation early (Max 15)
		confScore := 5.0
		if paRejection == 1.0 {
			confScore = 15.0
		}
		rawScore += confScore
		notes = append(notes, fmt.Sprintf("ConfEarly: +%0.1f", confScore))

		// 4. Crowded/volume/OI (Max 15)
		volScore := 0.0
		if volumeSpike == 1.0 {
			volScore = 15.0
		}
		rawScore += volScore
		notes = append(notes, fmt.Sprintf("VolSpike: +%0.1f", volScore))

		// 5. RR (Max 15)
		rrScore := 0.0
		if rr >= 2.0 {
			rrScore = 15.0
		} else if rr >= 1.5 {
			rrScore = 10.0
		}
		rawScore += rrScore
		notes = append(notes, fmt.Sprintf("RRScore: +%0.1f", rrScore))

		// Playbook specific penalties
		if volumeSpike == 0.0 {
			rawScore -= 5.0
			notes = append(notes, "PENALTY: Sweep reversal missing volume confirmation (-30)")
		}
		if resolvedDirection == LONG && sweepLow != 1.0 {
			rawScore -= 5.0
			notes = append(notes, "PENALTY: LONG reversal missing lower sweep reclaim (-30)")
		}
		if resolvedDirection == SHORT && sweepHigh != 1.0 {
			rawScore -= 5.0
			notes = append(notes, "PENALTY: SHORT reversal missing upper sweep rejection (-30)")
		}

	case COMPRESSION_BREAKOUT_RETEST:
		// 1. Compression quality (Max 25)
		compScore := 5.0
		if contraction == 1.0 {
			compScore = 25.0
		}
		rawScore += compScore
		notes = append(notes, fmt.Sprintf("CompressionQual: +%0.1f", compScore))

		// 2. Breakout strength (Max 20)
		breakScore := 5.0
		if volumeSpike == 1.0 {
			breakScore = 20.0
		}
		rawScore += breakScore
		notes = append(notes, fmt.Sprintf("BreakoutStrength: +%0.1f", breakScore))

		// 3. Retest quality (Max 25)
		retestHold := tech.IndicatorValues[IndicatorRetestHold]
		retestQuality := calculateRetestQuality(tech.IndicatorValues)
		retestScore := 25.0 * retestQuality
		rawScore += retestScore
		notes = append(notes, fmt.Sprintf("RetestQual: +%0.1f (quality=%0.2f)", retestScore, retestQuality))

		// 4. Volume/OI support (Max 15)
		supportScore := 5.0
		hasVolumeSupport := volumeSpike == 1.0
		hasDerivativeSupport := extremeOI == 1.0 || tech.IndicatorValues[IndicatorOIChange] > 0
		switch {
		case hasVolumeSupport && hasDerivativeSupport:
			supportScore = 15.0
		case hasVolumeSupport || hasDerivativeSupport:
			supportScore = 10.0
		}
		rawScore += supportScore
		notes = append(notes, fmt.Sprintf("VolOISupport: +%0.1f", supportScore))

		// 5. RR (Max 15)
		rrScore := 0.0
		if rr >= 2.2 {
			rrScore = 15.0
		} else if rr >= 1.7 {
			rrScore = 10.0
		}
		rawScore += rrScore
		notes = append(notes, fmt.Sprintf("RRScore: +%0.1f", rrScore))

		// Playbook specific penalties
		if policy.RequireFreshEntry && retestHold == 0.0 {
			rawScore -= 5.0
			notes = append(notes, "PENALTY: Breakout retest required but no retest hold evidence (-30)")
		}
		if resolvedDirection == LONG && retestHold == 0.0 {
			rawScore -= 10.0
			notes = append(notes, "PENALTY: LONG breakout entry missing clear retest support hold (-20)")
		}
		if resolvedDirection == SHORT && retestHold == 0.0 {
			rawScore -= 10.0
			notes = append(notes, "PENALTY: SHORT breakout entry missing clear retest resistance reject (-20)")
		}

	case RANGE_EDGE_REVERSAL:
		// 1. Range clarity (Max 25)
		clarityScore := 5.0
		if adxVal < 25.0 {
			clarityScore = 25.0
		} else if adxVal < 30.0 {
			clarityScore = 15.0
		}
		rawScore += clarityScore
		notes = append(notes, fmt.Sprintf("RangeClarity: +%0.1f", clarityScore))

		// 2. Edge distance (Max 25)
		distScore := 5.0
		if nearRangeEdge == 1.0 {
			distScore = 25.0
		}
		rawScore += distScore
		notes = append(notes, fmt.Sprintf("EdgeDistance: +%0.1f", distScore))

		// 3. Rejection quality (Max 20)
		rejScore := 5.0
		if wickRejection == 1.0 {
			rejScore = 20.0
		}
		rawScore += rejScore
		notes = append(notes, fmt.Sprintf("RejectionQual: +%0.1f", rejScore))

		// 4. Trend risk penalty (Max 15)
		riskScore := 0.0
		if adxVal < 20.0 {
			riskScore = 15.0
		} else if adxVal < 25.0 {
			riskScore = 10.0
		}
		rawScore += riskScore
		notes = append(notes, fmt.Sprintf("TrendRiskScore: +%0.1f", riskScore))

		// 5. RR (Max 15)
		rrScore := 0.0
		if rr >= 2.0 {
			rrScore = 15.0
		} else if rr >= 1.5 {
			rrScore = 10.0
		}
		rawScore += rrScore
		notes = append(notes, fmt.Sprintf("RRScore: +%0.1f", rrScore))

		// Playbook specific penalties
		if resolvedDirection == LONG && adxVal > 25.0 && quant.H4Trend == "BEARISH" {
			rawScore -= 5.0
			notes = append(notes, "PENALTY: LONG range reversal under strong bearish trend expansion (-30)")
		}
		if resolvedDirection == SHORT && adxVal > 25.0 && quant.H4Trend == "BULLISH" {
			rawScore -= 5.0
			notes = append(notes, "PENALTY: SHORT range reversal under strong bullish trend expansion (-30)")
		}

	case CROWDED_POSITIONING_SQUEEZE:
		// 1. Crowding evidence (Max 25)
		crowdScore := 5.0
		if fundingExtreme && supportsSqueezeDirection {
			crowdScore = 25.0
		} else if extremeOI == 1.0 {
			crowdScore = 15.0
		} else if fundingExtreme {
			crowdScore = 8.0
		}
		rawScore += crowdScore
		notes = append(notes, fmt.Sprintf("CrowdEvidence: +%0.1f", crowdScore))

		// 2. OI/funding context (Max 20)
		contextScore := 5.0
		if fundingExtreme && supportsSqueezeDirection && extremeOI == 1.0 {
			contextScore = 20.0
		} else if extremeOI == 1.0 || (fundingExtreme && supportsSqueezeDirection) {
			contextScore = 12.0
		}
		rawScore += contextScore
		notes = append(notes, fmt.Sprintf("OIFundContext: +%0.1f", contextScore))

		// 3. Rejection/reclaim quality (Max 25)
		reclaimScore := 5.0
		if paRejection == 1.0 {
			reclaimScore = 25.0
		}
		rawScore += reclaimScore
		notes = append(notes, fmt.Sprintf("ReclaimQual: +%0.1f", reclaimScore))

		// 4. Timing freshness (Max 15)
		timeScore := 5.0
		if !policy.RequireFreshEntry {
			timeScore = 15.0
		}
		rawScore += timeScore
		notes = append(notes, fmt.Sprintf("TimingFresh: +%0.1f", timeScore))

		// 5. RR (Max 15)
		rrScore := 0.0
		if rr >= 2.0 {
			rrScore = 15.0
		} else if rr >= 1.5 {
			rrScore = 10.0
		}
		rawScore += rrScore
		notes = append(notes, fmt.Sprintf("RRScore: +%0.1f", rrScore))

		// Playbook specific penalties
		if paRejection == 0.0 {
			rawScore -= 5.0
			notes = append(notes, "PENALTY: Squeeze setup missing price action confirmation (-30)")
		}
		if resolvedDirection == LONG && sweepLow == 0.0 {
			rawScore -= 25.0
			notes = append(notes, "PENALTY: LONG squeeze missing failed breakdown reclaim (-25)")
		}
		if resolvedDirection == SHORT && sweepHigh == 0.0 {
			rawScore -= 25.0
			notes = append(notes, "PENALTY: SHORT squeeze missing failed breakout rejection (-25)")
		}
		if !fundingExtreme && extremeOI == 0.0 {
			rawScore -= 25.0
			notes = append(notes, "PENALTY: Squeeze missing crowding derivatives data (-25)")
		}
		if fundingExtreme && !supportsSqueezeDirection {
			rawScore -= 5.0
			notes = append(notes, "PENALTY: Squeeze funding direction does not support proposed squeeze (-15)")
		}
	}

	// --- Global Penalties ---
	penalty := 0.0

	// 1. Melawan MarketPolicy direction
	if resolvedDirection == LONG && !policy.AllowLong {
		penalty += 25.0
		notes = append(notes, "GLOBAL PENALTY: LONG trades disallowed by policy (-25)")
	}
	if resolvedDirection == SHORT && !policy.AllowShort {
		penalty += 25.0
		notes = append(notes, "GLOBAL PENALTY: SHORT trades disallowed by policy (-25)")
	}

	// 2. Funding berat melawan arah
	if quant.Playbook != CROWDED_POSITIONING_SQUEEZE && resolvedDirection == LONG && fundingExtreme && fundingRate > 0 {
		penalty += 8.0
		notes = append(notes, "GLOBAL PENALTY: positive funding rate unfavorable for LONGs (-8)")
	}
	if quant.Playbook != CROWDED_POSITIONING_SQUEEZE && resolvedDirection == SHORT && fundingExtreme && fundingRate < 0 {
		penalty += 8.0
		notes = append(notes, "GLOBAL PENALTY: negative funding rate unfavorable for SHORTs (-8)")
	}

	// 3. Regime-wide volatility penalty (multiplicative, applied below)
	regime := policy.EffectiveRegime()
	if volPenalty, volPenaltyNote := resolveVolatilityRegimePenalty(quant.Playbook, regime); volPenalty > 0 {
		penalty += volPenalty
		notes = append(notes, volPenaltyNote)
	}

	// 4. Tier C saat chaos / high vol
	isChaos := regime == BTC_CHAOS || regime == HIGH_VOL
	if isChaos && quant.Tier == TierC {
		penalty += 10.0
		notes = append(notes, "GLOBAL PENALTY: Tier C trading under chaos/high vol regime (-10)")
	}

	// 5. Entry jauh dari closed price (> 1% mismatch)
	if resolvedDirection == LONG && quant.TriggerPrice < quant.StopLoss {
		penalty += 8.0
		notes = append(notes, "GLOBAL PENALTY: Entry price too far from closed price support (-8)")
	}

	// 6. ADX tidak cocok dengan playbook
	if quant.Playbook == TREND_PULLBACK && adxVal < 20.0 {
		penalty += 8.0
		notes = append(notes, "GLOBAL PENALTY: Trend pullback playbook requires ADX >= 20 (-8)")
	}
	if quant.Playbook == RANGE_EDGE_REVERSAL && adxVal > safetyADXExpansionCeiling {
		penalty += 8.0
		notes = append(notes, "GLOBAL PENALTY: Range edge reversal disallowed under ADX > safety expansion ceiling (-8)")
	}

	// 7. MFI / RSI anomaly
	if rsi > 80.0 || rsi < 20.0 {
		penalty += 8.0
		notes = append(notes, "GLOBAL PENALTY: RSI value is highly anomalous/exhausted (-8)")
	}

	// 8. Setup overextended
	if resolvedDirection == LONG && rsi > 70.0 {
		penalty += 8.0
		notes = append(notes, "GLOBAL PENALTY: LONG setup is overextended / RSI > 70 (-8)")
	}
	if resolvedDirection == SHORT && rsi < 30.0 && rsi > 0.0 {
		penalty += 8.0
		notes = append(notes, "GLOBAL PENALTY: SHORT setup is overextended / RSI < 30 (-8)")
	}

	// 9. RR invalid or poor
	if rr <= 0.0 {
		penalty += 30.0
		notes = append(notes, "GLOBAL PENALTY: Risk-to-Reward ratio is negative or invalid (-30)")
	} else if rr < 1.5 {
		penalty += 8.0
		notes = append(notes, "GLOBAL PENALTY: Poor Risk-to-Reward ratio (< 1.5) (-8)")
	}

	// 10. Contra-directional extreme symbol price move penalty
	// Penalizes LONG entries during heavy dumps and SHORT entries during heavy pumps.
	absPriceChange24h := math.Abs(quant.TechnicalSnapshot.PriceChange24h)
	if absPriceChange24h > 8.0 { // >8% 24h move on this specific symbol
		isContraMove := (resolvedDirection == LONG && quant.TechnicalSnapshot.PriceChange24h < 0) ||
			(resolvedDirection == SHORT && quant.TechnicalSnapshot.PriceChange24h > 0)
		if isContraMove {
			penalty += 10.0
			notes = append(notes, fmt.Sprintf("GLOBAL PENALTY: Contra-move entry during extreme 24h price change %0.1f%% (-10)", quant.TechnicalSnapshot.PriceChange24h))
		}
	}

	// Apply penalties
	rawScore -= penalty

	// Clamp rawScore between 0.0 and 100.0
	if rawScore < 0.0 {
		rawScore = 0.0
	}
	if rawScore > 100.0 {
		rawScore = 100.0
	}

	finalScore := rawScore / 10.0
	grade := GetGrade(finalScore)

	quant.Score = finalScore
	// Persist notes and breakdown back inside quant.Reason field
	quant.Reason = fmt.Sprintf("Score: %0.1f | Grade: %s | Breakdown: %s", finalScore, grade, strings.Join(notes, ", "))

	return finalScore
}

// GetGrade maps numerical score to qualitative Grades: S+, S, A, B.
func GetGrade(score float64) string {
	if score >= 8.5 {
		return "S+"
	}
	if score >= 7.8 {
		return "S"
	}
	if score >= 7.0 {
		return "A"
	}
	return "B"
}
