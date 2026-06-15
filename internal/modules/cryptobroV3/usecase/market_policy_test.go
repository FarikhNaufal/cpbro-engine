package usecase

import (
	"context"
	"testing"
)

func TestEvaluatePolicy_HighVolExcludesCompressionBreakout(t *testing.T) {
	uc := NewMarketPolicyUsecase()

	policy := uc.EvaluatePolicy(
		context.Background(),
		"BULLISH",
		55.0,
		0.0,
		0.0,
		"HIGH",
		0.60,
	)

	if policy.Regime != HIGH_VOL {
		t.Fatalf("expected HIGH_VOL regime, got %s", policy.Regime)
	}

	for _, playbook := range policy.AllowedPlaybooks {
		if playbook == COMPRESSION_BREAKOUT_RETEST {
			t.Fatalf("HIGH_VOL policy should not allow %s", COMPRESSION_BREAKOUT_RETEST)
		}
	}

	expected := map[Playbook]bool{
		LIQUIDITY_SWEEP_REVERSAL: true,
		TREND_PULLBACK:           true,
	}
	if len(policy.AllowedPlaybooks) != len(expected) {
		t.Fatalf("expected %d HIGH_VOL playbooks, got %d", len(expected), len(policy.AllowedPlaybooks))
	}
	for _, playbook := range policy.AllowedPlaybooks {
		if !expected[playbook] {
			t.Fatalf("unexpected HIGH_VOL playbook: %s", playbook)
		}
	}
}
