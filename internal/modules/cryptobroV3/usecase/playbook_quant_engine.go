package usecase

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

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
	m15Closed := GetClosedCandlesOnly(data.M15Candles)
	h1Closed := GetClosedCandlesOnly(data.H1Candles)
	h4Closed := GetClosedCandlesOnly(data.H4Candles)

	if len(m15Closed) < 14 {
		return QuantResult{
			Symbol:       data.Symbol,
			Direction:    WAIT,
			Status:       PLAYBOOK_REJECTED,
			Reason:       "Insufficient closed M15 candles available for indicator calculations (minimum 14 required)",
			IndicatorMet: false,
		}
	}

	// Verify indicator inputs (Latest Price not mixed as candle close indicator)
	if !VerifyIndicatorInput(m15Closed, data.LatestPrice) {
		// Slice it if it matches the latest price to ensure zero pollution
		if len(m15Closed) > 0 && m15Closed[len(m15Closed)-1].Close == data.LatestPrice {
			m15Closed = m15Closed[:len(m15Closed)-1]
		}
	}

	// Save M15 raw klines (last 30 closed candles)
	uc.saveM15RawKlines(data.Symbol, m15Closed)

	lastM15 := m15Closed[len(m15Closed)-1]
	triggerPrice := lastM15.Close

	// Build snapshots using PopulateSnapshots helper
	techSnapPtr, structSnapPtr := PopulateSnapshots(data.M15Candles, data.H1Candles, data.H4Candles, data.FundingRate, data.LatestPrice)
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

	case RANGE_EDGE_REVERSAL:
		res.SetupType = "RANGE"
		if len(m15Closed) < 41 {
			res.Direction = WAIT
			res.IndicatorMet = false
			res.Reason = "Insufficient candles for range calculation"
			break
		}
		midPrice := (HighestHigh(m15Closed, 40) + LowestLow(m15Closed, 40)) / 2.0
		adxVal := techSnap.IndicatorValues["ADX"]

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
	if len(candles) == 0 {
		return
	}
	limit := 30
	if len(candles) < limit {
		limit = len(candles)
	}
	closedCandles := candles[len(candles)-limit:]

	dir := "storage"
	_ = os.MkdirAll(dir, 0755)
	filePath := filepath.Join(dir, fmt.Sprintf("raw_klines_%s.json", symbol))

	bytes, _ := json.MarshalIndent(closedCandles, "", "  ")
	_ = os.WriteFile(filePath, bytes, 0644)
}
