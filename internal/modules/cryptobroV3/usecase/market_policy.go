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

func resolvePolicyBaseline(name string) (MarketPolicy, bool) {
	reg := GetGlobalConfigRegistry()
	if reg != nil {
		if policy, found := reg.GetMarketPolicy(name); found {
			return policy, true
		}
	}
	return GetDefaultMarketPolicy(name)
}

func mustResolvePolicyBaseline(name string) MarketPolicy {
	policy, found := resolvePolicyBaseline(name)
	if found {
		return policy
	}
	return MarketPolicy{}
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

	policy = mustResolvePolicyBaseline("DEFAULT")
	policy.Regime = DEFAULT
	policy.BtcTrend = btcTrend
	policy.Reason = "Default normal policy"

	// 1. Check BTC Chaos first (takes precedence)
	if btcChaos > 0.8 {
		chaosPolicy := mustResolvePolicyBaseline("BTC_CHAOS")
		chaosPolicy.Regime = BTC_CHAOS
		chaosPolicy.BtcTrend = btcTrend
		chaosPolicy.Reason = "BTC_CHAOS active - strict restrictions applied"
		return chaosPolicy
	}

	// 2. Check BTC Dominance
	// If BTC dominance is high or ethBtcPerf shows ETH lagging significantly
	if ethBtcPerf < -0.05 || btcScore > 80.0 {
		domPolicy := mustResolvePolicyBaseline("BTC_DOMINANCE")
		domPolicy.Regime = BTC_DOMINANCE
		domPolicy.BtcTrend = btcTrend
		domPolicy.Reason = "BTC_DOMINANCE active - altcoins restricted"
		return domPolicy
	}

	// 3. Check ALT_SUPPORTIVE + BTC Bullish
	if ethBtcPerf >= 0.02 && btcTrend == "BULLISH" {
		altPolicy := mustResolvePolicyBaseline("ALT_SUPPORTIVE")
		altPolicy.Regime = ALT_SUPPORTIVE
		altPolicy.BtcTrend = btcTrend
		altPolicy.Reason = "ALT_SUPPORTIVE + BTC Bullish active - favorable conditions"
		return altPolicy
	}

	// 4. Check RISK_OFF + BTC Bearish
	if btcTrend == "BEARISH" || breadth < 0.3 {
		roPolicy := mustResolvePolicyBaseline("RISK_OFF")
		roPolicy.Regime = RISK_OFF
		roPolicy.BtcTrend = btcTrend
		roPolicy.Reason = "RISK_OFF + BTC Bearish active - short bias"
		return roPolicy
	}

	// 5. Check CHOP_RANGE
	if btcTrend == "SIDEWAYS" && volatility == "NORMAL" {
		chopPolicy := mustResolvePolicyBaseline("CHOP_RANGE")
		chopPolicy.Regime = CHOP_RANGE
		chopPolicy.BtcTrend = btcTrend
		chopPolicy.Reason = "CHOP_RANGE active - mean reversion only"
		return chopPolicy
	}

	// 6. Check COMPRESSION
	// Do not force the whole market into breakout-only behavior just because BTC is quiet.
	// A low-volatility macro regime should look broadly balanced first; otherwise LOW_VOL reversal mode is safer.
	if volatility == "LOW" && btcScore > 50.0 && isCompressionMacroContext(breadth) {
		return normalizeCompressionPolicy(mustResolvePolicyBaseline("COMPRESSION"), btcTrend)
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
