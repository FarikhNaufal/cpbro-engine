package usecase

import "cpbro-engine/internal/modules/cryptobroV3/dto"

const (
	compressionNeutralBreadthLower           = 0.35
	compressionNeutralBreadthUpper           = 0.65
	compressionMaxBBWidth                    = 0.10
	compressionZeroEligibleFallbackThreshold = 2
	broaderVolatilitySampleFloor             = 6
	defaultSweepVolumeRatio                  = 1.3
	defaultCompressionVolumeRatio            = 1.2
)

func isCompressionMacroContext(breadth float64) bool {
	return breadth >= compressionNeutralBreadthLower && breadth <= compressionNeutralBreadthUpper
}

func hasCompressionEvidence(indicators map[string]float64) bool {
	bbWidth := GetIndicator(indicators, IndicatorBBWidth)
	return GetIndicator(indicators, IndicatorContraction) == 1.0 || (bbWidth > 0 && bbWidth <= compressionMaxBBWidth)
}

func resolveConfiguredMinVolumeRatio(playbook Playbook, policy MarketPolicy, tier Tier) float64 {
	profile := GetPlaybookThresholdProfile(playbook, policy, tier)
	if profile.MinVolumeRatio > 0 {
		return profile.MinVolumeRatio
	}
	if playbook == COMPRESSION_BREAKOUT_RETEST {
		return defaultCompressionVolumeRatio
	}
	return defaultSweepVolumeRatio
}

func hasVolumeConfirmation(snapshot *TechnicalSnapshot, candles []dto.Candle, minVolumeRatio float64) bool {
	if minVolumeRatio <= 0 {
		minVolumeRatio = defaultSweepVolumeRatio
	}
	if snapshot != nil && snapshot.VolumeRatio >= minVolumeRatio {
		return true
	}
	return ConfirmLiquiditySweep(candles, 20, minVolumeRatio)
}
