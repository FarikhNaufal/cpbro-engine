package usecase

import (
	"testing"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
)

func TestNormalizeWatchSignalForFrontend_PreservesReasonsAndHotInfo(t *testing.T) {
	now := time.Now().UTC()
	in := dto.SignalResponse{
		Symbol:             "SOLUSDT",
		Direction:          "LONG",
		Timeframe:          "M15",
		TriggerPrice:       100,
		StopLoss:           98,
		TakeProfit:         105,
		Score:              8.1,
		Strategy:           "LIQUIDITY_SWEEP_REVERSAL",
		AISentiment:        "BULLISH",
		IsFinalExecute:     false,
		ReconciledTime:     now,
		Status:             "FINAL_WATCH",
		Reason:             "AI decision is WAIT",
		FinalReason:        "SOFT_PLAN_CONFLICT / NEED_RETEST",
		IsHot:              true,
		HotScore:           88,
		HotSource:          "Trending, Social Hype",
		HotRankType:        30,
		HotOverlaySelected: true,
	}

	out := NormalizeWatchSignalForFrontend(in)
	if out.Reason != in.Reason {
		t.Fatalf("expected reason %q, got %q", in.Reason, out.Reason)
	}
	if out.FinalReason != in.FinalReason {
		t.Fatalf("expected final reason %q, got %q", in.FinalReason, out.FinalReason)
	}
	if !out.IsHot || out.HotScore != in.HotScore || out.HotSource != in.HotSource || out.HotRankType != in.HotRankType || !out.HotOverlaySelected {
		t.Fatalf("expected hot metadata to be preserved, got %+v", out)
	}
}

func TestNormalizeSignalForFrontend_PreservesHotInfo(t *testing.T) {
	now := time.Now().UTC()
	in := dto.SignalResponse{
		Symbol:             "ETHUSDT",
		Direction:          "LONG",
		Timeframe:          "M15",
		TriggerPrice:       2500,
		StopLoss:           2450,
		TakeProfit:         2600,
		Score:              9.2,
		Strategy:           "COMPRESSION_BREAKOUT_RETEST",
		AISentiment:        "BULLISH",
		IsFinalExecute:     true,
		ReconciledTime:     now,
		Status:             "FINAL_EXECUTE",
		IsHot:              true,
		HotScore:           93,
		HotSource:          "Top Search",
		HotRankType:        11,
		HotOverlaySelected: true,
	}

	out := NormalizeSignalForFrontend(in)
	if !out.IsHot || out.HotScore != in.HotScore || out.HotSource != in.HotSource || out.HotRankType != in.HotRankType || !out.HotOverlaySelected {
		t.Fatalf("expected hot metadata to be preserved, got %+v", out)
	}
}

func TestBuildDecisionBrief(t *testing.T) {
	audit := DecisionAudit{
		Playbook:                LIQUIDITY_SWEEP_REVERSAL,
		FinalStatus:             FINAL_EXECUTE,
		FinalPrimaryReasonLayer: "AI_CONFIRM",
		AIDecision:              "CONFIRM",
		AIConfidence:            "HIGH",
		StalenessStatus:         "FRESH",
		M5ConfirmationUsed:      true,
		M5ConfirmationStatus:    "CONFIRMED",
		FinalReason:             "AI and local gates aligned",
	}

	brief := buildDecisionBrief(audit)
	expected := "FINAL_EXECUTE | LIQUIDITY_SWEEP_REVERSAL | layer=AI_CONFIRM | ai=CONFIRM/HIGH | stale=FRESH | m5=CONFIRMED | reason=AI and local gates aligned"
	if brief != expected {
		t.Fatalf("unexpected brief\nwant: %s\ngot:  %s", expected, brief)
	}
}
