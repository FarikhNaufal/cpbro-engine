package usecase

import (
	"cpbro-engine/internal/modules/cryptobroV3/dto"
)

type PlanReconciliationUsecase struct{}

func NewPlanReconciliationUsecase() *PlanReconciliationUsecase {
	return &PlanReconciliationUsecase{}
}

// Reconcile aligns the quant calculations with AI sentiment recommendations according to 11 strict rules.
func (uc *PlanReconciliationUsecase) Reconcile(quant QuantResult, ai dto.AIAuditResponse) PlanReview {
	review := PlanReview{
		Conflicted:      false,
		EntryStillValid: true,
		NeedRetest:      false,
		Status:          PLAN_VALID,
		Reason:          "Plan reconciled successfully",
	}

	// Rule 1: AI decision = REJECT
	if ai.Decision == "REJECT" {
		review.Conflicted = true
		review.Status = PLAN_CONFLICT
		review.EntryStillValid = false
		review.Reason = "AI rejected setup"
	}

	// Rule 2: AI decision = WAIT
	if ai.Decision == "WAIT" {
		review.NeedRetest = true
		review.Status = PLAN_NEED_RETEST
		review.Reason = "AI decision is WAIT"
	}

	// Rule 3: AI confidence = LOW
	if ai.Confidence == "LOW" {
		review.Conflicted = true
		review.Status = PLAN_CONFLICT
		review.Reason = "AI confidence is LOW"
	}

	// Rule 4: AI confidence = MEDIUM
	if ai.Confidence == "MEDIUM" {
		review.NeedRetest = true
		review.Status = PLAN_NEED_RETEST
		review.Reason = "AI confidence is MEDIUM"
	}

	// Rule 5: AI entry_timing = LATE
	if ai.EntryTiming == "LATE" {
		review.NeedRetest = true
		review.EntryStillValid = false
		review.Status = PLAN_NEED_RETEST
		review.Reason = "AI entry timing LATE"
	}

	// Rule 6: AI entry_timing = MISSED
	if ai.EntryTiming == "MISSED" {
		review.EntryStillValid = false
		review.NeedRetest = false
		review.Status = PLAN_CONFLICT
		review.Conflicted = true
		review.Reason = "AI entry timing MISSED"
	}

	// Rule 7: AI conflict_with_bot = true
	if ai.ConflictWithBot {
		review.Conflicted = true
		review.Status = PLAN_CONFLICT
		review.Reason = "AI indicates conflict with bot direction"
	}

	// Rule 8: AI suggested_action check
	if ai.SuggestedAction == "WAIT_RETEST" {
		review.NeedRetest = true
		if review.Status != PLAN_CONFLICT {
			review.Status = PLAN_NEED_RETEST
			review.Reason = "AI suggested action WAIT_RETEST"
		}
	} else if ai.SuggestedAction == "REJECT" {
		review.Conflicted = true
		review.EntryStillValid = false
		review.Status = PLAN_CONFLICT
		review.Reason = "AI suggested action REJECT"
	} else if ai.SuggestedAction == "WATCH_ONLY" {
		review.NeedRetest = true
		if review.Status != PLAN_CONFLICT {
			review.Status = PLAN_NEED_RETEST
			review.Reason = "AI suggested action WATCH_ONLY"
		}
	}

	// Rule 9: Reversal playbook -> has_rejection wajib true, has_confirmation wajib true
	isReversal := quant.Playbook == LIQUIDITY_SWEEP_REVERSAL ||
		quant.Playbook == RANGE_EDGE_REVERSAL ||
		quant.Playbook == CROWDED_POSITIONING_SQUEEZE
	if isReversal {
		if !ai.HasRejection || !ai.HasConfirmation {
			review.NeedRetest = true
			review.Conflicted = true
			review.Status = PLAN_CONFLICT
			review.Reason = "Reversal playbook lacks rejection or confirmation"
		}
	}

	// Rule 10: Trend pullback -> last_5_candles_bias searah, candle_narrative tidak boleh EXHAUSTED
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
			review.Status = PLAN_CONFLICT
			review.Reason = "Trend pullback direction mismatch or candle is exhausted"
		}
	}

	// Rule 11: Breakout retest -> AI must see retest hold or continuation. Chase breakout -> NeedRetest true
	if quant.Playbook == COMPRESSION_BREAKOUT_RETEST {
		validRetest := ai.CandleNarrative == "CONTINUATION" || ai.CandleNarrative == "REJECTION" || ai.HasConfirmation
		if !validRetest {
			review.Conflicted = true
			review.Status = PLAN_CONFLICT
			review.Reason = "Breakout retest lacks valid retest hold or continuation narrative"
		}
		if ai.EntryTiming == "LATE" || ai.SuggestedAction == "EXECUTE_IF_NOT_STALE" {
			review.NeedRetest = true
			if review.Status != PLAN_CONFLICT {
				review.Status = PLAN_NEED_RETEST
			}
		}
	}

	// Rule 12: Range reversal -> AI must see rejection at edge. Continuation breaking range -> PlanConflict true
	if quant.Playbook == RANGE_EDGE_REVERSAL {
		if !ai.HasRejection {
			review.Conflicted = true
			review.Status = PLAN_CONFLICT
			review.Reason = "Range reversal lacks rejection candle"
		}
		if ai.CandleNarrative == "CONTINUATION" {
			review.Conflicted = true
			review.Status = PLAN_CONFLICT
			review.Reason = "Range reversal candle narrative suggests continuation breaking out of range"
		}
	}

	// Final Gate Check for PLAN_VALID: Only allow PLAN_VALID if all conditions are strictly satisfied
	if review.Status == PLAN_VALID {
		if ai.Decision != "CONFIRM" ||
			ai.Confidence != "HIGH" ||
			(ai.EntryTiming != "FRESH" && ai.EntryTiming != "ACCEPTABLE") ||
			ai.ConflictWithBot ||
			(ai.SuggestedAction == "WAIT_RETEST" || ai.SuggestedAction == "REJECT" || ai.SuggestedAction == "WATCH_ONLY") {

			// Downgrade to PLAN_NEED_RETEST or PLAN_CONFLICT
			if ai.Decision == "REJECT" || ai.SuggestedAction == "REJECT" || ai.ConflictWithBot {
				review.Status = PLAN_CONFLICT
				review.Conflicted = true
				review.EntryStillValid = false
				review.Reason = "Failed final validation checklist for PLAN_VALID: rejected or conflict detected"
			} else {
				review.Status = PLAN_NEED_RETEST
				review.NeedRetest = true
				review.Reason = "Failed final validation checklist for PLAN_VALID: waiting/retest needed"
			}
		}
	}

	return review
}
