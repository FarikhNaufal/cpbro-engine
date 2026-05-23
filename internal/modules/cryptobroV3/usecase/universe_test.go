package usecase

import (
	"testing"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
)

func TestFilterUniverse(t *testing.T) {
	uc := NewUniverseUsecase()

	// Default Policy for Testing
	policy := MarketPolicy{
		AllowedTiers:    []Tier{TierA, TierB, TierC},
		MaxSymbols:      3,
		MinVolume:       1000000.0,
		MaxFundingAbs:   0.01, // 1%
		MaxPriceMove24h: 0.20, // 20%
	}

	tickers := []dto.Ticker24h{
		{Symbol: "BTCUSDT", QuoteVolume: 500000000.0, PriceChangePercent: 2.0},      // Should skip BTCUSDT macro index
		{Symbol: "ETHUSDT", QuoteVolume: 200000000.0, PriceChangePercent: 3.0},      // Tier A (>= 150M)
		{Symbol: "SOLUSDT", QuoteVolume: 100000000.0, PriceChangePercent: 5.0},      // Tier B (>= 50M)
		{Symbol: "XRPUSDT", QuoteVolume: 20000000.0, PriceChangePercent: 4.0},       // Tier C (< 50M)
		{Symbol: "DOGEUSDT", QuoteVolume: 5000000.0, PriceChangePercent: 1.0},       // Tier C (< 50M)
		{Symbol: "ADAUSDT", QuoteVolume: 500000.0, PriceChangePercent: 1.0},         // Below policy.MinVolume (1M)
		{Symbol: "LTCBTC", QuoteVolume: 2000000.0, PriceChangePercent: 1.0},         // Not a USDT pair
		{Symbol: "USDCUSDT", QuoteVolume: 50000000.0, PriceChangePercent: 0.1},      // Abnormal (stablecoin)
		{Symbol: "HIGHFUNDUSDT", QuoteVolume: 80000000.0, PriceChangePercent: 2.0},  // High funding rate
		{Symbol: "HIGHMOVEUSDT", QuoteVolume: 90000000.0, PriceChangePercent: 25.0}, // High 24h price move (25%)
	}

	fundingRates := map[string]float64{
		"ETHUSDT":      0.0001,
		"SOLUSDT":      0.0005,
		"XRPUSDT":      -0.0002,
		"DOGEUSDT":     0.0001,
		"HIGHFUNDUSDT": 0.015, // > MaxFundingAbs (0.01)
	}

	candidates, rejected := uc.FilterUniverse(tickers, fundingRates, policy)

	// Verify Candidates
	// Expected candidates passed: ETHUSDT, SOLUSDT, XRPUSDT, DOGEUSDT (sorted by volume)
	// But limit is MaxSymbols = 3. So only top 3 by volume: ETHUSDT, SOLUSDT, XRPUSDT.
	// DOGEUSDT should be rejected due to MaxSymbols limit.
	if len(candidates) != 3 {
		t.Fatalf("Expected 3 candidates, got %d", len(candidates))
	}

	// Verify sorting (volume desc)
	if candidates[0].Symbol != "ETHUSDT" || candidates[1].Symbol != "SOLUSDT" || candidates[2].Symbol != "XRPUSDT" {
		t.Errorf("Unexpected candidates or sorting: %+v", candidates)
	}

	// Verify Tiers
	if candidates[0].Tier != TierA {
		t.Errorf("Expected ETHUSDT to be TierA, got %v", candidates[0].Tier)
	}
	if candidates[1].Tier != TierB {
		t.Errorf("Expected SOLUSDT to be TierB, got %v", candidates[1].Tier)
	}
	if candidates[2].Tier != TierC {
		t.Errorf("Expected XRPUSDT to be TierC, got %v", candidates[2].Tier)
	}

	// Verify Rejected Lists and Reasons
	rejectedMap := make(map[string]string)
	for _, r := range rejected {
		rejectedMap[r.Symbol] = r.Reason
		if r.Status != UNIVERSE_REJECT {
			t.Errorf("Expected UNIVERSE_REJECT status for %s, got %v", r.Symbol, r.Status)
		}
		if r.Reason == "" {
			t.Errorf("Expected rejection reason for %s, got empty", r.Symbol)
		}
	}

	expectedRejections := map[string]string{
		"BTCUSDT":      "skipped BTCUSDT macro index",
		"LTCBTC":       "not a USDT pair",
		"USDCUSDT":     "abnormal or fiat/stable peg symbol",
		"ADAUSDT":      "volume below policy minimum threshold",
		"HIGHFUNDUSDT": "funding rate exceeds max absolute limit",
		"HIGHMOVEUSDT": "24h price move exceeds policy limit",
		"DOGEUSDT":     "excluded due to MaxSymbols limit",
	}

	for sym, expectedReason := range expectedRejections {
		reason, ok := rejectedMap[sym]
		if !ok {
			t.Errorf("Expected symbol %s to be rejected", sym)
		} else if reason != expectedReason {
			t.Errorf("Expected rejection reason for %s to be %q, got %q", sym, expectedReason, reason)
		}
	}
}

func TestFilterUniverseTiers(t *testing.T) {
	uc := NewUniverseUsecase()

	// Only allow TierA and TierB
	policy := MarketPolicy{
		AllowedTiers:    []Tier{TierA, TierB},
		MaxSymbols:      5,
		MinVolume:       1000000.0,
		MaxFundingAbs:   0.01,
		MaxPriceMove24h: 0.20,
	}

	tickers := []dto.Ticker24h{
		{Symbol: "ETHUSDT", QuoteVolume: 200000000.0, PriceChangePercent: 3.0}, // Tier A
		{Symbol: "SOLUSDT", QuoteVolume: 100000000.0, PriceChangePercent: 5.0}, // Tier B
		{Symbol: "XRPUSDT", QuoteVolume: 20000000.0, PriceChangePercent: 4.0},  // Tier C
	}

	candidates, rejected := uc.FilterUniverse(tickers, nil, policy)

	// XRPUSDT (Tier C) should be rejected due to Tier policy
	if len(candidates) != 2 {
		t.Fatalf("Expected 2 candidates, got %d", len(candidates))
	}
	if candidates[0].Symbol != "ETHUSDT" || candidates[1].Symbol != "SOLUSDT" {
		t.Errorf("Unexpected candidates: %+v", candidates)
	}

	foundXRP := false
	for _, r := range rejected {
		if r.Symbol == "XRPUSDT" {
			foundXRP = true
			if r.Reason != "tier not allowed by active market policy" {
				t.Errorf("Unexpected XRP rejection reason: %q", r.Reason)
			}
		}
	}
	if !foundXRP {
		t.Error("Expected XRPUSDT to be rejected")
	}
}

func TestFilterUniverseAbnormal(t *testing.T) {
	abnormals := []string{
		"USDCUSDT", "BUSDUSDT", "FDUSDUSDT", "TUSDUSDT", "EURUSDT", "GBPUSDT",
		"DAIUSDT", "AEURUSDT", "USDPUSDT", "ETHUPUSDT", "ETHDOWNUSDT", "BTCBULLUSDT", "BTCBEARUSDT",
	}

	for _, sym := range abnormals {
		if !isAbnormal(sym) {
			t.Errorf("Expected %s to be abnormal", sym)
		}
	}
}
