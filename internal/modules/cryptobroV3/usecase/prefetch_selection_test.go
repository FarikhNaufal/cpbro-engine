package usecase

import "testing"

func TestSelectPrefetchCandidates_ReservesHotAndRotationSlots(t *testing.T) {
	policy := MarketPolicy{
		Regime:               DEFAULT,
		HotPrefetchSlotRatio: 0.34,
	}

	candidates := []UniverseCandidate{
		{Symbol: "AAAUSDT", LiquidityScore: 1.0, ActivityScore: 0.10, CompositeScore: 0.95},
		{Symbol: "HOT1USDT", IsHot: true, LiquidityScore: 0.90, ActivityScore: 0.30, CompositeScore: 0.90},
		{Symbol: "BBBUSDT", LiquidityScore: 0.88, ActivityScore: 0.20, CompositeScore: 0.88},
		{Symbol: "HOT2USDT", IsHot: true, LiquidityScore: 0.84, ActivityScore: 0.40, CompositeScore: 0.84},
		{Symbol: "CCCUSDT", LiquidityScore: 0.82, ActivityScore: 0.25, CompositeScore: 0.82},
		{Symbol: "DDDUSDT", LiquidityScore: 0.80, ActivityScore: 0.15, CompositeScore: 0.80},
		{Symbol: "ROTUSDT", LiquidityScore: 0.62, ActivityScore: 0.90, CompositeScore: 0.62},
	}

	selected, deferred, debug := selectPrefetchCandidates(candidates, 6, policy)
	if len(selected) != 6 {
		t.Fatalf("expected 6 selected candidates, got %d", len(selected))
	}
	if len(deferred) != 1 || deferred[0].Symbol != "DDDUSDT" {
		t.Fatalf("expected DDDUSDT to be deferred after rotation reservation, got %+v", deferred)
	}
	if debug.HotSlots != 2 {
		t.Fatalf("expected 2 hot slots, got %d", debug.HotSlots)
	}
	if debug.RotationSlots != 1 {
		t.Fatalf("expected 1 rotation slot, got %d", debug.RotationSlots)
	}

	foundHot1 := false
	foundHot2 := false
	foundRotation := false
	for _, candidate := range selected {
		if candidate.Symbol == "HOT1USDT" && candidate.HotOverlaySelected {
			foundHot1 = true
		}
		if candidate.Symbol == "HOT2USDT" && candidate.HotOverlaySelected {
			foundHot2 = true
		}
		if candidate.Symbol == "ROTUSDT" {
			foundRotation = true
		}
	}

	if !foundHot1 || !foundHot2 {
		t.Fatalf("expected both hot candidates to be reserved in prefetch selection")
	}
	if !foundRotation {
		t.Fatalf("expected high-activity rotation candidate to be selected even outside plain top-liquidity slice")
	}
}

func TestResolveRotationPrefetchSlots_RegimeAware(t *testing.T) {
	if slots := resolveRotationPrefetchSlots(MarketPolicy{Regime: RISK_OFF}, 10, 3); slots != 1 {
		t.Fatalf("expected RISK_OFF to reserve 1 rotation slot, got %d", slots)
	}
	if slots := resolveRotationPrefetchSlots(MarketPolicy{Regime: ALT_SUPPORTIVE}, 10, 3); slots != 2 {
		t.Fatalf("expected ALT_SUPPORTIVE to reserve 2 rotation slots, got %d", slots)
	}
	if slots := resolveRotationPrefetchSlots(MarketPolicy{Regime: DEFAULT}, 5, 3); slots != 0 {
		t.Fatalf("expected no rotation slots below minimum prefetch size, got %d", slots)
	}
}

func TestResolveRotationThresholds_UseRuntimeSettings(t *testing.T) {
	original := SnapshotRuntimeSettings()
	t.Cleanup(func() { SetRuntimeSettings(original) })

	settings := original
	settings.RotationActivityThresholdAlt = 0.33
	settings.RotationPrefetchRatioAlt = 0.30
	SetRuntimeSettings(settings)

	if threshold := resolveRotationActivityThreshold(MarketPolicy{Regime: ALT_SUPPORTIVE}); threshold != 0.33 {
		t.Fatalf("expected runtime rotation activity threshold 0.33, got %0.2f", threshold)
	}
	if slots := resolveRotationPrefetchSlots(MarketPolicy{Regime: ALT_SUPPORTIVE}, 10, 5); slots != 3 {
		t.Fatalf("expected runtime rotation prefetch ratio to reserve 3 slots, got %d", slots)
	}
}
