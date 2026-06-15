package usecase

import (
	"context"
	"fmt"
	"math"
)

type MarketPolicyUsecase struct{}

func NewMarketPolicyUsecase() *MarketPolicyUsecase {
	return &MarketPolicyUsecase{}
}

func normalizeCompressionPolicy(policy MarketPolicy, btcTrend string) MarketPolicy {
	policy.Regime = COMPRESSION
	policy.BtcTrend = btcTrend
	policy.LongMode = NORMAL
	policy.ShortMode = NORMAL
	policy.AllowedPlaybooks = []Playbook{COMPRESSION_BREAKOUT_RETEST, LIQUIDITY_SWEEP_REVERSAL, RANGE_EDGE_REVERSAL}
	policy.RequireFreshEntry = true
	policy.Reason = "COMPRESSION active - breakout preferred, reversal fallback enabled"
	return policy
}

func normalizeLowVolPolicy(policy MarketPolicy, btcTrend string, reason string) MarketPolicy {
	policy.Regime = LOW_VOL
	policy.BtcTrend = btcTrend
	policy.MaxSymbols = 40
	policy.MaxAICandidates = 2
	policy.MinVolume = 750000.0
	policy.AllowedTiers = []Tier{TierA, TierB}
	policy.RequireFreshEntry = true
	policy.LongMode = REVERSAL_ONLY
	policy.ShortMode = REVERSAL_ONLY
	policy.AllowedPlaybooks = []Playbook{LIQUIDITY_SWEEP_REVERSAL, RANGE_EDGE_REVERSAL}
	policy.RequireAIConfidence = AIConfidenceHigh
	policy.MaxPriceMove24hLong = 0.10
	policy.MaxPriceMove24hShort = 0.10
	policy.Reason = reason
	return policy
}

// EvaluatePolicy generates operating constraints based on macro inputs.
func (uc *MarketPolicyUsecase) EvaluatePolicy(
	ctx context.Context,
	btcTrend string, // "BULLISH", "BEARISH", "SIDEWAYS"
	btcScore float64,
	ethBtcPerf float64,
	btcChaos float64,
	volatility string, // "HIGH", "LOW", "NORMAL"
	breadth float64,
) (policy MarketPolicy) {
	defer func() {
		policy.BtcScore = btcScore
		policy.BtcChaos = btcChaos
	}()

	found := false

	reg := GetGlobalConfigRegistry()
	if reg != nil {
		policy, found = reg.GetMarketPolicy("DEFAULT")
	}

	if !found {
		// Start with default policy
		policy = MarketPolicy{
			Regime:                 DEFAULT,
			AllowLong:              true,
			AllowShort:             true,
			LongMode:               NORMAL,
			ShortMode:              NORMAL,
			AllowedTiers:           []Tier{TierA, TierB, TierC},
			AllowedPlaybooks:       []Playbook{TREND_PULLBACK, LIQUIDITY_SWEEP_REVERSAL, COMPRESSION_BREAKOUT_RETEST, RANGE_EDGE_REVERSAL, CROWDED_POSITIONING_SQUEEZE},
			MaxSymbols:             50,
			MaxAICandidates:        3,
			MaxFinalExecute:        5,
			MinVolume:              1000000.0,
			MaxFundingAbs:          0.01,
			MaxPriceMove24h:        0.20,
			MaxPriceMove24hLong:    0.15,
			MaxPriceMove24hShort:   0.15,
			MinScoreAI:             6.0,
			MinScoreExecute:        7.0,
			MinRRExecute:           1.5,
			MinADXExecute:          20.0,
			RequireAIConfidence:    AIConfidenceMedium,
			RequireFreshEntry:      false,
			StalenessATRMultiplier: 1.5,
			CooldownMinutes:        15,
			BtcTrend:               btcTrend,
			Reason:                 "Default normal policy",
		}
	} else {
		policy.Regime = DEFAULT
		policy.BtcTrend = btcTrend
		policy.Reason = "Default normal policy"
	}

	// 1. Check BTC Chaos first (takes precedence)
	if btcChaos > 0.8 {
		if reg != nil {
			if chaosPolicy, foundChaos := reg.GetMarketPolicy("BTC_CHAOS"); foundChaos {
				chaosPolicy.Regime = BTC_CHAOS
				chaosPolicy.BtcTrend = btcTrend
				chaosPolicy.Reason = "BTC_CHAOS active - strict restrictions applied"
				return chaosPolicy
			}
		}
		policy.Regime = BTC_CHAOS
		policy.AllowedTiers = []Tier{TierA, TierB} // block Tier C
		policy.MaxSymbols = 35
		policy.MaxAICandidates = 1
		policy.MaxFinalExecute = 1
		policy.MinVolume = 10000000.0
		policy.MaxPriceMove24h = 0.12
		policy.MaxPriceMove24hLong = 0.05
		policy.MaxPriceMove24hShort = 0.12
		policy.MinScoreAI = 7.8
		policy.MinScoreExecute = 8.5
		policy.MinRRExecute = 2.0
		policy.RequireAIConfidence = AIConfidenceHigh
		policy.RequireFreshEntry = true
		policy.StalenessATRMultiplier = 0.5
		policy.Reason = "BTC_CHAOS active - strict restrictions applied"
		return policy
	}

	// 2. Check BTC Dominance
	// If BTC dominance is high or ethBtcPerf shows ETH lagging significantly
	if ethBtcPerf < -0.05 || btcScore > 80.0 {
		if reg != nil {
			if domPolicy, foundDom := reg.GetMarketPolicy("BTC_DOMINANCE"); foundDom {
				domPolicy.Regime = BTC_DOMINANCE
				domPolicy.BtcTrend = btcTrend
				domPolicy.Reason = "BTC_DOMINANCE active - altcoins restricted"
				return domPolicy
			}
		}
		policy.Regime = BTC_DOMINANCE
		policy.LongMode = PULLBACK_ONLY
		policy.ShortMode = SWEEP_ONLY
		policy.AllowedTiers = []Tier{TierA, TierB} // Tier C limited
		policy.AllowedPlaybooks = []Playbook{TREND_PULLBACK, LIQUIDITY_SWEEP_REVERSAL}
		policy.MaxSymbols = 50
		policy.MaxAICandidates = 2
		policy.MinScoreExecute = 7.2
		policy.MinVolume = 5000000.0 // Min volume raised
		policy.MaxPriceMove24hLong = 0.10
		policy.MaxPriceMove24hShort = 0.15
		policy.Reason = "BTC_DOMINANCE active - altcoins restricted"
		return policy
	}

	// 3. Check ALT_SUPPORTIVE + BTC Bullish
	if ethBtcPerf >= 0.02 && btcTrend == "BULLISH" {
		if reg != nil {
			if altPolicy, foundAlt := reg.GetMarketPolicy("ALT_SUPPORTIVE"); foundAlt {
				altPolicy.Regime = ALT_SUPPORTIVE
				altPolicy.BtcTrend = btcTrend
				altPolicy.Reason = "ALT_SUPPORTIVE + BTC Bullish active - favorable conditions"
				return altPolicy
			}
		}
		policy.Regime = ALT_SUPPORTIVE
		policy.LongMode = NORMAL
		policy.ShortMode = SWEEP_ONLY
		policy.AllowedPlaybooks = []Playbook{TREND_PULLBACK, COMPRESSION_BREAKOUT_RETEST, LIQUIDITY_SWEEP_REVERSAL}
		policy.AllowedTiers = []Tier{TierA, TierB, TierC}
		policy.MaxSymbols = 75
		policy.MaxAICandidates = 3
		policy.MinScoreExecute = 7.0
		policy.MinRRExecute = 1.5
		policy.MaxPriceMove24hLong = 0.18
		policy.MaxPriceMove24hShort = 0.10
		policy.Reason = "ALT_SUPPORTIVE + BTC Bullish active - favorable conditions"
		return policy
	}

	// 4. Check RISK_OFF + BTC Bearish
	if btcTrend == "BEARISH" || breadth < 0.3 {
		if reg != nil {
			if roPolicy, foundRo := reg.GetMarketPolicy("RISK_OFF"); foundRo {
				roPolicy.Regime = RISK_OFF
				roPolicy.BtcTrend = btcTrend
				roPolicy.Reason = "RISK_OFF + BTC Bearish active - short bias"
				return roPolicy
			}
		}
		policy.Regime = RISK_OFF
		policy.ShortMode = NORMAL
		policy.LongMode = REVERSAL_ONLY
		policy.AllowedTiers = []Tier{TierA, TierB}
		// Bear market: prioritize SHORT continuation (trend pullback) + defensive reversals.
		// LONG should remain reversal-only via LongMode=REVERSAL_ONLY.
		policy.AllowedPlaybooks = []Playbook{TREND_PULLBACK, LIQUIDITY_SWEEP_REVERSAL, RANGE_EDGE_REVERSAL}
		policy.MaxSymbols = 50
		policy.MaxAICandidates = 3
		policy.MinScoreExecute = 7.4
		policy.MinRRExecute = 1.7
		policy.MaxFundingAbs = 0.005       // tighter funding check
		policy.MaxPriceMove24h = 0.18      // allow liquid SHORT continuation while downstream gates restrict LONG reversal
		policy.MaxPriceMove24hLong = 0.08  // block LONG on symbols dumping >8%
		policy.MaxPriceMove24hShort = 0.18 // SHORT allowed on heavy dumps
		policy.Reason = "RISK_OFF + BTC Bearish active - short bias"
		return policy
	}

	// 5. Check CHOP_RANGE
	if btcTrend == "SIDEWAYS" && volatility == "NORMAL" {
		if reg != nil {
			if chopPolicy, foundChop := reg.GetMarketPolicy("CHOP_RANGE"); foundChop {
				chopPolicy.Regime = CHOP_RANGE
				chopPolicy.BtcTrend = btcTrend
				chopPolicy.Reason = "CHOP_RANGE active - mean reversion only"
				return chopPolicy
			}
		}
		policy.Regime = CHOP_RANGE
		policy.LongMode = REVERSAL_ONLY
		policy.ShortMode = REVERSAL_ONLY
		policy.AllowedPlaybooks = []Playbook{LIQUIDITY_SWEEP_REVERSAL, RANGE_EDGE_REVERSAL} // trend continuation/breakout disabled
		policy.MaxSymbols = 50
		policy.MaxAICandidates = 2
		policy.MinRRExecute = 1.8
		policy.RequireAIConfidence = AIConfidenceHigh
		policy.Reason = "CHOP_RANGE active - mean reversion only"
		return policy
	}

	// 6. Check COMPRESSION
	// Do not force the whole market into breakout-only behavior just because BTC is quiet.
	// A low-volatility macro regime should look broadly balanced first; otherwise LOW_VOL reversal mode is safer.
	if volatility == "LOW" && btcScore > 50.0 && isCompressionMacroContext(breadth) {
		if reg != nil {
			if compPolicy, foundComp := reg.GetMarketPolicy("COMPRESSION"); foundComp {
				return normalizeCompressionPolicy(compPolicy, btcTrend)
			}
		}
		policy.Regime = COMPRESSION
		policy.AllowedTiers = []Tier{TierA, TierB, TierC}
		policy.MaxAICandidates = 3
		return normalizeCompressionPolicy(policy, btcTrend)
	}

	// 7. Modifiers based on Volatility
	if volatility == "LOW" {
		policy = normalizeLowVolPolicy(policy, btcTrend, "LOW_VOL active - reversal/watch mode")
	} else if volatility == "HIGH" {
		policy.Regime = HIGH_VOL
		policy.MinVolume = 10000000.0              // higher volume limit
		policy.AllowedTiers = []Tier{TierA, TierB} // Tier C limited
		policy.MaxFinalExecute = 2                 // limit executes
		policy.StalenessATRMultiplier = 0.8        // stricter staleness
		// High volatility: avoid admitting breakout-retest compression plays.
		// Fresh production data shows they dominate rejects under HIGH_VOL instead of producing viable AI candidates.
		policy.AllowedPlaybooks = []Playbook{LIQUIDITY_SWEEP_REVERSAL, TREND_PULLBACK}
		policy.RequireAIConfidence = AIConfidenceHigh
		policy.RequireFreshEntry = true
		policy.MaxPriceMove24hLong = 0.08
		policy.MaxPriceMove24hShort = 0.10
		policy.Reason = "HIGH_VOL active - strict risk reduction mode"
	}

	return policy
}

// IsAllowed evaluates a specific symbol's metrics against the active MarketPolicy constraints.
func (uc *MarketPolicyUsecase) IsAllowed(
	symbol string,
	policy MarketPolicy,
	volume24h float64,
	fundingRate float64,
	priceMove24h float64,
	tier Tier,
) (bool, string) {
	// check volume
	if volume24h < policy.MinVolume {
		return false, fmt.Sprintf("volume %f below threshold %f", volume24h, policy.MinVolume)
	}

	// check funding rate
	if math.Abs(fundingRate) > policy.MaxFundingAbs {
		return false, fmt.Sprintf("funding rate %f exceeds max absolute limit %f", fundingRate, policy.MaxFundingAbs)
	}

	// check price move
	if math.Abs(priceMove24h) > policy.MaxPriceMove24h {
		return false, fmt.Sprintf("price move 24h %f exceeds limit %f", priceMove24h, policy.MaxPriceMove24h)
	}

	// check tier
	allowedTier := false
	for _, t := range policy.AllowedTiers {
		if t == tier {
			allowedTier = true
			break
		}
	}
	if !allowedTier {
		return false, fmt.Sprintf("tier %s not allowed by policy", tier)
	}

	return true, ""
}
