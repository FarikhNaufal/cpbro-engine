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
	return &UniverseUsecase{
		symbols: []string{"BTCUSDT", "ETHUSDT", "SOLUSDT", "BNBUSDT"},
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

	for _, t := range tickers {
		sym := t.Symbol

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
		if tier == TierC && t.QuoteVolume < 15000000.0 && t.QuoteVolume < thresholds.MinVolume {
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

		candidates = append(candidates, UniverseCandidate{
			Symbol:             sym,
			Tier:               tier,
			Status:             UNIVERSE_PASS,
			Notes:              "passed universe criteria",
			IsHot:              isHot,
			HotScore:           hotScore,
			HotSource:          hotSource,
			HotRankType:        hotRankType,
			HotOverlaySelected: false,
		})
	}

	// 9. Sort candidates by composite score (volume + hot boost) descending
	volumeMap := make(map[string]float64)
	for _, t := range tickers {
		volumeMap[t.Symbol] = t.QuoteVolume
	}

	maxBoost := policy.HotMaxBoost
	if maxBoost <= 0 {
		maxBoost = 1.25
	}

	compositeScoreMap := make(map[string]float64)
	for _, c := range candidates {
		volume := volumeMap[c.Symbol]
		if c.IsHot {
			scoreFactor := c.HotScore / 100.0
			if scoreFactor > 1.0 {
				scoreFactor = 1.0
			}
			if scoreFactor < 0.0 {
				scoreFactor = 0.0
			}
			boost := 1.0 + (maxBoost - 1.0)*scoreFactor
			compositeScoreMap[c.Symbol] = volume * boost
		} else {
			compositeScoreMap[c.Symbol] = volume
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return compositeScoreMap[candidates[i].Symbol] > compositeScoreMap[candidates[j].Symbol]
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

func classifyUniverseTier(quoteVolume float64) Tier {
	if quoteVolume >= 150000000.0 {
		return TierA
	}
	if quoteVolume >= 50000000.0 {
		return TierB
	}
	return TierC
}

func GetEffectiveUniverseThresholds(policy MarketPolicy, tier Tier) UniverseThresholds {
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
		minVolume = math.Max(minVolume, 10000000.0)
	case HIGH_VOL:
		recommendedMove = 0.20
		minVolume = math.Max(minVolume, 10000000.0)
	case LOW_VOL:
		recommendedMove = 0.12
		minVolume = math.Max(minVolume, 750000.0)
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
		minVolume = math.Max(minVolume, 15000000.0)
	}

	maxMove = math.Min(maxMove, recommendedMove)

	minFundingVolume := math.Max(minVolume*2.0, 50000000.0)
	if regime == BTC_CHAOS || regime == HIGH_VOL {
		minFundingVolume = math.Max(minFundingVolume, 150000000.0)
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
