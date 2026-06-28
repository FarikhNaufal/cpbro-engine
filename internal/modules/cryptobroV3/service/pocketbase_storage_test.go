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

func TestPocketBaseStorageService_LoadWatchJournal_PrefersNewerLocalTerminalState(t *testing.T) {
	now := time.Now().UTC()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/collections/_superusers/auth-with-password":
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "testtoken"})
			return
		case r.Method == http.MethodGet && r.URL.Path == "/api/collections/watch_journals/records":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"page":       1,
				"perPage":    200,
				"totalItems": 1,
				"totalPages": 1,
				"items": []map[string]any{
					{
						"id":         "pb-watch-1",
						"signal_id":  "watch_1",
						"symbol":     "SOLUSDT",
						"direction":  "LONG",
						"playbook":   "TREND_PULLBACK",
						"status":     "WATCH_MONITORING",
						"created_at": now.Add(-20 * time.Minute).Format(time.RFC3339Nano),
						"updated_at": now.Add(-15 * time.Minute).Format(time.RFC3339Nano),
						"expires_at": now.Add(45 * time.Minute).Format(time.RFC3339Nano),
					},
				},
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
	if err := fallback.SaveWatchJournal([]usecase.WatchJournal{
		{
			ID:        "watch_1",
			Symbol:    "SOLUSDT",
			Direction: usecase.LONG,
			Playbook:  usecase.TREND_PULLBACK,
			Status:    usecase.WATCH_EXPIRED,
			CreatedAt: now.Add(-20 * time.Minute),
			UpdatedAt: now.Add(-2 * time.Minute),
			ClosedAt:  now.Add(-2 * time.Minute),
			ExpiresAt: now.Add(-1 * time.Minute),
			Reason:    "local terminal state",
		},
	}); err != nil {
		t.Fatalf("SaveWatchJournal: %v", err)
	}

	client, err := NewPocketBaseClientWithHTTPClient("http://pocketbase.local", httpClient, 2*time.Second, PocketBaseAuthModeSuperuser, "", "admin@example.com", "pass", 1)
	if err != nil {
		t.Fatalf("NewPocketBaseClient: %v", err)
	}

	st, err := NewPocketBaseStorageService(fallback, client, "pocketbase_first")
	if err != nil {
		t.Fatalf("NewPocketBaseStorageService: %v", err)
	}

	journal, err := st.LoadWatchJournal()
	if err != nil {
		t.Fatalf("LoadWatchJournal: %v", err)
	}
	if len(journal) != 1 {
		t.Fatalf("expected 1 watch journal row, got %d", len(journal))
	}
	if journal[0].Status != usecase.WATCH_EXPIRED {
		t.Fatalf("expected local terminal status to win, got %+v", journal[0])
	}
	if journal[0].ClosedAt.IsZero() {
		t.Fatalf("expected merged watch journal to keep local close timestamp, got %+v", journal[0])
	}
}

func TestPocketBaseStorageService_LoadSignalJournal_PrefersLocalPolicySnapshot(t *testing.T) {
	now := time.Now().UTC()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/collections/_superusers/auth-with-password":
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "testtoken"})
			return
		case r.Method == http.MethodGet && r.URL.Path == "/api/collections/signal_journals/records":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"page":       1,
				"perPage":    200,
				"totalItems": 1,
				"totalPages": 1,
				"items": []map[string]any{
					{
						"id":                        "pb-signal-1",
						"signal_id":                 "sig_1",
						"symbol":                    "BTCUSDT",
						"direction":                 "LONG",
						"playbook":                  "TREND_PULLBACK",
						"status":                    "MONITORING",
						"policy_mode":               "NORMAL",
						"market_regime":             "ALT_SUPPORTIVE",
						"created_at":                now.Add(-20 * time.Minute).Format(time.RFC3339Nano),
						"updated_at":                now.Add(-10 * time.Minute).Format(time.RFC3339Nano),
						"expires_at":                now.Add(45 * time.Minute).Format(time.RFC3339Nano),
						"threshold_profile_summary": "remote row without snapshot",
					},
				},
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
	if err := fallback.SaveSignalJournal([]usecase.SignalJournal{
		{
			ID:                        "sig_1",
			Symbol:                    "BTCUSDT",
			Direction:                 usecase.LONG,
			Playbook:                  usecase.TREND_PULLBACK,
			Status:                    usecase.MONITORING,
			PolicyMode:                string(usecase.NORMAL),
			PolicyLongMode:            string(usecase.NORMAL),
			PolicyShortMode:           string(usecase.SWEEP_ONLY),
			PolicyRequireAIConfidence: string(usecase.AIConfidenceHigh),
			PolicyAllowedPlaybooks:    []string{string(usecase.TREND_PULLBACK), string(usecase.LIQUIDITY_SWEEP_REVERSAL)},
			PolicyReason:              "local snapshot should survive pb sync",
			CreatedAt:                 now.Add(-20 * time.Minute),
			UpdatedAt:                 now.Add(-25 * time.Minute),
			ExpiresAt:                 now.Add(45 * time.Minute),
		},
	}); err != nil {
		t.Fatalf("SaveSignalJournal: %v", err)
	}

	client, err := NewPocketBaseClientWithHTTPClient("http://pocketbase.local", httpClient, 2*time.Second, PocketBaseAuthModeSuperuser, "", "admin@example.com", "pass", 1)
	if err != nil {
		t.Fatalf("NewPocketBaseClient: %v", err)
	}

	st, err := NewPocketBaseStorageService(fallback, client, "pocketbase_first")
	if err != nil {
		t.Fatalf("NewPocketBaseStorageService: %v", err)
	}

	journal, err := st.LoadSignalJournal()
	if err != nil {
		t.Fatalf("LoadSignalJournal: %v", err)
	}
	if len(journal) != 1 {
		t.Fatalf("expected 1 signal journal row, got %d", len(journal))
	}
	if journal[0].PolicyLongMode != string(usecase.NORMAL) || journal[0].PolicyShortMode != string(usecase.SWEEP_ONLY) {
		t.Fatalf("expected local policy snapshot to win, got %+v", journal[0])
	}
	if journal[0].PolicyRequireAIConfidence != string(usecase.AIConfidenceHigh) {
		t.Fatalf("expected local effective AI requirement to survive, got %+v", journal[0])
	}
}

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
		ID:                        "sig_1",
		Symbol:                    "BTCUSDT",
		Direction:                 usecase.LONG,
		Playbook:                  usecase.TREND_PULLBACK,
		EntryPrice:                100,
		StopLoss:                  98,
		TP1:                       105,
		TP2:                       110,
		RR:                        2.5,
		Status:                    usecase.MONITORING,
		PolicyLongMode:            string(usecase.NORMAL),
		PolicyShortMode:           string(usecase.SWEEP_ONLY),
		PolicyRequireAIConfidence: string(usecase.AIConfidenceHigh),
		PolicyRequireFreshEntry:   true,
		PolicyAllowedPlaybooks:    []string{string(usecase.TREND_PULLBACK), string(usecase.LIQUIDITY_SWEEP_REVERSAL)},
		PolicyReason:              "snapshot payload check",
		CreatedAt:                 time.Now().UTC(),
		ExpiresAt:                 time.Now().UTC().Add(2 * time.Hour),
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
	if createdSignal["policy_long_mode"] != string(usecase.NORMAL) || createdSignal["policy_short_mode"] != string(usecase.SWEEP_ONLY) {
		t.Fatalf("expected PB payload to include policy mode snapshot, got %+v", createdSignal)
	}
	if createdSignal["policy_require_ai_confidence"] != string(usecase.AIConfidenceHigh) {
		t.Fatalf("expected PB payload to include effective AI confidence snapshot, got %+v", createdSignal)
	}
}

func TestPocketBaseStorageService_WatchJournalAppendAndLoad(t *testing.T) {
	var mu sync.Mutex
	var createdWatch map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/collections/_superusers/auth-with-password":
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "testtoken"})
			return
		case r.Method == http.MethodPost && r.URL.Path == "/api/collections/watch_journals/records":
			if got := r.Header.Get("Authorization"); got != "Bearer testtoken" {
				http.Error(w, "missing bearer", http.StatusUnauthorized)
				return
			}
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &createdWatch)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "watchrec1"})
			return
		case r.Method == http.MethodGet && r.URL.Path == "/api/collections/watch_journals/records":
			mu.Lock()
			defer mu.Unlock()
			items := []map[string]any{}
			if createdWatch != nil {
				createdWatch["id"] = "watchrec1"
				createdWatch["created_at"] = time.Now().UTC().Format(time.RFC3339Nano)
				items = append(items, createdWatch)
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

	entry := usecase.WatchJournal{
		ID:         "watch_1",
		Symbol:     "XLMUSDT",
		Direction:  usecase.LONG,
		Playbook:   usecase.LIQUIDITY_SWEEP_REVERSAL,
		EntryPrice: 0.229,
		StopLoss:   0.226,
		TP1:        0.233,
		TP2:        0.236,
		RR:         1.7,
		Status:     usecase.WATCH_MONITORING,
		CreatedAt:  time.Now().UTC(),
		ExpiresAt:  time.Now().UTC().Add(2 * time.Hour),
	}

	if err := st.AppendWatchJournal(entry); err != nil {
		t.Fatalf("AppendWatchJournal: %v", err)
	}

	journal, err := st.LoadWatchJournal()
	if err != nil {
		t.Fatalf("LoadWatchJournal: %v", err)
	}
	if len(journal) != 1 {
		t.Fatalf("expected 1 watch journal row, got %d", len(journal))
	}
	if journal[0].ID != "watch_1" || journal[0].Symbol != "XLMUSDT" {
		t.Fatalf("unexpected watch journal row: %+v", journal[0])
	}
}

func TestPocketBaseStorageService_FindWatchJournalCandidates_UsesFilteredLookup(t *testing.T) {
	var requestedFilter string
	var requestedSort string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/collections/_superusers/auth-with-password":
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "testtoken"})
			return
		case r.Method == http.MethodGet && r.URL.Path == "/api/collections/watch_journals/records":
			if got := r.Header.Get("Authorization"); got != "Bearer testtoken" {
				http.Error(w, "missing bearer", http.StatusUnauthorized)
				return
			}
			requestedFilter = r.URL.Query().Get("filter")
			requestedSort = r.URL.Query().Get("sort")
			if requestedFilter == "" {
				http.Error(w, "expected targeted filter", http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"page":       1,
				"perPage":    50,
				"totalItems": 1,
				"totalPages": 1,
				"items": []map[string]any{
					{
						"id":         "watchrec1",
						"signal_id":  "watch_1",
						"symbol":     "SOLUSDT",
						"direction":  "LONG",
						"playbook":   "LIQUIDITY_SWEEP_REVERSAL",
						"entry":      100.0,
						"sl":         98.0,
						"tp1":        103.0,
						"tp2":        105.0,
						"rr":         2.5,
						"status":     "WATCH_MONITORING",
						"created_at": time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339Nano),
						"updated_at": time.Now().UTC().Format(time.RFC3339Nano),
						"expires_at": time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339Nano),
					},
				},
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

	candidates, err := st.FindWatchJournalCandidates(usecase.WatchJournal{
		Symbol:    "SOLUSDT",
		Direction: usecase.LONG,
		Playbook:  usecase.LIQUIDITY_SWEEP_REVERSAL,
	})
	if err != nil {
		t.Fatalf("FindWatchJournalCandidates: %v", err)
	}

	if len(candidates) != 1 || candidates[0].ID != "watch_1" {
		t.Fatalf("unexpected candidates: %+v", candidates)
	}
	if !strings.Contains(requestedFilter, "symbol='SOLUSDT'") || !strings.Contains(requestedFilter, "direction='LONG'") || !strings.Contains(requestedFilter, "playbook='LIQUIDITY_SWEEP_REVERSAL'") {
		t.Fatalf("unexpected filter: %s", requestedFilter)
	}
	if requestedSort != "-updated_at,-created_at" {
		t.Fatalf("unexpected sort: %s", requestedSort)
	}
}

func TestPocketBaseStorageService_UpsertSignalJournalEntries_UsesTargetedLookup(t *testing.T) {
	var patchedPayload map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/collections/_superusers/auth-with-password":
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "testtoken"})
			return
		case r.Method == http.MethodGet && r.URL.Path == "/api/collections/signal_journals/records":
			if got := r.Header.Get("Authorization"); got != "Bearer testtoken" {
				http.Error(w, "missing bearer", http.StatusUnauthorized)
				return
			}
			if r.URL.Query().Get("filter") == "" {
				http.Error(w, "unexpected full collection scan", http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"page":       1,
				"perPage":    1,
				"totalItems": 1,
				"totalPages": 1,
				"items": []map[string]any{
					{"id": "rec-sig-1", "signal_id": "sig_1"},
				},
			})
			return
		case r.Method == http.MethodPatch && r.URL.Path == "/api/collections/signal_journals/records/rec-sig-1":
			if got := r.Header.Get("Authorization"); got != "Bearer testtoken" {
				http.Error(w, "missing bearer", http.StatusUnauthorized)
				return
			}
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &patchedPayload)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "rec-sig-1"})
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

	err = st.UpsertSignalJournalEntries([]usecase.SignalJournal{
		{
			ID:          "sig_1",
			Symbol:      "BTCUSDT",
			Direction:   usecase.LONG,
			Playbook:    usecase.TREND_PULLBACK,
			EntryPrice:  100,
			StopLoss:    98,
			TP1:         105,
			TP2:         110,
			RR:          2.5,
			Status:      usecase.TP1_HIT,
			CreatedAt:   time.Now().UTC(),
			ExpiresAt:   time.Now().UTC().Add(2 * time.Hour),
			LatestPrice: 105,
		},
	})
	if err != nil {
		t.Fatalf("UpsertSignalJournalEntries: %v", err)
	}

	if patchedPayload == nil {
		t.Fatal("expected PATCH payload to be sent")
	}
	if got, _ := patchedPayload["signal_id"].(string); got != "sig_1" {
		t.Fatalf("unexpected patched signal_id: %v", patchedPayload["signal_id"])
	}
}

func TestPocketBaseStorageService_LoadSignalJournal_LocalFirstModePrefersMirror(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			t.Fatalf("unexpected PocketBase request during local mirror load: %s %s", r.Method, r.URL.Path)
			return nil, nil
		}),
	}

	tmpDir := t.TempDir()
	fallback, err := NewJSONStorageService(filepath.Join(tmpDir, "storage"))
	if err != nil {
		t.Fatalf("NewJSONStorageService: %v", err)
	}

	localEntry := usecase.SignalJournal{
		ID:         "sig_local",
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
	if err := fallback.SaveSignalJournal([]usecase.SignalJournal{localEntry}); err != nil {
		t.Fatalf("SaveSignalJournal local mirror: %v", err)
	}

	client, err := NewPocketBaseClientWithHTTPClient("http://pocketbase.local", httpClient, 2*time.Second, PocketBaseAuthModeSuperuser, "", "admin@example.com", "pass", 1)
	if err != nil {
		t.Fatalf("NewPocketBaseClient: %v", err)
	}

	st, err := NewPocketBaseStorageService(fallback, client, "local_first")
	if err != nil {
		t.Fatalf("NewPocketBaseStorageService: %v", err)
	}

	journal, err := st.LoadSignalJournal()
	if err != nil {
		t.Fatalf("LoadSignalJournal: %v", err)
	}
	if len(journal) != 1 || journal[0].ID != "sig_local" {
		t.Fatalf("unexpected local mirror journal result: %+v", journal)
	}
}

func TestPocketBaseStorageService_LoadSignalJournal_PocketBaseFirstIgnoresLocalMirror(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/collections/_superusers/auth-with-password":
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "testtoken"})
			return
		case r.Method == http.MethodGet && r.URL.Path == "/api/collections/signal_journals/records":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"page":       1,
				"perPage":    200,
				"totalItems": 1,
				"totalPages": 1,
				"items": []map[string]any{
					{
						"id":         "rec-pb-1",
						"signal_id":  "sig_pb",
						"symbol":     "ETHUSDT",
						"direction":  "SHORT",
						"playbook":   "LIQUIDITY_SWEEP_REVERSAL",
						"entry":      100.0,
						"sl":         102.0,
						"tp1":        97.0,
						"tp2":        95.0,
						"rr":         2.0,
						"status":     "MONITORING",
						"created_at": time.Now().UTC().Format(time.RFC3339Nano),
						"expires_at": time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339Nano),
					},
				},
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
	if err := fallback.SaveSignalJournal([]usecase.SignalJournal{{
		ID:         "sig_local",
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
	}}); err != nil {
		t.Fatalf("SaveSignalJournal local mirror: %v", err)
	}

	client, err := NewPocketBaseClientWithHTTPClient("http://pocketbase.local", httpClient, 2*time.Second, PocketBaseAuthModeSuperuser, "", "admin@example.com", "pass", 1)
	if err != nil {
		t.Fatalf("NewPocketBaseClient: %v", err)
	}

	st, err := NewPocketBaseStorageService(fallback, client)
	if err != nil {
		t.Fatalf("NewPocketBaseStorageService: %v", err)
	}

	journal, err := st.LoadSignalJournal()
	if err != nil {
		t.Fatalf("LoadSignalJournal: %v", err)
	}
	if len(journal) != 1 || journal[0].ID != "sig_pb" {
		t.Fatalf("expected PocketBase-first row, got %+v", journal)
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
		LongRegimePlaybookStats: []usecase.SetupDiagnosticStats{
			{
				Direction:    string(usecase.LONG),
				MarketRegime: string(usecase.CHOP_RANGE),
				Playbook:     string(usecase.TREND_PULLBACK),
				TotalSignals: 4,
				WinRate:      25,
				SLRate:       75,
			},
		},
		WeakLongSetups: []usecase.SetupDiagnosticStats{
			{
				Direction:    string(usecase.LONG),
				MarketRegime: string(usecase.CHOP_RANGE),
				Playbook:     string(usecase.TREND_PULLBACK),
				TotalSignals: 4,
				WinRate:      25,
				SLRate:       75,
			},
		},
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
	if len(loaded.LongRegimePlaybookStats) != 1 || loaded.LongRegimePlaybookStats[0].Playbook != string(usecase.TREND_PULLBACK) {
		t.Fatalf("expected long diagnostics roundtrip, got %+v", loaded.LongRegimePlaybookStats)
	}
	if len(loaded.WeakLongSetups) != 1 || loaded.WeakLongSetups[0].MarketRegime != string(usecase.CHOP_RANGE) {
		t.Fatalf("expected weak long setups roundtrip, got %+v", loaded.WeakLongSetups)
	}
}

func TestPocketBaseStorageService_FallbackWhenPocketBaseUnavailable(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			rr := httptest.NewRecorder()
			http.Error(rr, "pocketbase down", http.StatusInternalServerError)
			return rr.Result(), nil
		}),
	}

	tmpDir := t.TempDir()
	fallback, err := NewJSONStorageService(filepath.Join(tmpDir, "storage"))
	if err != nil {
		t.Fatalf("NewJSONStorageService: %v", err)
	}
	client, err := NewPocketBaseClientWithHTTPClient("http://pocketbase.local", httpClient, 2*time.Second, PocketBaseAuthModeToken, "static-token", "", "", 0)
	if err != nil {
		t.Fatalf("NewPocketBaseClient: %v", err)
	}
	st, err := NewPocketBaseStorageService(fallback, client)
	if err != nil {
		t.Fatalf("NewPocketBaseStorageService: %v", err)
	}

	entry := usecase.SignalJournal{
		ID:         "sig_fallback",
		Symbol:     "ETHUSDT",
		Direction:  usecase.SHORT,
		Playbook:   usecase.LIQUIDITY_SWEEP_REVERSAL,
		EntryPrice: 100,
		StopLoss:   102,
		TP1:        97,
		TP2:        95,
		RR:         2,
		Status:     usecase.MONITORING,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}

	if err := st.AppendSignalJournal(entry); err != nil {
		t.Fatalf("AppendSignalJournal should fall back to JSON, got: %v", err)
	}
	journal, err := st.LoadSignalJournal()
	if err != nil {
		t.Fatalf("LoadSignalJournal should fall back to JSON, got: %v", err)
	}
	if len(journal) != 1 || journal[0].ID != entry.ID {
		t.Fatalf("expected fallback journal row, got %+v", journal)
	}

	report := &usecase.EvaluationReport{
		GeneratedAt:     time.Now().UTC(),
		TotalSignals:    1,
		Metrics:         map[string]float64{"win_rate": 100},
		GateBugFindings: []string{},
		Recommendations: []usecase.ThresholdRecommendation{},
	}
	if err := st.SaveEvaluationReport(report); err != nil {
		t.Fatalf("SaveEvaluationReport should fall back to JSON, got: %v", err)
	}
	loadedReport, err := st.LoadEvaluationReport()
	if err != nil {
		t.Fatalf("LoadEvaluationReport should fall back to JSON, got: %v", err)
	}
	if loadedReport == nil || loadedReport.TotalSignals != 1 {
		t.Fatalf("expected fallback evaluation report, got %+v", loadedReport)
	}
}
