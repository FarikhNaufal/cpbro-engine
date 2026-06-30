package usecase

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestScannerUsecase_ApplyLiveActualRRGuard(t *testing.T) {
	stalenessUC := NewStalenessUsecase(30 * time.Minute)
	stalenessUC.SetLatestPriceFeed(&mockLatestPriceFeed{
		prices: map[string]struct {
			price float64
			at    time.Time
			ok    bool
		}{
			"BTCUSDT": {price: 101, at: time.Now(), ok: true},
			"ETHUSDT": {price: 101.4, at: time.Now(), ok: true},
		},
	})

	uc := &ScannerUsecase{stalenessUsecase: stalenessUC}
	policy := MarketPolicy{MinRRExecute: 1.7}
	baseLocalGate := LocalGateResult{Passed: true, Status: AI_CANDIDATE, Reason: "All local quality gate criteria met successfully"}

	rejectRes := uc.applyLiveActualRRGuard(context.Background(), QuantResult{
		Symbol:    "BTCUSDT",
		Direction: LONG,
		Playbook:  TREND_PULLBACK,
		Tier:      TierA,
		TradePlan: TradePlan{
			EntryPrice: 100,
			StopLoss:   99,
			TakeProfit: 102,
		},
	}, policy, baseLocalGate)

	if rejectRes.Passed || rejectRes.Status != LOCAL_REJECT {
		t.Fatalf("expected LOCAL_REJECT, got passed=%v status=%s reason=%s", rejectRes.Passed, rejectRes.Status, rejectRes.Reason)
	}
	if !strings.Contains(rejectRes.Reason, "Actual RR 0.50 below minimum required RR 1.70") {
		t.Fatalf("unexpected reject reason: %s", rejectRes.Reason)
	}

	watchRes := uc.applyLiveActualRRGuard(context.Background(), QuantResult{
		Symbol:    "ETHUSDT",
		Direction: LONG,
		Playbook:  TREND_PULLBACK,
		Tier:      TierA,
		TradePlan: TradePlan{
			EntryPrice: 101.2,
			StopLoss:   100.9,
			TakeProfit: 102.15,
		},
	}, policy, baseLocalGate)

	if watchRes.Passed || watchRes.Status != LOCAL_WATCH {
		t.Fatalf("expected LOCAL_WATCH, got passed=%v status=%s reason=%s", watchRes.Passed, watchRes.Status, watchRes.Reason)
	}
	if !strings.Contains(watchRes.Reason, "above hard minimum 1.50") {
		t.Fatalf("unexpected watch reason: %s", watchRes.Reason)
	}
}
