package usecase

import (
	"cpbro-engine/internal/modules/cryptobroV3/dto"
	"testing"
)

func TestPlaybookEligibility_TrendPullback(t *testing.T) {
	uc := NewPlaybookEligibilityUsecase()

	sel := StrategySelection{
		StrategyName: string(TREND_PULLBACK),
		Direction:    LONG,
		Tier:         TierA,
	}

	policy := MarketPolicy{
		AllowLong:    true,
		AllowShort:   true,
		AllowedTiers: []Tier{TierA},
	}

	// Closed-only candles (no extra open kline).
	// 200 candles for H4 EMA(200) trend.
	h4Candles := make([]dto.Candle, 200)
	for i := 0; i < 200; i++ {
		h4Candles[i] = dto.Candle{Close: 100.0}
	}
	h4Candles[199].Close = 105.0 // H4 trend Bullish

	// 50 candles for H1 EMA(50) trend.
	h1Candles := make([]dto.Candle, 50)
	for i := 0; i < 50; i++ {
		h1Candles[i] = dto.Candle{Close: 100.0}
	}
	h1Candles[49].Close = 105.0 // H1 trend Bullish

	// M15 candles for value area check (needs 50 candles to calculate EMA20/50)
	// We want the last close to pull back between EMA20 and EMA50.
	m15Candles := make([]dto.Candle, 60)
	for i := 0; i < 60; i++ {
		m15Candles[i] = dto.Candle{Close: 100.0}
	}
	for i := 0; i < 50; i++ {
		m15Candles[i].Close = 100.0
	}
	for i := 50; i < 58; i++ {
		m15Candles[i].Close = 110.0
	}
	m15Candles[59].Close = 104.0 // Last closed candle is inside value area (closed-only input)

	data := MarketData{
		Symbol:     "BTCUSDT",
		H4Candles:  h4Candles,
		H1Candles:  h1Candles,
		M15Candles: m15Candles,
	}

	tech := &TechnicalSnapshot{
		RSI: 50.0,
		IndicatorValues: map[string]float64{
			IndicatorADX: 25.0,
		},
	}

	structure := &StructureSnapshot{}

	// Test 1: Valid setup should pass
	res := uc.CheckEligibility(sel, policy, data, tech, structure)
	if !res.Eligible {
		t.Errorf("Expected Trend Pullback to be eligible, but got rejected: %s", res.Reason)
	}

	// Test 2: H4 Trend not aligned
	dataWrongTrend := data
	dataWrongTrend.H4Candles = make([]dto.Candle, 200)
	for i := 0; i < 200; i++ {
		dataWrongTrend.H4Candles[i] = dto.Candle{Close: 100.0}
	}
	dataWrongTrend.H4Candles[199].Close = 95.0 // H4 Bearish, but direction is LONG

	resWrongTrend := uc.CheckEligibility(sel, policy, dataWrongTrend, tech, structure)
	if resWrongTrend.Eligible {
		t.Errorf("Expected Trend Pullback to be rejected due to trend mismatch, but it passed")
	}

	// Test 3: Price outside value area
	dataNoPullback := data
	dataNoPullback.M15Candles = make([]dto.Candle, 60)
	for i := 0; i < 60; i++ {
		dataNoPullback.M15Candles[i] = dto.Candle{Close: 100.0}
	}
	dataNoPullback.M15Candles[58].Close = 150.0 // Far above value area (last closed)

	resNoPullback := uc.CheckEligibility(sel, policy, dataNoPullback, tech, structure)
	if resNoPullback.Eligible {
		t.Errorf("Expected Trend Pullback to be rejected because price is outside value area, but it passed")
	}
}

func TestPlaybookEligibility_LiquiditySweepReversal(t *testing.T) {
	uc := NewPlaybookEligibilityUsecase()

	sel := StrategySelection{
		StrategyName: string(LIQUIDITY_SWEEP_REVERSAL),
		Direction:    LONG,
		Tier:         TierA,
	}

	policy := MarketPolicy{
		AllowLong:    true,
		AllowShort:   true,
		AllowedTiers: []Tier{TierA},
	}

	m15Candles := make([]dto.Candle, 25)
	for i := 0; i < 25; i++ {
		m15Candles[i] = dto.Candle{
			Open:  100.0,
			Close: 100.0,
			High:  105.0,
			Low:   95.0,
			Vol:   10.0,
		}
	}
	// For sweep low: low of last closed candle (index 23) must be below lowest of prior 20, but close must be above it.
	// Prior 20 low is 95.0.
	m15Candles[23].Low = 90.0
	m15Candles[23].Close = 98.0 // Close returned inside (> 95.0)

	data := MarketData{
		Symbol:     "BTCUSDT",
		M15Candles: m15Candles,
	}

	tech := &TechnicalSnapshot{
		RSI: 50.0,
		IndicatorValues: map[string]float64{
			"sweep_low":      1.0,
			"wick_rejection": 1.0,
			"volume_spike":   1.0,
		},
	}

	structure := &StructureSnapshot{}

	// Test 1: Valid sweep should pass
	res := uc.CheckEligibility(sel, policy, data, tech, structure)
	if !res.Eligible {
		t.Errorf("Expected sweep low to be eligible, but got rejected: %s", res.Reason)
	}

	// Test 2: Sweep without volume spike should reject
	techNoVol := &TechnicalSnapshot{
		RSI: 50.0,
		IndicatorValues: map[string]float64{
			"sweep_low":      1.0,
			"wick_rejection": 1.0,
			"volume_spike":   -1.0,
		},
	}
	resNoVol := uc.CheckEligibility(sel, policy, data, techNoVol, structure)
	if resNoVol.Eligible {
		t.Errorf("Expected sweep without volume spike to be rejected, but it passed")
	}

	// Test 3: Close did not return inside range (breakout)
	dataBreakout := data
	dataBreakout.M15Candles = make([]dto.Candle, 25)
	for i := 0; i < 25; i++ {
		dataBreakout.M15Candles[i] = dto.Candle{
			Open:  100.0,
			Close: 100.0,
			High:  105.0,
			Low:   95.0,
		}
	}
	dataBreakout.M15Candles[23].Low = 90.0
	dataBreakout.M15Candles[24].Close = 88.0 // last closed at/below lowest20 (should reject)

	resBreakout := uc.CheckEligibility(sel, policy, dataBreakout, tech, structure)
	if resBreakout.Eligible {
		t.Errorf("Expected sweep with breakout close to be rejected, but it passed")
	}
}

func TestPlaybookEligibility_RangeEdgeReversal(t *testing.T) {
	uc := NewPlaybookEligibilityUsecase()

	sel := StrategySelection{
		StrategyName: string(RANGE_EDGE_REVERSAL),
		Direction:    LONG,
		Tier:         TierA,
	}

	policy := MarketPolicy{
		AllowLong:    true,
		AllowShort:   true,
		AllowedTiers: []Tier{TierA},
		Reason:       "CHOP_RANGE", // sideways regime
	}

	tech := &TechnicalSnapshot{
		RSI: 50.0,
		IndicatorValues: map[string]float64{
			IndicatorADX:      20.0, // low ADX (not trending)
			"near_range_edge": 1.0,
			"wick_rejection":  1.0,
		},
	}

	structure := &StructureSnapshot{}

	// Test 1: Valid range edge reversal should pass
	res := uc.CheckEligibility(sel, policy, MarketData{}, tech, structure)
	if !res.Eligible {
		t.Errorf("Expected Range Edge Reversal to be eligible, but got rejected: %s", res.Reason)
	}

	// Test 1b: RISK_OFF reason should not hard-reject if policy allows the playbook
	policyRiskOff := MarketPolicy{
		AllowLong:         true,
		AllowShort:        true,
		AllowedTiers:      []Tier{TierA},
		AllowedPlaybooks:  []Playbook{LIQUIDITY_SWEEP_REVERSAL, RANGE_EDGE_REVERSAL},
		LongMode:          REVERSAL_ONLY,
		ShortMode:         NORMAL,
		Reason:            "RISK_OFF + BTC Bearish active - short bias",
		RequireFreshEntry: false,
	}
	resRiskOff := uc.CheckEligibility(sel, policyRiskOff, MarketData{}, tech, structure)
	if !resRiskOff.Eligible {
		t.Errorf("Expected Range Edge Reversal to be eligible under RISK_OFF when policy allows it, but got rejected: %s", resRiskOff.Reason)
	}

	// Test 2: Strong trending regime (ADX > 30) should reject
	techHighADX := &TechnicalSnapshot{
		RSI: 50.0,
		IndicatorValues: map[string]float64{
			IndicatorADX:      35.0, // strong ADX trend
			"near_range_edge": 1.0,
			"wick_rejection":  1.0,
		},
	}
	resHighADX := uc.CheckEligibility(sel, policy, MarketData{}, techHighADX, structure)
	if resHighADX.Eligible {
		t.Errorf("Expected Range Edge Reversal with high ADX (>30) to be rejected, but it passed")
	}

	// Test 3: No wick rejection should reject
	techNoRej := &TechnicalSnapshot{
		RSI: 50.0,
		IndicatorValues: map[string]float64{
			IndicatorADX:      20.0,
			"near_range_edge": 1.0,
			"wick_rejection":  -1.0,
		},
	}
	resNoRej := uc.CheckEligibility(sel, policy, MarketData{}, techNoRej, structure)
	if resNoRej.Eligible {
		t.Errorf("Expected Range Edge Reversal without wick rejection to be rejected, but it passed")
	}
}

func TestPlaybookEligibility_CrowdedPositioningSqueeze(t *testing.T) {
	uc := NewPlaybookEligibilityUsecase()

	sel := StrategySelection{
		StrategyName: string(CROWDED_POSITIONING_SQUEEZE),
		Direction:    LONG,
		Tier:         TierA,
	}

	// Crowd is short (negative funding rate), we enter LONG to squeeze them
	policy := MarketPolicy{
		AllowLong:    true,
		AllowShort:   true,
		AllowedTiers: []Tier{TierA},
	}

	data := MarketData{
		Symbol:      "BTCUSDT",
		FundingRate: -0.004,
		M15Candles: []dto.Candle{
			{Low: 100.0, Close: 100.0},
			{Low: 99.0, Close: 101.0}, // previous candle
			{Low: 98.0, Close: 102.0}, // last candle dipped low but closed high
		},
	}

	tech := &TechnicalSnapshot{
		RSI: 50.0,
		IndicatorValues: map[string]float64{
			"extreme_funding": 1.0,
			"extreme_oi":      1.0,
			"pa_rejection":    1.0,
		},
	}

	structure := &StructureSnapshot{}

	// Test 1: Valid Squeeze should pass
	res := uc.CheckEligibility(sel, policy, data, tech, structure)
	if !res.Eligible {
		t.Errorf("Expected Squeeze to be eligible, but got rejected: %s", res.Reason)
	}

	// Test 2: Direction matching crowd (entering SHORT when funding is already negative) should reject
	selWrongDir := sel
	selWrongDir.Direction = SHORT

	resWrongDir := uc.CheckEligibility(selWrongDir, policy, data, tech, structure)
	if resWrongDir.Eligible {
		t.Errorf("Expected Squeeze in direction of crowd to be rejected, but it passed")
	}
}

func TestPlaybookEligibility_CompressionBreakoutRetest(t *testing.T) {
	uc := NewPlaybookEligibilityUsecase()

	sel := StrategySelection{
		StrategyName: string(COMPRESSION_BREAKOUT_RETEST),
		Direction:    LONG,
		Tier:         TierA,
	}

	policy := MarketPolicy{
		AllowLong:    true,
		AllowShort:   true,
		AllowedTiers: []Tier{TierA},
	}

	// Closed-only candles (no extra open kline).
	m15Candles := make([]dto.Candle, 25)
	for i := 0; i < 25; i++ {
		m15Candles[i] = dto.Candle{Close: 100.0, Vol: 10.0}
	}
	// Breakout close inside the last 5 candles.
	m15Candles[20].Close = 105.0
	// Last candle represents retest/hold with volume expansion.
	m15Candles[24].Close = 101.0
	m15Candles[24].Vol = 15.0

	data := MarketData{
		Symbol:     "BTCUSDT",
		M15Candles: m15Candles,
	}

	tech := &TechnicalSnapshot{
		RSI: 50.0,
		IndicatorValues: map[string]float64{
			"contraction":           1.0,
			"first_breakout_candle": -1.0,
		},
	}

	structure := &StructureSnapshot{}

	// Test 1: Valid breakout retest should pass
	res := uc.CheckEligibility(sel, policy, data, tech, structure)
	if !res.Eligible {
		t.Errorf("Expected Compression Breakout Retest to be eligible, but got rejected: %s", res.Reason)
	}

	// Test 2: No breakout close in last 5 candles should reject
	dataNoBreakout := data
	dataNoBreakout.M15Candles = make([]dto.Candle, 25)
	for i := 0; i < 25; i++ {
		dataNoBreakout.M15Candles[i] = dto.Candle{Close: 100.0, Vol: 10.0}
	}
	dataNoBreakout.M15Candles[24].Vol = 15.0

	resNoBreakout := uc.CheckEligibility(sel, policy, dataNoBreakout, tech, structure)
	if resNoBreakout.Eligible {
		t.Errorf("Expected Compression Breakout Retest without prior breakout to be rejected, but it passed")
	}

	// Test 3: Entry on first breakout candle should reject
	techFirstBreakout := &TechnicalSnapshot{
		RSI: 50.0,
		IndicatorValues: map[string]float64{
			"contraction":           1.0,
			"first_breakout_candle": 1.0,
		},
	}
	resFirstBreakout := uc.CheckEligibility(sel, policy, data, techFirstBreakout, structure)
	if resFirstBreakout.Eligible {
		t.Errorf("Expected entry on first breakout candle to be rejected, but it passed")
	}
}
