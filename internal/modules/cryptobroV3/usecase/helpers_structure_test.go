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

func TestPopulateSnapshots_UsesRealBollingerWidthForContraction(t *testing.T) {
	baseTime := time.Now().Add(-12 * time.Hour)
	h1 := makeTrendCandles(baseTime, time.Hour, 60, 100, 0.1)
	h4 := makeTrendCandles(baseTime, 4*time.Hour, 220, 100, 0.1)

	tight := make([]dto.Candle, 40)
	wide := make([]dto.Candle, 40)
	for i := 0; i < 40; i++ {
		tightClose := 100.0
		if i%2 == 0 {
			tightClose = 100.05
		}
		wideClose := 100.0
		if i%2 == 0 {
			wideClose = 110.0
		} else {
			wideClose = 90.0
		}

		tight[i] = dto.Candle{
			Time:  baseTime.Add(time.Duration(i) * 15 * time.Minute),
			Open:  tightClose,
			High:  tightClose + 0.2,
			Low:   tightClose - 0.2,
			Close: tightClose,
			Vol:   100,
		}
		wide[i] = dto.Candle{
			Time:  baseTime.Add(time.Duration(i) * 15 * time.Minute),
			Open:  wideClose,
			High:  wideClose + 1,
			Low:   wideClose - 1,
			Close: wideClose,
			Vol:   100,
		}
	}

	tightTech, _ := PopulateSnapshots(tight, h1, h4, 0, 0, 0, 0, 0)
	wideTech, _ := PopulateSnapshots(wide, h1, h4, 0, 0, 0, 0, 0)

	if tightTech.IndicatorValues[IndicatorContraction] != 1.0 {
		t.Fatalf("expected tight series to register contraction, got %v", tightTech.IndicatorValues[IndicatorContraction])
	}
	maxWidth := compressionMaxBBWidth()
	if tightTech.IndicatorValues[IndicatorBBWidth] <= 0 || tightTech.IndicatorValues[IndicatorBBWidth] > maxWidth {
		t.Fatalf("expected tight series to have bb width <= %0.2f, got %0.4f", maxWidth, tightTech.IndicatorValues[IndicatorBBWidth])
	}
	if wideTech.IndicatorValues[IndicatorBBWidth] <= maxWidth {
		t.Fatalf("expected wide series to have bb width > %0.2f, got %0.4f", maxWidth, wideTech.IndicatorValues[IndicatorBBWidth])
	}
	if wideTech.IndicatorValues[IndicatorContraction] == 1.0 {
		t.Fatalf("expected wide series not to register contraction")
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
