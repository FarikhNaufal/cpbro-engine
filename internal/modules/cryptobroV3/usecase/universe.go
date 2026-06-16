package usecase

import (
	"math"
	"sort"
	"strings"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
)

type UniverseUsecase struct {
	symbols []string
}

type UniverseThresholds struct {
	MinVolume        float64
	MaxFundingAbs    float64
	MaxPriceMove24h  float64
	MinFundingVolume float64
}

func NewUniverseUsecase() *UniverseUsecase {
	settings := getRuntimeSettings()
	symbols := settings.UniverseDefaultSymbols
	if len(symbols) == 0 {
		symbols = []string{"BTCUSDT", "ETHUSDT", "SOLUSDT", "BNBUSDT"}
	}
	return &UniverseUsecase{
		symbols: append([]string(nil), symbols...),
	}
}

func (uc *UniverseUsecase) GetSymbols() []string {
	return uc.symbols
}

// FilterUniverse evaluates all tickers and filters them into candidates and rejected lists.
func (uc *UniverseUsecase) FilterUniverse(
	tickers []dto.Ticker24h,
	fundingRates map[string]float64,
	policy MarketPolicy,
	hotSymbols map[string]HotSymbol,
) ([]UniverseCandidate, []UniverseRejected) {
	var candidates []UniverseCandidate
	var rejected []UniverseRejected
	volumeMap := make(map[string]float64, len(tickers))
	liquidityCeiling := 0.0

	for _, t := range tickers {
		sym := t.Symbol
		volumeMap[sym] = t.QuoteVolume

		// 1. Only USDT pairs
		if !strings.HasSuffix(sym, "USDT") {
			rejected = append(rejected, UniverseRejected{
				Symbol: sym,
				Status: UNIVERSE_REJECT,
				Reason: "not a USDT pair",
			})
			continue
		}

		// 2. Skip BTCUSDT
		if sym == "BTCUSDT" {
			rejected = append(rejected, UniverseRejected{
				Symbol: sym,
				Status: UNIVERSE_REJECT,
				Reason: "skipped BTCUSDT macro index",
			})
			continue
		}

		// 3. Skip abnormal symbols (leveraged tokens, stables, fiat pegs)
		if isAbnormal(sym) {
			rejected = append(rejected, UniverseRejected{
				Symbol: sym,
				Status: UNIVERSE_REJECT,
				Reason: "abnormal or fiat/stable peg symbol",
			})
			continue
		}

		tier := classifyUniverseTier(t.QuoteVolume)
		thresholds := GetEffectiveUniverseThresholds(policy, tier)

		// 4. Volume check (QuoteVolume represents USDT value)
		if t.QuoteVolume < thresholds.MinVolume {
			rejected = append(rejected, UniverseRejected{
				Symbol: sym,
				Status: UNIVERSE_REJECT,
				Reason: "volume below policy minimum threshold",
			})
			continue
		}

		// 5. Funding rate check
		fr := 0.0
		if val, ok := fundingRates[sym]; ok {
			fr = val
		}
		if math.Abs(fr) > thresholds.MaxFundingAbs && t.QuoteVolume < thresholds.MinFundingVolume {
			rejected = append(rejected, UniverseRejected{
				Symbol: sym,
				Status: UNIVERSE_REJECT,
				Reason: "funding rate exceeds limit without sufficient liquidity guardrail",
			})
			continue
		}

		// 6. Price change check
		if math.Abs(t.PriceChangePercent/100.0) > thresholds.MaxPriceMove24h {
			rejected = append(rejected, UniverseRejected{
				Symbol: sym,
				Status: UNIVERSE_REJECT,
				Reason: "24h price move exceeds policy limit",
			})
			continue
		}

		// 7. Determine Tier
		tierCMinVolume := getRuntimeSettings().UniverseTierCMinVolume
		if tierCMinVolume <= 0 {
			tierCMinVolume = 15000000.0
		}
		if tier == TierC && t.QuoteVolume < tierCMinVolume && t.QuoteVolume < thresholds.MinVolume {
			rejected = append(rejected, UniverseRejected{
				Symbol: sym,
				Status: UNIVERSE_REJECT,
				Reason: "volume below Tier C minimum requirement",
			})
			continue
		}

		// 8. Tier allowance check
		tierAllowed := false
		for _, allowedTier := range policy.AllowedTiers {
			if allowedTier == tier {
				tierAllowed = true
				break
			}
		}
		if !tierAllowed {
			rejected = append(rejected, UniverseRejected{
				Symbol: sym,
				Status: UNIVERSE_REJECT,
				Reason: "tier not allowed by active market policy",
			})
			continue
		}

		// Passed all base universe filters
		isHot := false
		var hotScore float64
		var hotSource string
		var hotRankType int
		baseSym := NormalizeBaseSymbol(sym)
		if h, ok := hotSymbols[baseSym]; ok {
			isHot = true
			hotScore = h.Score
			hotSource = h.Source
			hotRankType = h.RankType
		}
		activityScore := normalizeUniverseActivityScore(t.PriceChangePercent, thresholds)

		candidate := UniverseCandidate{
			Symbol:             sym,
			Tier:               tier,
			Status:             UNIVERSE_PASS,
			Notes:              "passed universe criteria",
			IsHot:              isHot,
			HotScore:           hotScore,
			HotSource:          hotSource,
			HotRankType:        hotRankType,
			HotOverlaySelected: false,
			ActivityScore:      activityScore,
		}
		candidates = append(candidates, candidate)
		if t.QuoteVolume > liquidityCeiling {
			liquidityCeiling = t.QuoteVolume
		}
	}

	// 9. Rank candidates by liquidity-first composite score.
	if liquidityCeiling <= 0 {
		liquidityCeiling = policy.MinVolume
	}
	for i := range candidates {
		volume := volumeMap[candidates[i].Symbol]
		candidates[i].LiquidityScore = normalizeUniverseLiquidityScore(volume, policy.MinVolume, liquidityCeiling)
		candidates[i].CompositeScore = calculateUniverseCompositeScore(policy, candidates[i])
	}

	sort.Slice(candidates, func(i, j int) bool {
		if math.Abs(candidates[i].CompositeScore-candidates[j].CompositeScore) > 0.0001 {
			return candidates[i].CompositeScore > candidates[j].CompositeScore
		}
		if math.Abs(candidates[i].LiquidityScore-candidates[j].LiquidityScore) > 0.0001 {
			return candidates[i].LiquidityScore > candidates[j].LiquidityScore
		}
		return volumeMap[candidates[i].Symbol] > volumeMap[candidates[j].Symbol]
	})

	// 10. Limit candidates to policy.MaxSymbols
	maxSymbols := policy.MaxSymbols
	if maxSymbols <= 0 {
		maxSymbols = 1
	}
	if len(candidates) > maxSymbols {
		excess := candidates[maxSymbols:]
		for _, c := range excess {
			rejected = append(rejected, UniverseRejected{
				Symbol: c.Symbol,
				Status: UNIVERSE_REJECT,
				Reason: "excluded due to MaxSymbols limit",
			})
		}
		candidates = candidates[:maxSymbols]
	}

	return candidates, rejected
}

func normalizeUniverseLiquidityScore(volume, floor, ceiling float64) float64 {
	settings := getRuntimeSettings()
	if volume <= 0 {
		return 0
	}
	if floor <= 0 {
		floor = settings.UniverseDefaultMinFundingVolume / 50.0
		if floor <= 0 {
			floor = 1000000.0
		}
	}
	if ceiling <= floor {
		return 1
	}
	logVolume := math.Log10(math.Max(volume, floor))
	logFloor := math.Log10(floor)
	logCeiling := math.Log10(ceiling)
	if logCeiling <= logFloor {
		return 1
	}
	return clampUniverseUnit((logVolume - logFloor) / (logCeiling - logFloor))
}

func normalizeUniverseActivityScore(priceChangePercent float64, thresholds UniverseThresholds) float64 {
	maxMove := thresholds.MaxPriceMove24h
	if maxMove <= 0 {
		return 0
	}
	activity := math.Abs(priceChangePercent/100.0) / maxMove
	return clampUniverseUnit(activity)
}

func calculateUniverseCompositeScore(policy MarketPolicy, candidate UniverseCandidate) float64 {
	liquidityWeight, activityWeight, hotWeight := getUniverseRankingWeights(policy)
	liquidityGatedActivity := candidate.ActivityScore * candidate.LiquidityScore
	hotScore := 0.0
	if candidate.IsHot {
		hotScore = clampUniverseUnit(candidate.HotScore/100.0) * clampUniverseHotBoost(policy.HotMaxBoost) * candidate.LiquidityScore
	}

	return (candidate.LiquidityScore * liquidityWeight) +
		(liquidityGatedActivity * activityWeight) +
		(hotScore * hotWeight)
}

func getUniverseRankingWeights(policy MarketPolicy) (liquidity float64, activity float64, hot float64) {
	settings := getRuntimeSettings()
	switch policy.EffectiveRegime() {
	case ALT_SUPPORTIVE:
		return settings.UniverseWeightLiquidityAlt, settings.UniverseWeightActivityAlt, settings.UniverseWeightHotAlt
	case COMPRESSION:
		return settings.UniverseWeightLiquidityCompression, settings.UniverseWeightActivityCompression, settings.UniverseWeightHotCompression
	case RISK_OFF:
		return settings.UniverseWeightLiquidityRiskOff, settings.UniverseWeightActivityRiskOff, settings.UniverseWeightHotRiskOff
	case BTC_CHAOS, HIGH_VOL:
		return settings.UniverseWeightLiquidityChaos, settings.UniverseWeightActivityChaos, settings.UniverseWeightHotChaos
	case LOW_VOL, CHOP_RANGE:
		return settings.UniverseWeightLiquidityLowVol, settings.UniverseWeightActivityLowVol, settings.UniverseWeightHotLowVol
	case BTC_DOMINANCE:
		return settings.UniverseWeightLiquidityDominance, settings.UniverseWeightActivityDominance, settings.UniverseWeightHotDominance
	default:
		return settings.UniverseWeightLiquidityDefault, settings.UniverseWeightActivityDefault, settings.UniverseWeightHotDefault
	}
}

func clampUniverseHotBoost(boost float64) float64 {
	settings := getRuntimeSettings()
	if boost <= 0 {
		boost = settings.UniverseDefaultHotBoost
		if boost <= 0 {
			boost = 1.25
		}
	}
	if boost < 1.0 {
		boost = 1.0
	}
	maxBoost := settings.UniverseMaxHotBoost
	if maxBoost < 1.0 {
		maxBoost = 1.5
	}
	if boost > maxBoost {
		boost = maxBoost
	}
	return boost
}

func clampUniverseUnit(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func classifyUniverseTier(quoteVolume float64) Tier {
	settings := getRuntimeSettings()
	tierAMin := settings.UniverseTierAMinQuoteVolume
	if tierAMin <= 0 {
		tierAMin = 150000000.0
	}
	tierBMin := settings.UniverseTierBMinQuoteVolume
	if tierBMin <= 0 {
		tierBMin = 50000000.0
	}
	if quoteVolume >= tierAMin {
		return TierA
	}
	if quoteVolume >= tierBMin {
		return TierB
	}
	return TierC
}

func GetEffectiveUniverseThresholds(policy MarketPolicy, tier Tier) UniverseThresholds {
	settings := getRuntimeSettings()
	minVolume := policy.MinVolume
	if minVolume <= 0 {
		minVolume = 1000000.0
	}
	maxFundingAbs := policy.MaxFundingAbs
	if maxFundingAbs <= 0 {
		maxFundingAbs = 0.01
	}
	maxMove := policy.MaxPriceMove24h
	if maxMove <= 0 {
		maxMove = 0.15
	}

	regime := policy.EffectiveRegime()
	recommendedMove := 0.15
	switch regime {
	case ALT_SUPPORTIVE:
		recommendedMove = 0.18
	case RISK_OFF:
		recommendedMove = 0.18
	case BTC_CHAOS:
		recommendedMove = 0.12
		minVolume = math.Max(minVolume, settings.UniverseChaosHighVolMinVolumeFloor)
	case HIGH_VOL:
		recommendedMove = 0.20
		minVolume = math.Max(minVolume, settings.UniverseChaosHighVolMinVolumeFloor)
	case LOW_VOL:
		recommendedMove = 0.12
		minVolume = math.Max(minVolume, settings.UniverseLowVolMinVolumeFloor)
	case CHOP_RANGE:
		recommendedMove = 0.12
	case COMPRESSION:
		recommendedMove = 0.15
	case BTC_DOMINANCE:
		recommendedMove = 0.15
	}

	switch tier {
	case TierA, TierB:
		if regime == ALT_SUPPORTIVE {
			recommendedMove = math.Max(recommendedMove, 0.18)
		}
	case TierC:
		if regime == ALT_SUPPORTIVE {
			recommendedMove = math.Min(recommendedMove, 0.15)
		} else if regime == BTC_CHAOS || regime == HIGH_VOL || regime == LOW_VOL {
			recommendedMove = math.Min(recommendedMove, 0.10)
		} else {
			recommendedMove = math.Min(recommendedMove, 0.12)
		}
		minVolume = math.Max(minVolume, settings.UniverseTierCMinVolume)
	}

	maxMove = math.Min(maxMove, recommendedMove)

	minFundingVolume := math.Max(minVolume*2.0, settings.UniverseDefaultMinFundingVolume)
	if regime == BTC_CHAOS || regime == HIGH_VOL {
		minFundingVolume = math.Max(minFundingVolume, settings.UniverseChaosMinFundingVolume)
	}

	return UniverseThresholds{
		MinVolume:        minVolume,
		MaxFundingAbs:    maxFundingAbs,
		MaxPriceMove24h:  maxMove,
		MinFundingVolume: minFundingVolume,
	}
}

func isAbnormal(sym string) bool {
	abnormalPatterns := []string{
		"USDCUSDT", "BUSDUSDT", "FDUSDUSDT", "TUSDUSDT", "EURUSDT", "GBPUSDT",
		"DAIUSDT", "AEURUSDT", "USDPUSDT", "UPUSDT", "DOWNUSDT", "BULLUSDT", "BEARUSDT",
	}
	for _, p := range abnormalPatterns {
		if strings.Contains(sym, p) {
			return true
		}
	}
	return false
}
