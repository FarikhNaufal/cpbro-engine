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
