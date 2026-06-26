package usecase

import (
	"fmt"
	"math"
	"time"
)

type PlaybookEligibilityUsecase struct{}

func NewPlaybookEligibilityUsecase() *PlaybookEligibilityUsecase {
	return &PlaybookEligibilityUsecase{}
}

// Check is kept for compatibility with simpler strategy selector paths.
func (uc *PlaybookEligibilityUsecase) Check(symbol string, strategy string) bool {
	return true
}

// CheckEligibility evaluates the playbook against snapshot conditions and active policy constraints.
func (uc *PlaybookEligibilityUsecase) CheckEligibility(
	sel StrategySelection,
	policy MarketPolicy,
	data MarketData,
	tech *TechnicalSnapshot,
	structure *StructureSnapshot,
) PlaybookEligibilityResult {
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

	// Initialize Snapshots if nil
	if tech == nil {
		tech = &TechnicalSnapshot{IndicatorValues: make(map[string]float64)}
	}
	if tech.IndicatorValues == nil {
		tech.IndicatorValues = make(map[string]float64)
	}
	if structure == nil {
		structure = &StructureSnapshot{}
	}

	// Check allowed tiers
	tierAllowed := false
	for _, t := range policy.AllowedTiers {
		if t == sel.Tier {
			tierAllowed = true
			break
		}
	}
	if !tierAllowed {
		return PlaybookEligibilityResult{
			Playbook: playbook,
			Eligible: false,
			Status:   PLAYBOOK_REJECTED,
			Reason:   fmt.Sprintf("Tier %s is not allowed by active policy constraints", sel.Tier),
		}
	}

	// General Policy Direction Allow Checks
	if sel.Direction == LONG && !policy.AllowLong {
		return PlaybookEligibilityResult{
			Playbook: playbook,
			Eligible: false,
			Status:   PLAYBOOK_REJECTED,
			Reason:   "LONG direction is explicitly disabled by MarketPolicy",
		}
	}
	if sel.Direction == SHORT && !policy.AllowShort {
		return PlaybookEligibilityResult{
			Playbook: playbook,
			Eligible: false,
			Status:   PLAYBOOK_REJECTED,
			Reason:   "SHORT direction is explicitly disabled by MarketPolicy",
		}
	}
	if !ValidateDirectionalPath(policy, sel.Direction, playbook) {
		return PlaybookEligibilityResult{
			Playbook: playbook,
			Eligible: false,
			Status:   PLAYBOOK_REJECTED,
			Reason:   modeRejectReason(policy, sel.Direction, playbook),
		}
	}

	switch playbook {
	case TREND_PULLBACK:
		// Check direction-specific policy mode
		if sel.Direction == LONG {
			if policy.LongMode == REVERSAL_ONLY {
				return PlaybookEligibilityResult{
					Playbook: playbook,
					Eligible: false,
					Status:   PLAYBOOK_REJECTED,
					Reason:   "LONG trend pullback is disabled under REVERSAL_ONLY policy mode",
				}
			}
			if policy.LongMode == SWEEP_ONLY {
				return PlaybookEligibilityResult{
					Playbook: playbook,
					Eligible: false,
					Status:   PLAYBOOK_REJECTED,
					Reason:   "LONG trend pullback is disabled under SWEEP_ONLY policy mode",
				}
			}
			if policy.LongMode == BREAKOUT_RETEST_ONLY {
				return PlaybookEligibilityResult{
					Playbook: playbook,
					Eligible: false,
					Status:   PLAYBOOK_REJECTED,
					Reason:   "LONG trend pullback is disabled under BREAKOUT_RETEST_ONLY policy mode",
				}
			}
			if policy.LongMode == DISABLED {
				return PlaybookEligibilityResult{
					Playbook: playbook,
					Eligible: false,
					Status:   PLAYBOOK_REJECTED,
					Reason:   "LONG trades are disabled",
				}
			}
		} else if sel.Direction == SHORT {
			if policy.ShortMode == REVERSAL_ONLY {
				return PlaybookEligibilityResult{
					Playbook: playbook,
					Eligible: false,
					Status:   PLAYBOOK_REJECTED,
					Reason:   "SHORT trend pullback is disabled under REVERSAL_ONLY policy mode",
				}
			}
			if policy.ShortMode == SWEEP_ONLY {
				return PlaybookEligibilityResult{
					Playbook: playbook,
					Eligible: false,
					Status:   PLAYBOOK_REJECTED,
					Reason:   "SHORT trend pullback is disabled under SWEEP_ONLY policy mode",
				}
			}
			if policy.ShortMode == BREAKOUT_RETEST_ONLY {
				return PlaybookEligibilityResult{
					Playbook: playbook,
					Eligible: false,
					Status:   PLAYBOOK_REJECTED,
					Reason:   "SHORT trend pullback is disabled under BREAKOUT_RETEST_ONLY policy mode",
				}
			}
			if policy.ShortMode == DISABLED {
				return PlaybookEligibilityResult{
					Playbook: playbook,
					Eligible: false,
					Status:   PLAYBOOK_REJECTED,
					Reason:   "SHORT trades are disabled",
				}
			}
		}

		// 1. Trend direction check (H4/H1 trend must match direction)
		h4Trend := CalculateH4Trend(GetClosedCandlesOnly(data.H4Candles, 4*time.Hour), 200)
		h1Trend := CalculateH4Trend(GetClosedCandlesOnly(data.H1Candles, time.Hour), 50)

		regime := policy.EffectiveRegime()
		if !isTrendPullbackTrendAligned(sel.Direction, h4Trend, h1Trend, regime) {
			return PlaybookEligibilityResult{
				Playbook: playbook,
				Eligible: false,
				Status:   PLAYBOOK_REJECTED,
				Reason:   fmt.Sprintf("Trend alignment failed: H4 trend is %s, H1 trend is %s, but trade direction is %s", h4Trend, h1Trend, sel.Direction),
			}
		}

		// 1.5. Pullback to value area
		m15Closed := GetClosedCandlesOnly(data.M15Candles, defaultClosedCandleTimeframe)
		if ok, reason := validateTrendPullbackValueArea(m15Closed, sel.Direction); !ok {
			return PlaybookEligibilityResult{
				Playbook: playbook,
				Eligible: false,
				Status:   PLAYBOOK_REJECTED,
				Reason:   reason,
			}
		}

		// 2. Overextended check (RSI limit)
		if sel.Direction == LONG && tech.RSI > 70.0 {
			return PlaybookEligibilityResult{
				Playbook: playbook,
				Eligible: false,
				Status:   PLAYBOOK_REJECTED,
				Reason:   "RSI overextended (>70) for LONG pullback",
			}
		} else if sel.Direction == SHORT && tech.RSI < 30.0 && tech.RSI > 0.0 {
			return PlaybookEligibilityResult{
				Playbook: playbook,
				Eligible: false,
				Status:   PLAYBOOK_REJECTED,
				Reason:   "RSI overextended (<30) for SHORT pullback",
			}
		}

		// 3. ADX threshold check
		adx := tech.IndicatorValues[IndicatorADX]
		minADX := policy.MinADXExecute
		if minADX == 0 {
			minADX = 20.0
		}
		if adx > 0 && adx < minADX {
			return PlaybookEligibilityResult{
				Playbook: playbook,
				Eligible: false,
				Status:   PLAYBOOK_REJECTED,
				Reason:   fmt.Sprintf("ADX %f is below minimum requirement %f", adx, minADX),
			}
		}

		// 4. Chop range check (Not allowed unless permitted)
		if policy.Regime == CHOP_RANGE {
			allowed := false
			for _, p := range policy.AllowedPlaybooks {
				if p == TREND_PULLBACK {
					allowed = true
					break
				}
			}
			if !allowed {
				return PlaybookEligibilityResult{
					Playbook: playbook,
					Eligible: false,
					Status:   PLAYBOOK_REJECTED,
					Reason:   "Trend pullback is disabled during CHOP_RANGE regime",
				}
			}
		}

	case LIQUIDITY_SWEEP_REVERSAL:
		// Check direction-specific policy mode
		if sel.Direction == LONG {
			if policy.LongMode == PULLBACK_ONLY {
				return PlaybookEligibilityResult{
					Playbook: playbook,
					Eligible: false,
					Status:   PLAYBOOK_REJECTED,
					Reason:   "LONG sweep reversal is disabled under PULLBACK_ONLY policy mode",
				}
			}
			if policy.LongMode == BREAKOUT_RETEST_ONLY {
				return PlaybookEligibilityResult{
					Playbook: playbook,
					Eligible: false,
					Status:   PLAYBOOK_REJECTED,
					Reason:   "LONG sweep reversal is disabled under BREAKOUT_RETEST_ONLY policy mode",
				}
			}
			if policy.LongMode == DISABLED {
				return PlaybookEligibilityResult{
					Playbook: playbook,
					Eligible: false,
					Status:   PLAYBOOK_REJECTED,
					Reason:   "LONG trades are disabled",
				}
			}
		} else if sel.Direction == SHORT {
			if policy.ShortMode == PULLBACK_ONLY {
				return PlaybookEligibilityResult{
					Playbook: playbook,
					Eligible: false,
					Status:   PLAYBOOK_REJECTED,
					Reason:   "SHORT sweep reversal is disabled under PULLBACK_ONLY policy mode",
				}
			}
			if policy.ShortMode == BREAKOUT_RETEST_ONLY {
				return PlaybookEligibilityResult{
					Playbook: playbook,
					Eligible: false,
					Status:   PLAYBOOK_REJECTED,
					Reason:   "SHORT sweep reversal is disabled under BREAKOUT_RETEST_ONLY policy mode",
				}
			}
			if policy.ShortMode == DISABLED {
				return PlaybookEligibilityResult{
					Playbook: playbook,
					Eligible: false,
					Status:   PLAYBOOK_REJECTED,
					Reason:   "SHORT trades are disabled",
				}
			}
		}

		// 1. Sweep check and Close returned to range
		if sel.Direction == LONG {
			sweepLow := tech.IndicatorValues["sweep_low"]
			if sweepLow != 1.0 {
				return PlaybookEligibilityResult{
					Playbook: playbook,
					Eligible: false,
					Status:   PLAYBOOK_REJECTED,
					Reason:   "No lower liquidity sweep detected for LONG setup",
				}
			}
			m15Closed := GetClosedCandlesOnly(data.M15Candles, defaultClosedCandleTimeframe)
			if len(m15Closed) >= 21 {
				lowest20 := LowestLow(m15Closed[:len(m15Closed)-1], 20)
				lastClose := m15Closed[len(m15Closed)-1].Close
				if lastClose <= lowest20 {
					return PlaybookEligibilityResult{
						Playbook: playbook,
						Eligible: false,
						Status:   PLAYBOOK_REJECTED,
						Reason:   fmt.Sprintf("Sweep low invalid: close price %f did not return above range low %f", lastClose, lowest20),
					}
				}
			}
		} else if sel.Direction == SHORT {
			sweepHigh := tech.IndicatorValues["sweep_high"]
			if sweepHigh != 1.0 {
				return PlaybookEligibilityResult{
					Playbook: playbook,
					Eligible: false,
					Status:   PLAYBOOK_REJECTED,
					Reason:   "No upper liquidity sweep detected for SHORT setup",
				}
			}

			m15Closed := GetClosedCandlesOnly(data.M15Candles, defaultClosedCandleTimeframe)
			if len(m15Closed) >= 21 {
				highest20 := HighestHigh(m15Closed[:len(m15Closed)-1], 20)
				lastClose := m15Closed[len(m15Closed)-1].Close
				if lastClose >= highest20 {
					return PlaybookEligibilityResult{
						Playbook: playbook,
						Eligible: false,
						Status:   PLAYBOOK_REJECTED,
						Reason:   fmt.Sprintf("Sweep high invalid: close price %f did not return below range high %f", lastClose, highest20),
					}
				}
			}
		}

		// 2. Wick rejection and Pinbar check
		wickRejection := tech.IndicatorValues["wick_rejection"]
		isPinbarOnly := tech.IndicatorValues["pinbar_only"]
		if wickRejection == 0.0 && isPinbarOnly == 1.0 {
			return PlaybookEligibilityResult{
				Playbook: playbook,
				Eligible: false,
				Status:   PLAYBOOK_REJECTED,
				Reason:   "Pinbar without clear wick rejection / range reclaim",
			}
		}

		// 3. Volume spike confirmation
		m15Closed := GetClosedCandlesOnly(data.M15Candles, defaultClosedCandleTimeframe)
		minVolRatio := resolveConfiguredMinVolumeRatio(playbook, policy, sel.Tier)
		if !hasVolumeConfirmation(tech, m15Closed, minVolRatio) {
			return PlaybookEligibilityResult{
				Playbook: playbook,
				Eligible: false,
				Status:   PLAYBOOK_REJECTED,
				Reason:   "Liquidity sweep lacks volume spike confirmation",
			}
		}

	case COMPRESSION_BREAKOUT_RETEST:
		// Check direction-specific policy mode
		if sel.Direction == LONG {
			if policy.LongMode == PULLBACK_ONLY {
				return PlaybookEligibilityResult{
					Playbook: playbook,
					Eligible: false,
					Status:   PLAYBOOK_REJECTED,
					Reason:   "LONG compression breakout is disabled under PULLBACK_ONLY policy mode",
				}
			}
			if policy.LongMode == SWEEP_ONLY {
				return PlaybookEligibilityResult{
					Playbook: playbook,
					Eligible: false,
					Status:   PLAYBOOK_REJECTED,
					Reason:   "LONG compression breakout is disabled under SWEEP_ONLY policy mode",
				}
			}
			if policy.LongMode == REVERSAL_ONLY {
				return PlaybookEligibilityResult{
					Playbook: playbook,
					Eligible: false,
					Status:   PLAYBOOK_REJECTED,
					Reason:   "LONG compression breakout is disabled under REVERSAL_ONLY policy mode",
				}
			}
			if policy.LongMode == DISABLED {
				return PlaybookEligibilityResult{
					Playbook: playbook,
					Eligible: false,
					Status:   PLAYBOOK_REJECTED,
					Reason:   "LONG trades are disabled",
				}
			}
			if policy.EffectiveRegime() == DEFAULT {
				return PlaybookEligibilityResult{
					Playbook: playbook,
					Eligible: false,
					Status:   PLAYBOOK_REJECTED,
					Reason:   "LONG compression breakout retest is disabled in DEFAULT regime; reserve it for COMPRESSION or ALT_SUPPORTIVE conditions",
				}
			}
		} else if sel.Direction == SHORT {
			if policy.ShortMode == PULLBACK_ONLY {
				return PlaybookEligibilityResult{
					Playbook: playbook,
					Eligible: false,
					Status:   PLAYBOOK_REJECTED,
					Reason:   "SHORT compression breakout is disabled under PULLBACK_ONLY policy mode",
				}
			}
			if policy.ShortMode == SWEEP_ONLY {
				return PlaybookEligibilityResult{
					Playbook: playbook,
					Eligible: false,
					Status:   PLAYBOOK_REJECTED,
					Reason:   "SHORT compression breakout is disabled under SWEEP_ONLY policy mode",
				}
			}
			if policy.ShortMode == REVERSAL_ONLY {
				return PlaybookEligibilityResult{
					Playbook: playbook,
					Eligible: false,
					Status:   PLAYBOOK_REJECTED,
					Reason:   "SHORT compression breakout is disabled under REVERSAL_ONLY policy mode",
				}
			}
			if policy.ShortMode == DISABLED {
				return PlaybookEligibilityResult{
					Playbook: playbook,
					Eligible: false,
					Status:   PLAYBOOK_REJECTED,
					Reason:   "SHORT trades are disabled",
				}
			}
		}

		// 1. Contraction check
		contraction := tech.IndicatorValues["contraction"]
		bbWidth := tech.IndicatorValues["bb_width"]
		if contraction != 1.0 && (bbWidth <= 0 || bbWidth > compressionMaxBBWidth()) {
			return PlaybookEligibilityResult{
				Playbook: playbook,
				Eligible: false,
				Status:   PLAYBOOK_REJECTED,
				Reason:   "Contraction / BB width compression is not present",
			}
		}

		// 2. Breakout close valid & Retest check
		m15Closed := GetClosedCandlesOnly(data.M15Candles, defaultClosedCandleTimeframe)
		if len(m15Closed) >= 20 {
			_, upperBands, lowerBands := CalculateBollingerBands(m15Closed, 20, 2.0)
			if len(upperBands) > 0 && len(lowerBands) > 0 {
				lastClose := m15Closed[len(m15Closed)-1].Close
				upperBand := upperBands[len(upperBands)-1]
				lowerBand := lowerBands[len(lowerBands)-1]

				// Look at the past 5 closed candles for a valid breakout
				breakoutFound := false
				retestValid := false

				for i := len(m15Closed) - 2; i >= len(m15Closed)-6 && i >= 19; i-- {
					if sel.Direction == LONG && m15Closed[i].Close > upperBands[i] {
						breakoutFound = true
						break
					} else if sel.Direction == SHORT && m15Closed[i].Close < lowerBands[i] {
						breakoutFound = true
						break
					}
				}

				if !breakoutFound {
					return PlaybookEligibilityResult{
						Playbook: playbook,
						Eligible: false,
						Status:   PLAYBOOK_REJECTED,
						Reason:   "No valid breakout close outside Bollinger Bands in the last 5 candles",
					}
				}

				// Retest: price must have pulled back and is currently holding above the basis or upper band
				basisEMA := CalculateEMA(m15Closed, 20)
				if len(basisEMA) > 0 {
					basis := basisEMA[len(basisEMA)-1]
					if sel.Direction == LONG {
						if lastClose >= basis && lastClose <= upperBand*1.01 {
							retestValid = true
						}
					} else {
						if lastClose <= basis && lastClose >= lowerBand*0.99 {
							retestValid = true
						}
					}
				}

				if !retestValid {
					return PlaybookEligibilityResult{
						Playbook: playbook,
						Eligible: false,
						Status:   PLAYBOOK_REJECTED,
						Reason:   "Retest failed or invalid: price has broken back inside the channel or is too far",
					}
				}
			}
		}

		// 3. Breakout candle check (Do not entry first breakout candle)
		isFirstBreakout := tech.IndicatorValues["first_breakout_candle"]
		if isFirstBreakout == 1.0 {
			return PlaybookEligibilityResult{
				Playbook: playbook,
				Eligible: false,
				Status:   PLAYBOOK_REJECTED,
				Reason:   "Retest required: entry on first breakout candle is forbidden",
			}
		}

		// 4. Volume / OI expansion check
		volExpand := false
		minVolRatio := resolveConfiguredMinVolumeRatio(playbook, policy, sel.Tier)
		hasOIExpansion := GetIndicator(tech.IndicatorValues, IndicatorExtremeOI) == 1.0 || GetIndicator(tech.IndicatorValues, IndicatorOIChange) > 0
		volExpand = hasVolumeConfirmation(tech, m15Closed, minVolRatio) || hasOIExpansion
		if !volExpand {
			return PlaybookEligibilityResult{
				Playbook: playbook,
				Eligible: false,
				Status:   PLAYBOOK_REJECTED,
				Reason:   "Volume or Open Interest expansion not detected",
			}
		}

	case RANGE_EDGE_REVERSAL:
		// Check direction-specific policy mode
		if sel.Direction == LONG {
			if policy.LongMode == PULLBACK_ONLY {
				return PlaybookEligibilityResult{
					Playbook: playbook,
					Eligible: false,
					Status:   PLAYBOOK_REJECTED,
					Reason:   "LONG range edge reversal is disabled under PULLBACK_ONLY policy mode",
				}
			}
			if policy.LongMode == SWEEP_ONLY {
				return PlaybookEligibilityResult{
					Playbook: playbook,
					Eligible: false,
					Status:   PLAYBOOK_REJECTED,
					Reason:   "LONG range edge reversal is disabled under SWEEP_ONLY policy mode",
				}
			}
			if policy.LongMode == BREAKOUT_RETEST_ONLY {
				return PlaybookEligibilityResult{
					Playbook: playbook,
					Eligible: false,
					Status:   PLAYBOOK_REJECTED,
					Reason:   "LONG range edge reversal is disabled under BREAKOUT_RETEST_ONLY policy mode",
				}
			}
			if policy.LongMode == DISABLED {
				return PlaybookEligibilityResult{
					Playbook: playbook,
					Eligible: false,
					Status:   PLAYBOOK_REJECTED,
					Reason:   "LONG trades are disabled",
				}
			}
		} else if sel.Direction == SHORT {
			if policy.ShortMode == PULLBACK_ONLY {
				return PlaybookEligibilityResult{
					Playbook: playbook,
					Eligible: false,
					Status:   PLAYBOOK_REJECTED,
					Reason:   "SHORT range edge reversal is disabled under PULLBACK_ONLY policy mode",
				}
			}
			if policy.ShortMode == BREAKOUT_RETEST_ONLY {
				return PlaybookEligibilityResult{
					Playbook: playbook,
					Eligible: false,
					Status:   PLAYBOOK_REJECTED,
					Reason:   "SHORT range edge reversal is disabled under BREAKOUT_RETEST_ONLY policy mode",
				}
			}
			if policy.ShortMode == DISABLED {
				return PlaybookEligibilityResult{
					Playbook: playbook,
					Eligible: false,
					Status:   PLAYBOOK_REJECTED,
					Reason:   "SHORT trades are disabled",
				}
			}
		}

		// 1. Regime gate is enforced by MarketPolicy (AllowedPlaybooks + modes).
		// Avoid brittle substring checks against policy.Reason that can drift.

		// 1.5. ADX not expanding strongly check
		adx := tech.IndicatorValues[IndicatorADX]
		if adx > safetyADXExpansionCeiling {
			return PlaybookEligibilityResult{
				Playbook: playbook,
				Eligible: false,
				Status:   PLAYBOOK_REJECTED,
				Reason:   fmt.Sprintf("Range edge reversal invalid: strong trending expansion (ADX = %f > %0.1f)", adx, safetyADXExpansionCeiling),
			}
		}

		// 2. Price near range edge
		nearEdge := tech.IndicatorValues["near_range_edge"]
		if nearEdge != 1.0 {
			return PlaybookEligibilityResult{
				Playbook: playbook,
				Eligible: false,
				Status:   PLAYBOOK_REJECTED,
				Reason:   "Price is too far from the range highs/lows",
			}
		}

		// 3. Clear wick rejection check
		wickRejection := tech.IndicatorValues["wick_rejection"]
		if wickRejection != 1.0 {
			return PlaybookEligibilityResult{
				Playbook: playbook,
				Eligible: false,
				Status:   PLAYBOOK_REJECTED,
				Reason:   "Range edge reversal requires a clear wick rejection",
			}
		}

	case CROWDED_POSITIONING_SQUEEZE:
		// Check direction-specific policy mode
		if sel.Direction == LONG {
			if policy.LongMode == PULLBACK_ONLY {
				return PlaybookEligibilityResult{
					Playbook: playbook,
					Eligible: false,
					Status:   PLAYBOOK_REJECTED,
					Reason:   "LONG crowded squeeze is disabled under PULLBACK_ONLY policy mode",
				}
			}
			if policy.LongMode == SWEEP_ONLY {
				return PlaybookEligibilityResult{
					Playbook: playbook,
					Eligible: false,
					Status:   PLAYBOOK_REJECTED,
					Reason:   "LONG crowded squeeze is disabled under SWEEP_ONLY policy mode",
				}
			}
			if policy.LongMode == BREAKOUT_RETEST_ONLY {
				return PlaybookEligibilityResult{
					Playbook: playbook,
					Eligible: false,
					Status:   PLAYBOOK_REJECTED,
					Reason:   "LONG crowded squeeze is disabled under BREAKOUT_RETEST_ONLY policy mode",
				}
			}
			if policy.LongMode == DISABLED {
				return PlaybookEligibilityResult{
					Playbook: playbook,
					Eligible: false,
					Status:   PLAYBOOK_REJECTED,
					Reason:   "LONG trades are disabled",
				}
			}
		} else if sel.Direction == SHORT {
			if policy.ShortMode == PULLBACK_ONLY {
				return PlaybookEligibilityResult{
					Playbook: playbook,
					Eligible: false,
					Status:   PLAYBOOK_REJECTED,
					Reason:   "SHORT crowded squeeze is disabled under PULLBACK_ONLY policy mode",
				}
			}
			if policy.ShortMode == BREAKOUT_RETEST_ONLY {
				return PlaybookEligibilityResult{
					Playbook: playbook,
					Eligible: false,
					Status:   PLAYBOOK_REJECTED,
					Reason:   "SHORT crowded squeeze is disabled under BREAKOUT_RETEST_ONLY policy mode",
				}
			}
			if policy.ShortMode == DISABLED {
				return PlaybookEligibilityResult{
					Playbook: playbook,
					Eligible: false,
					Status:   PLAYBOOK_REJECTED,
					Reason:   "SHORT trades are disabled",
				}
			}
		}

		// 1. Funding / OI check
		extremeFunding := tech.IndicatorValues["extreme_funding"]
		extremeOI := tech.IndicatorValues["extreme_oi"]
		if extremeFunding != 1.0 && extremeOI != 1.0 && math.Abs(data.FundingRate) < fundingExtremeThreshold() {
			return PlaybookEligibilityResult{
				Playbook: playbook,
				Eligible: false,
				Status:   PLAYBOOK_REJECTED,
				Reason:   "OI / Funding positioning is not crowded/extreme enough",
			}
		}

		// 2. PA Rejection check
		rejectionPA := tech.IndicatorValues["pa_rejection"]
		if rejectionPA != 1.0 {
			return PlaybookEligibilityResult{
				Playbook: playbook,
				Eligible: false,
				Status:   PLAYBOOK_REJECTED,
				Reason:   "No price action rejection or reclaim confirmation detected",
			}
		}

		// 3. Crowd direction alignment check
		if data.FundingRate != 0 {
			if data.FundingRate > 0 && sel.Direction != SHORT {
				return PlaybookEligibilityResult{
					Playbook: playbook,
					Eligible: false,
					Status:   PLAYBOOK_REJECTED,
					Reason:   "LONG trade is invalid for crowded long positioning (must be SHORT squeeze)",
				}
			} else if data.FundingRate < 0 && sel.Direction != LONG {
				return PlaybookEligibilityResult{
					Playbook: playbook,
					Eligible: false,
					Status:   PLAYBOOK_REJECTED,
					Reason:   "SHORT trade is invalid for crowded short positioning (must be LONG squeeze)",
				}
			}
		}

	}

	return PlaybookEligibilityResult{
		Playbook: playbook,
		Eligible: true,
		Status:   PLAYBOOK_ELIGIBLE,
		Reason:   "Eligible for scoring and audit",
	}
}
