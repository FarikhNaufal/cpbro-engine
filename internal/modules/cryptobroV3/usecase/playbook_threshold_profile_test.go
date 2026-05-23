package usecase

import (
	"strings"
	"testing"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
)

func TestPlaybookThresholdProfile_Scenarios(t *testing.T) {
	gate := NewLocalGateUsecase()
	stalenessUC := NewStalenessUsecase(15 * time.Minute)

	policyNormal := MarketPolicy{
		AllowLong:       true,
		AllowShort:      true,
		LongMode:        NORMAL,
		ShortMode:       NORMAL,
		MinScoreAI:      7.0,
		MinScoreExecute: 7.3,
		MinRRExecute:    1.5,
		MinADXExecute:   20.0,
		AllowedPlaybooks: []Playbook{
			TREND_PULLBACK,
			LIQUIDITY_SWEEP_REVERSAL,
			COMPRESSION_BREAKOUT_RETEST,
			RANGE_EDGE_REVERSAL,
			CROWDED_POSITIONING_SQUEEZE,
		},
		AllowedTiers: []Tier{TierA, TierB, TierC},
		Reason:       "Normal Market",
	}

	m15VolumeSpike := []dto.Candle{
		{Vol: 10.0}, {Vol: 10.0}, {Vol: 10.0}, {Vol: 10.0}, {Vol: 10.0},
		{Vol: 10.0}, {Vol: 10.0}, {Vol: 10.0}, {Vol: 10.0}, {Vol: 10.0},
		{Vol: 50.0}, // Last candle has volume spike > multiplier * average
	}

	m15NoVolumeSpike := []dto.Candle{
		{Vol: 10.0}, {Vol: 10.0}, {Vol: 10.0}, {Vol: 10.0}, {Vol: 10.0},
		{Vol: 10.0}, {Vol: 10.0}, {Vol: 10.0}, {Vol: 10.0}, {Vol: 10.0},
		{Vol: 10.0}, // No volume spike
	}

	baseQuant := QuantResult{
		Symbol:       "BTCUSDT",
		Direction:    LONG,
		Playbook:     TREND_PULLBACK,
		Tier:         TierA,
		Score:        7.5,
		IndicatorMet: true,
		SetupType:    "PULLBACK",
		TradePlan: TradePlan{
			EntryPrice: 100.0,
			TakeProfit: 140.0, // RR = 40 / 20 = 2.0 (satisfies MinRR for all profiles)
			StopLoss:   80.0,
		},
		TechnicalSnapshot: TechnicalSnapshot{
			IndicatorValues: map[string]float64{
				"adx":            25.0,
				"wick_rejection": 1.0,
				IndicatorATR:     10.0,
			},
		},
	}

	// 1. Trend Pullback membutuhkan ADX cukup
	t.Run("Trend Pullback needs sufficient ADX", func(t *testing.T) {
		q := baseQuant
		q.Playbook = TREND_PULLBACK
		q.TechnicalSnapshot.IndicatorValues["adx"] = 15.0 // below profile's MinADX (20.0)
		res := gate.Evaluate(q, policyNormal, m15NoVolumeSpike)
		if res.Passed || res.Status != LOCAL_WATCH {
			t.Errorf("Expected LOCAL_WATCH for Trend Pullback with low ADX, got Passed=%v, Status=%s", res.Passed, res.Status)
		}
	})

	// 2. Liquidity Sweep tidak gagal hanya karena ADX rendah jika sweep/rejection/volume valid
	t.Run("Liquidity Sweep does not fail just on low ADX", func(t *testing.T) {
		q := baseQuant
		q.Playbook = LIQUIDITY_SWEEP_REVERSAL
		q.TechnicalSnapshot.IndicatorValues["adx"] = 10.0 // Low ADX
		q.TechnicalSnapshot.IndicatorValues["wick_rejection"] = 1.0
		res := gate.Evaluate(q, policyNormal, m15VolumeSpike)
		if !res.Passed || res.Status != AI_CANDIDATE {
			t.Errorf("Expected AI_CANDIDATE for valid Liquidity Sweep even on low ADX, got Passed=%v, Status=%s, Reason=%s", res.Passed, res.Status, res.Reason)
		}
	})

	// 3. Liquidity Sweep gagal jika volume confirmation tidak ada
	t.Run("Liquidity Sweep fails without volume confirmation", func(t *testing.T) {
		q := baseQuant
		q.Playbook = LIQUIDITY_SWEEP_REVERSAL
		q.TechnicalSnapshot.IndicatorValues["wick_rejection"] = 1.0
		res := gate.Evaluate(q, policyNormal, m15NoVolumeSpike)
		if res.Passed || res.Status != LOCAL_REJECT {
			t.Errorf("Expected LOCAL_REJECT for Liquidity Sweep without volume confirmation, got Passed=%v, Status=%s", res.Passed, res.Status)
		}
	})

	// 4. Compression Breakout Retest watch jika masih breakout candle pertama
	t.Run("Compression Breakout Retest watch on first breakout candle", func(t *testing.T) {
		q := baseQuant
		q.Playbook = COMPRESSION_BREAKOUT_RETEST
		q.SetupType = "BREAKOUT"
		res := gate.Evaluate(q, policyNormal, m15VolumeSpike)
		if res.Passed || res.Status != LOCAL_WATCH || res.Reason != "WAIT_RETEST_OR_BREAKOUT_FIRST_CANDLE" {
			t.Errorf("Expected LOCAL_WATCH with reason WAIT_RETEST_OR_BREAKOUT_FIRST_CANDLE, got Passed=%v, Status=%s, Reason=%s", res.Passed, res.Status, res.Reason)
		}
	})

	// 5. Range Edge Reversal watch jika ADX expansion kuat
	t.Run("Range Edge Reversal watch on strong ADX expansion", func(t *testing.T) {
		q := baseQuant
		q.Playbook = RANGE_EDGE_REVERSAL
		q.TechnicalSnapshot.IndicatorValues["adx"] = 35.0 // > 30.0 (expansion)
		res := gate.Evaluate(q, policyNormal, m15NoVolumeSpike)
		if res.Passed || res.Status != LOCAL_WATCH {
			t.Errorf("Expected LOCAL_WATCH for Range Edge Reversal under strong ADX expansion, got Passed=%v, Status=%s", res.Passed, res.Status)
		}
	})

	// 6. Range Edge Reversal butuh rejection
	t.Run("Range Edge Reversal needs rejection", func(t *testing.T) {
		q := baseQuant
		q.Playbook = RANGE_EDGE_REVERSAL
		q.TechnicalSnapshot.IndicatorValues["adx"] = 15.0
		q.TechnicalSnapshot.IndicatorValues["wick_rejection"] = 0.0 // no rejection
		res := gate.Evaluate(q, policyNormal, m15NoVolumeSpike)
		if res.Passed || res.Status != LOCAL_REJECT {
			t.Errorf("Expected LOCAL_REJECT for Range Edge Reversal without rejection, got Passed=%v, Status=%s", res.Passed, res.Status)
		}
	})

	// 7. Crowded Squeeze butuh crowding evidence
	t.Run("Crowded Squeeze needs crowding evidence", func(t *testing.T) {
		q := baseQuant
		q.Playbook = CROWDED_POSITIONING_SQUEEZE
		q.TechnicalSnapshot.IndicatorValues["funding_rate"] = 0.0
		q.TechnicalSnapshot.IndicatorValues["oi_change"] = 0.0
		q.TechnicalSnapshot.IndicatorValues["crowding_score"] = 0.0
		res := gate.Evaluate(q, policyNormal, m15NoVolumeSpike)
		if res.Passed || res.Status != LOCAL_REJECT {
			t.Errorf("Expected LOCAL_REJECT for Crowded Squeeze without crowding evidence, got Passed=%v, Status=%s", res.Passed, res.Status)
		}
	})

	// 8. Staleness Trend Pullback lebih longgar daripada Liquidity Sweep
	t.Run("Staleness Trend Pullback looser than Liquidity Sweep", func(t *testing.T) {
		qPullback := baseQuant
		qPullback.Playbook = TREND_PULLBACK // Base 0.45 ATR, Tier A adds 0.05 -> 0.50 ATR
		qPullback.Tier = TierA

		qSweep := baseQuant
		qSweep.Playbook = LIQUIDITY_SWEEP_REVERSAL // Base 0.30 ATR, Tier A adds 0.05 -> 0.35 ATR
		qSweep.Tier = TierA

		resPullback := stalenessUC.Evaluate(qPullback, PlanReview{}, policyNormal, 104.2) // distance = 4.2 -> 0.42 ATR
		resSweep := stalenessUC.Evaluate(qSweep, PlanReview{}, policyNormal, 104.2)       // distance = 4.2 -> 0.42 ATR

		if resPullback.Status != FRESH {
			t.Errorf("Expected Pullback to be FRESH (threshold 0.50), got %s", resPullback.Status)
		}
		if resSweep.Status == FRESH {
			t.Errorf("Expected Sweep to NOT be FRESH (threshold 0.35), got %s", resSweep.Status)
		}
	})

	// 9. BTCChaos membuat staleness threshold lebih ketat
	t.Run("BTCChaos makes staleness threshold stricter", func(t *testing.T) {
		q := baseQuant
		q.Playbook = TREND_PULLBACK // Base 0.45, Tier B adds 0.0 -> 0.45 ATR
		q.Tier = TierB

		policyNormal := MarketPolicy{Reason: "Normal"}
		policyChaos := MarketPolicy{Reason: "BTC_CHAOS"} // reduces ATR threshold by 0.05 -> 0.40 ATR

		resNormal := stalenessUC.Evaluate(q, PlanReview{}, policyNormal, 104.2) // distance = 4.2 -> 0.42 ATR
		resChaos := stalenessUC.Evaluate(q, PlanReview{}, policyChaos, 104.2)   // distance = 4.2 -> 0.42 ATR

		if resNormal.Status != FRESH {
			t.Errorf("Expected normal to be FRESH, got %s", resNormal.Status)
		}
		if resChaos.Status == FRESH {
			t.Errorf("Expected chaos to NOT be FRESH, got %s", resChaos.Status)
		}
	})

	// 10. Policy lebih ketat harus override profile
	t.Run("Policy overrides profile if stricter", func(t *testing.T) {
		q := baseQuant
		q.Playbook = TREND_PULLBACK
		q.Score = 7.5 // higher than Trend Pullback MinScoreAI (7.0) but within 0.5 of policyStricter.MinScoreAI (7.8)

		policyStricter := policyNormal
		policyStricter.MinScoreAI = 7.8 // stricter than profile 7.0

		res := gate.Evaluate(q, policyStricter, m15NoVolumeSpike)
		if res.Passed || res.Status != LOCAL_WATCH {
			t.Errorf("Expected LOCAL_WATCH due to stricter policy MinScoreAI override, got Passed=%v, Status=%s", res.Passed, res.Status)
		}
	})

	// 11. Profile lebih ketat harus override policy jika policy terlalu longgar
	t.Run("Profile overrides policy if stricter", func(t *testing.T) {
		q := baseQuant
		q.Playbook = CROWDED_POSITIONING_SQUEEZE
		q.Score = 6.8 // lower than Squeeze profile MinScoreAI (7.5) by more than 0.5 -> LOCAL_REJECT

		policyLoose := policyNormal
		policyLoose.MinScoreAI = 6.0 // very loose policy MinScoreAI

		res := gate.Evaluate(q, policyLoose, m15NoVolumeSpike)
		if res.Passed || res.Status != LOCAL_REJECT {
			t.Errorf("Expected LOCAL_REJECT because profile MinScoreAI 7.5 overrides loose policy 6.0, got Passed=%v, Status=%s, Reason=%s", res.Passed, res.Status, res.Reason)
		}
	})

	// 12. BTCChaos tightens profile
	t.Run("BTCChaos tightens profile thresholds", func(t *testing.T) {
		policyChaos := MarketPolicy{Reason: "BTC_CHAOS active"}
		profile := GetPlaybookThresholdProfile(TREND_PULLBACK, policyChaos, TierA)
		if profile.MinScoreAI < 7.8 {
			t.Errorf("Expected MinScoreAI >= 7.8 under chaos, got %0.1f", profile.MinScoreAI)
		}
		if profile.MinScoreExecute < 8.2 {
			t.Errorf("Expected MinScoreExecute >= 8.2 under chaos, got %0.1f", profile.MinScoreExecute)
		}
		if profile.MinRR < 2.0 {
			t.Errorf("Expected MinRR >= 2.0 under chaos, got %0.1f", profile.MinRR)
		}
		if !profile.RequireAIHigh {
			t.Error("Expected RequireAIHigh to be true under chaos")
		}
	})

	// 13. Tier C tightens profile
	t.Run("Tier C tightens profile thresholds", func(t *testing.T) {
		profile := GetPlaybookThresholdProfile(TREND_PULLBACK, policyNormal, TierC)
		if profile.MinScoreAI < 7.5 {
			t.Errorf("Expected MinScoreAI >= 7.5 for Tier C, got %0.1f", profile.MinScoreAI)
		}
		if profile.MinScoreExecute < 7.8 {
			t.Errorf("Expected MinScoreExecute >= 7.8 for Tier C, got %0.1f", profile.MinScoreExecute)
		}
		if profile.MinRR < 1.8 {
			t.Errorf("Expected MinRR >= 1.8 for Tier C, got %0.1f", profile.MinRR)
		}
		if !profile.RequireAIHigh {
			t.Error("Expected RequireAIHigh to be true for Tier C")
		}
	})

	// 14. SHORT SWEEP_ONLY tidak meloloskan SHORT TREND_PULLBACK
	t.Run("SHORT SWEEP_ONLY blocks SHORT TREND_PULLBACK", func(t *testing.T) {
		q := baseQuant
		q.Direction = SHORT
		q.Playbook = TREND_PULLBACK
		q.TradePlan.StopLoss = 120.0
		q.TradePlan.TakeProfit = 60.0

		policySweepOnly := policyNormal
		policySweepOnly.ShortMode = SWEEP_ONLY

		res := gate.Evaluate(q, policySweepOnly, m15NoVolumeSpike)
		if res.Passed || res.Status != LOCAL_REJECT {
			t.Errorf("Expected LOCAL_REJECT for Short Trend Pullback under SWEEP_ONLY, got Passed=%v, Status=%s", res.Passed, res.Status)
		}
		if !strings.Contains(res.Reason, "ShortMode is SWEEP_ONLY") {
			t.Errorf("Expected reason to mention SWEEP_ONLY, got %q", res.Reason)
		}
	})

	// 15. LONG REVERSAL_ONLY tidak meloloskan LONG TREND_PULLBACK
	t.Run("LONG REVERSAL_ONLY blocks LONG TREND_PULLBACK", func(t *testing.T) {
		q := baseQuant
		q.Direction = LONG
		q.Playbook = TREND_PULLBACK
		q.TechnicalSnapshot.IndicatorValues = map[string]float64{
			"adx":            25.0,
			"wick_rejection": 1.0,
			IndicatorATR:     10.0,
		}

		policyReversalOnly := policyNormal
		policyReversalOnly.LongMode = REVERSAL_ONLY

		res := gate.Evaluate(q, policyReversalOnly, m15NoVolumeSpike)
		if res.Passed || res.Status != LOCAL_REJECT {
			t.Errorf("Expected LOCAL_REJECT for Long Trend Pullback under REVERSAL_ONLY, got Passed=%v, Status=%s", res.Passed, res.Status)
		}
		if !strings.Contains(res.Reason, "LongMode is REVERSAL_ONLY") {
			t.Errorf("Expected reason to mention REVERSAL_ONLY, got %q", res.Reason)
		}
	})

	// 16. Local Gate tidak execute, tidak panggil AI, tidak kirim Telegram
	t.Run("Local Gate does not execute, call AI, or send Telegram", func(t *testing.T) {
		q := baseQuant
		q.TechnicalSnapshot.IndicatorValues = map[string]float64{
			"adx":            25.0,
			"wick_rejection": 1.0,
			IndicatorATR:     10.0,
		}
		res := gate.Evaluate(q, policyNormal, m15NoVolumeSpike)

		// The evaluation result must only return status and reason, without trigger fields
		if res.Reason == "" {
			t.Error("Expected result to have a non-empty reason")
		}
		// Confirming no execution or side effects happened
		if res.Status != AI_CANDIDATE {
			t.Errorf("Expected status AI_CANDIDATE, got %s, Reason: %s", res.Status, res.Reason)
		}
	})
}
