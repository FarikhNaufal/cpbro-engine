package main

import (
	"reflect"
	"testing"

	"cpbro-engine/internal/modules/cryptobroV3/config"
	"cpbro-engine/internal/modules/cryptobroV3/usecase"
)

func TestBuildRuntimeSettingsMapsConfigSurfaces(t *testing.T) {
	cfg, err := config.LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadConfigFromEnv() error = %v", err)
	}

	cfg.Monitoring.MaxHoldMinutes = 321
	cfg.Worker.MaxMarketDataConcurrency = 11
	cfg.Worker.MaxCandidatePipelineConcurrency = 12
	cfg.Worker.MaxAIConcurrency = 13
	cfg.Worker.MaxMonitoringCandleConcurrency = 14
	cfg.Universe.TierAMinQuoteVolume = 101
	cfg.Universe.TierBMinQuoteVolume = 102
	cfg.Universe.TierCMinVolume = 103
	cfg.Universe.DefaultSymbols = []string{"AAAUSDT", "BBBUSDT"}
	cfg.Universe.DefaultHotBoost = 1.11
	cfg.Universe.MaxHotBoost = 1.22
	cfg.Universe.DefaultMinFundingVolume = 104
	cfg.Universe.ChaosMinFundingVolume = 105
	cfg.Universe.LowVolMinVolumeFloor = 106
	cfg.Universe.ChaosHighVolMinVolumeFloor = 107
	cfg.Universe.WeightLiquidityDefault = 0.11
	cfg.Universe.WeightActivityDefault = 0.12
	cfg.Universe.WeightHotDefault = 0.13
	cfg.Universe.WeightLiquidityAlt = 0.21
	cfg.Universe.WeightActivityAlt = 0.22
	cfg.Universe.WeightHotAlt = 0.23
	cfg.Universe.WeightLiquidityCompression = 0.31
	cfg.Universe.WeightActivityCompression = 0.32
	cfg.Universe.WeightHotCompression = 0.33
	cfg.Universe.WeightLiquidityRiskOff = 0.41
	cfg.Universe.WeightActivityRiskOff = 0.42
	cfg.Universe.WeightHotRiskOff = 0.43
	cfg.Universe.WeightLiquidityChaos = 0.51
	cfg.Universe.WeightActivityChaos = 0.52
	cfg.Universe.WeightHotChaos = 0.53
	cfg.Universe.WeightLiquidityLowVol = 0.61
	cfg.Universe.WeightActivityLowVol = 0.62
	cfg.Universe.WeightHotLowVol = 0.63
	cfg.Universe.WeightLiquidityDominance = 0.71
	cfg.Universe.WeightActivityDominance = 0.72
	cfg.Universe.WeightHotDominance = 0.73
	cfg.Strategy.RequireAIHighForExecute = false
	cfg.Strategy.RequireFreshEntryForExecute = false
	cfg.Strategy.WatchCooldownMinutes = 401
	cfg.Strategy.WatchDedupPriceToleranceBps = 402
	cfg.Strategy.EvaluationMinSampleWarning = 403
	cfg.Strategy.EvaluationMinSampleMedium = 404
	cfg.Strategy.EvaluationMinSampleHigh = 405
	cfg.Strategy.DebugSaveRawKlines = true
	cfg.Strategy.RawKlinesDebugDir = "debug/custom"
	cfg.Strategy.MaxMarketDataPrefetchSymbols = 406
	cfg.Strategy.ScanRequestWeightBudget = 407
	cfg.Strategy.CompressionNeutralBreadthLower = 0.81
	cfg.Strategy.CompressionNeutralBreadthUpper = 0.82
	cfg.Strategy.CompressionMaxBBWidth = 0.83
	cfg.Strategy.CompressionZeroEligibleFallbackThreshold = 408
	cfg.Strategy.BroaderVolatilitySampleFloor = 409
	cfg.Strategy.FundingExtremeThreshold = 0.84
	cfg.Strategy.ATRFallbackPercent = 0.85
	cfg.Strategy.MinSLATRMultiplierBase = 0.86
	cfg.Strategy.MinSLATRMultiplierReversal = 0.87
	cfg.Strategy.MinSLATRMultiplierHighVol = 0.88
	cfg.Strategy.RotationActivityThresholdDefault = 0.89
	cfg.Strategy.RotationActivityThresholdAlt = 0.90
	cfg.Strategy.RotationActivityThresholdDefensive = 0.91
	cfg.Strategy.RotationActivityThresholdLowVol = 0.92
	cfg.Strategy.RotationPrefetchRatioDefault = 0.93
	cfg.Strategy.RotationPrefetchRatioAlt = 0.94
	cfg.Strategy.RotationPrefetchRatioDefensive = 0.95
	cfg.Strategy.StalenessPolicyScaleBase = 0.96
	cfg.Strategy.StalenessPolicyScaleMin = 0.97
	cfg.Strategy.StalenessPolicyScaleMax = 0.98
	cfg.Strategy.StalenessLateThresholdMultiplier = 0.99
	cfg.Strategy.StalenessBasePctChaos = 0.24
	cfg.Strategy.StalenessBasePctHighVol = 0.25
	cfg.Strategy.StalenessBasePctTierC = 0.26
	cfg.Strategy.StalenessBasePctDefault = 0.27
	cfg.Safety.AIAuditEnabled = false
	cfg.Safety.DecisionAuditEnabled = false
	cfg.Safety.HealthStorageCheck = false
	cfg.Safety.HealthCheckTimeoutSeconds = 501

	got := usecase.BuildRuntimeSettings(cfg)
	want := usecase.RuntimeSettings{
		MonitoringMaxHoldMinutes:                 321,
		RequireAIHighForExecute:                  false,
		RequireFreshEntryForExecute:              false,
		WatchCooldownMinutes:                     401,
		WatchDedupPriceToleranceBps:              402,
		MaxMarketDataConcurrency:                 11,
		MaxCandidatePipelineConcurrency:          12,
		MaxAIConcurrency:                         13,
		AIAuditEnabled:                           false,
		DecisionAuditEnabled:                     false,
		MaxMarketDataPrefetchSymbols:             406,
		ScanRequestWeightBudget:                  407,
		MaxMonitoringCandleConcurrency:           14,
		EvaluationMinSampleWarning:               403,
		EvaluationMinSampleMedium:                404,
		EvaluationMinSampleHigh:                  405,
		HealthStorageCheck:                       false,
		HealthCheckTimeoutSeconds:                501,
		DebugSaveRawKlines:                       true,
		RawKlinesDebugDir:                        "debug/custom",
		UniverseTierAMinQuoteVolume:              101,
		UniverseTierBMinQuoteVolume:              102,
		UniverseTierCMinVolume:                   103,
		UniverseDefaultSymbols:                   []string{"AAAUSDT", "BBBUSDT"},
		UniverseDefaultHotBoost:                  1.11,
		UniverseMaxHotBoost:                      1.22,
		UniverseDefaultMinFundingVolume:          104,
		UniverseChaosMinFundingVolume:            105,
		UniverseLowVolMinVolumeFloor:             106,
		UniverseChaosHighVolMinVolumeFloor:       107,
		UniverseWeightLiquidityDefault:           0.11,
		UniverseWeightActivityDefault:            0.12,
		UniverseWeightHotDefault:                 0.13,
		UniverseWeightLiquidityAlt:               0.21,
		UniverseWeightActivityAlt:                0.22,
		UniverseWeightHotAlt:                     0.23,
		UniverseWeightLiquidityCompression:       0.31,
		UniverseWeightActivityCompression:        0.32,
		UniverseWeightHotCompression:             0.33,
		UniverseWeightLiquidityRiskOff:           0.41,
		UniverseWeightActivityRiskOff:            0.42,
		UniverseWeightHotRiskOff:                 0.43,
		UniverseWeightLiquidityChaos:             0.51,
		UniverseWeightActivityChaos:              0.52,
		UniverseWeightHotChaos:                   0.53,
		UniverseWeightLiquidityLowVol:            0.61,
		UniverseWeightActivityLowVol:             0.62,
		UniverseWeightHotLowVol:                  0.63,
		UniverseWeightLiquidityDominance:         0.71,
		UniverseWeightActivityDominance:          0.72,
		UniverseWeightHotDominance:               0.73,
		CompressionNeutralBreadthLower:           0.81,
		CompressionNeutralBreadthUpper:           0.82,
		CompressionMaxBBWidth:                    0.83,
		CompressionZeroEligibleFallbackThreshold: 408,
		BroaderVolatilitySampleFloor:             409,
		FundingExtremeThreshold:                  0.84,
		ATRFallbackPercent:                       0.85,
		MinSLATRMultiplierBase:                   0.86,
		MinSLATRMultiplierReversal:               0.87,
		MinSLATRMultiplierHighVol:                0.88,
		RotationActivityThresholdDefault:         0.89,
		RotationActivityThresholdAlt:             0.90,
		RotationActivityThresholdDefensive:       0.91,
		RotationActivityThresholdLowVol:          0.92,
		RotationPrefetchRatioDefault:             0.93,
		RotationPrefetchRatioAlt:                 0.94,
		RotationPrefetchRatioDefensive:           0.95,
		StalenessPolicyScaleBase:                 0.96,
		StalenessPolicyScaleMin:                  0.97,
		StalenessPolicyScaleMax:                  0.98,
		StalenessLateThresholdMultiplier:         0.99,
		StalenessBasePctChaos:                    0.24,
		StalenessBasePctHighVol:                  0.25,
		StalenessBasePctTierC:                    0.26,
		StalenessBasePctDefault:                  0.27,
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildRuntimeSettings() mismatch\nwant: %#v\ngot:  %#v", want, got)
	}

	got.UniverseDefaultSymbols[0] = "CHANGED"
	if cfg.Universe.DefaultSymbols[0] != "AAAUSDT" {
		t.Fatal("BuildRuntimeSettings should not alias universe default symbols slice")
	}
}
