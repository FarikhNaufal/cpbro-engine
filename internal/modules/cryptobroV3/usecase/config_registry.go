package usecase

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
)

// PolicyProfileConfig represents the structured config loaded from JSON
type PolicyProfileConfig struct {
	Version  string                  `json:"version"`
	Policies map[string]MarketPolicy `json:"policies"`
}

// PlaybookThresholdProfileConfig represents the structured threshold config loaded from JSON
type PlaybookThresholdProfileConfig struct {
	Version  string                                `json:"version"`
	Profiles map[Playbook]PlaybookThresholdProfile `json:"profiles"`
}

// ConfigRegistry manages and audits config versions and parameter enforcement
type ConfigRegistry struct {
	mu              sync.RWMutex
	policyVersion   string
	policyHash      string
	playbookVersion string
	playbookHash    string

	policies map[string]MarketPolicy
	profiles map[Playbook]PlaybookThresholdProfile
}

var (
	globalRegistry   *ConfigRegistry
	registryOnce     sync.Once
	globalRegistryMu sync.Mutex
)

// SetGlobalConfigRegistry updates the global registry thread-safely
func SetGlobalConfigRegistry(r *ConfigRegistry) {
	globalRegistryMu.Lock()
	defer globalRegistryMu.Unlock()
	globalRegistry = r
}

// GetGlobalConfigRegistry retrieves the global registry thread-safely
func GetGlobalConfigRegistry() *ConfigRegistry {
	globalRegistryMu.Lock()
	defer globalRegistryMu.Unlock()
	if globalRegistry == nil {
		// Auto-initialize with fallback defaults if never loaded explicitly
		globalRegistry = NewDefaultConfigRegistry()
	}
	return globalRegistry
}

// NewDefaultConfigRegistry initializes a registry with code-defined fallback configurations
func NewDefaultConfigRegistry() *ConfigRegistry {
	return &ConfigRegistry{
		policyVersion:   "default-code",
		policyHash:      "none",
		playbookVersion: "default-code",
		playbookHash:    "none",
		policies:        getDefaultPolicies(),
		profiles:        getDefaultProfiles(),
	}
}

// LoadConfigRegistry reads configuration files, calculates checksums, and enforces safety bounds.
// In case of any read/syntax errors, it logs warnings and falls back to safe configurations.
func LoadConfigRegistry(policyPath, playbookPath string) (*ConfigRegistry, error) {
	registry := NewDefaultConfigRegistry()

	// 1. Load policy config
	policyData, err := os.ReadFile(policyPath)
	if err != nil {
		slog.Warn("Failed to read policy config file, using code defaults", "path", policyPath, "error", err)
	} else {
		hash := sha256.Sum256(policyData)
		registry.policyHash = fmt.Sprintf("%x", hash[:4])

		var policyConf PolicyProfileConfig
		if err := json.Unmarshal(policyData, &policyConf); err != nil {
			slog.Warn("Failed to parse policy config JSON, using code defaults", "path", policyPath, "error", err)
		} else {
			registry.policyVersion = policyConf.Version
			if registry.policyVersion == "" {
				registry.policyVersion = "unspecified"
			}
			// Load and validate
			for k, v := range policyConf.Policies {
				registry.policies[k] = validateAndClampPolicy(k, v)
			}
		}
	}

	// 2. Load playbook threshold config
	playbookData, err := os.ReadFile(playbookPath)
	if err != nil {
		slog.Warn("Failed to read playbook threshold config file, using code defaults", "path", playbookPath, "error", err)
	} else {
		hash := sha256.Sum256(playbookData)
		registry.playbookHash = fmt.Sprintf("%x", hash[:4])

		var playbookConf PlaybookThresholdProfileConfig
		if err := json.Unmarshal(playbookData, &playbookConf); err != nil {
			slog.Warn("Failed to parse playbook threshold config JSON, using code defaults", "path", playbookPath, "error", err)
		} else {
			registry.playbookVersion = playbookConf.Version
			if registry.playbookVersion == "" {
				registry.playbookVersion = "unspecified"
			}
			// Load and validate
			for k, v := range playbookConf.Profiles {
				registry.profiles[k] = validateAndClampProfile(k, v)
			}
		}
	}

	return registry, nil
}

// GetMarketPolicy retrieves a market policy configuration thread-safely
func (r *ConfigRegistry) GetMarketPolicy(name string) (MarketPolicy, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.policies[name]
	return p, ok
}

// GetPlaybookProfile retrieves a playbook threshold profile thread-safely
func (r *ConfigRegistry) GetPlaybookProfile(playbook Playbook) (PlaybookThresholdProfile, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.profiles[playbook]
	return p, ok
}

// GetVersion returns a version and audit hash summary of the loaded configurations
func (r *ConfigRegistry) GetVersion() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return fmt.Sprintf("policy:%s(%s);playbook:%s(%s)", r.policyVersion, r.policyHash, r.playbookVersion, r.playbookHash)
}

// Helper: validate and clamp policy configurations to prevent weakening hard rules
func validateAndClampPolicy(name string, policy MarketPolicy) MarketPolicy {
	// Baseline rules: MinScoreExecute cannot drop below 7.0, MinRRExecute cannot drop below 1.5
	if policy.MinScoreExecute < 7.0 {
		slog.Warn("Enforcing hard limit: MinScoreExecute clamped to 7.0", "policy", name, "original", policy.MinScoreExecute)
		policy.MinScoreExecute = 7.0
	}
	if policy.MinRRExecute < 1.5 {
		slog.Warn("Enforcing hard limit: MinRRExecute clamped to 1.5", "policy", name, "original", policy.MinRRExecute)
		policy.MinRRExecute = 1.5
	}
	if policy.MaxFinalExecute < 1 {
		slog.Warn("Enforcing hard limit: MaxFinalExecute clamped to 1", "policy", name, "original", policy.MaxFinalExecute)
		policy.MaxFinalExecute = 1
	}

	// Specific checks for chaos mode to ensure safety
	if name == "BTC_CHAOS" {
		if policy.MinScoreExecute < 8.2 {
			policy.MinScoreExecute = 8.2
		}
		if policy.MinRRExecute < 2.0 {
			policy.MinRRExecute = 2.0
		}
		policy.RequireAIConfidence = AIConfidenceHigh
		policy.RequireFreshEntry = true
	}

	return policy
}

// Helper: validate and clamp playbook thresholds to prevent weakening hard rules
func validateAndClampProfile(playbook Playbook, profile PlaybookThresholdProfile) PlaybookThresholdProfile {
	// Safety validation for Liquidity Sweep
	if playbook == LIQUIDITY_SWEEP_REVERSAL {
		if !profile.RequireVolumeConfirm {
			slog.Warn("Liquidity Sweep requires volume confirm. Overriding RequireVolumeConfirm to true.")
			profile.RequireVolumeConfirm = true
		}
		if profile.MinVolumeRatio < 1.1 {
			slog.Warn("Liquidity Sweep requires a minimum volume ratio of at least 1.1. Overriding.", "original", profile.MinVolumeRatio)
			profile.MinVolumeRatio = 1.1
		}
		if !profile.RequireRejection {
			profile.RequireRejection = true
		}
		if !profile.RequireConfirmation {
			profile.RequireConfirmation = true
		}
	}

	// Safety validation for Compression Breakout Retest
	if playbook == COMPRESSION_BREAKOUT_RETEST {
		if !profile.RequireRetest {
			slog.Warn("Compression Breakout Retest requires a retest confirmation. Overriding RequireRetest to true.")
			profile.RequireRetest = true
		}
		if !profile.RequireConfirmation {
			profile.RequireConfirmation = true
		}
	}

	// Safety validation for Crowded Squeeze
	if playbook == CROWDED_POSITIONING_SQUEEZE {
		if profile.MinScoreExecute < 7.8 {
			slog.Warn("Crowded Positioning Squeeze requires a minimum execution score of 7.8. Overriding.", "original", profile.MinScoreExecute)
			profile.MinScoreExecute = 7.8
		}
		if !profile.RequireRejection {
			profile.RequireRejection = true
		}
		if !profile.RequireConfirmation {
			profile.RequireConfirmation = true
		}
	}

	return profile
}

func getDefaultPolicies() map[string]MarketPolicy {
	return map[string]MarketPolicy{
		"DEFAULT": {
			Regime:                 DEFAULT,
			AllowLong:              true,
			AllowShort:             true,
			LongMode:               NORMAL,
			ShortMode:              NORMAL,
			AllowedTiers:           []Tier{TierA, TierB, TierC},
			AllowedPlaybooks:       []Playbook{TREND_PULLBACK, LIQUIDITY_SWEEP_REVERSAL, COMPRESSION_BREAKOUT_RETEST, RANGE_EDGE_REVERSAL, CROWDED_POSITIONING_SQUEEZE},
			MaxSymbols:             50,
			MaxAICandidates:        3,
			MaxFinalExecute:        5,
			MinVolume:              1000000.0,
			MaxFundingAbs:          0.01,
			MaxPriceMove24h:        0.20,
			MinScoreAI:             6.0,
			MinScoreExecute:        7.0,
			MinRRExecute:           1.5,
			MinADXExecute:          20.0,
			RequireAIConfidence:    AIConfidenceMedium,
			RequireFreshEntry:      false,
			StalenessATRMultiplier: 1.5,
			CooldownMinutes:        15,
			Reason:                 "Default normal policy",
		},
		"BTC_CHAOS": {
			Regime:       BTC_CHAOS,
			AllowLong:    true,
			AllowShort:   true,
			LongMode:     NORMAL,
			ShortMode:    NORMAL,
			AllowedTiers: []Tier{TierA, TierB},
			// Chaos regime should be highly selective: premium reversals only.
			AllowedPlaybooks:       []Playbook{LIQUIDITY_SWEEP_REVERSAL, CROWDED_POSITIONING_SQUEEZE},
			MaxSymbols:             35,
			MaxAICandidates:        1,
			MaxFinalExecute:        1,
			MinVolume:              1000000.0,
			MaxFundingAbs:          0.01,
			MaxPriceMove24h:        0.20,
			MinScoreAI:             6.0,
			MinScoreExecute:        8.2,
			MinRRExecute:           2.0,
			MinADXExecute:          20.0,
			RequireAIConfidence:    AIConfidenceHigh,
			RequireFreshEntry:      true,
			StalenessATRMultiplier: 0.5,
			CooldownMinutes:        15,
			Reason:                 "BTC_CHAOS active - strict restrictions applied",
		},
		"BTC_DOMINANCE": {
			Regime:                 BTC_DOMINANCE,
			AllowLong:              true,
			AllowShort:             true,
			LongMode:               PULLBACK_ONLY,
			ShortMode:              SWEEP_ONLY,
			AllowedTiers:           []Tier{TierA, TierB},
			AllowedPlaybooks:       []Playbook{TREND_PULLBACK, LIQUIDITY_SWEEP_REVERSAL},
			MaxSymbols:             50,
			MaxAICandidates:        2,
			MaxFinalExecute:        5,
			MinVolume:              5000000.0,
			MaxFundingAbs:          0.01,
			MaxPriceMove24h:        0.20,
			MinScoreAI:             6.0,
			MinScoreExecute:        7.2,
			MinRRExecute:           1.5,
			MinADXExecute:          20.0,
			RequireAIConfidence:    AIConfidenceMedium,
			RequireFreshEntry:      false,
			StalenessATRMultiplier: 1.5,
			CooldownMinutes:        15,
			Reason:                 "BTC_DOMINANCE active - altcoins restricted",
		},
		"ALT_SUPPORTIVE": {
			Regime:                 ALT_SUPPORTIVE,
			AllowLong:              true,
			AllowShort:             true,
			LongMode:               NORMAL,
			ShortMode:              SWEEP_ONLY,
			AllowedTiers:           []Tier{TierA, TierB, TierC},
			AllowedPlaybooks:       []Playbook{TREND_PULLBACK, COMPRESSION_BREAKOUT_RETEST, LIQUIDITY_SWEEP_REVERSAL},
			MaxSymbols:             75,
			MaxAICandidates:        3,
			MaxFinalExecute:        5,
			MinVolume:              1000000.0,
			MaxFundingAbs:          0.01,
			MaxPriceMove24h:        0.20,
			MinScoreAI:             6.0,
			MinScoreExecute:        7.0,
			MinRRExecute:           1.5,
			MinADXExecute:          20.0,
			RequireAIConfidence:    AIConfidenceMedium,
			RequireFreshEntry:      false,
			StalenessATRMultiplier: 1.5,
			CooldownMinutes:        15,
			Reason:                 "ALT_SUPPORTIVE + BTC Bullish active - favorable conditions",
		},
		"RISK_OFF": {
			Regime:                 RISK_OFF,
			AllowLong:              true,
			AllowShort:             true,
			LongMode:               REVERSAL_ONLY,
			ShortMode:              NORMAL,
			AllowedTiers:           []Tier{TierA, TierB},
			AllowedPlaybooks:       []Playbook{TREND_PULLBACK, LIQUIDITY_SWEEP_REVERSAL, RANGE_EDGE_REVERSAL},
			MaxSymbols:             50,
			MaxAICandidates:        3,
			MaxFinalExecute:        5,
			MinVolume:              1000000.0,
			MaxFundingAbs:          0.005,
			MaxPriceMove24h:        0.10,
			MinScoreAI:             6.0,
			MinScoreExecute:        7.4,
			MinRRExecute:           1.7,
			MinADXExecute:          20.0,
			RequireAIConfidence:    AIConfidenceMedium,
			RequireFreshEntry:      false,
			StalenessATRMultiplier: 1.5,
			CooldownMinutes:        15,
			Reason:                 "RISK_OFF + BTC Bearish active - short bias",
		},
		"CHOP_RANGE": {
			Regime:                 CHOP_RANGE,
			AllowLong:              true,
			AllowShort:             true,
			LongMode:               REVERSAL_ONLY,
			ShortMode:              REVERSAL_ONLY,
			AllowedTiers:           []Tier{TierA, TierB, TierC},
			AllowedPlaybooks:       []Playbook{LIQUIDITY_SWEEP_REVERSAL, RANGE_EDGE_REVERSAL},
			MaxSymbols:             50,
			MaxAICandidates:        2,
			MaxFinalExecute:        5,
			MinVolume:              1000000.0,
			MaxFundingAbs:          0.01,
			MaxPriceMove24h:        0.20,
			MinScoreAI:             6.0,
			MinScoreExecute:        7.0,
			MinRRExecute:           1.8,
			MinADXExecute:          20.0,
			RequireAIConfidence:    AIConfidenceHigh,
			RequireFreshEntry:      false,
			StalenessATRMultiplier: 1.5,
			CooldownMinutes:        15,
			Reason:                 "CHOP_RANGE active - mean reversion only",
		},
		"COMPRESSION": {
			Regime:                 COMPRESSION,
			AllowLong:              true,
			AllowShort:             true,
			LongMode:               BREAKOUT_RETEST_ONLY,
			ShortMode:              BREAKOUT_RETEST_ONLY,
			AllowedTiers:           []Tier{TierA, TierB, TierC},
			AllowedPlaybooks:       []Playbook{COMPRESSION_BREAKOUT_RETEST},
			MaxSymbols:             50,
			MaxAICandidates:        3,
			MaxFinalExecute:        5,
			MinVolume:              1000000.0,
			MaxFundingAbs:          0.01,
			MaxPriceMove24h:        0.20,
			MinScoreAI:             6.0,
			MinScoreExecute:        7.0,
			MinRRExecute:           1.5,
			MinADXExecute:          20.0,
			RequireAIConfidence:    AIConfidenceMedium,
			RequireFreshEntry:      true,
			StalenessATRMultiplier: 1.5,
			CooldownMinutes:        15,
			Reason:                 "COMPRESSION active - awaiting breakout retest confirmation",
		},
	}
}

func getDefaultProfiles() map[Playbook]PlaybookThresholdProfile {
	return map[Playbook]PlaybookThresholdProfile{
		TREND_PULLBACK: {
			Playbook:                 TREND_PULLBACK,
			MinScoreAI:               7.0,
			MinScoreExecute:          7.3,
			MinRR:                    1.5,
			MinADX:                   20.0,
			RequireADX:               true,
			RejectADXExpansion:       false,
			RequireVolumeConfirm:     false,
			RequireRejection:         false,
			RequireConfirmation:      false,
			RequireRetest:            false,
			AllowBreakoutCandleEntry: false,
			StalenessATR:             0.45,
			Reason:                   "Trend Pullback profile",
		},
		LIQUIDITY_SWEEP_REVERSAL: {
			Playbook:                 LIQUIDITY_SWEEP_REVERSAL,
			MinScoreAI:               7.0,
			MinScoreExecute:          7.3,
			MinRR:                    1.7,
			MinADX:                   0.0,
			RequireADX:               false,
			RejectADXExpansion:       false,
			RequireVolumeConfirm:     true,
			RequireRejection:         true,
			RequireConfirmation:      true,
			RequireRetest:            false,
			MinVolumeRatio:           1.3,
			MinWickRatio:             0.3,
			AllowBreakoutCandleEntry: false,
			StalenessATR:             0.30,
			Reason:                   "Liquidity Sweep Reversal profile",
		},
		COMPRESSION_BREAKOUT_RETEST: {
			Playbook:                 COMPRESSION_BREAKOUT_RETEST,
			MinScoreAI:               7.0,
			MinScoreExecute:          7.3,
			MinRR:                    1.6,
			MinADX:                   0.0,
			RequireADX:               false,
			RejectADXExpansion:       false,
			RequireVolumeConfirm:     true,
			RequireRejection:         false,
			RequireConfirmation:      true,
			RequireRetest:            true,
			MinVolumeRatio:           1.2,
			MinRetestQuality:         0.5,
			AllowBreakoutCandleEntry: false,
			StalenessATR:             0.30,
			Reason:                   "Compression Breakout Retest profile",
		},
		RANGE_EDGE_REVERSAL: {
			Playbook:                 RANGE_EDGE_REVERSAL,
			MinScoreAI:               7.2,
			MinScoreExecute:          7.5,
			MinRR:                    1.7,
			MinADX:                   0.0,
			MaxADX:                   30.0,
			RejectADXExpansion:       true,
			RequireVolumeConfirm:     false,
			RequireRejection:         true,
			RequireConfirmation:      true,
			RequireRetest:            false,
			MinRangeClarity:          0.5,
			AllowBreakoutCandleEntry: false,
			StalenessATR:             0.30,
			Reason:                   "Range Edge Reversal profile",
		},
		CROWDED_POSITIONING_SQUEEZE: {
			Playbook:                 CROWDED_POSITIONING_SQUEEZE,
			MinScoreAI:               7.5,
			MinScoreExecute:          7.8,
			MinRR:                    1.8,
			MinADX:                   0.0,
			RequireADX:               false,
			RequireVolumeConfirm:     false,
			RequireRejection:         true,
			RequireConfirmation:      true,
			RequireCrowdingEvidence:  true,
			MinCrowdingScore:         0.5,
			RequireAIHigh:            true,
			AllowBreakoutCandleEntry: false,
			StalenessATR:             0.35,
			Reason:                   "Crowded Positioning Squeeze profile",
		},
	}
}
