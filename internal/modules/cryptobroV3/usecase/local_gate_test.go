package usecase

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
)

func TestLocalGate_RuleChecks(t *testing.T) {
	gate := NewLocalGateUsecase()

	policy := MarketPolicy{
		AllowLong:        true,
		AllowShort:       true,
		LongMode:         NORMAL,
		ShortMode:        NORMAL,
		AllowedTiers:     []Tier{TierA, TierB},
		AllowedPlaybooks: []Playbook{TREND_PULLBACK, LIQUIDITY_SWEEP_REVERSAL, COMPRESSION_BREAKOUT_RETEST, RANGE_EDGE_REVERSAL, CROWDED_POSITIONING_SQUEEZE},
		MinScoreAI:       7.5,
		MinADXExecute:    20.0,
		MinScoreExecute:  8.0,
		Reason:           "Normal conditions",
	}

	m15 := []dto.Candle{
		{Vol: 100}, {Vol: 100}, {Vol: 100}, {Vol: 100}, {Vol: 100},
		{Vol: 100}, {Vol: 100}, {Vol: 100}, {Vol: 100}, {Vol: 100},
		{Vol: 100}, {Vol: 100}, {Vol: 100}, {Vol: 100}, {Vol: 100},
		{Vol: 100}, {Vol: 100}, {Vol: 100}, {Vol: 100}, {Vol: 100},
		{Vol: 200}, // volume spike at last candle (200 vs 100 avg = 2.0x > 1.3x)
	}

	// 1. Valid passing case
	quantPass := QuantResult{
		Symbol:       "NEARUSDT",
		Direction:    LONG,
		Playbook:     TREND_PULLBACK,
		Score:        8.5,
		Tier:         TierA,
		IndicatorMet: true,
		TechnicalSnapshot: TechnicalSnapshot{
			IndicatorValues: map[string]float64{
				"adx": 25.0,
			},
		},
		TradePlan: TradePlan{
			EntryPrice: 5.0,
			TakeProfit: 6.0, // Risk = 0.5, Reward = 1.0 (RR = 2.0 > 1.5)
			StopLoss:   4.5,
		},
	}

	res := gate.Evaluate(quantPass, policy, m15)
	if !res.Passed || res.Status != AI_CANDIDATE {
		t.Errorf("Expected pass as AI_CANDIDATE, got status %s reason %s", res.Status, res.Reason)
	}

	// 2. Rule 1: Direction WAIT
	quantWait := quantPass
	quantWait.Direction = WAIT
	res = gate.Evaluate(quantWait, policy, m15)
	if res.Passed || res.Status != LOCAL_REJECT {
		t.Errorf("Expected reject for WAIT, got status %s", res.Status)
	}

	// 3. Rule 2: AllowLong false
	policyNoLong := policy
	policyNoLong.AllowLong = false
	res = gate.Evaluate(quantPass, policyNoLong, m15)
	if res.Passed || res.Status != LOCAL_REJECT {
		t.Errorf("Expected reject for disallowed LONG, got status %s", res.Status)
	}

	// 4. Rule 4: LongMode disabled
	policyLongDisabled := policy
	policyLongDisabled.LongMode = DISABLED
	res = gate.Evaluate(quantPass, policyLongDisabled, m15)
	if res.Passed || res.Status != LOCAL_REJECT {
		t.Errorf("Expected reject for DISABLED LongMode, got status %s", res.Status)
	}

	// 5. Rule 5: ShortMode SWEEP_ONLY but got TREND_PULLBACK
	policyShortSweep := policy
	policyShortSweep.ShortMode = SWEEP_ONLY
	quantShortTP := quantPass
	quantShortTP.Direction = SHORT
	quantShortTP.Playbook = TREND_PULLBACK
	quantShortTP.TradePlan = TradePlan{
		EntryPrice: 5.0,
		TakeProfit: 4.0, // Reward = 1.0, Risk = 0.5 (RR = 2.0)
		StopLoss:   5.5,
	}
	res = gate.Evaluate(quantShortTP, policyShortSweep, m15)
	if res.Passed || res.Status != LOCAL_REJECT {
		t.Errorf("Expected reject for SWEEP_ONLY blocking TREND_PULLBACK, got status %s reason %s", res.Status, res.Reason)
	}

	quantShortRange := quantShortTP
	quantShortRange.Playbook = RANGE_EDGE_REVERSAL
	res = gate.Evaluate(quantShortRange, policyShortSweep, m15)
	if res.Passed || res.Status != LOCAL_REJECT {
		t.Errorf("Expected reject for ShortMode SWEEP_ONLY blocking RANGE_EDGE_REVERSAL, got status %s reason %s", res.Status, res.Reason)
	}

	// 5b. LongMode SWEEP_ONLY must block non-sweep reversal playbooks
	policyLongSweep := policy
	policyLongSweep.LongMode = SWEEP_ONLY
	quantLongRange := quantPass
	quantLongRange.Playbook = RANGE_EDGE_REVERSAL
	res = gate.Evaluate(quantLongRange, policyLongSweep, m15)
	if res.Passed || res.Status != LOCAL_REJECT {
		t.Errorf("Expected reject for LongMode SWEEP_ONLY blocking RANGE_EDGE_REVERSAL, got status %s reason %s", res.Status, res.Reason)
	}

	// 6. Rule 8: RR < 1.5
	quantBadRR := quantPass
	quantBadRR.TradePlan.TakeProfit = 5.2 // Reward = 0.2, Risk = 0.5 (RR = 0.4 < 1.5)
	res = gate.Evaluate(quantBadRR, policy, m15)
	if res.Passed || res.Status != LOCAL_REJECT {
		t.Errorf("Expected reject for low RR, got status %s reason %s", res.Status, res.Reason)
	}

	// 6b. RR above hard minimum but below stricter policy/profile requirement -> Watch
	policyStrictRR := policy
	policyStrictRR.MinRRExecute = 2.0
	quantBorderlineRR := quantPass
	quantBorderlineRR.TradePlan.TakeProfit = 5.8 // Reward = 0.8, Risk = 0.5 (RR = 1.6)
	res = gate.Evaluate(quantBorderlineRR, policyStrictRR, m15)
	if res.Passed || res.Status != LOCAL_WATCH {
		t.Errorf("Expected watch for borderline RR below strict policy, got status %s reason %s", res.Status, res.Reason)
	}

	// 7. Rule 7: Score < MinScoreAI (deviation checks)
	quantLowScore := quantPass
	quantLowScore.Score = 7.3 // 7.3 is within 0.5 from 7.5 -> Watch
	res = gate.Evaluate(quantLowScore, policy, m15)
	if res.Passed || res.Status != LOCAL_WATCH {
		t.Errorf("Expected LOCAL_WATCH for slightly low score, got status %s reason %s", res.Status, res.Reason)
	}

	quantVeryLowScore := quantPass
	quantVeryLowScore.Score = 6.8 // 6.8 is > 0.5 below 7.5 -> Reject
	res = gate.Evaluate(quantVeryLowScore, policy, m15)
	if res.Passed || res.Status != LOCAL_REJECT {
		t.Errorf("Expected LOCAL_REJECT for very low score, got status %s reason %s", res.Status, res.Reason)
	}

	// 8. Rule 9: ADX < MinADXExecute (without strong confirmation -> Watch)
	quantLowADX := quantPass
	quantLowADX.TechnicalSnapshot.IndicatorValues["adx"] = 15.0 // below 20.0
	res = gate.Evaluate(quantLowADX, policy, m15)
	if res.Passed || res.Status != LOCAL_WATCH {
		t.Errorf("Expected LOCAL_WATCH for low ADX, got status %s reason %s", res.Status, res.Reason)
	}

	// ADX < MinADXExecute (with strong confirmation -> Pass)
	quantLowADXSweep := quantPass
	quantLowADXSweep.Playbook = LIQUIDITY_SWEEP_REVERSAL
	quantLowADXSweep.TechnicalSnapshot.IndicatorValues["adx"] = 15.0
	quantLowADXSweep.TechnicalSnapshot.IndicatorValues["volume_spike"] = 1.0
	quantLowADXSweep.TechnicalSnapshot.IndicatorValues["wick_rejection"] = 1.0
	res = gate.Evaluate(quantLowADXSweep, policy, m15)
	if !res.Passed || res.Status != AI_CANDIDATE {
		t.Errorf("Expected AI_CANDIDATE for low ADX sweep with confirmation, got status %s reason %s", res.Status, res.Reason)
	}

	// 9. Rule 10: BTCChaos & score < MinScoreExecute -> Watch
	policyChaos := policy
	policyChaos.Reason = "BTC_CHAOS active"
	quantChaosLowScore := quantPass
	quantChaosLowScore.Score = 7.8 // above MinScoreAI (7.5) but below MinScoreExecute (8.0)
	res = gate.Evaluate(quantChaosLowScore, policyChaos, m15)
	if res.Passed || res.Status != LOCAL_WATCH {
		t.Errorf("Expected LOCAL_WATCH under BTC_CHAOS with score below execute threshold, got status %s reason %s", res.Status, res.Reason)
	}

	// 10. Rule 11: Tier not allowed
	quantTierC := quantPass
	quantTierC.Tier = TierC
	res = gate.Evaluate(quantTierC, policy, m15)
	if res.Passed || res.Status != LOCAL_REJECT {
		t.Errorf("Expected LOCAL_REJECT for Tier C (not in AllowedTiers), got status %s reason %s", res.Status, res.Reason)
	}

	// 11. TradePlan reversed SL/TP
	quantReversedLONG := quantPass
	quantReversedLONG.TradePlan.StopLoss = 5.5 // sl > entry for LONG
	res = gate.Evaluate(quantReversedLONG, policy, m15)
	if res.Passed || res.Status != LOCAL_REJECT {
		t.Errorf("Expected LOCAL_REJECT for reversed SL/TP on LONG, got status %s reason %s", res.Status, res.Reason)
	}

	// 12. Range Edge Reversal high ADX expansion watch
	quantRangeHighADX := quantPass
	quantRangeHighADX.Playbook = RANGE_EDGE_REVERSAL
	quantRangeHighADX.TechnicalSnapshot.IndicatorValues["adx"] = 35.0 // > 30.0
	res = gate.Evaluate(quantRangeHighADX, policy, m15)
	if res.Passed || res.Status != LOCAL_WATCH {
		t.Errorf("Expected LOCAL_WATCH for high ADX Range Edge Reversal, got status %s reason %s", res.Status, res.Reason)
	}
}

type mockLocalGateMarketDataProvider struct {
	m5Candles []dto.Candle
	m5Err     error
}

func (m *mockLocalGateMarketDataProvider) FetchClosedCandles(ctx context.Context, symbol string, interval string, limit int) ([]dto.Candle, error) {
	if interval == "5m" {
		return m.m5Candles, m.m5Err
	}
	return nil, fmt.Errorf("unexpected interval: %s", interval)
}
func (m *mockLocalGateMarketDataProvider) FetchLatestPrice(ctx context.Context, symbol string) (float64, error) {
	return 0, nil
}
func (m *mockLocalGateMarketDataProvider) FetchAllFuturesTickers24h(ctx context.Context) ([]dto.Ticker24h, error) {
	return nil, nil
}
func (m *mockLocalGateMarketDataProvider) FetchPremiumFundingRates(ctx context.Context) (map[string]float64, error) {
	return nil, nil
}
func (m *mockLocalGateMarketDataProvider) FetchOpenInterest(ctx context.Context, symbol string) (float64, error) {
	return 0, nil
}
func (m *mockLocalGateMarketDataProvider) FetchHistoricalCandles(ctx context.Context, symbol string, interval string, startTime time.Time, endTime time.Time) ([]dto.Candle, error) {
	return nil, nil
}

func TestLocalGate_M5Confirmation(t *testing.T) {
	// Setup standard policy and candidate
	policy := MarketPolicy{
		AllowLong:        true,
		AllowShort:       true,
		LongMode:         NORMAL,
		ShortMode:        NORMAL,
		AllowedTiers:     []Tier{TierA},
		AllowedPlaybooks: []Playbook{LIQUIDITY_SWEEP_REVERSAL, TREND_PULLBACK},
		MinScoreAI:       7.0,
		MinScoreExecute:  7.0,
		Reason:           "Normal conditions",
	}

	m15 := []dto.Candle{
		{Vol: 100}, {Vol: 100}, {Vol: 100}, {Vol: 100}, {Vol: 100},
		{Vol: 100}, {Vol: 100}, {Vol: 100}, {Vol: 100}, {Vol: 100},
		{Vol: 100}, {Vol: 100}, {Vol: 100}, {Vol: 100}, {Vol: 100},
		{Vol: 100}, {Vol: 100}, {Vol: 100}, {Vol: 100}, {Vol: 100},
		{Vol: 200}, // volume confirmation spike
	}

	quantPass := QuantResult{
		Symbol:       "NEARUSDT",
		Direction:    LONG,
		Playbook:     LIQUIDITY_SWEEP_REVERSAL,
		Score:        8.0,
		Tier:         TierA,
		IndicatorMet: true,
		TechnicalSnapshot: TechnicalSnapshot{
			IndicatorValues: map[string]float64{
				"adx":            25.0,
				"volume_spike":   1.0,
				"wick_rejection": 1.0,
			},
		},
		TradePlan: TradePlan{
			EntryPrice: 5.0,
			TakeProfit: 6.0,
			StopLoss:   4.5,
		},
	}

	// 1. Verify M5 disabled: passes without fetching M5
	gate := NewLocalGateUsecase()
	// No marketData setup, both flags false by default
	res := gate.Evaluate(quantPass, policy, m15)
	if !res.Passed || res.Status != AI_CANDIDATE {
		t.Errorf("Expected pass with M5 disabled, got status %s reason %s", res.Status, res.Reason)
	}

	// 2. Enable M5 confirmations and setup mock market data
	reg := GetGlobalConfigRegistry()
	originalProfile, exists := reg.GetPlaybookProfile(LIQUIDITY_SWEEP_REVERSAL)
	if !exists {
		originalProfile = PlaybookThresholdProfile{Playbook: LIQUIDITY_SWEEP_REVERSAL}
	}
	defer func() {
		// Restore
		reg.mu.Lock()
		reg.profiles[LIQUIDITY_SWEEP_REVERSAL] = originalProfile
		reg.mu.Unlock()
	}()

	evaluateWithM5 := func(m5 []dto.Candle) LocalGateResult {
		mockProv := &mockLocalGateMarketDataProvider{m5Candles: m5}
		mdu := NewMarketDataUsecase(mockProv)
		gate.SetMarketData(mdu)
		return gate.Evaluate(quantPass, policy, m15)
	}

	// A. Early Invalidation check
	reg.mu.Lock()
	reg.profiles[LIQUIDITY_SWEEP_REVERSAL] = PlaybookThresholdProfile{
		Playbook:                  LIQUIDITY_SWEEP_REVERSAL,
		RequireM5RejectionConfirm: true,
		M5ConfirmationMode:        M5ConfirmationHardConfirm,
		MinRR:                     1.0,
	}
	reg.mu.Unlock()

	// Last M5 Close crosses Stop Loss (4.5)
	res = evaluateWithM5([]dto.Candle{
		{Open: 4.8, Close: 4.4, High: 4.9, Low: 4.3}, // Close 4.4 is <= StopLoss 4.5
	})
	if res.Passed || res.Status != LOCAL_REJECT || !strings.Contains(res.Reason, "M5 early invalidation") {
		t.Errorf("Expected LOCAL_REJECT for M5 early invalidation, got status %s reason %s", res.Status, res.Reason)
	}

	// B. Sweep Rejection Confirmation (succeeds)
	// At least one candle has wick ratio >= 0.35. For LONG, lower wick ratio.
	// Candle range: 5.0 - 4.5 = 0.5. Open: 4.8, Close: 4.8, Low: 4.6
	// Min(Open, Close) = 4.8. Lower wick = (4.8 - 4.6)/0.5 = 0.4 >= 0.35
	res = evaluateWithM5([]dto.Candle{
		{Open: 4.8, Close: 4.8, High: 5.0, Low: 4.6}, // Lower wick ratio = 0.40
	})
	if !res.Passed || res.Status != AI_CANDIDATE {
		t.Errorf("Expected pass for Sweep Rejection Confirmation, got status %s reason %s", res.Status, res.Reason)
	}
	if res.M5Summary == nil || res.M5Summary.Status != M5ConfirmationConfirmed {
		t.Fatalf("Expected confirmed M5 summary, got %+v", res.M5Summary)
	}

	// C. Sweep Rejection Confirmation (fails)
	// Candle: Open: 4.8, Close: 4.7, Low: 4.68. Range: 5.0 - 4.68 = 0.32
	// Min(Open, Close) = 4.7. Lower wick = (4.7 - 4.68)/0.32 = 0.02 / 0.32 = 0.06 < 0.35
	res = evaluateWithM5([]dto.Candle{
		{Open: 4.8, Close: 4.7, High: 5.0, Low: 4.68},
	})
	if res.Passed || res.Status != LOCAL_WATCH || !strings.Contains(res.Reason, "M5 sweep rejection confirmation failed") {
		t.Errorf("Expected LOCAL_WATCH for failed Sweep Rejection Confirmation, got status %s reason %s", res.Status, res.Reason)
	}

	// D. Pullback Continuation Confirmation (succeeds)
	reg.mu.Lock()
	reg.profiles[LIQUIDITY_SWEEP_REVERSAL] = PlaybookThresholdProfile{
		Playbook:                     LIQUIDITY_SWEEP_REVERSAL,
		RequireM5ContinuationConfirm: true,
		M5ConfirmationMode:           M5ConfirmationSoftConfirm,
		MinRR:                        1.0,
	}
	reg.mu.Unlock()

	// Setup 10 candles where Close of last candle is above its EMA9.
	// Let's create a series of 10 identical candles at Close 5.0, High 5.1, Low 4.9.
	// EMA9 should resolve to 5.0, last closed Close is 5.1 (above 5.0).
	res = evaluateWithM5([]dto.Candle{
		{Open: 5.0, Close: 5.0, High: 5.0, Low: 5.0},
		{Open: 5.0, Close: 5.0, High: 5.0, Low: 5.0},
		{Open: 5.0, Close: 5.0, High: 5.0, Low: 5.0},
		{Open: 5.0, Close: 5.0, High: 5.0, Low: 5.0},
		{Open: 5.0, Close: 5.0, High: 5.0, Low: 5.0},
		{Open: 5.0, Close: 5.0, High: 5.0, Low: 5.0},
		{Open: 5.0, Close: 5.0, High: 5.0, Low: 5.0},
		{Open: 5.0, Close: 5.0, High: 5.0, Low: 5.0},
		{Open: 5.0, Close: 5.0, High: 5.0, Low: 5.0},
		{Open: 5.0, Close: 5.1, High: 5.2, Low: 5.0}, // Last Close: 5.1
	})
	if !res.Passed || res.Status != AI_CANDIDATE {
		t.Errorf("Expected pass for Pullback Continuation Confirmation, got status %s reason %s", res.Status, res.Reason)
	}

	// E. Pullback Continuation Confirmation (fails)
	// Last Close is 4.9 (below EMA9).
	res = evaluateWithM5([]dto.Candle{
		{Open: 5.0, Close: 5.0, High: 5.0, Low: 5.0},
		{Open: 5.0, Close: 5.0, High: 5.0, Low: 5.0},
		{Open: 5.0, Close: 5.0, High: 5.0, Low: 5.0},
		{Open: 5.0, Close: 5.0, High: 5.0, Low: 5.0},
		{Open: 5.0, Close: 5.0, High: 5.0, Low: 5.0},
		{Open: 5.0, Close: 5.0, High: 5.0, Low: 5.0},
		{Open: 5.0, Close: 5.0, High: 5.0, Low: 5.0},
		{Open: 5.0, Close: 5.0, High: 5.0, Low: 5.0},
		{Open: 5.0, Close: 5.0, High: 5.0, Low: 5.0},
		{Open: 5.0, Close: 4.9, High: 5.0, Low: 4.8}, // Last Close: 4.9
	})
	if res.Passed || res.Status != LOCAL_WATCH || !strings.Contains(res.Reason, "M5 pullback continuation confirmation failed") {
		t.Errorf("Expected LOCAL_WATCH for failed Pullback Continuation Confirmation, got status %s reason %s", res.Status, res.Reason)
	}
}

func TestLocalGate_M5WatchOnlyHintDoesNotBlockOnUnavailableOrFailedConfirmation(t *testing.T) {
	policy := MarketPolicy{
		AllowLong:        true,
		AllowShort:       true,
		LongMode:         NORMAL,
		ShortMode:        NORMAL,
		AllowedTiers:     []Tier{TierA},
		AllowedPlaybooks: []Playbook{TREND_PULLBACK},
		MinScoreAI:       7.0,
		MinScoreExecute:  7.0,
	}

	quant := QuantResult{
		Symbol:       "NEARUSDT",
		Direction:    LONG,
		Playbook:     TREND_PULLBACK,
		Score:        8.0,
		Tier:         TierA,
		IndicatorMet: true,
		TechnicalSnapshot: TechnicalSnapshot{
			IndicatorValues: map[string]float64{
				IndicatorADX: 25.0,
			},
		},
		TradePlan: TradePlan{
			EntryPrice: 5.0,
			TakeProfit: 6.0,
			StopLoss:   4.5,
		},
	}

	reg := GetGlobalConfigRegistry()
	originalProfile, exists := reg.GetPlaybookProfile(TREND_PULLBACK)
	if !exists {
		originalProfile = PlaybookThresholdProfile{Playbook: TREND_PULLBACK}
	}
	defer func() {
		reg.mu.Lock()
		reg.profiles[TREND_PULLBACK] = originalProfile
		reg.mu.Unlock()
	}()

	reg.mu.Lock()
	reg.profiles[TREND_PULLBACK] = PlaybookThresholdProfile{
		Playbook:                     TREND_PULLBACK,
		RequireADX:                   true,
		MinADX:                       20,
		RequireM5ContinuationConfirm: true,
		M5ConfirmationMode:           M5ConfirmationWatchOnlyHint,
		MinRR:                        1.0,
	}
	reg.mu.Unlock()

	gate := NewLocalGateUsecase()
	m15 := []dto.Candle{{Vol: 100}}

	gate.SetMarketData(NewMarketDataUsecase(&mockLocalGateMarketDataProvider{m5Err: fmt.Errorf("network down")}))
	res := gate.Evaluate(quant, policy, m15)
	if !res.Passed || res.Status != AI_CANDIDATE {
		t.Fatalf("Expected pass with unavailable M5 in watch-only mode, got status %s reason %s", res.Status, res.Reason)
	}
	if res.M5Summary == nil || res.M5Summary.Status != M5ConfirmationUnavailable {
		t.Fatalf("Expected unavailable M5 summary, got %+v", res.M5Summary)
	}

	gate.SetMarketData(NewMarketDataUsecase(&mockLocalGateMarketDataProvider{m5Candles: []dto.Candle{
		{Open: 5.0, Close: 5.0, High: 5.0, Low: 5.0},
		{Open: 5.0, Close: 5.0, High: 5.0, Low: 5.0},
		{Open: 5.0, Close: 5.0, High: 5.0, Low: 5.0},
		{Open: 5.0, Close: 5.0, High: 5.0, Low: 5.0},
		{Open: 5.0, Close: 5.0, High: 5.0, Low: 5.0},
		{Open: 5.0, Close: 5.0, High: 5.0, Low: 5.0},
		{Open: 5.0, Close: 5.0, High: 5.0, Low: 5.0},
		{Open: 5.0, Close: 5.0, High: 5.0, Low: 5.0},
		{Open: 5.0, Close: 5.0, High: 5.0, Low: 5.0},
		{Open: 5.0, Close: 4.9, High: 5.0, Low: 4.8},
	}}))
	res = gate.Evaluate(quant, policy, m15)
	if !res.Passed || res.Status != AI_CANDIDATE {
		t.Fatalf("Expected pass with failed M5 in watch-only mode, got status %s reason %s", res.Status, res.Reason)
	}
	if res.M5Summary == nil || res.M5Summary.Status != M5ConfirmationFailed {
		t.Fatalf("Expected failed M5 summary, got %+v", res.M5Summary)
	}
}
