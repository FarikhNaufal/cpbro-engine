package usecase

import (
	"math"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
)

func calculateDirectionalWickRatio(candles []dto.Candle, direction Direction) (float64, bool) {
	if len(candles) == 0 {
		return 0, false
	}
	last := candles[len(candles)-1]
	candleRange := last.High - last.Low
	if candleRange <= 0 {
		return 0, false
	}

	lowerWick := (math.Min(last.Open, last.Close) - last.Low) / candleRange
	upperWick := (last.High - math.Max(last.Open, last.Close)) / candleRange
	lowerWick = math.Max(0, math.Min(1, lowerWick))
	upperWick = math.Max(0, math.Min(1, upperWick))

	switch direction {
	case LONG:
		return lowerWick, true
	case SHORT:
		return upperWick, true
	default:
		return math.Max(lowerWick, upperWick), true
	}
}

func calculateRetestQuality(indicators map[string]float64) float64 {
	if GetIndicator(indicators, IndicatorRetestHold) != 1.0 {
		return 0.0
	}
	touches := math.Max(0, GetIndicator(indicators, IndicatorRetestTouches))
	return math.Min(1.0, 0.25+math.Min(touches, 3.0)*0.25)
}

func calculateRangeClarity(structure StructureSnapshot) float64 {
	if structure.SessionHigh <= structure.SessionLow {
		return 0.0
	}
	highTouches := math.Min(float64(len(structure.Highs)), 2.0)
	lowTouches := math.Min(float64(len(structure.Lows)), 2.0)
	if highTouches == 0 || lowTouches == 0 {
		return 0.0
	}
	return (highTouches + lowTouches) / 4.0
}
