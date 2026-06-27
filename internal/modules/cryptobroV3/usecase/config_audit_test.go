package usecase

import (
	"strings"
	"testing"
)

func TestConfigAuditor_RecordConfigLoad(t *testing.T) {
	auditor := &ConfigAuditor{maxEntries: 100}

	auditor.RecordConfigLoad("policy", "v1.2.3", "/path/to/policy.json", "startup")
	auditor.RecordConfigLoad("playbook", "v2.0.1", "/path/to/playbook.json", "startup")

	entries := auditor.GetEntries()
	if len(entries) != 2 {
		t.Fatalf("Expected 2 entries, got %d", len(entries))
	}

	// Check first entry
	if entries[0].Component != "policy" {
		t.Errorf("Expected first entry component 'policy', got '%s'", entries[0].Component)
	}
	if entries[0].Version != "v1.2.3" {
		t.Errorf("Expected version 'v1.2.3', got '%s'", entries[0].Version)
	}
	if entries[0].Action != "load" {
		t.Errorf("Expected action 'load', got '%s'", entries[0].Action)
	}
	if entries[0].TriggerSource != "startup" {
		t.Errorf("Expected trigger 'startup', got '%s'", entries[0].TriggerSource)
	}
	if entries[0].Hash == "" {
		t.Error("Expected non-empty hash")
	}
}

func TestConfigAuditor_GetLatestForComponent(t *testing.T) {
	auditor := &ConfigAuditor{maxEntries: 100}

	auditor.RecordConfigLoad("policy", "v1.0.0", "/path/v1.json", "startup")
	auditor.RecordConfigLoad("playbook", "v1.0.0", "/path/p1.json", "startup")
	auditor.RecordConfigUpdate("policy", "v1.1.0", "Updated thresholds")
	auditor.RecordConfigLoad("policy", "v2.0.0", "/path/v2.json", "api")

	latest, found := auditor.GetLatestForComponent("policy")
	if !found {
		t.Fatal("Expected to find latest policy entry")
	}
	if latest.Version != "v2.0.0" {
		t.Errorf("Expected latest version 'v2.0.0', got '%s'", latest.Version)
	}
	if latest.TriggerSource != "api" {
		t.Errorf("Expected trigger 'api', got '%s'", latest.TriggerSource)
	}
}

func TestConfigAuditor_VerifyHash(t *testing.T) {
	auditor := &ConfigAuditor{maxEntries: 100}

	auditor.RecordConfigLoad("policy", "v1.0.0", "/path.json", "startup")

	latest, _ := auditor.GetLatestForComponent("policy")
	if !auditor.VerifyHash("policy", "v1.0.0", latest.Hash) {
		t.Error("VerifyHash should return true for matching version")
	}

	// Wrong version should not verify
	if auditor.VerifyHash("policy", "v2.0.0", latest.Hash) {
		t.Error("VerifyHash should return false for wrong version")
	}
}

func TestConfigAuditor_MaxEntriesTrimming(t *testing.T) {
	auditor := &ConfigAuditor{maxEntries: 3}

	// Add 5 entries (more than max)
	for i := 0; i < 5; i++ {
		auditor.RecordConfigLoad("policy", "v1.0.0", "/path.json", "startup")
	}

	entries := auditor.GetEntries()
	if len(entries) != 3 {
		t.Errorf("Expected 3 entries after trim, got %d", len(entries))
	}
}

func TestConfigAuditor_ConcurrentSafe(t *testing.T) {
	auditor := &ConfigAuditor{maxEntries: 100}

	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			auditor.RecordConfigLoad("policy", "v1.0.0", "/path.json", "startup")
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	entries := auditor.GetEntries()
	if len(entries) != 10 {
		t.Errorf("Expected 10 concurrent entries, got %d", len(entries))
	}

	// All entries should have non-empty hash
	for _, e := range entries {
		if !strings.HasPrefix(e.Hash, "") || e.Hash == "" {
			t.Errorf("Entry hash is empty for version %s", e.Version)
		}
	}
}
