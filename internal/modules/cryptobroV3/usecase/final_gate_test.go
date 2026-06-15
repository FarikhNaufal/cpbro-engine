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

	t.Run("Policy medium AI can execute when global HIGH override is disabled", func(t *testing.T) {
		t.Setenv("REQUIRE_AI_HIGH_FOR_EXECUTE", "false")
		pol := policy
		pol.RequireAIConfidence = AIConfidenceMedium
		ai := baseAI
		ai.Confidence = "MEDIUM"

		res := uc.Evaluate(baseQuant, baseLocalGate, ai, basePlanReview, baseStaleness, pol, 50000, nil, nil, nil)
		if res.Status != FINAL_EXECUTE {
			t.Errorf("expected status %s, got %s (reason: %s)", FINAL_EXECUTE, res.Status, res.Reason)
		}
	})

	t.Run("Policy allows LATE entry when global fresh override is disabled", func(t *testing.T) {
		t.Setenv("REQUIRE_FRESH_ENTRY_FOR_EXECUTE", "false")
		pol := policy
		pol.RequireFreshEntry = false
		st := baseStaleness
		st.Status = LATE

		res := uc.Evaluate(baseQuant, baseLocalGate, baseAI, basePlanReview, st, pol, 50000, nil, nil, nil)
		if res.Status != FINAL_EXECUTE {
			t.Errorf("expected status %s, got %s (reason: %s)", FINAL_EXECUTE, res.Status, res.Reason)
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

	// Rule 24: Symbol-level directional price move guard tests
	t.Run("Rule24 LONG blocked during extreme dump", func(t *testing.T) {
		pol := policy
		pol.MaxPriceMove24hLong = 0.08  // 8% limit
		pol.MaxPriceMove24hShort = 0.18 // 18% limit

		q := baseQuant
		q.Direction = LONG
		q.TechnicalSnapshot = TechnicalSnapshot{
			IndicatorValues: map[string]float64{
				IndicatorADX: 25.0,
			},
			PriceChange24h: -11.2, // XLM-like: symbol dumped 11.2%
		}

		res := uc.Evaluate(q, baseLocalGate, baseAI, basePlanReview, baseStaleness, pol, 50000, nil, nil, nil)
		if res.Status != FINAL_REJECT {
			t.Errorf("expected FINAL_REJECT for LONG during -11.2%% dump (limit 8%%), got %s (reason: %s)", res.Status, res.Reason)
		}
	})

	t.Run("Rule24 SHORT still allowed during dump", func(t *testing.T) {
		pol := policy
		pol.MaxPriceMove24hLong = 0.08
		pol.MaxPriceMove24hShort = 0.18

		q := baseQuant
		q.Direction = SHORT
		q.TradePlan.Direction = SHORT
		q.TradePlan.StopLoss = 51000
		q.TradePlan.TakeProfit = 48000
		q.TechnicalSnapshot = TechnicalSnapshot{
			IndicatorValues: map[string]float64{
				IndicatorADX: 25.0,
			},
			PriceChange24h: -11.2, // dump but SHORT is fine
		}

		res := uc.Evaluate(q, baseLocalGate, baseAI, basePlanReview, baseStaleness, pol, 50000, nil, nil, nil)
		// Should NOT be rejected by Rule 24 (dump doesn't block SHORT)
		if res.Status == FINAL_REJECT {
			if contains(res.Reason, "directional SHORT limit") {
				t.Errorf("SHORT should NOT be blocked by Rule 24 during dump, got: %s", res.Reason)
			}
		}
	})

	t.Run("Rule24 LONG allowed during moderate move", func(t *testing.T) {
		pol := policy
		pol.MaxPriceMove24hLong = 0.15
		pol.MaxPriceMove24hShort = 0.15

		q := baseQuant
		q.TechnicalSnapshot = TechnicalSnapshot{
			IndicatorValues: map[string]float64{
				IndicatorADX: 25.0,
			},
			PriceChange24h: -5.0, // moderate 5% dip, within 15% limit
		}

		res := uc.Evaluate(q, baseLocalGate, baseAI, basePlanReview, baseStaleness, pol, 50000, nil, nil, nil)
		if res.Status == FINAL_REJECT {
			if contains(res.Reason, "directional LONG limit") {
				t.Errorf("LONG should NOT be blocked by Rule 24 during moderate -5%% move (limit 15%%), got: %s", res.Reason)
			}
		}
	})

	// Rule 25: SL-to-ATR ratio guard tests
	t.Run("Rule25 SL too tight relative to ATR is WATCH", func(t *testing.T) {
		q := baseQuant
		q.Playbook = LIQUIDITY_SWEEP_REVERSAL
		q.TradePlan.EntryPrice = 0.22907
		q.TradePlan.StopLoss = 0.226946  // SL distance = 0.002124
		q.TradePlan.TakeProfit = 0.2332
		q.TriggerPrice = 0.22907
		q.TechnicalSnapshot = TechnicalSnapshot{
			IndicatorValues: map[string]float64{
				IndicatorADX:           25.0,
				IndicatorATR:           0.0139,  // ATR = 0.0139, SL = 0.002124 = 0.15x ATR
				IndicatorWickRejection: 1.0,
				IndicatorVolumeSpike:   1.0,
				IndicatorSweepLow:      1.0,
				IndicatorPARejection:   1.0,
			},
		}

		ai := baseAI
		ai.HasRejection = true
		ai.HasConfirmation = true

		res := uc.Evaluate(q, baseLocalGate, ai, basePlanReview, baseStaleness, policy, 0.22907, nil, nil, nil)
		// SL distance 0.002124 / ATR 0.0139 = 0.15x ATR, far below 1.2x minimum → WATCH
		if res.Status == FINAL_EXECUTE {
			t.Errorf("expected non-EXECUTE status for SL too tight (0.15x ATR vs 1.2x required), got %s (reason: %s)", res.Status, res.Reason)
		}
	})

	t.Run("Rule25 SL adequate passes", func(t *testing.T) {
		q := baseQuant
		q.TradePlan.EntryPrice = 50000
		q.TradePlan.StopLoss = 49000    // SL distance = 1000
		q.TradePlan.TakeProfit = 52000
		q.TechnicalSnapshot = TechnicalSnapshot{
			IndicatorValues: map[string]float64{
				IndicatorADX: 25.0,
				IndicatorATR: 500.0, // ATR = 500, SL distance = 1000 = 2.0x ATR → adequate
			},
		}

		res := uc.Evaluate(q, baseLocalGate, baseAI, basePlanReview, baseStaleness, policy, 50000, nil, nil, nil)
		if res.Status == FINAL_WATCH || res.Status == FINAL_REJECT {
			if contains(res.Reason, "SL distance") {
				t.Errorf("SL should be adequate (2.0x ATR), but got rejected/watched for SL: %s", res.Reason)
			}
		}
	})

	t.Run("Rule24 Registry Policies clamp and enforce directional check", func(t *testing.T) {
		reg := NewDefaultConfigRegistry()
		
		// 1. BTC_CHAOS check
		chaosPolicy, found := reg.GetMarketPolicy("BTC_CHAOS")
		if !found {
			t.Fatal("BTC_CHAOS policy not found in default registry")
		}
		
		// Ensure clamp/validate worked
		chaosPolicy = validateAndClampPolicy("BTC_CHAOS", chaosPolicy)
		if chaosPolicy.MaxPriceMove24hLong != 0.05 {
			t.Errorf("expected BTC_CHAOS MaxPriceMove24hLong = 0.05, got %f", chaosPolicy.MaxPriceMove24hLong)
		}

		qLong := baseQuant
		qLong.Direction = LONG
		qLong.TechnicalSnapshot = TechnicalSnapshot{
			IndicatorValues: map[string]float64{IndicatorADX: 25.0},
			PriceChange24h: -6.0, // -6% dump exceeds 5% chaos long limit
		}

		resLong := uc.Evaluate(qLong, baseLocalGate, baseAI, basePlanReview, baseStaleness, chaosPolicy, 50000, nil, nil, nil)
		if resLong.Status != FINAL_REJECT || !contains(resLong.Reason, "exceeds directional LONG limit") {
			t.Errorf("expected LONG to be rejected under BTC_CHAOS due to dump, got status %s (reason: %s)", resLong.Status, resLong.Reason)
		}

		// 2. RISK_OFF check
		roPolicy, found := reg.GetMarketPolicy("RISK_OFF")
		if !found {
			t.Fatal("RISK_OFF policy not found in default registry")
		}
		roPolicy = validateAndClampPolicy("RISK_OFF", roPolicy)
		if roPolicy.MaxPriceMove24hLong != 0.08 {
			t.Errorf("expected RISK_OFF MaxPriceMove24hLong = 0.08, got %f", roPolicy.MaxPriceMove24hLong)
		}

		qRo := baseQuant
		qRo.Direction = LONG
		qRo.TechnicalSnapshot = TechnicalSnapshot{
			IndicatorValues: map[string]float64{IndicatorADX: 25.0},
			PriceChange24h: -9.0, // -9% dump exceeds 8% limit
		}

		resRo := uc.Evaluate(qRo, baseLocalGate, baseAI, basePlanReview, baseStaleness, roPolicy, 50000, nil, nil, nil)
		if resRo.Status != FINAL_REJECT || !contains(resRo.Reason, "exceeds directional LONG limit") {
			t.Errorf("expected LONG to be rejected under RISK_OFF due to dump, got status %s (reason: %s)", resRo.Status, resRo.Reason)
		}
	})

	t.Run("Rule25 SL borderline case passes due to tolerance", func(t *testing.T) {
		pol := policy
		pol.Reason = "HIGH_VOL active - strict risk reduction mode" // regime high vol, requires 1.5x SL

		q := baseQuant
		q.TradePlan.EntryPrice = 100.0
		q.TradePlan.StopLoss = 93.928214  // SL distance = 6.071786
		q.TradePlan.TakeProfit = 110.0
		q.TechnicalSnapshot = TechnicalSnapshot{
			IndicatorValues: map[string]float64{
				IndicatorADX: 25.0,
				IndicatorATR: 4.047857, // 6.071786 / 4.047857 = 1.50000012
			},
		}

		res := uc.Evaluate(q, baseLocalGate, baseAI, basePlanReview, baseStaleness, pol, 100.0, nil, nil, nil)
		if contains(res.Reason, "SL distance") {
			t.Errorf("SL distance should pass with tolerance, but got rejected/watched: %s", res.Reason)
		}
	})
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
