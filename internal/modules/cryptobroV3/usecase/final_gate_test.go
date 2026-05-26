package usecase

import (
	"cpbro-engine/internal/modules/cryptobroV3/dto"
	"testing"
	"time"
)

func TestFinalGateUsecase_Evaluate(t *testing.T) {
	uc := NewFinalGateUsecase()

	policy := MarketPolicy{
		AllowLong:        true,
		AllowShort:       true,
		LongMode:         NORMAL,
		ShortMode:        NORMAL,
		AllowedTiers:     []Tier{TierA, TierB},
		AllowedPlaybooks: []Playbook{TREND_PULLBACK, LIQUIDITY_SWEEP_REVERSAL, COMPRESSION_BREAKOUT_RETEST, RANGE_EDGE_REVERSAL},
		MinScoreExecute:  7.0,
		MinRRExecute:     1.5,
		MaxFinalExecute:  3,
		CooldownMinutes:  60,
	}

	// Base QuantResult that passes standard filters
	baseQuant := QuantResult{
		Symbol:       "BTCUSDT",
		Direction:    LONG,
		Playbook:     TREND_PULLBACK,
		Score:        7.5,
		Tier:         TierA,
		H1Trend:      "BULLISH",
		H4Trend:      "BULLISH",
		IndicatorMet: true,
		TechnicalSnapshot: TechnicalSnapshot{
			IndicatorValues: map[string]float64{
				IndicatorADX:     25.0,
				"wick_rejection": 0.0,
			},
		},
		TradePlan: TradePlan{
			Symbol:     "BTCUSDT",
			Direction:  LONG,
			EntryPrice: 50000,
			StopLoss:   49000,
			TakeProfit: 52000,
		},
	}

	baseLocalGate := LocalGateResult{
		Passed: true,
		Status: AI_CANDIDATE,
	}

	baseAI := dto.AIAuditResponse{
		Symbol:          "BTCUSDT",
		Decision:        "CONFIRM",
		Confidence:      "HIGH",
		IsApproved:      true,
		Sentiment:       "BULLISH",
		HasRejection:    true,
		HasConfirmation: true,
	}

	basePlanReview := PlanReview{
		Conflicted:      false,
		EntryStillValid: true,
		NeedRetest:      false,
		Status:          PLAN_VALID,
	}

	baseStaleness := StalenessResult{
		IsStale: false,
		Status:  FRESH,
	}

	t.Run("Fully Valid Execute", func(t *testing.T) {
		res := uc.Evaluate(
			baseQuant,
			baseLocalGate,
			baseAI,
			basePlanReview,
			baseStaleness,
			policy,
			50000, // latest price matches entry
			nil,
			nil,
			nil,
		)
		if res.Status != FINAL_EXECUTE {
			t.Errorf("expected status %s, got %s (reason: %s)", FINAL_EXECUTE, res.Status, res.Reason)
		}
		if !res.IsExecutable {
			t.Errorf("expected IsExecutable to be true")
		}
	})

	t.Run("Fail LocalGate Status", func(t *testing.T) {
		lg := baseLocalGate
		lg.Status = LOCAL_REJECT

		res := uc.Evaluate(baseQuant, lg, baseAI, basePlanReview, baseStaleness, policy, 50000, nil, nil, nil)
		if res.Status != FINAL_REJECT {
			t.Errorf("expected status %s, got %s", FINAL_REJECT, res.Status)
		}
	})

	t.Run("Fail AI Decision Reject", func(t *testing.T) {
		ai := baseAI
		ai.Decision = "REJECT"

		res := uc.Evaluate(baseQuant, baseLocalGate, ai, basePlanReview, baseStaleness, policy, 50000, nil, nil, nil)
		if res.Status != FINAL_REJECT {
			t.Errorf("expected status %s, got %s", FINAL_REJECT, res.Status)
		}
	})

	t.Run("AI Decision Wait is WATCH", func(t *testing.T) {
		ai := baseAI
		ai.Decision = "WAIT"

		res := uc.Evaluate(baseQuant, baseLocalGate, ai, basePlanReview, baseStaleness, policy, 50000, nil, nil, nil)
		if res.Status != FINAL_WATCH {
			t.Errorf("expected status %s, got %s", FINAL_WATCH, res.Status)
		}
	})

	t.Run("Fail AI Confidence Low is REJECT", func(t *testing.T) {
		ai := baseAI
		ai.Confidence = "LOW"

		res := uc.Evaluate(baseQuant, baseLocalGate, ai, basePlanReview, baseStaleness, policy, 50000, nil, nil, nil)
		if res.Status != FINAL_REJECT {
			t.Errorf("expected status %s, got %s", FINAL_REJECT, res.Status)
		}
	})

	t.Run("Fail AI Conflict With Bot", func(t *testing.T) {
		ai := baseAI
		ai.ConflictWithBot = true

		res := uc.Evaluate(baseQuant, baseLocalGate, ai, basePlanReview, baseStaleness, policy, 50000, nil, nil, nil)
		if res.Status != FINAL_REJECT {
			t.Errorf("expected status %s, got %s", FINAL_REJECT, res.Status)
		}
	})

	t.Run("Fail Staleness LATE is WATCH", func(t *testing.T) {
		st := baseStaleness
		st.Status = LATE

		res := uc.Evaluate(baseQuant, baseLocalGate, baseAI, basePlanReview, st, policy, 50000, nil, nil, nil)
		if res.Status != FINAL_WATCH {
			t.Errorf("expected status %s, got %s", FINAL_WATCH, res.Status)
		}
	})

	t.Run("Fail Staleness MISSED is REJECT", func(t *testing.T) {
		st := baseStaleness
		st.Status = MISSED

		res := uc.Evaluate(baseQuant, baseLocalGate, baseAI, basePlanReview, st, policy, 50000, nil, nil, nil)
		if res.Status != FINAL_REJECT {
			t.Errorf("expected status %s, got %s", FINAL_REJECT, res.Status)
		}
	})

	t.Run("Fail MarketPolicy Disallow Long", func(t *testing.T) {
		pol := policy
		pol.AllowLong = false

		res := uc.Evaluate(baseQuant, baseLocalGate, baseAI, basePlanReview, baseStaleness, pol, 50000, nil, nil, nil)
		if res.Status != FINAL_REJECT {
			t.Errorf("expected status %s, got %s", FINAL_REJECT, res.Status)
		}
	})

	t.Run("Fail Playbook Not Allowed", func(t *testing.T) {
		pol := policy
		pol.AllowedPlaybooks = []Playbook{LIQUIDITY_SWEEP_REVERSAL}

		res := uc.Evaluate(baseQuant, baseLocalGate, baseAI, basePlanReview, baseStaleness, pol, 50000, nil, nil, nil)
		if res.Status != FINAL_REJECT {
			t.Errorf("expected status %s, got %s", FINAL_REJECT, res.Status)
		}
	})

	t.Run("Fail Tier Not Allowed", func(t *testing.T) {
		pol := policy
		pol.AllowedTiers = []Tier{TierB}

		res := uc.Evaluate(baseQuant, baseLocalGate, baseAI, basePlanReview, baseStaleness, pol, 50000, nil, nil, nil)
		if res.Status != FINAL_REJECT {
			t.Errorf("expected status %s, got %s", FINAL_REJECT, res.Status)
		}
	})

	t.Run("Fail Score Too Low", func(t *testing.T) {
		q := baseQuant
		q.Score = 6.5

		res := uc.Evaluate(q, baseLocalGate, baseAI, basePlanReview, baseStaleness, policy, 50000, nil, nil, nil)
		if res.Status != FINAL_REJECT {
			t.Errorf("expected status %s, got %s", FINAL_REJECT, res.Status)
		}
	})

	t.Run("Fail RR Too Low", func(t *testing.T) {
		// Entry: 50k, SL: 49k (risk 1k), TP: 51k (reward 1k) -> RR = 1.0 (fails MinRRExecute = 1.5)
		q := baseQuant
		q.TradePlan.TakeProfit = 51000

		res := uc.Evaluate(q, baseLocalGate, baseAI, basePlanReview, baseStaleness, policy, 50000, nil, nil, nil)
		if res.Status != FINAL_REJECT {
			t.Errorf("expected status %s, got %s", FINAL_REJECT, res.Status)
		}
	})

	t.Run("ADX Expansion Check", func(t *testing.T) {
		q := baseQuant
		q.Playbook = RANGE_EDGE_REVERSAL
		q.TechnicalSnapshot.IndicatorValues = map[string]float64{
			IndicatorADX:      35.0,
			"wick_rejection":  1.0,
			"near_range_edge": 1.0,
		}

		res := uc.Evaluate(q, baseLocalGate, baseAI, basePlanReview, baseStaleness, policy, 50000, nil, nil, nil)
		if res.Status != FINAL_WATCH {
			t.Errorf("expected status %s (due to ADX expansion watch rule), got %s (reason: %s)", FINAL_WATCH, res.Status, res.Reason)
		}
	})

	t.Run("Fail Rejection Missing", func(t *testing.T) {
		q := baseQuant
		q.Playbook = LIQUIDITY_SWEEP_REVERSAL
		q.TechnicalSnapshot.IndicatorValues = map[string]float64{
			"wick_rejection": 0.0,
		}
		ai := baseAI
		ai.HasRejection = false

		res := uc.Evaluate(q, baseLocalGate, ai, basePlanReview, baseStaleness, policy, 50000, nil, nil, nil)
		if res.Status != FINAL_REJECT {
			t.Errorf("expected status %s, got %s", FINAL_REJECT, res.Status)
		}
	})

	t.Run("Retest Required is WATCH", func(t *testing.T) {
		q := baseQuant
		q.Playbook = COMPRESSION_BREAKOUT_RETEST
		q.SetupType = "BREAKOUT" // First candle breakout, no retest
		q.TechnicalSnapshot.IndicatorValues = map[string]float64{
			"contraction":  1.0,
			"extreme_oi":   1.0,
			"volume_spike": 1.0,
		}

		res := uc.Evaluate(q, baseLocalGate, baseAI, basePlanReview, baseStaleness, policy, 50000, nil, nil, nil)
		if res.Status != FINAL_WATCH {
			t.Errorf("expected status %s, got %s", FINAL_WATCH, res.Status)
		}
	})

	t.Run("Cooldown Active is REJECT", func(t *testing.T) {
		active := []SignalJournal{
			{
				Symbol:    "BTCUSDT",
				Status:    MONITORING,
				Direction: LONG,
				CreatedAt: time.Now().Add(-10 * time.Minute),
			},
		}

		res := uc.Evaluate(baseQuant, baseLocalGate, baseAI, basePlanReview, baseStaleness, policy, 50000, active, nil, nil)
		if res.Status != FINAL_REJECT {
			t.Errorf("expected status %s, got %s", FINAL_REJECT, res.Status)
		}
	})

	t.Run("Max Concurrent Signals Exceeded is REJECT", func(t *testing.T) {
		active := []SignalJournal{
			{Symbol: "ETHUSDT", Status: MONITORING},
			{Symbol: "SOLUSDT", Status: MONITORING},
			{Symbol: "ADAUSDT", Status: MONITORING},
		}

		res := uc.Evaluate(baseQuant, baseLocalGate, baseAI, basePlanReview, baseStaleness, policy, 50000, active, nil, nil)
		if res.Status != FINAL_REJECT {
			t.Errorf("expected status %s, got %s", FINAL_REJECT, res.Status)
		}
	})

	t.Run("AI Error Policy is AI_ERROR_REVIEW", func(t *testing.T) {
		ai := baseAI
		ai.Reasoning = "AI_ERROR: Gemini Timeout"

		res := uc.Evaluate(baseQuant, baseLocalGate, ai, basePlanReview, baseStaleness, policy, 50000, nil, nil, nil)
		if res.Status != AI_ERROR_REVIEW {
			t.Errorf("expected status %s, got %s", AI_ERROR_REVIEW, res.Status)
		}
	})

	t.Run("AI Decision Wait with Soft Plan Conflict remains WATCH", func(t *testing.T) {
		ai := baseAI
		ai.Decision = "WAIT"
		pr := basePlanReview
		pr.Conflicted = true
		pr.NeedRetest = true
		pr.Reason = "retest needed"

		res := uc.Evaluate(baseQuant, baseLocalGate, ai, pr, baseStaleness, policy, 50000, nil, nil, nil)
		if res.Status != FINAL_WATCH {
			t.Errorf("expected status %s (downgraded to watch), got %s", FINAL_WATCH, res.Status)
		}
	})

	t.Run("AI Decision Wait with Hard Plan Conflict is REJECT", func(t *testing.T) {
		ai := baseAI
		ai.Decision = "WAIT"
		pr := basePlanReview
		pr.Conflicted = true
		pr.NeedRetest = false
		pr.Reason = "hard direction mismatch"

		res := uc.Evaluate(baseQuant, baseLocalGate, ai, pr, baseStaleness, policy, 50000, nil, nil, nil)
		if res.Status != FINAL_REJECT {
			t.Errorf("expected status %s (hard conflict is reject), got %s", FINAL_REJECT, res.Status)
		}
	})

	t.Run("AI Decision Wait with low score remains WATCH", func(t *testing.T) {
		ai := baseAI
		ai.Decision = "WAIT"
		q := baseQuant
		q.Score = 6.5 // below MinScoreExecute = 7.0

		res := uc.Evaluate(q, baseLocalGate, ai, basePlanReview, baseStaleness, policy, 50000, nil, nil, nil)
		if res.Status != FINAL_WATCH {
			t.Errorf("expected status %s, got %s", FINAL_WATCH, res.Status)
		}
	})

	t.Run("AI Decision Wait with low RR remains WATCH", func(t *testing.T) {
		ai := baseAI
		ai.Decision = "WAIT"
		q := baseQuant
		q.TradePlan.TakeProfit = 51000 // fails MinRRExecute

		res := uc.Evaluate(q, baseLocalGate, ai, basePlanReview, baseStaleness, policy, 50000, nil, nil, nil)
		if res.Status != FINAL_WATCH {
			t.Errorf("expected status %s, got %s", FINAL_WATCH, res.Status)
		}
	})

	t.Run("AI Decision Wait with Low Confidence is REJECT (Hard Safety)", func(t *testing.T) {
		ai := baseAI
		ai.Decision = "WAIT"
		ai.Confidence = "LOW"

		res := uc.Evaluate(baseQuant, baseLocalGate, ai, basePlanReview, baseStaleness, policy, 50000, nil, nil, nil)
		if res.Status != FINAL_REJECT {
			t.Errorf("expected status %s (confidence LOW is hard safety reject), got %s", FINAL_REJECT, res.Status)
		}
	})
}
