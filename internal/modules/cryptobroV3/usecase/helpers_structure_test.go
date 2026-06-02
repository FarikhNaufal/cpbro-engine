package usecase

import (
	"testing"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
)

func TestPopulateSnapshots_DoesNotLabelBOSFromH4TrendOnly(t *testing.T) {
	baseTime := time.Now().Add(-10 * time.Hour)
	m15 := []dto.Candle{
		{Time: baseTime, Open: 100, High: 102, Low: 99, Close: 100, Vol: 100},
		{Time: baseTime.Add(15 * time.Minute), Open: 100, High: 103, Low: 99, Close: 101, Vol: 100},
		{Time: baseTime.Add(30 * time.Minute), Open: 101, High: 103, Low: 100, Close: 101, Vol: 100},
		{Time: baseTime.Add(45 * time.Minute), Open: 101, High: 102, Low: 99, Close: 100, Vol: 100},
	}
	for i := 4; i < 40; i++ {
		m15 = append(m15, dto.Candle{
			Time:  baseTime.Add(time.Duration(i) * 15 * time.Minute),
			Open:  100,
			High:  103,
			Low:   99,
			Close: 100 + float64(i%2),
			Vol:   100,
		})
	}

	h1 := makeTrendCandles(baseTime, time.Hour, 60, 100, 1)
	h4 := makeTrendCandles(baseTime, 4*time.Hour, 220, 100, 1)

	_, structure := PopulateSnapshots(m15, h1, h4, 0, 0, 0, 0, 0)

	if structure.MarketStructure == "BULLISH_BOS" || structure.MarketStructure == "BEARISH_BOS" {
		t.Fatalf("expected M15 structure not to be derived from H4 trend alone, got %s", structure.MarketStructure)
	}
	if structure.BOS {
		t.Fatalf("expected BOS=false without a close beyond prior M15 range")
	}
	if structure.H1Structure == "" {
		t.Fatalf("expected H1Structure to be populated for AI context")
	}
}

func TestDescribeCandleStructure_DetectsBreakOfStructure(t *testing.T) {
	baseTime := time.Now().Add(-4 * time.Hour)
	candles := []dto.Candle{
		{Time: baseTime, Open: 100, High: 101, Low: 99, Close: 100, Vol: 100},
		{Time: baseTime.Add(15 * time.Minute), Open: 100, High: 102, Low: 99, Close: 101, Vol: 100},
		{Time: baseTime.Add(30 * time.Minute), Open: 101, High: 103, Low: 100, Close: 102, Vol: 100},
		{Time: baseTime.Add(45 * time.Minute), Open: 102, High: 104, Low: 101, Close: 105, Vol: 100},
	}

	structure, bos, choch := DescribeCandleStructure(candles, "M15", "BULLISH")
	if structure != "M15_BULLISH_BOS" || !bos || choch {
		t.Fatalf("expected bullish BOS, got structure=%s bos=%v choch=%v", structure, bos, choch)
	}
}

func makeTrendCandles(base time.Time, step time.Duration, count int, start float64, increment float64) []dto.Candle {
	candles := make([]dto.Candle, count)
	for i := 0; i < count; i++ {
		close := start + float64(i)*increment
		candles[i] = dto.Candle{
			Time:  base.Add(time.Duration(i) * step),
			Open:  close - increment*0.5,
			High:  close + 1,
			Low:   close - 1,
			Close: close,
			Vol:   100,
		}
	}
	return candles
}
