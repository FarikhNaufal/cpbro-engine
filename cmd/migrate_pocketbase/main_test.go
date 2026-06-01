package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/config"
	"cpbro-engine/internal/modules/cryptobroV3/service"
	"cpbro-engine/internal/modules/cryptobroV3/usecase"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestMigrateSignalJournal_UpsertsToPocketBase(t *testing.T) {
	var created int

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/collections/_superusers/auth-with-password":
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "tok"})
			return
		case r.Method == http.MethodPost && r.URL.Path == "/api/collections/signal_journals/records":
			if r.Header.Get("Authorization") != "Bearer tok" {
				http.Error(w, "unauth", http.StatusUnauthorized)
				return
			}
			created++
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "rec"})
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

	tmp := t.TempDir()
	jsonSt, err := service.NewJSONStorageService(tmp)
	if err != nil {
		t.Fatalf("NewJSONStorageService: %v", err)
	}
	cfg := &config.Config{Storage: config.StorageConfig{StoragePath: tmp, SignalJournalFile: "signal_journal.json"}}

	entry := []usecase.SignalJournal{
		{
			ID:         "sig_1",
			Symbol:     "BTCUSDT",
			Direction:  usecase.LONG,
			Playbook:   usecase.TREND_PULLBACK,
			EntryPrice: 100,
			StopLoss:   98,
			TP1:        105,
			Status:     usecase.MONITORING,
			CreatedAt:  time.Now().UTC(),
			ExpiresAt:  time.Now().UTC().Add(2 * time.Hour),
		},
	}
	if err := jsonSt.SaveSignalJournal(entry); err != nil {
		t.Fatalf("SaveSignalJournal: %v", err)
	}

	pbClient, err := service.NewPocketBaseClientWithHTTPClient("http://pb.local", httpClient, 2*time.Second, service.PocketBaseAuthModeSuperuser, "", "x", "y", 0)
	if err != nil {
		t.Fatalf("NewPocketBaseClientWithHTTPClient: %v", err)
	}
	pbStorage, err := service.NewPocketBaseStorageService(jsonSt, pbClient)
	if err != nil {
		t.Fatalf("NewPocketBaseStorageService: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := migrateSignalJournal(ctx, cfg, jsonSt, pbStorage, false); err != nil {
		t.Fatalf("migrateSignalJournal: %v", err)
	}
	if created != 1 {
		t.Fatalf("expected 1 created record, got %d", created)
	}
}

func TestMigrateEvaluationReport_CreatesOrPatches(t *testing.T) {
	var created int
	var patched int
	firstGet := true

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/collections/_superusers/auth-with-password":
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "tok"})
			return
		case r.Method == http.MethodGet && r.URL.Path == "/api/collections/evaluation_runs/records":
			// first time: no record; second time: record exists
			items := []any{}
			if !firstGet {
				items = append(items, map[string]any{"id": "ev1"})
			}
			firstGet = false
			_ = json.NewEncoder(w).Encode(map[string]any{"items": items})
			return
		case r.Method == http.MethodPost && r.URL.Path == "/api/collections/evaluation_runs/records":
			created++
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "ev1"})
			return
		case r.Method == http.MethodPatch && r.URL.Path == "/api/collections/evaluation_runs/records/ev1":
			patched++
			w.WriteHeader(http.StatusOK)
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

	tmp := t.TempDir()
	jsonSt, err := service.NewJSONStorageService(tmp)
	if err != nil {
		t.Fatalf("NewJSONStorageService: %v", err)
	}
	cfg := &config.Config{Storage: config.StorageConfig{StoragePath: tmp, EvaluationReportFile: "evaluation_report.json"}}

	report := &usecase.EvaluationReport{
		GeneratedAt:     time.Now().UTC(),
		TotalSignals:    1,
		Metrics:         map[string]float64{"win_rate": 50},
		PlaybookStats:   map[string]usecase.PlaybookStats{},
		RegimeStats:     map[string]usecase.RegimeStats{},
		TierStats:       map[string]usecase.TierStats{},
		DirectionStats:  map[string]usecase.DirectionStats{},
		AIStats:         map[string]usecase.AIStats{},
		StalenessStats:  map[string]usecase.StalenessStats{},
		ConflictStats:   map[string]int{},
		CooldownStats:   map[string]int{},
		GateBugFindings: []string{},
		Recommendations: []usecase.ThresholdRecommendation{},
	}
	if err := jsonSt.SaveEvaluationReport(report); err != nil {
		t.Fatalf("SaveEvaluationReport: %v", err)
	}

	pbClient, err := service.NewPocketBaseClientWithHTTPClient("http://pb.local", httpClient, 2*time.Second, service.PocketBaseAuthModeSuperuser, "", "x", "y", 0)
	if err != nil {
		t.Fatalf("NewPocketBaseClientWithHTTPClient: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := migrateEvaluationReport(ctx, cfg, jsonSt, pbClient, false); err != nil {
		t.Fatalf("migrateEvaluationReport: %v", err)
	}
	if created != 1 {
		t.Fatalf("expected 1 create, got %d", created)
	}

	// run again -> should PATCH
	if err := migrateEvaluationReport(ctx, cfg, jsonSt, pbClient, false); err != nil {
		t.Fatalf("migrateEvaluationReport 2: %v", err)
	}
	if patched != 1 {
		t.Fatalf("expected 1 patch, got %d", patched)
	}

	// ensure file exists as baseline
	if _, err := os.Stat(filepath.Join(tmp, "evaluation_report.json")); err != nil {
		t.Fatalf("expected evaluation_report.json exists: %v", err)
	}
}
