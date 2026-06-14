package usecase

import (
	"math"
	"sort"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
	"cpbro-engine/internal/modules/cryptobroV3/entity"
)

type macroMarketState struct {
	Breadth          float64
	MedianAbsMove24h float64
	ActiveMoveShare  float64
	QuietMoveShare   float64
	BTCTrend         string
	BTCScore         float64
	BTCChaos         float64
	Volatility       string
}

func deriveMacroMarketState(tickers []dto.Ticker24h) macroMarketState {
	state := macroMarketState{
		Breadth:    0.5,
		BTCTrend:   "SIDEWAYS",
		BTCScore:   50.0,
		BTCChaos:   0.2,
		Volatility: "NORMAL",
	}
	if len(tickers) == 0 {
		return state
	}

	var (
		advancing    int
		activeMoves  int
		quietMoves   int
		absMoves     = make([]float64, 0, len(tickers))
		btcAbsChange float64
	)

	for i := range tickers {
		ticker := tickers[i]
		if ticker.PriceChangePercent > 0 {
			advancing++
		}

		absChange := math.Abs(ticker.PriceChangePercent)
		absMoves = append(absMoves, absChange)
		if absChange >= 1.5 {
			activeMoves++
		}
		if absChange <= 0.5 {
			quietMoves++
		}

		if ticker.Symbol == "BTCUSDT" {
			btcAbsChange = absChange
			switch {
			case ticker.PriceChangePercent > 1.5:
				state.BTCTrend = "BULLISH"
			case ticker.PriceChangePercent < -1.5:
				state.BTCTrend = "BEARISH"
			}
			state.BTCScore = 50.0 + (ticker.PriceChangePercent * 5.0)
			if state.BTCScore > 100.0 {
				state.BTCScore = 100.0
			} else if state.BTCScore < 0.0 {
				state.BTCScore = 0.0
			}
			if btcAbsChange > 5.0 {
				state.BTCChaos = 0.85
			} else if btcAbsChange < 0.5 {
				state.BTCChaos = 0.1
			}
		}
	}

	total := len(tickers)
	state.Breadth = float64(advancing) / float64(total)
	state.ActiveMoveShare = float64(activeMoves) / float64(total)
	state.QuietMoveShare = float64(quietMoves) / float64(total)
	state.MedianAbsMove24h = medianFloat64(absMoves)
	broadSampleReliable := total >= broaderVolatilitySampleFloor

	switch {
	case btcAbsChange > 5.0 || state.MedianAbsMove24h >= 3.0 || (broadSampleReliable && state.ActiveMoveShare >= 0.40):
		state.Volatility = "HIGH"
	case btcAbsChange < 0.5 && ((broadSampleReliable && state.MedianAbsMove24h <= 0.8 && state.ActiveMoveShare < 0.20 && state.QuietMoveShare >= 0.55) || state.MedianAbsMove24h <= 0.3):
		state.Volatility = "LOW"
	default:
		state.Volatility = "NORMAL"
	}

	return state
}

func medianFloat64(values []float64) float64 {
	if len(values) == 0 {
		return 0.0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	mid := len(sorted) / 2
	if len(sorted)%2 == 0 {
		return (sorted[mid-1] + sorted[mid]) / 2.0
	}
	return sorted[mid]
}

func isCompressionMacroActive(state macroMarketState) bool {
	return state.Volatility == "LOW" && state.BTCScore > 50.0 && isCompressionMacroContext(state.Breadth)
}

func shouldFallbackCompressionToLowVol(previous *entity.LatestResult, compressionMacroActive bool) bool {
	if !compressionMacroActive || previous == nil {
		return false
	}
	return previous.CompressionZeroEligibleStreak >= compressionZeroEligibleFallbackThreshold
}

func nextCompressionZeroEligibleStreak(previous *entity.LatestResult, compressionMacroActive bool, totalPlaybookEligible int) int {
	if !compressionMacroActive || totalPlaybookEligible > 0 {
		return 0
	}

	streak := 0
	if previous != nil {
		streak = previous.CompressionZeroEligibleStreak
	}
	return streak + 1
}
