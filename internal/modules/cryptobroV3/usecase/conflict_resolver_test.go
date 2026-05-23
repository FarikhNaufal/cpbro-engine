package usecase_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
	"cpbro-engine/internal/modules/cryptobroV3/entity"
	"cpbro-engine/internal/modules/cryptobroV3/service"
	"cpbro-engine/internal/modules/cryptobroV3/usecase"
)

func TestConflictResolver_OppositeSignalConflict(t *testing.T) {
	uc := usecase.NewConflictResolverUsecase()

	policy := usecase.MarketPolicy{
		Reason:          "NORMAL",
		MaxFinalExecute: 3,
		AllowLong:       true,
		AllowShort:      true,
		MinScoreExecute: 7.0,
		MinRRExecute:    1.5,
	}

	// Case 1: Different AI confidence
	decisions := []usecase.FinalDecision{
		{
			Symbol:       "SOLUSDT",
			Direction:    usecase.LONG,
			Playbook:     usecase.TREND_PULLBACK,
			Status:       usecase.FINAL_EXECUTE,
			IsExecutable: true,
			Score:        8.0,
			RR:           2.0,
			AIConfidence: "HIGH",
		},
		{
			Symbol:       "SOLUSDT",
			Direction:    usecase.SHORT,
			Playbook:     usecase.LIQUIDITY_SWEEP_REVERSAL,
			Status:       usecase.FINAL_EXECUTE,
			IsExecutable: true,
			Score:        8.2,
			RR:           2.0,
			AIConfidence: "MEDIUM",
		},
	}

	resolved, _ := uc.ResolveConflicts(decisions, nil, nil, policy)
	// SOL LONG has HIGH confidence, so it must win. SOL SHORT (MEDIUM) must be watch.
	var longDec, shortDec usecase.FinalDecision
	for _, d := range resolved {
		if d.Direction == usecase.LONG {
			longDec = d
		} else {
			shortDec = d
		}
	}

	if longDec.Status != usecase.FINAL_EXECUTE || !longDec.IsExecutable {
		t.Errorf("expected SOL LONG to remain FINAL_EXECUTE, got %s", longDec.Status)
	}
	if shortDec.Status != usecase.FINAL_WATCH || shortDec.IsExecutable {
		t.Errorf("expected SOL SHORT to be downgraded to FINAL_WATCH, got %s", shortDec.Status)
	}
	if shortDec.WatchReason != "OPPOSITE_SIGNAL_CONFLICT" {
		t.Errorf("expected reason OPPOSITE_SIGNAL_CONFLICT, got %s", shortDec.WatchReason)
	}

	// Case 2: Same confidence, close score (diff < 0.5)
	decisions2 := []usecase.FinalDecision{
		{
			Symbol:       "SOLUSDT",
			Direction:    usecase.LONG,
			Playbook:     usecase.TREND_PULLBACK,
			Status:       usecase.FINAL_EXECUTE,
			IsExecutable: true,
			Score:        8.0,
			RR:           2.0,
			AIConfidence: "HIGH",
		},
		{
			Symbol:       "SOLUSDT",
			Direction:    usecase.SHORT,
			Playbook:     usecase.LIQUIDITY_SWEEP_REVERSAL,
			Status:       usecase.FINAL_EXECUTE,
			IsExecutable: true,
			Score:        8.2,
			RR:           2.0,
			AIConfidence: "HIGH",
		},
	}

	resolved2, _ := uc.ResolveConflicts(decisions2, nil, nil, policy)
	for _, d := range resolved2 {
		if d.Status != usecase.FINAL_WATCH || d.IsExecutable {
			t.Errorf("expected both to be downgraded to FINAL_WATCH, got %s for direction %s", d.Status, d.Direction)
		}
		if d.WatchReason != "DIRECTION_CONFLICT_SCORE_TOO_CLOSE" {
			t.Errorf("expected watch reason DIRECTION_CONFLICT_SCORE_TOO_CLOSE, got %s", d.WatchReason)
		}
	}
}

func TestConflictResolver_ActiveMonitoringExists(t *testing.T) {
	uc := usecase.NewConflictResolverUsecase()

	policy := usecase.MarketPolicy{
		Reason:          "NORMAL",
		MaxFinalExecute: 3,
		AllowLong:       true,
		MinScoreExecute: 7.0,
	}

	decisions := []usecase.FinalDecision{
		{
			Symbol:       "SOLUSDT",
			Direction:    usecase.LONG,
			Playbook:     usecase.TREND_PULLBACK,
			Status:       usecase.FINAL_EXECUTE,
			IsExecutable: true,
			Score:        8.0,
			RR:           2.0,
			AIConfidence: "HIGH",
		},
	}

	// Symbol SOLUSDT is already monitoring in journal
	active := []usecase.SignalJournal{
		{
			Symbol: "SOLUSDT",
			Status: usecase.MONITORING,
		},
	}

	resolved, _ := uc.ResolveConflicts(decisions, nil, active, policy)
	if resolved[0].Status != usecase.FINAL_WATCH || resolved[0].IsExecutable {
		t.Errorf("expected decision to be downgraded to FINAL_WATCH due to active monitoring, got %s", resolved[0].Status)
	}
	if resolved[0].WatchReason != "ACTIVE_MONITORING_EXISTS" {
		t.Errorf("expected watch reason ACTIVE_MONITORING_EXISTS, got %s", resolved[0].WatchReason)
	}
}

func TestConflictResolver_CooldownRules(t *testing.T) {
	uc := usecase.NewConflictResolverUsecase()

	// Dynamic cooldown mapping tests (using A grade score 7.5 to avoid S/S+ 2m override):
	// Low vol
	cooldownLow := uc.GetDynamicCooldownMinutes(7.5, usecase.MarketPolicy{Reason: "LOW_VOLATILITY"})
	if cooldownLow != 15 {
		t.Errorf("expected low vol cooldown to be 15, got %d", cooldownLow)
	}

	// High vol
	cooldownHigh := uc.GetDynamicCooldownMinutes(7.5, usecase.MarketPolicy{Reason: "HIGH_VOLATILITY"})
	if cooldownHigh != 5 {
		t.Errorf("expected high vol cooldown to be 5, got %d", cooldownHigh)
	}

	// S/S+ grade (score >= 7.8) is 2 mins
	cooldownS := uc.GetDynamicCooldownMinutes(8.0, usecase.MarketPolicy{Reason: "NORMAL"})
	if cooldownS != 2 {
		t.Errorf("expected S grade cooldown to be 2, got %d", cooldownS)
	}

	// BTCChaos forces minimum 10 mins
	cooldownChaos := uc.GetDynamicCooldownMinutes(8.5, usecase.MarketPolicy{Reason: "BTC_CHAOS"})
	if cooldownChaos != 10 {
		t.Errorf("expected BTCChaos cooldown to be 10, got %d", cooldownChaos)
	}

	// Duplicate price bucket check (within 1% price)
	history := []dto.SignalResponse{
		{
			Symbol:         "SOLUSDT",
			Direction:      "LONG",
			TriggerPrice:   100.0,
			Strategy:       string(usecase.TREND_PULLBACK),
			ReconciledTime: time.Now().Add(-1 * time.Minute),
		},
	}

	policy := usecase.MarketPolicy{
		Reason:          "NORMAL",
		MaxFinalExecute: 3,
		AllowLong:       true,
	}

	// Case A: Duplicate signal bucket (Entry price is 100.5, within 1% of 100.0)
	decisionsA := []usecase.FinalDecision{
		{
			Symbol:       "SOLUSDT",
			Direction:    usecase.LONG,
			Playbook:     usecase.TREND_PULLBACK,
			Status:       usecase.FINAL_EXECUTE,
			IsExecutable: true,
			Score:        7.5, // Grade A
			EntryPrice:   100.5,
			RR:           2.0,
		},
	}

	resolvedA, _ := uc.ResolveConflicts(decisionsA, history, nil, policy)
	if resolvedA[0].Status != usecase.FINAL_WATCH || resolvedA[0].WatchReason != "DUPLICATE_SIGNAL_BUCKET" {
		t.Errorf("expected DUPLICATE_SIGNAL_BUCKET downgrade, got status %s reason %s", resolvedA[0].Status, resolvedA[0].WatchReason)
	}

	// Case B: Symbol cooldown active (Entry price is 110.0, > 1% of 100.0)
	decisionsB := []usecase.FinalDecision{
		{
			Symbol:       "SOLUSDT",
			Direction:    usecase.LONG,
			Playbook:     usecase.TREND_PULLBACK,
			Status:       usecase.FINAL_EXECUTE,
			IsExecutable: true,
			Score:        7.5, // Grade A
			EntryPrice:   110.0,
			RR:           2.0,
		},
	}

	resolvedB, _ := uc.ResolveConflicts(decisionsB, history, nil, policy)
	if resolvedB[0].Status != usecase.FINAL_WATCH || resolvedB[0].WatchReason != "SYMBOL_COOLDOWN_ACTIVE" {
		t.Errorf("expected SYMBOL_COOLDOWN_ACTIVE downgrade, got status %s reason %s", resolvedB[0].Status, resolvedB[0].WatchReason)
	}
}

func TestConflictResolver_MaxConcurrentPruning(t *testing.T) {
	uc := usecase.NewConflictResolverUsecase()

	policy := usecase.MarketPolicy{
		Reason:          "NORMAL",
		MaxFinalExecute: 2,
	}

	decisions := []usecase.FinalDecision{
		{
			Symbol:       "SOLUSDT",
			Direction:    usecase.LONG,
			Playbook:     usecase.TREND_PULLBACK,
			Status:       usecase.FINAL_EXECUTE,
			IsExecutable: true,
			Score:        8.0,
			AIConfidence: "HIGH",
			Tier:         usecase.TierA,
		},
		{
			Symbol:       "AVAXUSDT",
			Direction:    usecase.LONG,
			Playbook:     usecase.TREND_PULLBACK,
			Status:       usecase.FINAL_EXECUTE,
			IsExecutable: true,
			Score:        8.5,
			AIConfidence: "HIGH",
			Tier:         usecase.TierA,
		},
		{
			Symbol:       "NEARUSDT",
			Direction:    usecase.LONG,
			Playbook:     usecase.TREND_PULLBACK,
			Status:       usecase.FINAL_EXECUTE,
			IsExecutable: true,
			Score:        7.5,
			AIConfidence: "MEDIUM",
			Tier:         usecase.TierA,
		},
	}

	resolved, _ := uc.ResolveConflicts(decisions, nil, nil, policy)

	// Since limit is 2, the highest sorted ones (AVAX, SOL) should stay execution. NEAR (lowest sorted) should be watch.
	var executed []string
	var watched []string

	for _, d := range resolved {
		if d.Status == usecase.FINAL_EXECUTE {
			executed = append(executed, d.Symbol)
		} else if d.Status == usecase.FINAL_WATCH {
			watched = append(watched, d.Symbol)
		}
	}

	if len(executed) != 2 {
		t.Errorf("expected 2 executed signals, got %d", len(executed))
	}
	if len(watched) != 1 {
		t.Errorf("expected 1 watched signal, got %d", len(watched))
	}
	if watched[0] != "NEARUSDT" {
		t.Errorf("expected NEARUSDT to be watched, got %s", watched[0])
	}
	if resolved[2].WatchReason != "MAX_FINAL_EXECUTE_LIMIT" {
		t.Errorf("expected watch reason MAX_FINAL_EXECUTE_LIMIT, got %s", resolved[2].WatchReason)
	}
}

func TestJSONStorage_AtomicWrite(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "storage_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	js, err := service.NewJSONStorageService(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	hist := &entity.SignalHistory{
		Signals: []dto.SignalResponse{
			{
				Symbol:    "BTCUSDT",
				Direction: "BUY",
			},
		},
	}

	if err := js.SaveSignalHistory(hist); err != nil {
		t.Fatalf("failed to save history: %v", err)
	}

	// Verify temp file does not exist after success
	tmpFile := filepath.Join(tmpDir, "signal_history.json.tmp")
	if _, err := os.Stat(tmpFile); !os.IsNotExist(err) {
		t.Errorf("expected temp file to be cleaned up, but it still exists")
	}

	loaded, err := js.LoadSignalHistory()
	if err != nil {
		t.Fatalf("failed to load history: %v", err)
	}

	if len(loaded.Signals) != 1 || loaded.Signals[0].Symbol != "BTCUSDT" {
		t.Errorf("loaded data mismatch")
	}
}
