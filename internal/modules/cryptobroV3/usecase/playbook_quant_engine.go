package usecase

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
)

type PlaybookQuantEngineUsecase struct{}

func NewPlaybookQuantEngineUsecase() *PlaybookQuantEngineUsecase {
	return &PlaybookQuantEngineUsecase{}
}

// Run is kept for simple backwards compatibility checks.
func (uc *PlaybookQuantEngineUsecase) Run(m15, h1, h4 []dto.Candle) QuantResult {
	if len(m15) == 0 {
		return QuantResult{Direction: WAIT, IndicatorMet: false}
	}
	lastClosed := m15[len(m15)-1]
	return QuantResult{
		Direction:    LONG,
		TriggerPrice: lastClosed.Close,
		StopLoss:     lastClosed.Close * 0.98,
		TakeProfit:   lastClosed.Close * 1.05,
		IndicatorMet: true,
	}
}

// RunEngine processes indicator logic using only closed candles, sets up TradePlan details, and saves M15 raw klines.
func (uc *PlaybookQuantEngineUsecase) RunEngine(
	playbook Playbook,
	direction Direction,
	data MarketData,
	policy MarketPolicy,
) QuantResult {
	// Filter to strictly closed candles for indicators (excluding open candle at index len-1)
	m15Closed := GetClosedCandlesOnly(data.M15Candles, 15*time.Minute)
	h1Closed := GetClosedCandlesOnly(data.H1Candles, time.Hour)
	h4Closed := GetClosedCandlesOnly(data.H4Candles, 4*time.Hour)

	if len(m15Closed) < 14 {
		return QuantResult{
			Symbol:       data.Symbol,
			Direction:    WAIT,
			Status:       PLAYBOOK_REJECTED,
			Reason:       "Insufficient closed M15 candles available for indicator calculations (minimum 14 required)",
			IndicatorMet: false,
		}
	}

	// Save M15 raw klines (last 30 closed candles)
	uc.saveM15RawKlines(data.Symbol, m15Closed)

	lastM15 := m15Closed[len(m15Closed)-1]
	triggerPrice := lastM15.Close

	// Build snapshots using PopulateSnapshots helper
	techSnapPtr, structSnapPtr := PopulateSnapshots(data.M15Candles, data.H1Candles, data.H4Candles, data.FundingRate, data.LatestPrice, data.PriceChange24h, data.OpenInterestM15, data.OIChangePct)
	techSnap := *techSnapPtr
	structSnap := *structSnapPtr

	h4Trend := CalculateH4Trend(h4Closed, 200)
	h1Trend := CalculateH4Trend(h1Closed, 50)

	// Build default result values
	res := QuantResult{
		Symbol:            data.Symbol,
		Direction:         direction,
		Playbook:          playbook,
		TriggerPrice:      triggerPrice,
		H4Trend:           h4Trend,
		H1Trend:           h1Trend,
		MarketStructure:   structSnap.MarketStructure,
		Status:            QUANT_CANDIDATE,
		IndicatorMet:      true,
		TechnicalSnapshot: techSnap,
		StructureSnapshot: structSnap,
	}

	atr := CalculateATR(m15Closed, 14)
	if atr == 0 {
		atr = triggerPrice * 0.01 // fallback to 1% ATR
	}

	// Policy and safety check for SHORT paths
	if direction == SHORT {
		if !ValidateShortPath(policy, playbook) {
			return QuantResult{
				Symbol:       data.Symbol,
				Direction:    WAIT,
				Status:       PLAYBOOK_REJECTED,
				Reason:       "SHORT direction rejected by short path policy validation",
				IndicatorMet: false,
			}
		}
		// Bullish short restriction checks
		isStrongRejection := false
		if techSnap.IndicatorValues["wick_rejection"] == 1.0 {
			isStrongRejection = true
		}
		if (h4Trend == "BULLISH" || policy.BtcTrend == "BULLISH") && !ValidateShortDuringBTCBullish(playbook, isStrongRejection) {
			return QuantResult{
				Symbol:       data.Symbol,
				Direction:    WAIT,
				Status:       PLAYBOOK_REJECTED,
				Reason:       "SHORT direction rejected by BTC bullish safety helper rules",
				IndicatorMet: false,
			}
		}
	}

	switch playbook {
	case TREND_PULLBACK:
		res.SetupType = "PULLBACK"
		if direction == LONG {
			res.StopLoss = triggerPrice - (1.5 * atr)
			res.TakeProfit = triggerPrice + (2.0 * atr)
		} else if direction == SHORT {
			res.StopLoss = triggerPrice + (1.5 * atr)
			res.TakeProfit = triggerPrice - (2.0 * atr)
		}

	case LIQUIDITY_SWEEP_REVERSAL:
		res.SetupType = "SWEEP"
		if len(m15Closed) < 21 {
			res.Direction = WAIT
			res.IndicatorMet = false
			res.Reason = "Insufficient candles for sweep high/low calculations"
			break
		}
		highest := HighestHigh(m15Closed[:len(m15Closed)-1], 20)
		lowest := LowestLow(m15Closed[:len(m15Closed)-1], 20)
		isSweepHigh := lastM15.High > highest
		isSweepLow := lastM15.Low < lowest

		// Volume confirmation
		volSpike := ConfirmLiquiditySweep(m15Closed, 20, 1.5)
		if !volSpike {
			res.Direction = WAIT
			res.IndicatorMet = false
			res.Reason = "Liquidity sweep lacks volume spike confirmation"
			break
		}

		if direction == LONG {
			if !isSweepLow {
				res.Direction = WAIT
				res.IndicatorMet = false
				res.Reason = "No sweep low confirmation detected"
				break
			}
			res.StopLoss = lowest - (0.5 * atr)
			res.TakeProfit = triggerPrice + (1.8 * atr)
		} else if direction == SHORT {
			if !isSweepHigh {
				res.Direction = WAIT
				res.IndicatorMet = false
				res.Reason = "No sweep high confirmation detected"
				break
			}
			res.StopLoss = highest + (0.5 * atr)
			res.TakeProfit = triggerPrice - (1.8 * atr)
		}

	case COMPRESSION_BREAKOUT_RETEST:
		res.SetupType = "BREAKOUT"
		if direction == LONG {
			res.StopLoss = triggerPrice - (1.2 * atr)
			res.TakeProfit = triggerPrice + (2.5 * atr)
		} else if direction == SHORT {
			res.StopLoss = triggerPrice + (1.2 * atr)
			res.TakeProfit = triggerPrice - (2.5 * atr)
		}

		// Breakout retest evidence (explicit, for scoring/AI context).
		if len(m15Closed) >= 22 {
			rangeHigh := HighestHigh(m15Closed[:len(m15Closed)-1], 20)
			rangeLow := LowestLow(m15Closed[:len(m15Closed)-1], 20)
			level := rangeHigh
			if direction == SHORT {
				level = rangeLow
			}

			retestTouches := 0.0
			retestHold := 0.0
			lookback := 5
			if len(m15Closed) < lookback {
				lookback = len(m15Closed)
			}
			start := len(m15Closed) - lookback
			for i := start; i < len(m15Closed); i++ {
				c := m15Closed[i]
				if direction == LONG {
					if c.Low <= level && c.Close >= level {
						retestTouches++
					}
				} else if direction == SHORT {
					if c.High >= level && c.Close <= level {
						retestTouches++
					}
				}
			}
			if retestTouches > 0 {
				retestHold = 1.0
				// Mark setup as a true retest stage so LocalGate/FinalGate will not treat it as
				// "first breakout candle" (which must be WATCH-only).
				res.SetupType = "BREAKOUT_RETEST"
			}

			techSnap.IndicatorValues[IndicatorBreakoutLevel] = level
			techSnap.IndicatorValues[IndicatorRetestTouches] = retestTouches
			techSnap.IndicatorValues[IndicatorRetestHold] = retestHold
		}

	case RANGE_EDGE_REVERSAL:
		res.SetupType = "RANGE"
		if len(m15Closed) < 41 {
			res.Direction = WAIT
			res.IndicatorMet = false
			res.Reason = "Insufficient candles for range calculation"
			break
		}
		midPrice := (HighestHigh(m15Closed, 40) + LowestLow(m15Closed, 40)) / 2.0
		adxVal := techSnap.IndicatorValues[IndicatorADX]

		if direction == LONG {
			if adxVal > 30.0 && h4Trend == "BEARISH" {
				res.Direction = WAIT
				res.IndicatorMet = false
				res.Reason = "LONG range edge reversal rejected due to strong bearish trend expansion"
				break
			}
			res.StopLoss = triggerPrice - (1.2 * atr)
			res.TakeProfit = midPrice
			if res.TakeProfit <= triggerPrice {
				res.TakeProfit = triggerPrice + (1.5 * atr)
			}
			res.Reason = "LONG range edge reversal near support"
		} else { // SHORT
			if adxVal > 30.0 && h4Trend == "BULLISH" {
				res.Direction = WAIT
				res.IndicatorMet = false
				res.Reason = "SHORT range edge reversal rejected due to strong bullish trend expansion"
				break
			}
			res.StopLoss = triggerPrice + (1.2 * atr)
			res.TakeProfit = midPrice
			if res.TakeProfit >= triggerPrice {
				res.TakeProfit = triggerPrice - (1.5 * atr)
			}
			res.Reason = "SHORT range edge reversal near resistance"
		}

	case CROWDED_POSITIONING_SQUEEZE:
		res.SetupType = "SQUEEZE"
		if direction == LONG {
			// Short squeeze: price fails breakdown, reclaim support
			res.StopLoss = triggerPrice - (1.5 * atr)
			res.TakeProfit = triggerPrice + (2.0 * atr)
			res.Reason = "LONG short-squeeze trade plan"
		} else { // SHORT
			// Long squeeze: price fails breakout, reject resistance
			res.StopLoss = triggerPrice + (1.5 * atr)
			res.TakeProfit = triggerPrice - (2.0 * atr)
			res.Reason = "SHORT long-squeeze trade plan"
		}
	}

	// If setup criteria failed
	if !res.IndicatorMet {
		res.Status = PLAYBOOK_REJECTED
		return res
	}

	// Validate TradePlan bounds and compute RR
	if res.StopLoss <= 0 || res.TakeProfit <= 0 {
		res.Direction = WAIT
		res.IndicatorMet = false
		res.Status = PLAYBOOK_REJECTED
		res.Reason = "Invalid TradePlan SL or TP values generated"
		return res
	}

	rr := 0.0
	if direction == LONG {
		if res.StopLoss >= triggerPrice {
			res.Direction = WAIT
			res.IndicatorMet = false
			res.Status = PLAYBOOK_REJECTED
			res.Reason = "TradePlan SL must be below trigger price for LONG setups"
			return res
		}
		if res.TakeProfit <= triggerPrice {
			res.Direction = WAIT
			res.IndicatorMet = false
			res.Status = PLAYBOOK_REJECTED
			res.Reason = "TradePlan TP must be above trigger price for LONG setups"
			return res
		}
		rr = (res.TakeProfit - triggerPrice) / (triggerPrice - res.StopLoss)
	} else if direction == SHORT {
		if res.StopLoss <= triggerPrice {
			res.Direction = WAIT
			res.IndicatorMet = false
			res.Status = PLAYBOOK_REJECTED
			res.Reason = "TradePlan SL must be above trigger price for SHORT setups"
			return res
		}
		if res.TakeProfit >= triggerPrice {
			res.Direction = WAIT
			res.IndicatorMet = false
			res.Status = PLAYBOOK_REJECTED
			res.Reason = "TradePlan TP must be below trigger price for SHORT setups"
			return res
		}
		rr = (triggerPrice - res.TakeProfit) / (res.StopLoss - triggerPrice)
	}

	if rr < 1.5 {
		res.Reason = fmt.Sprintf("%s (Caution: RR %0.2f is below ideal 1.5)", res.Reason, rr)
	}

	// Build and assign TradePlan
	tp := TradePlan{
		Symbol:     data.Symbol,
		Direction:  direction,
		EntryPrice: triggerPrice,
		StopLoss:   res.StopLoss,
		TakeProfit: res.TakeProfit,
		Status:     res.Status,
		Reason:     res.Reason,
	}
	res.TradePlan = tp

	return res
}

func (uc *PlaybookQuantEngineUsecase) saveM15RawKlines(symbol string, candles []dto.Candle) {
	// Debug-only raw kline dump (disabled by default).
	if strings.TrimSpace(strings.ToLower(os.Getenv("DEBUG_SAVE_RAW_KLINES"))) != "true" {
		return
	}

	if len(candles) == 0 {
		return
	}
	limit := 30
	if len(candles) < limit {
		limit = len(candles)
	}
	closedCandles := candles[len(candles)-limit:]

	dir := strings.TrimSpace(os.Getenv("RAW_KLINES_DEBUG_DIR"))
	if dir == "" {
		dir = "debug/klines"
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		slog.Warn("failed to create raw klines debug dir", "dir", dir, "error", err)
		return
	}
	filePath := filepath.Join(dir, fmt.Sprintf("raw_klines_%s.json", symbol))

	bytes, err := json.MarshalIndent(closedCandles, "", "  ")
	if err != nil {
		slog.Warn("failed to marshal raw klines debug", "symbol", symbol, "error", err)
		return
	}
	if err := os.WriteFile(filePath, bytes, 0644); err != nil {
		slog.Warn("failed to write raw klines debug file", "file", filePath, "error", err)
	}
}
