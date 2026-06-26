package usecase

import (
	"testing"

	"cpbro-engine/internal/modules/cryptobroV3/entity"
)

func TestApplyPolicySnapshotToLatestResult_UsesEffectiveExecutionFlags(t *testing.T) {
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
		RequireFreshEntry:   false,
		AllowedPlaybooks:    []Playbook{TREND_PULLBACK, LIQUIDITY_SWEEP_REVERSAL},
	}

	applyPolicySnapshotToLatestResult(latest, policy)

	if latest.ActivePolicyRequireAIConfidence != string(AIConfidenceHigh) {
		t.Fatalf("expected effective AI confidence HIGH, got %s", latest.ActivePolicyRequireAIConfidence)
	}
	if !latest.ActivePolicyRequireFreshEntry {
		t.Fatal("expected effective fresh-entry requirement to be true")
	}
	if latest.ActivePolicyShortMode != string(SWEEP_ONLY) {
		t.Fatalf("expected short mode snapshot, got %s", latest.ActivePolicyShortMode)
	}
}
