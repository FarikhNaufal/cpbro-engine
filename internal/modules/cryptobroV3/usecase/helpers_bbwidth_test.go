package usecase

import (
	"testing"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
)

func TestCalculateBBWidthPercentileThreshold(t *testing.T) {
	baseTime := time.Now().Add(-100 * time.Hour)

	// Scenario 1: Insufficient candles
	t.Run("Insufficient candles", func(t *testing.T) {
		candles := make([]dto.Candle, 10)
		res := CalculateBBWidthPercentileThreshold(candles, 100, 0.25)
		if res != 0 {
			t.Errorf("Expected 0 for insufficient candles, got %f", res)
		}
	})

	// Scenario 2: Varying volatility candles to test percentile selection
	t.Run("Percentile calculation order", func(t *testing.T) {
		candles := make([]dto.Candle, 120)
		for i := 0; i < 120; i++ {
			// Oscillating Close prices around 100 to produce different BB widths
			amplitude := 1.0
			if i > 80 {
				// Make the last 40 candles highly compressed (low amplitude)
				amplitude = 0.1
			}
			
			closeVal := 100.0
			if i%2 == 0 {
				closeVal += amplitude
			} else {
				closeVal -= amplitude
			}

			candles[i] = dto.Candle{
				Time:  baseTime.Add(time.Duration(i) * 15 * time.Minute),
				Open:  100.0,
				High:  closeVal + 0.1,
				Low:   closeVal - 0.1,
				Close: closeVal,
				Vol:   100,
			}
		}

		pct25 := CalculateBBWidthPercentileThreshold(candles, 100, 0.25)
		pct75 := CalculateBBWidthPercentileThreshold(candles, 100, 0.75)

		if pct25 <= 0 {
			t.Errorf("Expected positive 25th percentile threshold, got %f", pct25)
		}
		if pct25 >= pct75 {
			t.Errorf("Expected 25th percentile (%f) to be strictly less than 75th percentile (%f)", pct25, pct75)
		}
	})
}
