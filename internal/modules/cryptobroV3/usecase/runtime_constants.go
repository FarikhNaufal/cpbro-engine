package usecase

import "cpbro-engine/internal/modules/cryptobroV3/dto"

const (
	defaultSweepVolumeRatio       = 1.3
	defaultCompressionVolumeRatio = 1.2
)

func compressionNeutralBreadthLower() float64 {
	return getRuntimeSettings().CompressionNeutralBreadthLower
}

func compressionNeutralBreadthUpper() float64 {
	return getRuntimeSettings().CompressionNeutralBreadthUpper
}

func compressionMaxBBWidth() float64 {
	return getRuntimeSettings().CompressionMaxBBWidth
}

func compressionZeroEligibleFallbackThreshold() int {
	return getRuntimeSettings().CompressionZeroEligibleFallbackThreshold
}

func broaderVolatilitySampleFloor() int {
	return getRuntimeSettings().BroaderVolatilitySampleFloor
}

func fundingExtremeThreshold() float64 {
	return getRuntimeSettings().FundingExtremeThreshold
}

func isCompressionMacroContext(breadth float64) bool {
	return breadth >= compressionNeutralBreadthLower() && breadth <= compressionNeutralBreadthUpper()
}

func hasCompressionEvidence(indicators map[string]float64) bool {
	bbWidth := GetIndicator(indicators, IndicatorBBWidth)
	return GetIndicator(indicators, IndicatorContraction) == 1.0 || (bbWidth > 0 && bbWidth <= compressionMaxBBWidth())
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
