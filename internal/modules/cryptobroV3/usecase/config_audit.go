package usecase

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// ConfigAuditEntry records a config change event.
type ConfigAuditEntry struct {
	Timestamp     time.Time `json:"timestamp"`
	Component     string    `json:"component"`     // "policy", "playbook", "universe", etc.
	Action        string    `json:"action"`        // "load", "update", "reload"
	Version       string    `json:"version"`       // Version after change
	Hash          string    `json:"hash"`          // SHA256 hash of config content
	Details       string    `json:"details"`       // Human-readable description
	TriggerSource string    `json:"trigger_source"` // "startup", "reload", "api"
}

// ConfigAuditor tracks all config changes with versioning and hashes for compliance.
type ConfigAuditor struct {
	mu     sync.RWMutex
	entries []ConfigAuditEntry
	maxEntries int
}

var (
	globalAuditor     *ConfigAuditor
	globalAuditorOnce sync.Once
)

// GetGlobalConfigAuditor returns the singleton config auditor.
func GetGlobalConfigAuditor() *ConfigAuditor {
	globalAuditorOnce.Do(func() {
		globalAuditor = &ConfigAuditor{
			maxEntries: 100, // Keep last 100 audit entries
		}
	})
	return globalAuditor
}

// Record adds an audit entry.
func (a *ConfigAuditor) Record(entry ConfigAuditEntry) {
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}
	if entry.Hash == "" {
		entry.Hash = computeConfigHash(entry.Component, entry.Version)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.entries = append(a.entries, entry)
	// Trim if exceeded max
	if len(a.entries) > a.maxEntries {
		a.entries = a.entries[len(a.entries)-a.maxEntries:]
	}
}

// RecordConfigLoad records a config load event.
func (a *ConfigAuditor) RecordConfigLoad(component, version, path, triggerSource string) {
	a.Record(ConfigAuditEntry{
		Component:     component,
		Action:        "load",
		Version:       version,
		Details:       fmt.Sprintf("Loaded %s config from %s", component, path),
		TriggerSource: triggerSource,
	})
}

// RecordConfigUpdate records a config update event.
func (a *ConfigAuditor) RecordConfigUpdate(component, version, details string) {
	a.Record(ConfigAuditEntry{
		Component:     component,
		Action:        "update",
		Version:       version,
		Details:       details,
		TriggerSource: "runtime",
	})
}

// GetEntries returns a copy of all audit entries.
func (a *ConfigAuditor) GetEntries() []ConfigAuditEntry {
	a.mu.RLock()
	defer a.mu.RUnlock()
	result := make([]ConfigAuditEntry, len(a.entries))
	copy(result, a.entries)
	return result
}

// GetLatestForComponent returns the latest audit entry for a specific component.
func (a *ConfigAuditor) GetLatestForComponent(component string) (ConfigAuditEntry, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	for i := len(a.entries) - 1; i >= 0; i-- {
		if a.entries[i].Component == component {
			return a.entries[i], true
		}
	}
	return ConfigAuditEntry{}, false
}

// computeConfigHash computes a hash for a config version string.
func computeConfigHash(component, version string) string {
	h := sha256.Sum256([]byte(component + ":" + version))
	return hex.EncodeToString(h[:8]) // First 8 bytes = 16 hex chars
}

// VerifyHash verifies if a given hash matches the computed hash for a component/version.
func (a *ConfigAuditor) VerifyHash(component, version, hash string) bool {
	return computeConfigHash(component, version) == hash
}

// GetCurrentVersion returns the current version for a component.
func (a *ConfigAuditor) GetCurrentVersion(component string) string {
	if reg := GetGlobalConfigRegistry(); reg != nil {
		switch component {
		case "policy", "policy_profile":
			return reg.GetVersion() // Combined version string
		}
	}
	return "unknown"
}
