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

		// 4. Volume check (QuoteVolume represents USDT value)
		if t.QuoteVolume < policy.MinVolume {
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
		if math.Abs(fr) > policy.MaxFundingAbs {
			rejected = append(rejected, UniverseRejected{
				Symbol: sym,
				Status: UNIVERSE_REJECT,
				Reason: "funding rate exceeds max absolute limit",
			})
			continue
		}

		// 6. Price change check
		if math.Abs(t.PriceChangePercent/100.0) > policy.MaxPriceMove24h {
			rejected = append(rejected, UniverseRejected{
				Symbol: sym,
				Status: UNIVERSE_REJECT,
				Reason: "24h price move exceeds policy limit",
			})
			continue
		}

		// 7. Determine Tier
		tier := TierC
		if t.QuoteVolume >= 150000000.0 {
			tier = TierA
		} else if t.QuoteVolume >= 50000000.0 {
			tier = TierB
		} else if t.QuoteVolume < 15000000.0 && t.QuoteVolume < policy.MinVolume {
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
		candidates = append(candidates, UniverseCandidate{
			Symbol: sym,
			Tier:   tier,
			Status: UNIVERSE_PASS,
			Notes:  "passed universe criteria",
		})
	}

	// 9. Sort candidates by volume descending
	volumeMap := make(map[string]float64)
	for _, t := range tickers {
		volumeMap[t.Symbol] = t.QuoteVolume
	}

	sort.Slice(candidates, func(i, j int) bool {
		return volumeMap[candidates[i].Symbol] > volumeMap[candidates[j].Symbol]
	})

	// 10. Limit candidates to policy.MaxSymbols
	if len(candidates) > policy.MaxSymbols {
		excess := candidates[policy.MaxSymbols:]
		for _, c := range excess {
			rejected = append(rejected, UniverseRejected{
				Symbol: c.Symbol,
				Status: UNIVERSE_REJECT,
				Reason: "excluded due to MaxSymbols limit",
			})
		}
		candidates = candidates[:policy.MaxSymbols]
	}

	return candidates, rejected
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
