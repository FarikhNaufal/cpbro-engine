package usecase

import (
	"cpbro-engine/internal/modules/cryptobroV3/dto"
	"strings"
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

func TestPlaybookEligibility_TrendPullback_AllowsSidewaysH1AndValueAreaTouch(t *testing.T) {
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
		Regime:       DEFAULT,
	}

	h4Candles := make([]dto.Candle, 200)
	for i := 0; i < 200; i++ {
		h4Candles[i] = dto.Candle{Close: 100.0}
	}
	h4Candles[199].Close = 105.0

	h1Candles := make([]dto.Candle, 50)
	for i := 0; i < 50; i++ {
		h1Candles[i] = dto.Candle{Close: 100.0}
	}

	m15Candles := make([]dto.Candle, 60)
	for i := 0; i < 60; i++ {
		m15Candles[i] = dto.Candle{
			Open:  100.0,
			High:  100.5,
			Low:   99.5,
			Close: 100.0,
		}
	}
	for i := 50; i < 59; i++ {
		m15Candles[i] = dto.Candle{
			Open:  109.0,
			High:  110.5,
			Low:   108.5,
			Close: 110.0,
		}
	}
	m15Candles[59] = dto.Candle{
		Open:  106.0,
		High:  107.0,
		Low:   103.5,
		Close: 106.0,
	}

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

	res := uc.CheckEligibility(sel, policy, data, tech, &StructureSnapshot{})
	if !res.Eligible {
		t.Fatalf("Expected Trend Pullback to pass with H1 SIDEWAYS and wick touch into value area, but got rejected: %s", res.Reason)
	}
}

func TestPlaybookEligibility_TrendPullback_RejectsOppositeH1InNormalRegime(t *testing.T) {
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
		Regime:       DEFAULT,
	}

	h4Candles := make([]dto.Candle, 200)
	for i := 0; i < 200; i++ {
		h4Candles[i] = dto.Candle{Close: 100.0}
	}
	h4Candles[199].Close = 105.0

	h1Candles := make([]dto.Candle, 50)
	for i := 0; i < 50; i++ {
		h1Candles[i] = dto.Candle{Close: 100.0}
	}
	h1Candles[49].Close = 95.0

	m15Candles := make([]dto.Candle, 60)
	for i := 0; i < 60; i++ {
		m15Candles[i] = dto.Candle{Close: 100.0}
	}
	for i := 50; i < 58; i++ {
		m15Candles[i].Close = 110.0
	}
	m15Candles[59].Close = 104.0

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

	res := uc.CheckEligibility(sel, policy, data, tech, &StructureSnapshot{})
	if res.Eligible {
		t.Fatal("Expected Trend Pullback to be rejected when H1 trend is opposite in normal regime, but it passed")
	}
	if !strings.Contains(res.Reason, "Trend alignment failed") {
		t.Fatalf("Expected trend alignment rejection reason, got: %s", res.Reason)
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
	m15Candles[24].Vol = 135.0

	data := MarketData{
		Symbol:     "BTCUSDT",
		M15Candles: m15Candles,
	}

	tech := &TechnicalSnapshot{
		RSI:         50.0,
		VolumeRatio: 1.35,
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
	dataNoVol := data
	dataNoVol.M15Candles = make([]dto.Candle, len(data.M15Candles))
	copy(dataNoVol.M15Candles, data.M15Candles)
	for i := range dataNoVol.M15Candles {
		dataNoVol.M15Candles[i].Vol = 100.0
	}
	resNoVol := uc.CheckEligibility(sel, policy, dataNoVol, techNoVol, structure)
	if resNoVol.Eligible {
		t.Errorf("Expected sweep without volume spike to be rejected, but it passed")
	}

	// Test 2b: Profile-based volume ratio should pass even if legacy indicator flag is not pre-set.
	dataProfileVol := data
	dataProfileVol.M15Candles = make([]dto.Candle, 25)
	for i := 0; i < 25; i++ {
		dataProfileVol.M15Candles[i] = dto.Candle{
			Open:  100.0,
			Close: 100.0,
			High:  105.0,
			Low:   95.0,
			Vol:   100.0,
		}
	}
	dataProfileVol.M15Candles[23].Low = 90.0
	dataProfileVol.M15Candles[23].Close = 98.0
	dataProfileVol.M15Candles[24].Vol = 135.0 // 1.35x avg, above sweep profile min 1.3
	techProfileVol := &TechnicalSnapshot{
		RSI:         50.0,
		VolumeRatio: 1.35,
		IndicatorValues: map[string]float64{
			"sweep_low":      1.0,
			"wick_rejection": 1.0,
			"volume_spike":   -1.0,
		},
	}
	resProfileVol := uc.CheckEligibility(sel, policy, dataProfileVol, techProfileVol, structure)
	if !resProfileVol.Eligible {
		t.Errorf("Expected sweep with profile-based volume ratio to be eligible, but got rejected: %s", resProfileVol.Reason)
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

	policyRiskOffSweepOnly := policyRiskOff
	policyRiskOffSweepOnly.LongMode = SWEEP_ONLY
	resRiskOffSweepOnly := uc.CheckEligibility(sel, policyRiskOffSweepOnly, MarketData{}, tech, structure)
	if resRiskOffSweepOnly.Eligible {
		t.Errorf("Expected Range Edge Reversal LONG to be rejected under SWEEP_ONLY, but it passed")
	}

	selShort := sel
	selShort.Direction = SHORT
	policyShortSweepOnly := policyRiskOff
	policyShortSweepOnly.ShortMode = SWEEP_ONLY
	resShortSweepOnly := uc.CheckEligibility(selShort, policyShortSweepOnly, MarketData{}, tech, structure)
	if resShortSweepOnly.Eligible {
		t.Errorf("Expected Range Edge Reversal SHORT to be rejected under SWEEP_ONLY, but it passed")
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

	// Test 4: Existing OI alone must not count as expansion.
	dataNoExpansion := data
	dataNoExpansion.OpenInterestM15 = 12345.0
	dataNoExpansion.OIChangePct = 0.0
	dataNoExpansion.M15Candles = make([]dto.Candle, 25)
	for i := 0; i < 25; i++ {
		dataNoExpansion.M15Candles[i] = dto.Candle{Close: 100.0, Vol: 10.0}
	}
	dataNoExpansion.M15Candles[20].Close = 105.0
	dataNoExpansion.M15Candles[24].Close = 101.0
	dataNoExpansion.M15Candles[24].Vol = 10.0 // no volume expansion

	techNoExpansion := &TechnicalSnapshot{
		RSI: 50.0,
		IndicatorValues: map[string]float64{
			IndicatorContraction: 1.0,
			IndicatorExtremeOI:   0.0,
		},
	}
	resNoExpansion := uc.CheckEligibility(sel, policy, dataNoExpansion, techNoExpansion, structure)
	if resNoExpansion.Eligible {
		t.Errorf("Expected Compression Breakout Retest without volume/OI expansion to be rejected, but it passed")
	}

	// Test 5: Profile-based 1.2x volume ratio should qualify compression expansion.
	dataProfileExpansion := data
	dataProfileExpansion.M15Candles = make([]dto.Candle, 25)
	for i := 0; i < 25; i++ {
		dataProfileExpansion.M15Candles[i] = dto.Candle{Close: 100.0, Vol: 100.0}
	}
	dataProfileExpansion.M15Candles[20].Close = 105.0
	dataProfileExpansion.M15Candles[24].Close = 101.0
	dataProfileExpansion.M15Candles[24].Vol = 125.0
	techProfileExpansion := &TechnicalSnapshot{
		RSI:         50.0,
		VolumeRatio: 1.25,
		IndicatorValues: map[string]float64{
			IndicatorContraction: 1.0,
			IndicatorBBWidth:     0.08,
			IndicatorExtremeOI:   0.0,
			IndicatorOIChange:    0.0,
		},
	}
	resProfileExpansion := uc.CheckEligibility(sel, policy, dataProfileExpansion, techProfileExpansion, structure)
	if !resProfileExpansion.Eligible {
		t.Errorf("Expected Compression Breakout Retest with 1.25x volume ratio to be eligible, but got rejected: %s", resProfileExpansion.Reason)
	}

	// Test 6: LONG compression breakout retest must stay disabled in DEFAULT regime.
	policyDefault := policy
	policyDefault.Regime = DEFAULT
	resDefaultLong := uc.CheckEligibility(sel, policyDefault, dataProfileExpansion, techProfileExpansion, structure)
	if resDefaultLong.Eligible {
		t.Errorf("Expected LONG Compression Breakout Retest in DEFAULT regime to be rejected, but it passed")
	}
	if !strings.Contains(resDefaultLong.Reason, "DEFAULT regime") {
		t.Errorf("Expected DEFAULT regime rejection reason, got %s", resDefaultLong.Reason)
	}
}

func TestPlaybookEligibility_HighVolRelaxationsAndSubstringFix(t *testing.T) {
	uc := NewPlaybookEligibilityUsecase()

	t.Run("Trend Pullback relaxed trend in HIGH_VOL", func(t *testing.T) {
		sel := StrategySelection{
			StrategyName: string(TREND_PULLBACK),
			Direction:    LONG,
			Tier:         TierA,
		}

		policyHighVol := MarketPolicy{
			AllowLong:    true,
			AllowShort:   true,
			AllowedTiers: []Tier{TierA},
			Regime:       HIGH_VOL,
		}

		// H4 Trend: BULLISH
		h4Candles := make([]dto.Candle, 200)
		for i := 0; i < 200; i++ {
			h4Candles[i] = dto.Candle{Close: 100.0}
		}
		h4Candles[199].Close = 105.0

		// H1 Trend: BEARISH (opposite trend)
		h1Candles := make([]dto.Candle, 50)
		for i := 0; i < 50; i++ {
			h1Candles[i] = dto.Candle{Close: 100.0}
		}
		h1Candles[49].Close = 95.0

		// M15 inside value area
		m15Candles := make([]dto.Candle, 60)
		for i := 0; i < 60; i++ {
			m15Candles[i] = dto.Candle{Close: 100.0}
		}
		for i := 50; i < 58; i++ {
			m15Candles[i].Close = 110.0
		}
		m15Candles[59].Close = 104.0

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

		res := uc.CheckEligibility(sel, policyHighVol, data, tech, &StructureSnapshot{})
		if !res.Eligible {
			t.Errorf("Expected Trend Pullback in HIGH_VOL to pass even with H1 trend mismatch, but got rejected: %s", res.Reason)
		}
	})

	t.Run("Liquidity Sweep eligibility bug fix verify", func(t *testing.T) {
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
			m15Candles[i] = dto.Candle{Open: 100.0, Close: 100.0, High: 105.0, Low: 95.0, Vol: 10.0}
		}

		data := MarketData{
			Symbol:     "BTCUSDT",
			M15Candles: m15Candles,
		}

		// sweep_low is 0.0 (no sweep occurred)
		// but structure notes contains "sweep_low=false"
		tech := &TechnicalSnapshot{
			RSI:         50.0,
			VolumeRatio: 1.35,
			IndicatorValues: map[string]float64{
				"sweep_low":      0.0, // NO SWEEP
				"wick_rejection": 1.0,
				"volume_spike":   1.0,
			},
		}

		structure := &StructureSnapshot{
			Notes: "sweep_low=false", // this used to trigger the substring match bug!
		}

		res := uc.CheckEligibility(sel, policy, data, tech, structure)
		if res.Eligible {
			t.Errorf("Expected Liquidity Sweep with sweep_low=0.0 to be rejected, but it was accepted (substring match bug still active!)")
		}
	})
}
