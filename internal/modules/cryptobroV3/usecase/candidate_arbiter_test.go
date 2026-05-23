package usecase

import (
	"math"
	"testing"
)

func TestCandidateArbiter_SameDirectionTieBreaker(t *testing.T) {
	arbiter := NewCandidateArbiterUsecase()

	policy := MarketPolicy{
		Reason:           "Default normal policy",
		AllowLong:        true,
		AllowShort:       true,
		AllowedTiers:     []Tier{TierA, TierB, TierC},
		AllowedPlaybooks: []Playbook{TREND_PULLBACK, LIQUIDITY_SWEEP_REVERSAL, COMPRESSION_BREAKOUT_RETEST, RANGE_EDGE_REVERSAL, CROWDED_POSITIONING_SQUEEZE},
	}

	// Two LONG candidates for the same symbol
	candidates := []QuantResult{
		{
			Symbol:    "BTCUSDT",
			Direction: LONG,
			Playbook:  COMPRESSION_BREAKOUT_RETEST,
			Score:     7.5,
			Tier:      TierA,
		},
		{
			Symbol:    "BTCUSDT",
			Direction: LONG,
			Playbook:  TREND_PULLBACK,
			Score:     7.55, // Score difference is 0.05 (< 0.1)
			Tier:      TierA,
		},
	}

	// Default priorities: TREND_PULLBACK (0) > COMPRESSION_BREAKOUT_RETEST (2)
	// So TREND_PULLBACK should win because the score difference is within 0.1 and it has higher priority.
	selected, rejected := arbiter.Arbitrate(candidates, policy)

	if len(selected) != 1 {
		t.Fatalf("Expected 1 selected candidate, got %d", len(selected))
	}
	if selected[0].Playbook != TREND_PULLBACK {
		t.Errorf("Expected TREND_PULLBACK to win, got %s", selected[0].Playbook)
	}
	if len(rejected) != 1 {
		t.Fatalf("Expected 1 rejected candidate, got %d", len(rejected))
	}
}

func TestCandidateArbiter_OpposingDirectionConflict(t *testing.T) {
	arbiter := NewCandidateArbiterUsecase()

	policy := MarketPolicy{
		Reason:           "Default normal policy",
		AllowLong:        true,
		AllowShort:       true,
		AllowedTiers:     []Tier{TierA, TierB, TierC},
		AllowedPlaybooks: []Playbook{TREND_PULLBACK, LIQUIDITY_SWEEP_REVERSAL, COMPRESSION_BREAKOUT_RETEST, RANGE_EDGE_REVERSAL, CROWDED_POSITIONING_SQUEEZE},
	}

	// Opposing directions (LONG and SHORT) for the same symbol
	candidates := []QuantResult{
		{
			Symbol:    "ETHUSDT",
			Direction: LONG,
			Playbook:  TREND_PULLBACK,
			Score:     8.2,
			Tier:      TierA,
		},
		{
			Symbol:    "ETHUSDT",
			Direction: SHORT,
			Playbook:  TREND_PULLBACK,
			Score:     7.3, // Score difference is 0.9 (>= 0.7)
			Tier:      TierA,
		},
	}

	// LONG should win since score difference is >= 0.7
	selected, rejected := arbiter.Arbitrate(candidates, policy)

	if len(selected) != 1 {
		t.Fatalf("Expected 1 selected candidate, got %d", len(selected))
	}
	if selected[0].Direction != LONG {
		t.Errorf("Expected LONG direction to win, got %s", selected[0].Direction)
	}
	if len(rejected) != 1 {
		t.Fatalf("Expected 1 rejected candidate, got %d", len(rejected))
	}
}

func TestCandidateArbiter_BTCChaosRules(t *testing.T) {
	arbiter := NewCandidateArbiterUsecase()

	policy := MarketPolicy{
		Reason:           "BTC_CHAOS active",
		AllowLong:        true,
		AllowShort:       true,
		AllowedTiers:     []Tier{TierA, TierB, TierC},
		AllowedPlaybooks: []Playbook{TREND_PULLBACK, LIQUIDITY_SWEEP_REVERSAL, COMPRESSION_BREAKOUT_RETEST, RANGE_EDGE_REVERSAL, CROWDED_POSITIONING_SQUEEZE},
	}

	candidates := []QuantResult{
		{
			Symbol:    "SOLUSDT",
			Direction: LONG,
			Playbook:  TREND_PULLBACK,
			Score:     9.0, // S+ but not Sweep/Squeeze
			Tier:      TierA,
		},
		{
			Symbol:    "SOLUSDT",
			Direction: LONG,
			Playbook:  LIQUIDITY_SWEEP_REVERSAL,
			Score:     8.6, // S+ Sweep
			Tier:      TierA,
		},
	}

	// Under BTCChaos, TREND_PULLBACK is filtered out, leaving only the S+ Sweep.
	selected, rejected := arbiter.Arbitrate(candidates, policy)

	if len(selected) != 1 {
		t.Fatalf("Expected 1 selected candidate, got %d", len(selected))
	}
	if selected[0].Playbook != LIQUIDITY_SWEEP_REVERSAL {
		t.Errorf("Expected LIQUIDITY_SWEEP_REVERSAL, got %s", selected[0].Playbook)
	}
	if len(rejected) != 1 {
		t.Fatalf("Expected 1 rejected candidate, got %d", len(rejected))
	}
}

func TestCandidateArbiter_NaNSafetyGuard(t *testing.T) {
	arbiter := NewCandidateArbiterUsecase()

	policy := MarketPolicy{
		Reason:           "Default normal policy",
		AllowLong:        true,
		AllowShort:       true,
		AllowedTiers:     []Tier{TierA, TierB, TierC},
		AllowedPlaybooks: []Playbook{TREND_PULLBACK, LIQUIDITY_SWEEP_REVERSAL},
	}

	candidates := []QuantResult{
		{
			Symbol:    "ADAUSDT",
			Direction: LONG,
			Playbook:  TREND_PULLBACK,
			Score:     math.NaN(),
			Tier:      TierA,
		},
	}

	selected, rejected := arbiter.Arbitrate(candidates, policy)

	if len(selected) != 0 {
		t.Errorf("Expected 0 selected candidates for NaN score, got %d", len(selected))
	}
	if len(rejected) != 1 {
		t.Errorf("Expected 1 rejected candidate, got %d", len(rejected))
	}
}
