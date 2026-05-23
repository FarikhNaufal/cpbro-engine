package usecase

import (
	"cpbro-engine/internal/modules/cryptobroV3/dto"
)

type PlanReconciliationUsecase struct{}

func NewPlanReconciliationUsecase() *PlanReconciliationUsecase {
	return &PlanReconciliationUsecase{}
}

// Reconcile aligns the quant calculations with AI sentiment recommendations according to 11 strict rules.
func (uc *PlanReconciliationUsecase) Reconcile(quant QuantResult, ai dto.AIAuditResponse, policy MarketPolicy) PlanReview {
	review := PlanReview{
		Conflicted:      false,
		EntryStillValid: true,
		NeedRetest:      false,
		Status:          PLAN_VALID,
		Reason:          "Plan reconciled successfully",
	}

	// Rule 1: AI decision REJECT -> PlanConflict true
	if ai.Decision == "REJECT" {
		review.Conflicted = true
		review.Status = LOCAL_REJECT
		review.Reason = "AI rejected setup"
	}

	// Rule 2: AI conflict_with_bot true -> PlanConflict true
	if ai.ConflictWithBot {
		review.Conflicted = true
		review.Status = LOCAL_REJECT
		review.Reason = "AI indicates conflict with bot direction"
	}

	// Rule 3: AI confidence LOW -> PlanConflict true atau WATCH_ONLY
	if ai.Confidence == "LOW" {
		review.Conflicted = true
		review.Status = LOCAL_WATCH
		review.Reason = "AI confidence is LOW"
	}

	// Rule 4: AI entry_timing MISSED -> EntryStillValid false, NeedRetest false, status MISSED
	if ai.EntryTiming == "MISSED" {
		review.EntryStillValid = false
		review.NeedRetest = false
		review.Status = "MISSED"
		review.Reason = "AI entry timing MISSED"
	}

	// Rule 5: AI entry_timing LATE -> EntryStillValid false, NeedRetest true, status LATE
	if ai.EntryTiming == "LATE" {
		review.EntryStillValid = false
		review.NeedRetest = true
		review.Status = "LATE"
		review.Reason = "AI entry timing LATE"
	}

	// Rule 6: AI suggested_action WAIT_RETEST -> NeedRetest true
	if ai.SuggestedAction == "WAIT_RETEST" {
		review.NeedRetest = true
	}

	// Rule 7: Reversal playbook -> has_rejection wajib true, has_confirmation wajib true
	isReversal := quant.Playbook == LIQUIDITY_SWEEP_REVERSAL ||
		quant.Playbook == RANGE_EDGE_REVERSAL ||
		quant.Playbook == CROWDED_POSITIONING_SQUEEZE
	if isReversal {
		if !ai.HasRejection || !ai.HasConfirmation {
			review.NeedRetest = true
			review.Conflicted = true
			review.Reason = "Reversal playbook lacks rejection or confirmation"
		}
	}

	// Rule 8: Trend pullback -> last_5_candles_bias searah, candle_narrative tidak boleh EXHAUSTED
	if quant.Playbook == TREND_PULLBACK {
		directionMatches := false
		if quant.Direction == LONG && ai.Last5CandlesBias == "BULLISH" {
			directionMatches = true
		} else if quant.Direction == SHORT && ai.Last5CandlesBias == "BEARISH" {
			directionMatches = true
		}
		if !directionMatches || ai.CandleNarrative == "EXHAUSTED" {
			review.NeedRetest = true
			review.Conflicted = true
			review.Reason = "Trend pullback direction mismatch or candle is exhausted"
		}
	}

	// Rule 9: Breakout retest -> AI must see retest hold or continuation. Chase breakout -> NeedRetest true
	if quant.Playbook == COMPRESSION_BREAKOUT_RETEST {
		validRetest := ai.CandleNarrative == "CONTINUATION" || ai.CandleNarrative == "REJECTION" || ai.HasConfirmation
		if !validRetest {
			review.Conflicted = true
			review.Reason = "Breakout retest lacks valid retest hold or continuation narrative"
		}
		if ai.EntryTiming == "LATE" || ai.SuggestedAction == "EXECUTE_IF_NOT_STALE" {
			review.NeedRetest = true
		}
	}

	// Rule 10: Range reversal -> AI must see rejection at edge. Continuation breaking range -> PlanConflict true
	if quant.Playbook == RANGE_EDGE_REVERSAL {
		if !ai.HasRejection {
			review.Conflicted = true
			review.Reason = "Range reversal lacks rejection candle"
		}
		if ai.CandleNarrative == "CONTINUATION" {
			review.Conflicted = true
			review.Reason = "Range reversal candle narrative suggests continuation breaking out of range"
		}
	}

	// Rule 11: Protect entry/SL/TP final (Enforced by not adopting any AI level overrides)

	return review
}
