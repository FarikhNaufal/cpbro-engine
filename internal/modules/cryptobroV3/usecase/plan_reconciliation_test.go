package usecase

import (
	"testing"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
)

func TestPlanReconciliation_Rules(t *testing.T) {
	reconciler := NewPlanReconciliationUsecase()

	// Rule 1: AI decision REJECT -> Conflicted true, status LOCAL_REJECT
	t.Run("Rule 1 - AI decision REJECT", func(t *testing.T) {
		quant := QuantResult{Playbook: TREND_PULLBACK, Direction: LONG}
		ai := dto.AIAuditResponse{Decision: "REJECT", Confidence: "HIGH", Last5CandlesBias: "BULLISH"}
		review := reconciler.Reconcile(quant, ai)
		if !review.Conflicted {
			t.Errorf("Expected Conflicted to be true on REJECT")
		}
		if review.Status != LOCAL_REJECT {
			t.Errorf("Expected status to be LOCAL_REJECT, got %s", review.Status)
		}
	})

	// Rule 2: AI conflict_with_bot true -> Conflicted true, status LOCAL_REJECT
	t.Run("Rule 2 - AI conflict_with_bot", func(t *testing.T) {
		quant := QuantResult{Playbook: TREND_PULLBACK, Direction: LONG}
		ai := dto.AIAuditResponse{Decision: "CONFIRM", ConflictWithBot: true, Confidence: "HIGH", Last5CandlesBias: "BULLISH"}
		review := reconciler.Reconcile(quant, ai)
		if !review.Conflicted {
			t.Errorf("Expected Conflicted to be true on conflict_with_bot")
		}
	})

	// Rule 3: AI confidence LOW -> Conflicted true, status LOCAL_WATCH
	t.Run("Rule 3 - AI confidence LOW", func(t *testing.T) {
		quant := QuantResult{Playbook: TREND_PULLBACK, Direction: LONG}
		ai := dto.AIAuditResponse{Decision: "CONFIRM", Confidence: "LOW", Last5CandlesBias: "BULLISH"}
		review := reconciler.Reconcile(quant, ai)
		if !review.Conflicted {
			t.Errorf("Expected Conflicted to be true on LOW confidence")
		}
		if review.Status != LOCAL_WATCH {
			t.Errorf("Expected status to be LOCAL_WATCH, got %s", review.Status)
		}
	})

	// Rule 4: AI entry_timing MISSED -> EntryStillValid false, NeedRetest false, status MISSED
	t.Run("Rule 4 - AI entry_timing MISSED", func(t *testing.T) {
		quant := QuantResult{Playbook: TREND_PULLBACK, Direction: LONG}
		ai := dto.AIAuditResponse{Decision: "CONFIRM", Confidence: "HIGH", EntryTiming: "MISSED", Last5CandlesBias: "BULLISH"}
		review := reconciler.Reconcile(quant, ai)
		if review.EntryStillValid {
			t.Errorf("Expected EntryStillValid to be false")
		}
		if review.NeedRetest {
			t.Errorf("Expected NeedRetest to be false")
		}
		if review.Status != "MISSED" {
			t.Errorf("Expected status to be MISSED, got %s", review.Status)
		}
	})

	// Rule 5: AI entry_timing LATE -> EntryStillValid false, NeedRetest true, status LATE
	t.Run("Rule 5 - AI entry_timing LATE", func(t *testing.T) {
		quant := QuantResult{Playbook: TREND_PULLBACK, Direction: LONG}
		ai := dto.AIAuditResponse{Decision: "CONFIRM", Confidence: "HIGH", EntryTiming: "LATE", Last5CandlesBias: "BULLISH"}
		review := reconciler.Reconcile(quant, ai)
		if review.EntryStillValid {
			t.Errorf("Expected EntryStillValid to be false")
		}
		if !review.NeedRetest {
			t.Errorf("Expected NeedRetest to be true")
		}
		if review.Status != "LATE" {
			t.Errorf("Expected status to be LATE, got %s", review.Status)
		}
	})

	// Rule 6: AI suggested_action WAIT_RETEST -> NeedRetest true
	t.Run("Rule 6 - AI suggested_action WAIT_RETEST", func(t *testing.T) {
		quant := QuantResult{Playbook: TREND_PULLBACK, Direction: LONG}
		ai := dto.AIAuditResponse{Decision: "CONFIRM", Confidence: "HIGH", SuggestedAction: "WAIT_RETEST", Last5CandlesBias: "BULLISH"}
		review := reconciler.Reconcile(quant, ai)
		if !review.NeedRetest {
			t.Errorf("Expected NeedRetest to be true on WAIT_RETEST")
		}
	})

	// Rule 7: Reversal playbook missing rejection or confirmation
	t.Run("Rule 7 - Reversal playbook missing indicators", func(t *testing.T) {
		quant := QuantResult{Playbook: LIQUIDITY_SWEEP_REVERSAL, Direction: LONG}
		ai := dto.AIAuditResponse{Decision: "CONFIRM", Confidence: "HIGH", HasRejection: false, HasConfirmation: true}
		review := reconciler.Reconcile(quant, ai)
		if !review.Conflicted || !review.NeedRetest {
			t.Errorf("Expected Conflicted and NeedRetest on missing reversal indicators")
		}
	})

	// Rule 8: Trend pullback mismatched bias
	t.Run("Rule 8 - Trend pullback mismatched bias", func(t *testing.T) {
		quant := QuantResult{Playbook: TREND_PULLBACK, Direction: LONG}
		ai := dto.AIAuditResponse{Decision: "CONFIRM", Confidence: "HIGH", Last5CandlesBias: "BEARISH"}
		review := reconciler.Reconcile(quant, ai)
		if !review.Conflicted || !review.NeedRetest {
			t.Errorf("Expected Conflicted and NeedRetest on mismatched bias for TREND_PULLBACK")
		}
	})

	// Rule 8 (continued): Trend pullback exhausted narrative
	t.Run("Rule 8 - Trend pullback exhausted", func(t *testing.T) {
		quant := QuantResult{Playbook: TREND_PULLBACK, Direction: LONG}
		ai := dto.AIAuditResponse{Decision: "CONFIRM", Confidence: "HIGH", Last5CandlesBias: "BULLISH", CandleNarrative: "EXHAUSTED"}
		review := reconciler.Reconcile(quant, ai)
		if !review.Conflicted || !review.NeedRetest {
			t.Errorf("Expected Conflicted and NeedRetest on EXHAUSTED narrative")
		}
	})

	// Rule 9: Breakout retest lacks retest hold/continuation
	t.Run("Rule 9 - Breakout retest lacks validation", func(t *testing.T) {
		quant := QuantResult{Playbook: COMPRESSION_BREAKOUT_RETEST, Direction: LONG}
		ai := dto.AIAuditResponse{Decision: "CONFIRM", Confidence: "HIGH", CandleNarrative: "CHOP", HasConfirmation: false}
		review := reconciler.Reconcile(quant, ai)
		if !review.Conflicted {
			t.Errorf("Expected Conflicted to be true when breakout retest lacks validation")
		}
	})

	// Rule 10: Range reversal continuation breaking range
	t.Run("Rule 10 - Range reversal breaking range", func(t *testing.T) {
		quant := QuantResult{Playbook: RANGE_EDGE_REVERSAL, Direction: LONG}
		ai := dto.AIAuditResponse{Decision: "CONFIRM", Confidence: "HIGH", HasRejection: true, HasConfirmation: true, CandleNarrative: "CONTINUATION"}
		review := reconciler.Reconcile(quant, ai)
		if !review.Conflicted {
			t.Errorf("Expected Conflicted to be true when candle narrative indicates continuation through range edge")
		}
	})

	// Rule 11: Protect entry/SL/TP final (Unchanged values)
	t.Run("Rule 11 - Protect final price parameters", func(t *testing.T) {
		quant := QuantResult{
			Playbook:  TREND_PULLBACK,
			Direction: LONG,
			TradePlan: TradePlan{
				EntryPrice: 100.0,
				StopLoss:   95.0,
				TakeProfit: 110.0,
			},
		}
		// SuggestedStopLoss/SuggestedTakeProfit are present in response but must be ignored
		ai := dto.AIAuditResponse{
			Decision:            "CONFIRM",
			Confidence:          "HIGH",
			Last5CandlesBias:    "BULLISH",
			SuggestedStopLoss:   90.0,
			SuggestedTakeProfit: 120.0,
		}
		review := reconciler.Reconcile(quant, ai)
		if review.Conflicted {
			t.Errorf("Expected no conflict, got conflicted")
		}
		// In the caller/scanner logic, original quant.TradePlan entry/SL/TP is kept.
	})
}
