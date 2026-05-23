package usecase

import (
	"testing"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
)

func TestAICandidateSelector_SortingAndLimits(t *testing.T) {
	selector := NewAICandidateSelectorUsecase(7.5)

	policyNormal := MarketPolicy{
		MaxAICandidates: 3,
		Reason:          "Normal conditions",
	}

	m15HighVol := []dto.Candle{
		{Vol: 100}, {Vol: 100}, {Vol: 100}, {Vol: 100}, {Vol: 100},
		{Vol: 100}, {Vol: 100}, {Vol: 100}, {Vol: 100}, {Vol: 100}, {Vol: 300}, // volume ratio = 3.0
	}
	m15LowVol := []dto.Candle{
		{Vol: 100}, {Vol: 100}, {Vol: 100}, {Vol: 100}, {Vol: 100},
		{Vol: 100}, {Vol: 100}, {Vol: 100}, {Vol: 100}, {Vol: 100}, {Vol: 120}, // volume ratio = 1.2
	}

	candidates := []QuantResult{
		{
			Symbol:    "BTCUSDT",
			Direction: LONG,
			Playbook:  TREND_PULLBACK,
			Score:     8.5, // lower score than ETH
			Tier:      TierA,
			TradePlan: TradePlan{EntryPrice: 10.0, TakeProfit: 12.0, StopLoss: 9.0}, // RR = 2.0
		},
		{
			Symbol:    "ETHUSDT",
			Direction: LONG,
			Playbook:  TREND_PULLBACK,
			Score:     9.0, // highest score -> should sort 1st
			Tier:      TierA,
			TradePlan: TradePlan{EntryPrice: 10.0, TakeProfit: 12.0, StopLoss: 9.0}, // RR = 2.0
		},
		{
			Symbol:    "SOLUSDT",
			Direction: LONG,
			Playbook:  TREND_PULLBACK,
			Score:     8.5,
			Tier:      TierA,
			TradePlan: TradePlan{EntryPrice: 10.0, TakeProfit: 13.0, StopLoss: 9.0}, // RR = 3.0 (same score as BTC, higher RR -> sorts before BTC)
		},
		{
			Symbol:    "NEARUSDT",
			Direction: LONG,
			Playbook:  TREND_PULLBACK,
			Score:     8.0,
			Tier:      TierB,
			RawKlines: m15LowVol,
			TradePlan: TradePlan{EntryPrice: 10.0, TakeProfit: 12.0, StopLoss: 9.0}, // RR = 2.0
		},
		{
			Symbol:    "AVAXUSDT",
			Direction: LONG,
			Playbook:  TREND_PULLBACK,
			Score:     8.0,
			Tier:      TierB,
			RawKlines: m15HighVol,
			TradePlan: TradePlan{EntryPrice: 10.0, TakeProfit: 12.0, StopLoss: 9.0}, // RR = 2.0, higher volume ratio -> sorts before NEAR
		},
	}

	selected, skipped := selector.SelectCandidates(candidates, policyNormal)

	// Normal limit is 3, so length of selected should be 3
	if len(selected) != 3 {
		t.Errorf("Expected 3 selected candidates, got %d", len(selected))
	}
	if len(skipped) != 2 {
		t.Errorf("Expected 2 skipped candidates, got %d", len(skipped))
	}

	// Verify sorting order: ETH (9.0) -> SOL (8.5, RR 3.0) -> BTC (8.5, RR 2.0)
	if selected[0].Symbol != "ETHUSDT" {
		t.Errorf("Expected first selected candidate to be ETHUSDT, got %s", selected[0].Symbol)
	}
	if selected[1].Symbol != "SOLUSDT" {
		t.Errorf("Expected second selected candidate to be SOLUSDT, got %s", selected[1].Symbol)
	}
	if selected[2].Symbol != "BTCUSDT" {
		t.Errorf("Expected third selected candidate to be BTCUSDT, got %s", selected[2].Symbol)
	}

	// Verify volume ratio sorting order in skipped candidates: AVAXUSDT should be before NEARUSDT
	if skipped[0].Symbol != "AVAXUSDT" {
		t.Errorf("Expected first skipped candidate to be AVAXUSDT (due to higher vol ratio), got %s", skipped[0].Symbol)
	}
}

func TestAICandidateSelector_OpposingAndTiers(t *testing.T) {
	selector := NewAICandidateSelectorUsecase(7.5)

	// BTC Chaos regime (max 1 candidate, 0 Tier C candidates)
	policyChaos := MarketPolicy{
		MaxAICandidates: 0, // defaults to 1 under chaos
		Reason:          "BTC_CHAOS active",
	}

	candidates := []QuantResult{
		{
			Symbol:    "ETHUSDT",
			Direction: LONG,
			Playbook:  LIQUIDITY_SWEEP_REVERSAL,
			Score:     9.0,
			Tier:      TierA,
			TradePlan: TradePlan{EntryPrice: 10.0, TakeProfit: 12.0, StopLoss: 9.0},
		},
		{
			Symbol:    "ETHUSDT",
			Direction: SHORT, // opposing direction for same symbol
			Playbook:  LIQUIDITY_SWEEP_REVERSAL,
			Score:     8.8,
			Tier:      TierA,
			TradePlan: TradePlan{EntryPrice: 10.0, TakeProfit: 8.0, StopLoss: 11.0},
		},
		{
			Symbol:    "SOLUSDT",
			Direction: LONG,
			Playbook:  LIQUIDITY_SWEEP_REVERSAL,
			Score:     8.5,
			Tier:      TierC, // Tier C candidate
			TradePlan: TradePlan{EntryPrice: 10.0, TakeProfit: 12.0, StopLoss: 9.0},
		},
	}

	selected, skipped := selector.SelectCandidates(candidates, policyChaos)

	if len(selected) != 1 {
		t.Errorf("Expected 1 selected candidate under BTCChaos, got %d", len(selected))
	}
	if selected[0].Symbol != "ETHUSDT" || selected[0].Direction != LONG {
		t.Errorf("Expected ETHUSDT LONG to be selected, got %s %s", selected[0].Symbol, selected[0].Direction)
	}

	// Verify reason of skipped opposing candidate
	foundOpposing := false
	for _, sk := range skipped {
		if sk.Symbol == "ETHUSDT" && sk.Direction == SHORT {
			foundOpposing = true
			if sk.Status != LOCAL_WATCH || sk.Reason == "" {
				t.Errorf("Expected LOCAL_WATCH status and non-empty reason for skipped opposing ETHUSDT")
			}
		}
	}
	if !foundOpposing {
		t.Errorf("Expected opposing ETHUSDT SHORT to be in skipped list")
	}

	// Verify Tier C candidate was skipped under Chaos
	foundTierC := false
	for _, sk := range skipped {
		if sk.Symbol == "SOLUSDT" && sk.Tier == TierC {
			foundTierC = true
			if sk.Status != LOCAL_WATCH || sk.Reason == "" {
				t.Errorf("Expected LOCAL_WATCH status and non-empty reason for skipped Tier C under chaos")
			}
		}
	}
	if !foundTierC {
		t.Errorf("Expected SOLUSDT Tier C to be in skipped list under chaos")
	}
}
