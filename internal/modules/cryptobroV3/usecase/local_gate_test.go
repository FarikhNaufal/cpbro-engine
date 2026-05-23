package usecase

import (
	"testing"

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
		{Vol: 100}, {Vol: 100}, {Vol: 100}, {Vol: 100}, {Vol: 200}, // volume spike at last candle (200 vs 100 avg = 2.0x > 1.5x)
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

	// 6. Rule 8: RR < 1.5
	quantBadRR := quantPass
	quantBadRR.TradePlan.TakeProfit = 5.2 // Reward = 0.2, Risk = 0.5 (RR = 0.4 < 1.5)
	res = gate.Evaluate(quantBadRR, policy, m15)
	if res.Passed || res.Status != LOCAL_REJECT {
		t.Errorf("Expected reject for low RR, got status %s reason %s", res.Status, res.Reason)
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
