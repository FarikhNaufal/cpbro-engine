package service

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/usecase"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestPocketBaseStorageService_SignalJournalAppendAndLoad(t *testing.T) {
	var mu sync.Mutex
	var createdSignal map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/collections/_superusers/auth-with-password":
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "testtoken"})
			return
		case r.Method == http.MethodPost && r.URL.Path == "/api/collections/signal_journals/records":
			if got := r.Header.Get("Authorization"); got != "Bearer testtoken" {
				http.Error(w, "missing bearer", http.StatusUnauthorized)
				return
			}
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &createdSignal)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "rec1"})
			return
		case r.Method == http.MethodGet && r.URL.Path == "/api/collections/signal_journals/records":
			mu.Lock()
			defer mu.Unlock()
			items := []map[string]any{}
			if createdSignal != nil {
				createdSignal["id"] = "rec1"
				createdSignal["created_at"] = time.Now().UTC().Format(time.RFC3339Nano)
				items = append(items, createdSignal)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"page":       1,
				"perPage":    200,
				"totalItems": len(items),
				"totalPages": 1,
				"items":      items,
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	})

	httpClient := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, r)
			return rr.Result(), nil
		}),
	}

	tmpDir := t.TempDir()
	fallback, err := NewJSONStorageService(filepath.Join(tmpDir, "storage"))
	if err != nil {
		t.Fatalf("NewJSONStorageService: %v", err)
	}

	client, err := NewPocketBaseClientWithHTTPClient("http://pocketbase.local", httpClient, 2*time.Second, PocketBaseAuthModeSuperuser, "", "admin@example.com", "pass", 1)
	if err != nil {
		t.Fatalf("NewPocketBaseClient: %v", err)
	}

	st, err := NewPocketBaseStorageService(fallback, client)
	if err != nil {
		t.Fatalf("NewPocketBaseStorageService: %v", err)
	}

	entry := usecase.SignalJournal{
		ID:         "sig_1",
		Symbol:     "BTCUSDT",
		Direction:  usecase.LONG,
		Playbook:   usecase.TREND_PULLBACK,
		EntryPrice: 100,
		StopLoss:   98,
		TP1:        105,
		TP2:        110,
		RR:         2.5,
		Status:     usecase.MONITORING,
		CreatedAt:  time.Now().UTC(),
		ExpiresAt:  time.Now().UTC().Add(2 * time.Hour),
	}

	if err := st.AppendSignalJournal(entry); err != nil {
		t.Fatalf("AppendSignalJournal: %v", err)
	}

	journal, err := st.LoadSignalJournal()
	if err != nil {
		t.Fatalf("LoadSignalJournal: %v", err)
	}
	if len(journal) != 1 {
		t.Fatalf("expected 1 journal row, got %d", len(journal))
	}
	if journal[0].ID != "sig_1" || journal[0].Symbol != "BTCUSDT" {
		t.Fatalf("unexpected journal row: %+v", journal[0])
	}
}

func TestPocketBaseStorageService_SaveAndLoadEvaluationReport(t *testing.T) {
	var savedEval map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/admins/auth-with-password":
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "admintoken"})
			return
		case r.Method == http.MethodPost && r.URL.Path == "/api/collections/evaluation_runs/records":
			if got := r.Header.Get("Authorization"); got != "Bearer admintoken" {
				http.Error(w, "missing bearer", http.StatusUnauthorized)
				return
			}
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &savedEval)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "ev1"})
			return
		case r.Method == http.MethodGet && r.URL.Path == "/api/collections/evaluation_runs/records":
			items := []map[string]any{}
			if savedEval != nil {
				savedEval["id"] = "ev1"
				items = append(items, savedEval)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"page":       1,
				"perPage":    1,
				"totalItems": len(items),
				"totalPages": 1,
				"items":      items,
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	})

	httpClient := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, r)
			return rr.Result(), nil
		}),
	}

	tmpDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(tmpDir, "storage"), 0755)
	fallback, err := NewJSONStorageService(filepath.Join(tmpDir, "storage"))
	if err != nil {
		t.Fatalf("NewJSONStorageService: %v", err)
	}

	client, err := NewPocketBaseClientWithHTTPClient("http://pocketbase.local", httpClient, 2*time.Second, PocketBaseAuthModeAdmin, "", "admin@example.com", "pass", 1)
	if err != nil {
		t.Fatalf("NewPocketBaseClient: %v", err)
	}

	st, err := NewPocketBaseStorageService(fallback, client)
	if err != nil {
		t.Fatalf("NewPocketBaseStorageService: %v", err)
	}

	report := &usecase.EvaluationReport{
		GeneratedAt:  time.Now().UTC(),
		TotalSignals: 2,
		Metrics: map[string]float64{
			"win_rate": 50,
		},
		GateBugFindings: []string{},
		Recommendations: []usecase.ThresholdRecommendation{},
		PlaybookStats:   map[string]usecase.PlaybookStats{},
		RegimeStats:     map[string]usecase.RegimeStats{},
		TierStats:       map[string]usecase.TierStats{},
		DirectionStats:  map[string]usecase.DirectionStats{},
		AIStats:         map[string]usecase.AIStats{},
		StalenessStats:  map[string]usecase.StalenessStats{},
		ConflictStats:   map[string]int{},
		CooldownStats:   map[string]int{},
		SourceFilesUsed: []string{"signal_journal.json"},
	}

	if err := st.SaveEvaluationReport(report); err != nil {
		t.Fatalf("SaveEvaluationReport: %v", err)
	}
	if savedEval == nil {
		t.Fatalf("expected evaluation payload saved")
	}
	if v, _ := savedEval["evaluation_id"].(string); !strings.HasPrefix(v, "eval_") {
		t.Fatalf("expected evaluation_id prefix eval_, got %v", savedEval["evaluation_id"])
	}

	loaded, err := st.LoadEvaluationReport()
	if err != nil {
		t.Fatalf("LoadEvaluationReport: %v", err)
	}
	if loaded == nil {
		t.Fatalf("expected loaded report")
	}
	if loaded.TotalSignals != 2 {
		t.Fatalf("expected total_signals=2, got %d", loaded.TotalSignals)
	}
	if loaded.Metrics["win_rate"] != 50 {
		t.Fatalf("expected win_rate=50, got %v", loaded.Metrics["win_rate"])
	}
}
