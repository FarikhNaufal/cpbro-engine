package usecase

const (
	IndicatorADX                 = "adx"
	IndicatorATR                 = "atr"
	IndicatorFundingRate         = "funding_rate"
	IndicatorFundingAbs          = "funding_abs"
	IndicatorExtremeFunding      = "extreme_funding"
	IndicatorOIChange            = "oi_change"
	IndicatorExtremeOI           = "extreme_oi"
	IndicatorCrowdingScore       = "crowding_score"
	IndicatorHasOIData           = "has_oi_data"
	IndicatorHasCrowdingEvidence = "has_crowding_evidence"

	IndicatorWickRejection = "wick_rejection"
	IndicatorPARejection   = "pa_rejection"
	IndicatorSweepLow      = "sweep_low"
	IndicatorSweepHigh     = "sweep_high"
	IndicatorVolumeSpike   = "volume_spike"
	IndicatorContraction   = "contraction"
	IndicatorBBWidth       = "bb_width"
	IndicatorNearRangeEdge = "near_range_edge"

	IndicatorBreakoutLevel = "breakout_level"
	IndicatorRetestHold    = "retest_hold"
	IndicatorRetestTouches = "retest_touches"
)

func GetIndicator(values map[string]float64, key string) float64 {
	if values == nil {
		return 0
	}
	return values[key]
}
