package usecase

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
)

type UsecaseHelpers struct{}

func CalculateDirectionalRR(direction Direction, entry, takeProfit, stopLoss float64) float64 {
	if entry <= 0 || takeProfit <= 0 || stopLoss <= 0 {
		return 0.0
	}
	switch direction {
	case LONG:
		if entry > stopLoss {
			return (takeProfit - entry) / (entry - stopLoss)
		}
	case SHORT:
		if stopLoss > entry {
			return (entry - takeProfit) / (stopLoss - entry)
		}
	}
	return 0.0
}

func NormalizeAIAuditSource(source string) string {
	normalized := strings.ToUpper(strings.TrimSpace(source))
	if normalized == "" {
		return AIAuditSourceReal
	}
	return normalized
}

func IsRealAIAuditSource(source string) bool {
	switch NormalizeAIAuditSource(source) {
	case AIAuditSourceReal, AIAuditSourceRealError:
		return true
	default:
		return false
	}
}

func WasAIAuditCalled(source string) bool {
	return IsRealAIAuditSource(source)
}

func FormatReasonBreakdown(layer, reason string) string {
	if strings.TrimSpace(layer) == "" {
		return strings.TrimSpace(reason)
	}
	if strings.TrimSpace(reason) == "" {
		return strings.TrimSpace(layer)
	}
	return fmt.Sprintf("%s: %s", strings.TrimSpace(layer), strings.TrimSpace(reason))
}

// RoundToDecimalPlaces rounds a float64 to precision.
func RoundToDecimalPlaces(val float64, precision int) float64 {
	ratio := math.Pow(10, float64(precision))
	return math.Round(val*ratio) / ratio
}

// 1. CalculateEMA computes Exponential Moving Average for a series.
func CalculateEMA(candles []dto.Candle, period int) []float64 {
	if len(candles) < period {
		return nil
	}
	ema := make([]float64, len(candles))

	// Simple SMA for first EMA point
	sum := 0.0
	for i := 0; i < period; i++ {
		sum += candles[i].Close
	}
	ema[period-1] = sum / float64(period)

	multiplier := 2.0 / float64(period+1)
	for i := period; i < len(candles); i++ {
		ema[i] = (candles[i].Close-ema[i-1])*multiplier + ema[i-1]
	}
	return ema
}

// CalculateH4Trend determines H4 Trend exclusively based on closed H4 close vs EMA H4.
// NEVER uses M15 lastPrice for H4 Trend calculation.
func CalculateH4Trend(h4Candles []dto.Candle, emaPeriod int) string {
	if len(h4Candles) == 0 {
		return "UNKNOWN"
	}
	effectivePeriod := emaPeriod
	if len(h4Candles) < effectivePeriod {
		effectivePeriod = len(h4Candles)
	}
	emas := CalculateEMA(h4Candles, effectivePeriod)
	if len(emas) == 0 {
		return "UNKNOWN"
	}

	lastClosedClose := h4Candles[len(h4Candles)-1].Close
	lastEMA := emas[len(emas)-1]

	if lastClosedClose > lastEMA {
		return "BULLISH"
	} else if lastClosedClose < lastEMA {
		return "BEARISH"
	}
	return "SIDEWAYS"
}

func isTrendPullbackTrendAligned(direction Direction, h4Trend, h1Trend string, regime MarketRegime) bool {
	expectedTrend := "BULLISH"
	if direction == SHORT {
		expectedTrend = "BEARISH"
	}

	if regime == HIGH_VOL || regime == BTC_CHAOS {
		return h4Trend == expectedTrend
	}

	if h4Trend != expectedTrend {
		return false
	}

	switch h1Trend {
	case expectedTrend, "SIDEWAYS":
		return true
	default:
		return false
	}
}

func validateTrendPullbackValueArea(m15Closed []dto.Candle, direction Direction) (bool, string) {
	if len(m15Closed) < 50 {
		return true, ""
	}

	ema20s := CalculateEMA(m15Closed, 20)
	ema50s := CalculateEMA(m15Closed, 50)
	if len(ema20s) == 0 || len(ema50s) == 0 {
		return true, ""
	}

	ema20 := ema20s[len(ema20s)-1]
	ema50 := ema50s[len(ema50s)-1]
	lowerEMA := math.Min(ema20, ema50)
	upperEMA := math.Max(ema20, ema50)

	lastCandle := m15Closed[len(m15Closed)-1]
	lastClose := lastCandle.Close
	lastLow, lastHigh := effectiveCandleBounds(lastCandle)

	tolerance := lastClose * 0.001
	if hasUsableHighLowRange(m15Closed, 14) {
		if atr := CalculateATR(m15Closed, 14); atr > 0 {
			tolerance = math.Max(tolerance, atr*0.20)
		}
	}

	lowerBound := lowerEMA - tolerance
	upperBound := upperEMA + tolerance
	overlapsValueArea := lastHigh >= lowerBound && lastLow <= upperBound
	if !overlapsValueArea {
		return false, fmt.Sprintf(
			"Price %f is outside the value area EMA band [%f - %f] with tolerance %f (candle range [%f - %f])",
			lastClose, lowerEMA, upperEMA, tolerance, lastLow, lastHigh,
		)
	}

	if direction == LONG && lastClose < lowerBound {
		return false, fmt.Sprintf(
			"Price %f is outside the value area EMA band [%f - %f] because LONG close failed to hold above %f",
			lastClose, lowerEMA, upperEMA, lowerBound,
		)
	}
	if direction == SHORT && lastClose > upperBound {
		return false, fmt.Sprintf(
			"Price %f is outside the value area EMA band [%f - %f] because SHORT close failed to hold below %f",
			lastClose, lowerEMA, upperEMA, upperBound,
		)
	}

	return true, ""
}

func effectiveCandleBounds(candle dto.Candle) (low float64, high float64) {
	high = candle.High
	low = candle.Low

	if high <= 0 && low <= 0 {
		referenceHigh := math.Max(candle.Open, candle.Close)
		referenceLow := math.Min(candle.Open, candle.Close)
		if referenceHigh <= 0 && referenceLow <= 0 {
			return candle.Close, candle.Close
		}
		return referenceLow, referenceHigh
	}

	if high <= 0 {
		high = math.Max(candle.Open, candle.Close)
		if high <= 0 {
			high = candle.Close
		}
	}
	if low <= 0 {
		low = math.Min(candle.Open, candle.Close)
		if low <= 0 {
			low = candle.Close
		}
	}
	if high < low {
		high, low = low, high
	}
	return low, high
}

func hasUsableHighLowRange(candles []dto.Candle, lookback int) bool {
	if len(candles) == 0 {
		return false
	}
	if lookback <= 0 || lookback > len(candles) {
		lookback = len(candles)
	}

	valid := 0
	start := len(candles) - lookback
	for i := start; i < len(candles); i++ {
		candle := candles[i]
		if candle.High > 0 && candle.Low > 0 && candle.High >= candle.Low {
			valid++
		}
	}

	minValid := 3
	if lookback < minValid {
		minValid = lookback
	}
	return valid >= minValid
}

// 2. ValidateBreakoutLevels enforces that breakouts use matching timeframe levels.
// NEVER compares H4 Close directly with M15 session levels.
func ValidateBreakoutLevels(levelTimeframe string, priceTimeframe string) bool {
	return levelTimeframe == priceTimeframe
}

// 3. ConfirmLiquiditySweep confirms if a liquidity sweep has volume confirmation (volume spike).
// Pinbar without volume spike is not enough.
func ConfirmLiquiditySweep(candles []dto.Candle, averagePeriod int, multiplier float64) bool {
	if len(candles) <= averagePeriod {
		return false
	}

	// Last closed candle is evaluated
	lastCandle := candles[len(candles)-1]

	sumVol := 0.0
	for i := len(candles) - 2; i >= len(candles)-1-averagePeriod; i-- {
		sumVol += candles[i].Vol
	}
	avgVol := sumVol / float64(averagePeriod)

	return lastCandle.Vol >= avgVol*multiplier
}

// 4. ValidateLongPath checks if long trade matches MarketPolicy long mode constraint.
func ValidateLongPath(policy MarketPolicy, playbook Playbook) bool {
	if !policy.AllowLong {
		return false
	}
	switch policy.LongMode {
	case SWEEP_ONLY:
		return playbook == LIQUIDITY_SWEEP_REVERSAL
	case PULLBACK_ONLY:
		return playbook == TREND_PULLBACK
	case REVERSAL_ONLY:
		return playbook == LIQUIDITY_SWEEP_REVERSAL || playbook == RANGE_EDGE_REVERSAL || playbook == CROWDED_POSITIONING_SQUEEZE
	case BREAKOUT_RETEST_ONLY:
		return playbook == COMPRESSION_BREAKOUT_RETEST
	case DISABLED:
		return false
	}
	return true
}

// 5. ValidateShortPath checks if short trade matches MarketPolicy short mode constraint.
func ValidateShortPath(policy MarketPolicy, playbook Playbook) bool {
	if !policy.AllowShort {
		return false
	}
	switch policy.ShortMode {
	case SWEEP_ONLY:
		return playbook == LIQUIDITY_SWEEP_REVERSAL
	case PULLBACK_ONLY:
		return playbook == TREND_PULLBACK
	case REVERSAL_ONLY:
		return playbook == LIQUIDITY_SWEEP_REVERSAL || playbook == RANGE_EDGE_REVERSAL || playbook == CROWDED_POSITIONING_SQUEEZE
	case BREAKOUT_RETEST_ONLY:
		return playbook == COMPRESSION_BREAKOUT_RETEST
	case DISABLED:
		return false
	}
	return true
}

func ValidateDirectionalPath(policy MarketPolicy, direction Direction, playbook Playbook) bool {
	switch direction {
	case LONG:
		return ValidateLongPath(policy, playbook)
	case SHORT:
		return ValidateShortPath(policy, playbook)
	default:
		return false
	}
}

func modeRejectReason(policy MarketPolicy, direction Direction, playbook Playbook) string {
	mode := policy.LongMode
	dirLabel := "LONG"
	if direction == SHORT {
		mode = policy.ShortMode
		dirLabel = "SHORT"
	}
	if mode == DISABLED {
		return fmt.Sprintf("%s trades are disabled", dirLabel)
	}
	playbookLabel := strings.ToLower(strings.ReplaceAll(string(playbook), "_", " "))
	return fmt.Sprintf("%s %s is disabled under %s policy mode", dirLabel, playbookLabel, mode)
}

// 6. ValidateShortDuringBTCBullish restricts shorting when BTC trend is bullish.
// Allowed playbooks: liquidity_sweep_high, failed_breakout, range edge reversal with strong rejection.
func ValidateShortDuringBTCBullish(playbook Playbook, strongRejection bool) bool {
	switch playbook {
	case LIQUIDITY_SWEEP_REVERSAL: // liquidity sweep high
		return true
	case COMPRESSION_BREAKOUT_RETEST: // failed breakout
		return true
	case RANGE_EDGE_REVERSAL: // range edge reversal with strong rejection
		return strongRejection
	}
	return false
}

// GetClosedCandlesOnly returns candles that are safe to treat as "closed" for indicator calculations.
//
// IMPORTANT:
//   - The Binance read-only service already attempts to exclude the currently-open kline.
//   - This helper MUST NOT drop the last candle blindly; it only drops it when the open-time indicates
//     the candle is still in-progress for the given timeframe.
func GetClosedCandlesOnly(candles []dto.Candle, timeframe time.Duration) []dto.Candle {
	if len(candles) <= 1 {
		return candles
	}
	if timeframe <= 0 {
		return candles
	}

	// dto.Candle.Time is treated as candle open-time in this project.
	last := candles[len(candles)-1]
	if last.Time.Add(timeframe).After(time.Now()) {
		return candles[:len(candles)-1]
	}
	return candles
}

// 7. VerifyIndicatorInput is kept for backward compatibility.
//
// NOTE:
// The live/latest price may equal the last closed candle close without being "pollution".
// Latest price must not be used as indicator input, but equality is not an issue.
func VerifyIndicatorInput(candles []dto.Candle, latestPrice float64) bool {
	_ = candles
	_ = latestPrice
	return true
}

// 8. ValidateStaleness blocks executing trade plans if the entry is stale.
func ValidateStaleness(lastUpdated time.Time, maxStaleness time.Duration) bool {
	return time.Since(lastUpdated) <= maxStaleness
}

// Helper methods for calculating technical indicators standalone
func CalculateRSI(candles []dto.Candle, period int) float64 {
	if len(candles) <= period {
		return 50.0
	}
	gains := 0.0
	losses := 0.0
	for i := 1; i <= period; i++ {
		diff := candles[i].Close - candles[i-1].Close
		if diff > 0 {
			gains += diff
		} else {
			losses -= diff
		}
	}
	avgGain := gains / float64(period)
	avgLoss := losses / float64(period)

	for i := period + 1; i < len(candles); i++ {
		diff := candles[i].Close - candles[i-1].Close
		gain := 0.0
		loss := 0.0
		if diff > 0 {
			gain = diff
		} else {
			loss = -diff
		}
		avgGain = (avgGain*float64(period-1) + gain) / float64(period)
		avgLoss = (avgLoss*float64(period-1) + loss) / float64(period)
	}

	if avgLoss == 0 {
		return 100.0
	}
	rs := avgGain / avgLoss
	return 100.0 - (100.0 / (1 + rs))
}

func CalculateMFI(candles []dto.Candle, period int) float64 {
	if len(candles) <= period {
		return 50.0
	}
	tp := make([]float64, len(candles))
	for i := range candles {
		tp[i] = (candles[i].High + candles[i].Low + candles[i].Close) / 3.0
	}

	posFlow := 0.0
	negFlow := 0.0
	startIdx := len(candles) - period
	for i := startIdx; i < len(candles); i++ {
		if i == 0 {
			continue
		}
		flow := tp[i] * candles[i].Vol
		if tp[i] > tp[i-1] {
			posFlow += flow
		} else if tp[i] < tp[i-1] {
			negFlow += flow
		}
	}
	if negFlow == 0 {
		if posFlow == 0 {
			return 50.0
		}
		return 100.0
	}
	mr := posFlow / negFlow
	return 100.0 - (100.0 / (1.0 + mr))
}

func CalculateMACD(candles []dto.Candle, fast, slow, signal int) (float64, float64) {
	if len(candles) < slow {
		return 0.0, 0.0
	}
	fastEMA := CalculateEMA(candles, fast)
	slowEMA := CalculateEMA(candles, slow)
	if len(fastEMA) == 0 || len(slowEMA) == 0 {
		return 0.0, 0.0
	}

	start := slow - 1
	if start < 0 {
		start = 0
	}
	macdSeries := make([]float64, 0, len(candles)-start)
	for i := start; i < len(candles); i++ {
		macdSeries = append(macdSeries, fastEMA[i]-slowEMA[i])
	}
	if len(macdSeries) == 0 {
		return 0.0, 0.0
	}

	macdLine := macdSeries[len(macdSeries)-1]
	signalSeries := CalculateEMAFromValues(macdSeries, signal)
	signalLine := 0.0
	if len(signalSeries) > 0 {
		signalLine = signalSeries[len(signalSeries)-1]
	}
	return macdLine, signalLine
}

func CalculateEMAFromValues(values []float64, period int) []float64 {
	if len(values) < period || period <= 0 {
		return nil
	}
	ema := make([]float64, len(values))

	sum := 0.0
	for i := 0; i < period; i++ {
		sum += values[i]
	}
	ema[period-1] = sum / float64(period)

	multiplier := 2.0 / float64(period+1)
	for i := period; i < len(values); i++ {
		ema[i] = (values[i]-ema[i-1])*multiplier + ema[i-1]
	}
	return ema
}

func CalculateATR(candles []dto.Candle, period int) float64 {
	if len(candles) <= 1 {
		return 0.0
	}
	sumTR := 0.0
	limit := period
	if len(candles)-1 < limit {
		limit = len(candles) - 1
	}
	for i := len(candles) - limit; i < len(candles); i++ {
		h := candles[i].High
		l := candles[i].Low
		pc := candles[i-1].Close
		tr := math.Max(h-l, math.Max(math.Abs(h-pc), math.Abs(l-pc)))
		sumTR += tr
	}
	return sumTR / float64(limit)
}

func CalculateADX(candles []dto.Candle, period int) float64 {
	if len(candles) <= period {
		return 15.0
	}
	trSum := 0.0
	dmPlusSum := 0.0
	dmMinusSum := 0.0
	start := len(candles) - period
	if start < 1 {
		start = 1
	}
	for i := start; i < len(candles); i++ {
		h := candles[i].High
		l := candles[i].Low
		ph := candles[i-1].High
		pl := candles[i-1].Low
		pc := candles[i-1].Close

		tr := math.Max(h-l, math.Max(math.Abs(h-pc), math.Abs(l-pc)))
		trSum += tr

		dmPlus := h - ph
		dmMinus := pl - l
		if dmPlus > dmMinus && dmPlus > 0 {
			dmPlusSum += dmPlus
		} else if dmMinus > dmPlus && dmMinus > 0 {
			dmMinusSum += dmMinus
		}
	}

	if trSum == 0 {
		return 15.0
	}
	diPlus := 100.0 * dmPlusSum / trSum
	diMinus := 100.0 * dmMinusSum / trSum
	diff := math.Abs(diPlus - diMinus)
	sum := diPlus + diMinus
	if sum == 0 {
		return 15.0
	}
	return 100.0 * diff / sum
}

func HighestHigh(candles []dto.Candle, period int) float64 {
	if len(candles) == 0 {
		return 0.0
	}
	high := candles[len(candles)-1].High
	limit := period
	if len(candles) < limit {
		limit = len(candles)
	}
	for i := len(candles) - limit; i < len(candles); i++ {
		if candles[i].High > high {
			high = candles[i].High
		}
	}
	return high
}

func LowestLow(candles []dto.Candle, period int) float64 {
	if len(candles) == 0 {
		return 0.0
	}
	low := candles[len(candles)-1].Low
	limit := period
	if len(candles) < limit {
		limit = len(candles)
	}
	for i := len(candles) - limit; i < len(candles); i++ {
		if candles[i].Low < low {
			low = candles[i].Low
		}
	}
	return low
}

func DescribeCandleStructure(candles []dto.Candle, timeframe string, dominantTrend string) (string, bool, bool) {
	closed := candles
	if len(closed) < 3 {
		return timeframe + "_INSUFFICIENT_STRUCTURE_DATA", false, false
	}

	last := closed[len(closed)-1]
	lookback := 20
	if len(closed)-1 < lookback {
		lookback = len(closed) - 1
	}

	prior := closed[:len(closed)-1]
	priorHigh := HighestHigh(prior, lookback)
	priorLow := LowestLow(prior, lookback)

	bullishBreak := priorHigh > 0 && last.Close > priorHigh
	bearishBreak := priorLow > 0 && last.Close < priorLow

	if bullishBreak {
		if dominantTrend == "BEARISH" {
			return timeframe + "_BULLISH_CHOCH", false, true
		}
		return timeframe + "_BULLISH_BOS", true, false
	}
	if bearishBreak {
		if dominantTrend == "BULLISH" {
			return timeframe + "_BEARISH_CHOCH", false, true
		}
		return timeframe + "_BEARISH_BOS", true, false
	}

	prev := closed[len(closed)-2]
	prev2 := closed[len(closed)-3]
	if last.Close > prev.Close && prev.Close > prev2.Close {
		return timeframe + "_BULLISH_CONTINUATION", false, false
	}
	if last.Close < prev.Close && prev.Close < prev2.Close {
		return timeframe + "_BEARISH_CONTINUATION", false, false
	}

	return timeframe + "_RANGE_BOUND", false, false
}

// PopulateSnapshots builds high-fidelity TechnicalSnapshot and StructureSnapshot for use in selectors & gates.
func PopulateSnapshots(m15 []dto.Candle, h1 []dto.Candle, h4 []dto.Candle, fundingRate float64, latestPrice float64, priceChange24h float64, openInterest float64, oiChangePct float64) (*TechnicalSnapshot, *StructureSnapshot) {
	m15Closed := GetClosedCandlesOnly(m15, defaultClosedCandleTimeframe)
	h1Closed := GetClosedCandlesOnly(h1, time.Hour)
	h4Closed := GetClosedCandlesOnly(h4, 4*time.Hour)

	// Safe checks
	if len(m15Closed) < 14 {
		m15Closed = m15
	}
	if len(h1Closed) < 14 {
		h1Closed = h1
	}
	if len(h4Closed) < 14 {
		h4Closed = h4
	}

	tech, structure, _, _ := populateSnapshotsFromClosedCandles(m15Closed, h1Closed, h4Closed, fundingRate, latestPrice, priceChange24h, openInterest, oiChangePct)
	return tech, structure
}

func populateSnapshotsFromClosedCandles(m15Closed []dto.Candle, h1Closed []dto.Candle, h4Closed []dto.Candle, fundingRate float64, latestPrice float64, priceChange24h float64, openInterest float64, oiChangePct float64) (*TechnicalSnapshot, *StructureSnapshot, string, string) {
	h4Trend := CalculateH4Trend(h4Closed, 200)
	h1Trend := CalculateH4Trend(h1Closed, 50)
	rsiVal := CalculateRSI(m15Closed, 14)
	adxVal := CalculateADX(m15Closed, 14)
	macdLine, signalLine := CalculateMACD(m15Closed, 12, 26, 9)

	// Calculate slopes
	rsiSlope := 0.0
	if len(m15Closed) >= 15 {
		prevRsi := CalculateRSI(m15Closed[:len(m15Closed)-1], 14)
		rsiSlope = rsiVal - prevRsi
	}

	mfiVal := CalculateMFI(m15Closed, 14)
	mfiSlope := 0.0
	if len(m15Closed) >= 15 {
		prevMfi := CalculateMFI(m15Closed[:len(m15Closed)-1], 14)
		mfiSlope = mfiVal - prevMfi
	}

	adxSlope := 0.0
	if len(m15Closed) >= 15 {
		prevAdx := CalculateADX(m15Closed[:len(m15Closed)-1], 14)
		adxSlope = adxVal - prevAdx
	}

	atrVal := CalculateATR(m15Closed, 14)
	atrPercent := 0.0
	if len(m15Closed) > 0 {
		lastClose := m15Closed[len(m15Closed)-1].Close
		if lastClose > 0 {
			atrPercent = (atrVal / lastClose) * 100.0
		}
	}

	volumeRatio := 1.0
	if len(m15Closed) > 0 {
		lastVol := m15Closed[len(m15Closed)-1].Vol
		sumVol := 0.0
		count := 0
		for i := len(m15Closed) - 2; i >= 0 && i >= len(m15Closed)-21; i-- {
			sumVol += m15Closed[i].Vol
			count++
		}
		avgVol := 1.0
		if count > 0 && sumVol > 0 {
			avgVol = sumVol / float64(count)
		}
		volumeRatio = lastVol / avgVol
	}

	tech := &TechnicalSnapshot{
		RSI:             rsiVal,
		MACD:            macdLine,
		EMA200:          0.0,
		Timeframe:       "M15",
		Notes:           fmt.Sprintf("MACD Line: %0.4f | Signal Line: %0.4f", macdLine, signalLine),
		IndicatorValues: make(map[string]float64),
		RSISlope:        rsiSlope,
		MFI:             mfiVal,
		MFISlope:        mfiSlope,
		ADXSlope:        adxSlope,
		ATRPercent:      atrPercent,
		VolumeRatio:     volumeRatio,
		OIChange:        0.0,
		FundingRate:     fundingRate,
		PriceChange24h:  priceChange24h,
	}
	_ = latestPrice

	tech.IndicatorValues[IndicatorADX] = adxVal

	if len(m15Closed) > 0 {
		lastCandle := m15Closed[len(m15Closed)-1]
		// Rejections
		lowRej := (math.Min(lastCandle.Open, lastCandle.Close) - lastCandle.Low) / (lastCandle.High - lastCandle.Low + 0.000001)
		highRej := (lastCandle.High - math.Max(lastCandle.Open, lastCandle.Close)) / (lastCandle.High - lastCandle.Low + 0.000001)

		if lowRej > 0.4 || highRej > 0.4 {
			tech.IndicatorValues[IndicatorWickRejection] = 1.0
		}

		// Sweeps
		if len(m15Closed) >= 21 {
			highest20 := HighestHigh(m15Closed[:len(m15Closed)-1], 20)
			lowest20 := LowestLow(m15Closed[:len(m15Closed)-1], 20)
			if lastCandle.Low < lowest20 && lastCandle.Close > lowest20 {
				tech.IndicatorValues[IndicatorSweepLow] = 1.0
			}
			if lastCandle.High > highest20 && lastCandle.Close < highest20 {
				tech.IndicatorValues[IndicatorSweepHigh] = 1.0
			}
		}

		// Vol spike baseline
		if hasVolumeConfirmation(tech, m15Closed, defaultSweepVolumeRatio) {
			tech.IndicatorValues[IndicatorVolumeSpike] = 1.0
		} else {
			tech.IndicatorValues[IndicatorVolumeSpike] = -1.0
		}

		// Compression from real Bollinger bandwidth, not ATR proxy.
		tech.IndicatorValues[IndicatorATR] = atrVal
		tech.IndicatorValues[IndicatorContraction] = 0.0
		if basis, upper, lower := CalculateBollingerBands(m15Closed, 20, 2.0); len(basis) > 0 {
			lastIdx := len(m15Closed) - 1
			if lastIdx >= 0 && lastIdx < len(basis) && basis[lastIdx] > 0 {
				bbWidth := (upper[lastIdx] - lower[lastIdx]) / basis[lastIdx]
				tech.IndicatorValues[IndicatorBBWidth] = bbWidth

				// Adaptive: use rolling percentile if configured, fallback to absolute threshold
				contractionDetected := false
				pctThreshold := 0.0
				lookback := compressionBBWidthLookback()
				pct := compressionBBWidthPercentile()
				if lookback > 0 && pct > 0 {
					pctThreshold = CalculateBBWidthPercentileThreshold(m15Closed, lookback, pct)
					if pctThreshold > 0 {
						maxBB := compressionMaxBBWidth()
						if pctThreshold > maxBB {
							pctThreshold = maxBB
						} else if pctThreshold < 0.02 {
							pctThreshold = 0.02
						}
					}
				}
				if pctThreshold > 0 {
					contractionDetected = bbWidth <= pctThreshold
				} else {
					// Fallback to absolute threshold (legacy behavior)
					contractionDetected = bbWidth <= compressionMaxBBWidth()
				}
				if contractionDetected {
					tech.IndicatorValues[IndicatorContraction] = 1.0
				}
				// Store percentile threshold for debugging/observability
				tech.IndicatorValues["bb_width_percentile_threshold"] = pctThreshold
			}
		}

		// Funding (OI is optional; do NOT dummy-true).
		tech.IndicatorValues[IndicatorFundingRate] = fundingRate
		tech.IndicatorValues[IndicatorFundingAbs] = math.Abs(fundingRate)
		if math.Abs(fundingRate) > fundingExtremeThreshold() {
			tech.IndicatorValues[IndicatorExtremeFunding] = 1.0
		} else {
			tech.IndicatorValues[IndicatorExtremeFunding] = 0.0
		}

		// OI / crowding defaults when data is unavailable.
		tech.IndicatorValues[IndicatorHasOIData] = 0.0
		if openInterest > 0 {
			tech.IndicatorValues[IndicatorHasOIData] = 1.0
		}
		tech.OIChange = oiChangePct
		tech.IndicatorValues[IndicatorOIChange] = oiChangePct

		extremeOI := 0.0
		if math.Abs(oiChangePct) >= 1.0 {
			extremeOI = 1.0
		}
		tech.IndicatorValues[IndicatorExtremeOI] = extremeOI

		crowdingScore := math.Max(
			math.Min(1.0, math.Abs(fundingRate)/0.005),
			math.Min(1.0, math.Abs(oiChangePct)/1.0),
		)
		tech.IndicatorValues[IndicatorCrowdingScore] = crowdingScore

		hasEvidence := 0.0
		if openInterest > 0 && (tech.IndicatorValues[IndicatorExtremeFunding] == 1.0 || math.Abs(oiChangePct) > 0) {
			hasEvidence = 1.0
		}
		tech.IndicatorValues[IndicatorHasCrowdingEvidence] = hasEvidence

		// Set pa_rejection dynamically
		paRejection := -1.0
		if len(m15Closed) >= 5 {
			lastClosedCandle := m15Closed[len(m15Closed)-1]
			prevClosedCandle := m15Closed[len(m15Closed)-2]

			lowRejection := (math.Min(lastClosedCandle.Open, lastClosedCandle.Close) - lastClosedCandle.Low) / (lastClosedCandle.High - lastClosedCandle.Low + 0.000001)
			highRejection := (lastClosedCandle.High - math.Max(lastClosedCandle.Open, lastClosedCandle.Close)) / (lastClosedCandle.High - lastClosedCandle.Low + 0.000001)

			if lowRejection > 0.4 || highRejection > 0.4 || (lastClosedCandle.Low < prevClosedCandle.Low && lastClosedCandle.Close > prevClosedCandle.Close) || (lastClosedCandle.High > prevClosedCandle.High && lastClosedCandle.Close < prevClosedCandle.Close) {
				paRejection = 1.0
			}
		}
		tech.IndicatorValues[IndicatorPARejection] = paRejection

		// Near range edge
		if len(m15Closed) >= 40 {
			rangeHigh := HighestHigh(m15Closed, 40)
			rangeLow := LowestLow(m15Closed, 40)
			if lastCandle.Close > rangeHigh*0.99 || lastCandle.Close < rangeLow*1.01 {
				tech.IndicatorValues[IndicatorNearRangeEdge] = 1.0
			}
		}
	}

	sessionHigh := HighestHigh(m15Closed, 40)
	sessionLow := LowestLow(m15Closed, 40)
	liquidityUpper := HighestHigh(m15Closed, 20)
	liquidityLower := LowestLow(m15Closed, 20)

	var highs []float64
	var lows []float64
	for i := len(m15Closed) - 3; i >= 2 && len(highs) < 5; i-- {
		if m15Closed[i].High > m15Closed[i-1].High && m15Closed[i].High > m15Closed[i-2].High &&
			m15Closed[i].High > m15Closed[i+1].High && m15Closed[i].High > m15Closed[i+2].High {
			highs = append(highs, m15Closed[i].High)
		}
		if m15Closed[i].Low < m15Closed[i-1].Low && m15Closed[i].Low < m15Closed[i-2].Low &&
			m15Closed[i].Low < m15Closed[i+1].Low && m15Closed[i].Low < m15Closed[i+2].Low {
			lows = append(lows, m15Closed[i].Low)
		}
	}

	explicitSupport := 0.0
	if len(lows) > 0 {
		explicitSupport = lows[0]
	}

	explicitResistance := 0.0
	if len(highs) > 0 {
		explicitResistance = highs[0]
	}

	support := 0.0
	if explicitSupport > 0 {
		support = explicitSupport
	} else if sessionLow > 0 {
		support = sessionLow
	} else if liquidityLower > 0 {
		support = liquidityLower
	}

	resistance := 0.0
	if explicitResistance > 0 {
		resistance = explicitResistance
	} else if sessionHigh > 0 {
		resistance = sessionHigh
	} else if liquidityUpper > 0 {
		resistance = liquidityUpper
	}

	m15Structure, bos, choch := DescribeCandleStructure(m15Closed, "M15", h1Trend)
	h1Structure, _, _ := DescribeCandleStructure(h1Closed, "H1", h4Trend)
	sweepLow := tech.IndicatorValues[IndicatorSweepLow] == 1.0
	sweepHigh := tech.IndicatorValues[IndicatorSweepHigh] == 1.0

	structure := &StructureSnapshot{
		MarketStructure: m15Structure,
		H1Structure:     h1Structure,
		BOS:             bos,
		CHOCH:           choch,
		Timeframe:       "M15",
		Notes:           fmt.Sprintf("M15Structure: %s | H1Structure: %s | H4Trend: %s | H1Trend: %s | sweep_low=%v | sweep_high=%v", m15Structure, h1Structure, h4Trend, h1Trend, sweepLow, sweepHigh),
		Highs:           highs,
		Lows:            lows,
		Support:         support,
		Resistance:      resistance,
		SessionHigh:     sessionHigh,
		SessionLow:      sessionLow,
		LiquidityUpper:  liquidityUpper,
		LiquidityLower:  liquidityLower,
	}

	return tech, structure, h4Trend, h1Trend
}

func CalculateBollingerBands(candles []dto.Candle, period int, multiplier float64) (basis, upper, lower []float64) {
	if len(candles) < period {
		return nil, nil, nil
	}
	basis = make([]float64, len(candles))
	upper = make([]float64, len(candles))
	lower = make([]float64, len(candles))

	for i := period - 1; i < len(candles); i++ {
		sum := 0.0
		for j := i - period + 1; j <= i; j++ {
			sum += candles[j].Close
		}
		sma := sum / float64(period)
		basis[i] = sma

		variance := 0.0
		for j := i - period + 1; j <= i; j++ {
			diff := candles[j].Close - sma
			variance += diff * diff
		}
		stdDev := math.Sqrt(variance / float64(period))

		upper[i] = sma + multiplier*stdDev
		lower[i] = sma - multiplier*stdDev
	}
	return basis, upper, lower
}

var symbolExceptions = map[string]string{
	// "API_SYMBOL": "FUTURES_SYMBOL" (manual overrides if any mismatch occurs)
}

// NormalizeBaseSymbol converts a symbol (e.g. 1000PEPEUSDT, BONK, 1000000BABYDOGEUSDT)
// to its base clean symbol name (e.g. PEPE, BONK, BABYDOGE) to support contract aliasing.
func NormalizeBaseSymbol(sym string) string {
	sym = strings.ToUpper(strings.TrimSpace(sym))

	// Strip trailing USDT and USD
	sym = strings.TrimSuffix(sym, "USDT")
	sym = strings.TrimSuffix(sym, "USD")

	// Strip leading digits (e.g., 1000, 1000000)
	for len(sym) > 0 && sym[0] >= '0' && sym[0] <= '9' {
		sym = sym[1:]
	}

	// Check manual exceptions override
	if exc, ok := symbolExceptions[sym]; ok {
		return exc
	}
	return sym
}

// CalculateBBWidthPercentileThreshold computes the Nth percentile of BBWidth values
// over the last `lookback` closed candles. Used for adaptive compression detection.
// Returns 0 if insufficient candles.
func CalculateBBWidthPercentileThreshold(candles []dto.Candle, lookback int, percentile float64) float64 {
	if lookback < 20 || len(candles) < 20 {
		return 0
	}
	// Use up to `lookback` candles, but at least 20
	start := 0
	if len(candles) > lookback {
		start = len(candles) - lookback
	}
	window := candles[start:]

	basis, upper, lower := CalculateBollingerBands(window, 20, 2.0)
	if len(basis) == 0 {
		return 0
	}

	var widths []float64
	for i := 19; i < len(window); i++ { // skip first 19 where BB not computed
		if basis[i] > 0 {
			w := (upper[i] - lower[i]) / basis[i]
			if w > 0 {
				widths = append(widths, w)
			}
		}
	}
	if len(widths) == 0 {
		return 0
	}

	sorted := append([]float64(nil), widths...)
	sort.Float64s(sorted)

	idx := int(math.Floor(percentile * float64(len(sorted))))
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
