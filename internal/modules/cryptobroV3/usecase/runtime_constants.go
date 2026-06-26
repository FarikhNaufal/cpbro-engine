package usecase

import "cpbro-engine/internal/modules/cryptobroV3/dto"

const (
	defaultSweepVolumeRatio                   = 1.3
	defaultCompressionVolumeRatio             = 1.2
	defaultRotationActivityThresholdAlt       = 0.45
	defaultRotationActivityThresholdDefensive = 0.65
	defaultRotationActivityThresholdLowVol    = 0.50
	defaultRotationActivityThresholdDefault   = 0.55
	defaultRotationPrefetchRatio              = 0.15
	defaultRotationPrefetchRatioAlt           = 0.20
	defaultRotationPrefetchRatioDefensive     = 0.10
	defaultUniverseHotBoost                   = 1.25
	defaultUniverseMaxHotBoost                = 1.5
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

func compressionBBWidthPercentile() float64 {
	return getRuntimeSettings().CompressionBBWidthPercentile
}

func compressionBBWidthLookback() int {
	return getRuntimeSettings().CompressionBBWidthLookback
}

func hasCompressionEvidence(indicators map[string]float64, m15Closed []dto.Candle) bool {
	if GetIndicator(indicators, IndicatorContraction) == 1.0 {
		return true
	}
	bbWidth := GetIndicator(indicators, IndicatorBBWidth)
	if bbWidth <= 0 {
		return false
	}
	if len(m15Closed) > 0 {
		lookback := compressionBBWidthLookback()
		pct := compressionBBWidthPercentile()
		if lookback > 0 && pct > 0 {
			threshold := CalculateBBWidthPercentileThreshold(m15Closed, lookback, pct)
			if threshold > 0 {
				maxBB := compressionMaxBBWidth()
				if threshold > maxBB {
					threshold = maxBB
				} else if threshold < 0.02 {
					threshold = 0.02
				}
				return bbWidth <= threshold
			}
		}
	}
	return bbWidth <= compressionMaxBBWidth()
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
