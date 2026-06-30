package usecase

import (
	"cpbro-engine/internal/modules/cryptobroV3/config"
	"sync"
)

type RuntimeSettings struct {
	ScanBoundaryMinutes                      int
	ScanCloseCandleBufferSeconds             int
	ScanPreventOverlap                       bool
	MonitoringMaxHoldMinutes                 int
	RequireAIHighForExecute                  bool
	RequireFreshEntryForExecute              bool
	WatchCooldownMinutes                     int
	WatchDedupPriceToleranceBps              int
	WatchRecheckBoundaryMinutes              int
	WatchRecheckMaxAgeMinutes                int
	WatchRecheckBatchLimit                   int
	WatchRecheckAllowedPlaybooks             []string
	WatchRecheckAllowedReasonTokens          []string
	WatchRecheckBlockedReasonTokens          []string
	MaxMarketDataConcurrency                 int
	MaxCandidatePipelineConcurrency          int
	MaxAIConcurrency                         int
	GeminiMaxCandidatesDefault               int
	AIAuditEnabled                           bool
	DecisionAuditEnabled                     bool
	MaxMarketDataPrefetchSymbols             int
	ScanRequestWeightBudget                  int
	MaxMonitoringCandleConcurrency           int
	EvaluationMinSampleWarning               int
	EvaluationMinSampleMedium                int
	EvaluationMinSampleHigh                  int
	HealthStorageCheck                       bool
	HealthCheckTimeoutSeconds                int
	DebugSaveRawKlines                       bool
	RawKlinesDebugDir                        string
	LatestResultFile                         string
	SignalJournalFile                        string
	WatchJournalFile                         string
	DecisionAuditFile                        string
	HealthSnapshotFile                       string
	UniverseTierAMinQuoteVolume              float64
	UniverseTierBMinQuoteVolume              float64
	UniverseTierCMinVolume                   float64
	UniverseDefaultSymbols                   []string
	UniverseExcludedSymbols                  []string
	UniverseDefaultHotBoost                  float64
	UniverseMaxHotBoost                      float64
	UniverseDefaultMinFundingVolume          float64
	UniverseChaosMinFundingVolume            float64
	UniverseLowVolMinVolumeFloor             float64
	UniverseChaosHighVolMinVolumeFloor       float64
	UniverseWeightLiquidityDefault           float64
	UniverseWeightActivityDefault            float64
	UniverseWeightHotDefault                 float64
	UniverseWeightLiquidityAlt               float64
	UniverseWeightActivityAlt                float64
	UniverseWeightHotAlt                     float64
	UniverseWeightLiquidityCompression       float64
	UniverseWeightActivityCompression        float64
	UniverseWeightHotCompression             float64
	UniverseWeightLiquidityRiskOff           float64
	UniverseWeightActivityRiskOff            float64
	UniverseWeightHotRiskOff                 float64
	UniverseWeightLiquidityChaos             float64
	UniverseWeightActivityChaos              float64
	UniverseWeightHotChaos                   float64
	UniverseWeightLiquidityLowVol            float64
	UniverseWeightActivityLowVol             float64
	UniverseWeightHotLowVol                  float64
	UniverseWeightLiquidityDominance         float64
	UniverseWeightActivityDominance          float64
	UniverseWeightHotDominance               float64
	CompressionNeutralBreadthLower           float64
	CompressionNeutralBreadthUpper           float64
	CompressionMaxBBWidth                    float64
	CompressionBBWidthPercentile             float64
	CompressionBBWidthLookback               int
	CompressionZeroEligibleFallbackThreshold int
	BroaderVolatilitySampleFloor             int
	FundingExtremeThreshold                  float64
	ATRFallbackPercent                       float64
	MinSLATRMultiplierBase                   float64
	MinSLATRMultiplierReversal               float64
	MinSLATRMultiplierHighVol                float64
	RotationActivityThresholdDefault         float64
	RotationActivityThresholdAlt             float64
	RotationActivityThresholdDefensive       float64
	RotationActivityThresholdLowVol          float64
	RotationPrefetchRatioDefault             float64
	RotationPrefetchRatioAlt                 float64
	RotationPrefetchRatioDefensive           float64
	StalenessPolicyScaleBase                 float64
	StalenessPolicyScaleMin                  float64
	StalenessPolicyScaleMax                  float64
	StalenessLateThresholdMultiplier         float64
	StalenessBasePctChaos                    float64
	StalenessBasePctHighVol                  float64
	StalenessBasePctTierC                    float64
	StalenessBasePctDefault                  float64
}

var (
	runtimeSettingsMu sync.RWMutex
	runtimeSettings   = RuntimeSettings{
		ScanBoundaryMinutes:                      15,
		ScanCloseCandleBufferSeconds:             3,
		ScanPreventOverlap:                       true,
		MonitoringMaxHoldMinutes:                 120,
		RequireAIHighForExecute:                  false,
		RequireFreshEntryForExecute:              false,
		WatchCooldownMinutes:                     30,
		WatchDedupPriceToleranceBps:              50,
		WatchRecheckBoundaryMinutes:              5,
		WatchRecheckMaxAgeMinutes:                120,
		WatchRecheckBatchLimit:                   10,
		WatchRecheckAllowedPlaybooks:             []string{"TREND_PULLBACK", "LIQUIDITY_SWEEP_REVERSAL", "COMPRESSION_BREAKOUT_RETEST", "RANGE_EDGE_REVERSAL", "CROWDED_POSITIONING_SQUEEZE"},
		WatchRecheckAllowedReasonTokens:          []string{"AI DECISION IS WAIT", "AI_SKIPPED", "WATCH_ONLY", "WAIT_RETEST", "AI CONFIDENCE", "LOCAL_GATE_WATCH", "M5"},
		WatchRecheckBlockedReasonTokens:          []string{"ACTIVE_MONITORING_EXISTS", "OPPOSITE_SIGNAL_CONFLICT", "LOWER_PRIORITY_CONFLICT", "DUPLICATE_SIGNAL_BUCKET", "SYMBOL_COOLDOWN_ACTIVE", "MAX_FINAL_EXECUTE_LIMIT"},
		MaxMarketDataConcurrency:                 5,
		MaxCandidatePipelineConcurrency:          0,
		MaxAIConcurrency:                         3,
		GeminiMaxCandidatesDefault:               3,
		AIAuditEnabled:                           true,
		DecisionAuditEnabled:                     true,
		MaxMarketDataPrefetchSymbols:             0,
		ScanRequestWeightBudget:                  0,
		MaxMonitoringCandleConcurrency:           4,
		EvaluationMinSampleWarning:               10,
		EvaluationMinSampleMedium:                20,
		EvaluationMinSampleHigh:                  50,
		HealthStorageCheck:                       true,
		HealthCheckTimeoutSeconds:                2,
		DebugSaveRawKlines:                       false,
		RawKlinesDebugDir:                        config.DefaultRawKlinesDebugDir,
		LatestResultFile:                         config.DefaultLatestResultFile,
		SignalJournalFile:                        config.DefaultSignalJournalFile,
		WatchJournalFile:                         config.DefaultWatchJournalFile,
		DecisionAuditFile:                        config.DefaultDecisionAuditFile,
		HealthSnapshotFile:                       config.DefaultHealthSnapshotFile,
		UniverseTierAMinQuoteVolume:              150000000.0,
		UniverseTierBMinQuoteVolume:              50000000.0,
		UniverseTierCMinVolume:                   15000000.0,
		UniverseDefaultSymbols:                   []string{"BTCUSDT", "ETHUSDT", "SOLUSDT", "BNBUSDT"},
		UniverseExcludedSymbols:                  []string{},
		UniverseDefaultHotBoost:                  1.25,
		UniverseMaxHotBoost:                      1.5,
		UniverseDefaultMinFundingVolume:          50000000.0,
		UniverseChaosMinFundingVolume:            150000000.0,
		UniverseLowVolMinVolumeFloor:             750000.0,
		UniverseChaosHighVolMinVolumeFloor:       10000000.0,
		UniverseWeightLiquidityDefault:           0.65,
		UniverseWeightActivityDefault:            0.20,
		UniverseWeightHotDefault:                 0.15,
		UniverseWeightLiquidityAlt:               0.55,
		UniverseWeightActivityAlt:                0.25,
		UniverseWeightHotAlt:                     0.20,
		UniverseWeightLiquidityCompression:       0.60,
		UniverseWeightActivityCompression:        0.25,
		UniverseWeightHotCompression:             0.15,
		UniverseWeightLiquidityRiskOff:           0.75,
		UniverseWeightActivityRiskOff:            0.15,
		UniverseWeightHotRiskOff:                 0.10,
		UniverseWeightLiquidityChaos:             0.80,
		UniverseWeightActivityChaos:              0.15,
		UniverseWeightHotChaos:                   0.05,
		UniverseWeightLiquidityLowVol:            0.70,
		UniverseWeightActivityLowVol:             0.15,
		UniverseWeightHotLowVol:                  0.15,
		UniverseWeightLiquidityDominance:         0.72,
		UniverseWeightActivityDominance:          0.13,
		UniverseWeightHotDominance:               0.15,
		CompressionNeutralBreadthLower:           0.35,
		CompressionNeutralBreadthUpper:           0.65,
		CompressionMaxBBWidth:                    0.10,
		CompressionBBWidthPercentile:             0.25,
		CompressionBBWidthLookback:               100,
		CompressionZeroEligibleFallbackThreshold: 2,
		BroaderVolatilitySampleFloor:             6,
		FundingExtremeThreshold:                  0.003,
		ATRFallbackPercent:                       0.01,
		MinSLATRMultiplierBase:                   1.0,
		MinSLATRMultiplierReversal:               1.2,
		MinSLATRMultiplierHighVol:                1.5,
		RotationActivityThresholdDefault:         0.55,
		RotationActivityThresholdAlt:             0.45,
		RotationActivityThresholdDefensive:       0.65,
		RotationActivityThresholdLowVol:          0.50,
		RotationPrefetchRatioDefault:             0.15,
		RotationPrefetchRatioAlt:                 0.20,
		RotationPrefetchRatioDefensive:           0.10,
		StalenessPolicyScaleBase:                 1.5,
		StalenessPolicyScaleMin:                  0.50,
		StalenessPolicyScaleMax:                  1.20,
		StalenessLateThresholdMultiplier:         1.5,
		StalenessBasePctChaos:                    0.20,
		StalenessBasePctHighVol:                  0.50,
		StalenessBasePctTierC:                    0.25,
		StalenessBasePctDefault:                  0.35,
	}
)

func SetRuntimeSettings(settings RuntimeSettings) {
	runtimeSettingsMu.Lock()
	defer runtimeSettingsMu.Unlock()
	settings.UniverseDefaultSymbols = append([]string(nil), settings.UniverseDefaultSymbols...)
	settings.UniverseExcludedSymbols = append([]string(nil), settings.UniverseExcludedSymbols...)
	settings.WatchRecheckAllowedPlaybooks = append([]string(nil), settings.WatchRecheckAllowedPlaybooks...)
	settings.WatchRecheckAllowedReasonTokens = append([]string(nil), settings.WatchRecheckAllowedReasonTokens...)
	settings.WatchRecheckBlockedReasonTokens = append([]string(nil), settings.WatchRecheckBlockedReasonTokens...)
	runtimeSettings = settings
}

func getRuntimeSettings() RuntimeSettings {
	runtimeSettingsMu.RLock()
	settings := runtimeSettings
	runtimeSettingsMu.RUnlock()
	settings.UniverseDefaultSymbols = append([]string(nil), settings.UniverseDefaultSymbols...)
	settings.UniverseExcludedSymbols = append([]string(nil), settings.UniverseExcludedSymbols...)
	settings.WatchRecheckAllowedPlaybooks = append([]string(nil), settings.WatchRecheckAllowedPlaybooks...)
	settings.WatchRecheckAllowedReasonTokens = append([]string(nil), settings.WatchRecheckAllowedReasonTokens...)
	settings.WatchRecheckBlockedReasonTokens = append([]string(nil), settings.WatchRecheckBlockedReasonTokens...)

	return settings
}

func SnapshotRuntimeSettings() RuntimeSettings {
	return getRuntimeSettings()
}
