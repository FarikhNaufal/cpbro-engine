package usecase

import (
	"testing"

	"cpbro-engine/internal/modules/cryptobroV3/entity"
)

func TestFunnelSummaryAccumulator_BuildsSortedStageReasons(t *testing.T) {
	acc := newFunnelSummaryAccumulator()
	acc.Add(funnelStagePipelineDrop, "SOLUSDT: deferred by market data prefetch limit")
	acc.Add(funnelStagePipelineDrop, "ETHUSDT: deferred by market data prefetch limit")
	acc.Add(funnelStagePipelineDrop, "XRPUSDT: failed to fetch market data")
	acc.Add(funnelStageAIWait, "WAIT")
	acc.Add(funnelStageAIWait, "AI_SKIPPED")

	got := acc.Build()
	if len(got) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(got))
	}
	if got[0].Stage != funnelStagePipelineDrop {
		t.Fatalf("expected first stage %s, got %s", funnelStagePipelineDrop, got[0].Stage)
	}
	if got[0].Total != 3 {
		t.Fatalf("expected pipeline total 3, got %d", got[0].Total)
	}
	if len(got[0].Reasons) < 2 {
		t.Fatalf("expected at least 2 pipeline reasons, got %d", len(got[0].Reasons))
	}
	if got[0].Reasons[0].Reason != "Deferred by market data prefetch limit" || got[0].Reasons[0].Count != 2 {
		t.Fatalf("unexpected top pipeline reason: %+v", got[0].Reasons[0])
	}
	if got[1].Stage != funnelStageAIWait {
		t.Fatalf("expected second stage %s, got %s", funnelStageAIWait, got[1].Stage)
	}
	if got[1].Reasons[0].Reason != "AI decision WAIT" {
		t.Fatalf("expected WAIT normalization, got %+v", got[1].Reasons[0])
	}
}

func TestNormalizeFinalGateReason_CanonicalizesDynamicText(t *testing.T) {
	got := normalizeFunnelReason(funnelStageFinalReject, "Actual RR 1.27 below minimum required RR 1.80")
	if got != "Actual RR below minimum required RR" {
		t.Fatalf("unexpected normalization: %s", got)
	}
}

func TestBuildTopFunnelBlockersAndLogSummary(t *testing.T) {
	stages := []entity.FunnelStageSummary{
		{
			Stage: "eligibility_reject",
			Total: 12,
			Reasons: []entity.FunnelReasonCount{
				{Reason: "Disabled by REVERSAL_ONLY policy mode", Count: 7},
			},
		},
		{
			Stage: "final_watch",
			Total: 4,
			Reasons: []entity.FunnelReasonCount{
				{Reason: "AI confidence below required threshold", Count: 3},
			},
		},
	}

	blockers := buildTopFunnelBlockers(stages, 5)
	if len(blockers) != 2 {
		t.Fatalf("expected 2 blockers, got %d", len(blockers))
	}
	if blockers[0] != "eligibility_reject: Disabled by REVERSAL_ONLY policy mode (7)" {
		t.Fatalf("unexpected blocker text: %s", blockers[0])
	}

	summary := formatFunnelLogSummary(stages, 5)
	expected := "eligibility_reject=12[Disabled by REVERSAL_ONLY policy mode]; final_watch=4[AI confidence below required threshold]"
	if summary != expected {
		t.Fatalf("unexpected log summary: %s", summary)
	}
}

func TestPlaybookBlockerAccumulator_BuildsPerPlaybookStages(t *testing.T) {
	acc := newPlaybookBlockerAccumulator()
	acc.Add(LIQUIDITY_SWEEP_REVERSAL, funnelStageLocalWatch, "Risk-to-Reward ratio 1.60 is below policy requirement 1.80 but above hard minimum 1.50")
	acc.Add(LIQUIDITY_SWEEP_REVERSAL, funnelStageFinalWatch, "AI confidence MEDIUM is below required HIGH")
	acc.Add(LIQUIDITY_SWEEP_REVERSAL, funnelStageFinalWatch, "AI confidence HIGH is below required VERY_HIGH")
	acc.Add(COMPRESSION_BREAKOUT_RETEST, funnelStageEligibilityReject, "Retest required: entry on first breakout candle is forbidden")

	got := acc.Build()
	if len(got) != 2 {
		t.Fatalf("expected 2 playbook summaries, got %d", len(got))
	}
	if got[0].Playbook != string(LIQUIDITY_SWEEP_REVERSAL) {
		t.Fatalf("expected first summary for sweep, got %s", got[0].Playbook)
	}
	if got[0].Total != 3 {
		t.Fatalf("expected sweep total 3, got %d", got[0].Total)
	}
	if len(got[0].Stages) != 2 {
		t.Fatalf("expected 2 stages for sweep, got %d", len(got[0].Stages))
	}
	if got[0].Stages[0].Stage != funnelStageLocalWatch {
		t.Fatalf("expected local_watch first, got %s", got[0].Stages[0].Stage)
	}
	if got[0].Stages[1].Stage != funnelStageFinalWatch {
		t.Fatalf("expected final_watch second, got %s", got[0].Stages[1].Stage)
	}
	if got[0].Stages[1].Reasons[0].Reason != "AI confidence below required threshold" {
		t.Fatalf("expected normalized AI reason, got %s", got[0].Stages[1].Reasons[0].Reason)
	}
}
