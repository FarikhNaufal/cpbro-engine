package service

import (
	"sync"
	"testing"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/usecase"
)

func TestJSONStorageService_AppendSignalJournal_Concurrent(t *testing.T) {
	dir := t.TempDir()
	st, err := NewJSONStorageService(dir)
	if err != nil {
		t.Fatalf("NewJSONStorageService failed: %v", err)
	}

	var wg sync.WaitGroup
	const n = 100
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = st.AppendSignalJournal(usecase.SignalJournal{
				ID:        "id-" + time.Unix(0, int64(i)).Format("150405.000000000"),
				Symbol:    "BTCUSDT",
				Playbook:  usecase.TREND_PULLBACK,
				Direction: usecase.LONG,
				Status:    usecase.MONITORING,
				CreatedAt: time.Now(),
			})
		}(i)
	}
	wg.Wait()

	journal, err := st.LoadSignalJournal()
	if err != nil {
		t.Fatalf("LoadSignalJournal failed: %v", err)
	}
	if len(journal) != n {
		t.Fatalf("expected %d entries, got %d", n, len(journal))
	}
}

func TestJSONStorageService_AppendDecisionAudit_Concurrent(t *testing.T) {
	dir := t.TempDir()
	st, err := NewJSONStorageService(dir)
	if err != nil {
		t.Fatalf("NewJSONStorageService failed: %v", err)
	}

	var wg sync.WaitGroup
	const n = 100
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = st.AppendDecisionAudit(usecase.DecisionAudit{
				Symbol:      "BTCUSDT",
				Playbook:    usecase.TREND_PULLBACK,
				FinalStatus: usecase.FINAL_WATCH,
				CreatedAt:   time.Now(),
			})
		}(i)
	}
	wg.Wait()

	audits, err := st.LoadDecisionAudits()
	if err != nil {
		t.Fatalf("LoadDecisionAudits failed: %v", err)
	}
	if len(audits) != n {
		t.Fatalf("expected %d entries, got %d", n, len(audits))
	}
}

func TestJSONStorageService_UpsertSignalJournalEntries_MergesByID(t *testing.T) {
	dir := t.TempDir()
	st, err := NewJSONStorageService(dir)
	if err != nil {
		t.Fatalf("NewJSONStorageService failed: %v", err)
	}

	initial := []usecase.SignalJournal{
		{
			ID:          "sig-1",
			Symbol:      "BTCUSDT",
			Direction:   usecase.LONG,
			Playbook:    usecase.TREND_PULLBACK,
			Status:      usecase.MONITORING,
			CreatedAt:   time.Now().UTC(),
			ExpiresAt:   time.Now().UTC().Add(2 * time.Hour),
			LatestPrice: 100.0,
		},
	}
	if err := st.SaveSignalJournal(initial); err != nil {
		t.Fatalf("SaveSignalJournal failed: %v", err)
	}

	updates := []usecase.SignalJournal{
		{
			ID:          "sig-1",
			Symbol:      "BTCUSDT",
			Direction:   usecase.LONG,
			Playbook:    usecase.TREND_PULLBACK,
			Status:      usecase.TP1_HIT,
			CreatedAt:   initial[0].CreatedAt,
			ExpiresAt:   initial[0].ExpiresAt,
			LatestPrice: 105.0,
		},
		{
			ID:        "sig-2",
			Symbol:    "ETHUSDT",
			Direction: usecase.SHORT,
			Playbook:  usecase.LIQUIDITY_SWEEP_REVERSAL,
			Status:    usecase.MONITORING,
			CreatedAt: time.Now().UTC(),
			ExpiresAt: time.Now().UTC().Add(2 * time.Hour),
		},
	}

	if err := st.UpsertSignalJournalEntries(updates); err != nil {
		t.Fatalf("UpsertSignalJournalEntries failed: %v", err)
	}

	journal, err := st.LoadSignalJournal()
	if err != nil {
		t.Fatalf("LoadSignalJournal failed: %v", err)
	}
	if len(journal) != 2 {
		t.Fatalf("expected 2 entries after upsert, got %d", len(journal))
	}
	if journal[0].ID != "sig-1" || journal[0].Status != usecase.TP1_HIT || journal[0].LatestPrice != 105.0 {
		t.Fatalf("expected sig-1 to be updated in place, got %+v", journal[0])
	}
	if journal[1].ID != "sig-2" {
		t.Fatalf("expected sig-2 to be appended, got %+v", journal[1])
	}
}

func TestJSONStorageService_AppendDecisionAudits_BatchesAtomically(t *testing.T) {
	dir := t.TempDir()
	st, err := NewJSONStorageService(dir)
	if err != nil {
		t.Fatalf("NewJSONStorageService failed: %v", err)
	}

	batch := []usecase.DecisionAudit{
		{Symbol: "BTCUSDT", Playbook: usecase.TREND_PULLBACK, FinalStatus: usecase.FINAL_WATCH, CreatedAt: time.Now().UTC()},
		{Symbol: "ETHUSDT", Playbook: usecase.LIQUIDITY_SWEEP_REVERSAL, FinalStatus: usecase.FINAL_EXECUTE, CreatedAt: time.Now().UTC()},
	}
	if err := st.AppendDecisionAudits(batch); err != nil {
		t.Fatalf("AppendDecisionAudits failed: %v", err)
	}

	audits, err := st.LoadDecisionAudits()
	if err != nil {
		t.Fatalf("LoadDecisionAudits failed: %v", err)
	}
	if len(audits) != 2 {
		t.Fatalf("expected 2 audits after batch append, got %d", len(audits))
	}
	if audits[0].Symbol != "BTCUSDT" || audits[1].Symbol != "ETHUSDT" {
		t.Fatalf("unexpected decision audits after batch append: %+v", audits)
	}
}
