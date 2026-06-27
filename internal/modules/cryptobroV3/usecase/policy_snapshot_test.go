package usecase

import (
	"testing"

	"cpbro-engine/internal/modules/cryptobroV3/entity"
)

func TestApplyPolicySnapshotToLatestResult_UsesPolicyExecutionFlags(t *testing.T) {
	original := SnapshotRuntimeSettings()
	t.Cleanup(func() { SetRuntimeSettings(original) })

	settings := original
	settings.RequireAIHighForExecute = true
	settings.RequireFreshEntryForExecute = true
	SetRuntimeSettings(settings)

	latest := &entity.LatestResult{}
	policy := MarketPolicy{
		LongMode:            NORMAL,
		ShortMode:           SWEEP_ONLY,
		RequireAIConfidence: AIConfidenceMedium,
		RequireFreshEntry:   true,
		AllowedPlaybooks:    []Playbook{TREND_PULLBACK, LIQUIDITY_SWEEP_REVERSAL},
	}

	applyPolicySnapshotToLatestResult(latest, policy)

	if latest.ActivePolicyRequireAIConfidence != string(AIConfidenceMedium) {
		t.Fatalf("expected policy AI confidence MEDIUM without global override, got %s", latest.ActivePolicyRequireAIConfidence)
	}
	if !latest.ActivePolicyRequireFreshEntry {
		t.Fatal("expected policy fresh-entry requirement to remain true")
	}
	if latest.ActivePolicyShortMode != string(SWEEP_ONLY) {
		t.Fatalf("expected short mode snapshot, got %s", latest.ActivePolicyShortMode)
	}
}
