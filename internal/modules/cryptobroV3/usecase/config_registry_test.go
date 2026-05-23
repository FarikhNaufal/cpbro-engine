package usecase

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigRegistry_Fallback(t *testing.T) {
	// Loading non-existent files
	reg, err := LoadConfigRegistry("nonexistent_policy.json", "nonexistent_playbook.json")
	if err != nil {
		t.Fatalf("LoadConfigRegistry should not error on missing files: %v", err)
	}

	if reg.GetVersion() != "policy:default-code(none);playbook:default-code(none)" {
		t.Errorf("Expected default-code version, got: %s", reg.GetVersion())
	}

	// Check that we got standard default values
	policy, ok := reg.GetMarketPolicy("DEFAULT")
	if !ok {
		t.Fatal("Missing DEFAULT policy in fallback")
	}
	if policy.MinScoreExecute != 7.0 {
		t.Errorf("Expected default MinScoreExecute to be 7.0, got %f", policy.MinScoreExecute)
	}

	profile, ok := reg.GetPlaybookProfile(LIQUIDITY_SWEEP_REVERSAL)
	if !ok {
		t.Fatal("Missing LIQUIDITY_SWEEP_REVERSAL profile in fallback")
	}
	if !profile.RequireVolumeConfirm {
		t.Error("Expected default LIQUIDITY_SWEEP_REVERSAL to require volume confirm")
	}
}

func TestConfigRegistry_ParseResilience(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config_registry_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	badJSON := `{"version": "v2.0.0", "policies": { "DEFAULT": { "min_score_execute": "invalid-string" } }` // Malformed JSON

	policyPath := filepath.Join(tmpDir, "policy.json")
	playbookPath := filepath.Join(tmpDir, "playbook.json")

	if err := os.WriteFile(policyPath, []byte(badJSON), 0644); err != nil {
		t.Fatalf("failed to write bad json: %v", err)
	}
	if err := os.WriteFile(playbookPath, []byte(badJSON), 0644); err != nil {
		t.Fatalf("failed to write bad json: %v", err)
	}

	reg, err := LoadConfigRegistry(policyPath, playbookPath)
	if err != nil {
		t.Fatalf("LoadConfigRegistry should not error on malformed JSON: %v", err)
	}

	// Should fallback to default values
	policy, ok := reg.GetMarketPolicy("DEFAULT")
	if !ok {
		t.Fatal("Missing DEFAULT policy in fallback")
	}
	if policy.MinScoreExecute != 7.0 {
		t.Errorf("Expected fallback MinScoreExecute 7.0, got %f", policy.MinScoreExecute)
	}
}

func TestConfigRegistry_SafetyCompliance(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config_registry_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// User attempts to weaken limits below safety bounds:
	// MinScoreExecute = 5.0 (clamped to 7.0)
	// MinRRExecute = 1.0 (clamped to 1.5)
	// MaxFinalExecute = 0 (clamped to 1)
	weakPolicyJSON := `{
		"version": "v1.1.0",
		"policies": {
			"DEFAULT": {
				"min_score_execute": 5.0,
				"min_rr_execute": 1.0,
				"max_final_execute": 0,
				"allow_long": true,
				"allow_short": true
			}
		}
	}`

	// Playbook sweeps trying to disable safety flags
	weakPlaybookJSON := `{
		"version": "v1.2.0",
		"profiles": {
			"LIQUIDITY_SWEEP_REVERSAL": {
				"playbook": "LIQUIDITY_SWEEP_REVERSAL",
				"require_volume_confirm": false,
				"min_volume_ratio": 0.5,
				"require_rejection": false,
				"require_confirmation": false
			},
			"COMPRESSION_BREAKOUT_RETEST": {
				"playbook": "COMPRESSION_BREAKOUT_RETEST",
				"require_retest": false,
				"require_confirmation": false
			},
			"CROWDED_POSITIONING_SQUEEZE": {
				"playbook": "CROWDED_POSITIONING_SQUEEZE",
				"min_score_execute": 6.5,
				"require_rejection": false,
				"require_confirmation": false
			}
		}
	}`

	policyPath := filepath.Join(tmpDir, "policy.json")
	playbookPath := filepath.Join(tmpDir, "playbook.json")

	if err := os.WriteFile(policyPath, []byte(weakPolicyJSON), 0644); err != nil {
		t.Fatalf("failed to write policy: %v", err)
	}
	if err := os.WriteFile(playbookPath, []byte(weakPlaybookJSON), 0644); err != nil {
		t.Fatalf("failed to write playbook: %v", err)
	}

	reg, err := LoadConfigRegistry(policyPath, playbookPath)
	if err != nil {
		t.Fatalf("LoadConfigRegistry failed: %v", err)
	}

	// Verify clamping
	policy, _ := reg.GetMarketPolicy("DEFAULT")
	if policy.MinScoreExecute != 7.0 {
		t.Errorf("Expected MinScoreExecute clamped to 7.0, got %f", policy.MinScoreExecute)
	}
	if policy.MinRRExecute != 1.5 {
		t.Errorf("Expected MinRRExecute clamped to 1.5, got %f", policy.MinRRExecute)
	}
	if policy.MaxFinalExecute != 1 {
		t.Errorf("Expected MaxFinalExecute clamped to 1, got %d", policy.MaxFinalExecute)
	}

	// Verify Liquidity Sweep constraints
	sweep, _ := reg.GetPlaybookProfile(LIQUIDITY_SWEEP_REVERSAL)
	if !sweep.RequireVolumeConfirm {
		t.Error("Expected sweep RequireVolumeConfirm overridden to true")
	}
	if sweep.MinVolumeRatio != 1.1 {
		t.Errorf("Expected sweep MinVolumeRatio overridden to 1.1, got %f", sweep.MinVolumeRatio)
	}
	if !sweep.RequireRejection || !sweep.RequireConfirmation {
		t.Error("Expected sweep rejection and confirmation overridden to true")
	}

	// Verify Breakout Retest constraints
	breakout, _ := reg.GetPlaybookProfile(COMPRESSION_BREAKOUT_RETEST)
	if !breakout.RequireRetest {
		t.Error("Expected breakout RequireRetest overridden to true")
	}
	if !breakout.RequireConfirmation {
		t.Error("Expected breakout RequireConfirmation overridden to true")
	}

	// Verify Crowded Squeeze constraints
	squeeze, _ := reg.GetPlaybookProfile(CROWDED_POSITIONING_SQUEEZE)
	if squeeze.MinScoreExecute != 7.8 {
		t.Errorf("Expected squeeze MinScoreExecute clamped to 7.8, got %f", squeeze.MinScoreExecute)
	}
	if !squeeze.RequireRejection || !squeeze.RequireConfirmation {
		t.Error("Expected squeeze rejection and confirmation overridden to true")
	}

	// Check checksum is computed and not empty/none
	version := reg.GetVersion()
	if strings.Contains(version, "(none)") {
		t.Errorf("Checksums should not be none, got: %s", version)
	}
}
