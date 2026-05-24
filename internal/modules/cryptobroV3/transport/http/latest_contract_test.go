package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/entity"
	"cpbro-engine/internal/modules/cryptobroV3/service"
	"cpbro-engine/internal/modules/cryptobroV3/usecase"
	"github.com/gin-gonic/gin"
)

func TestLatest_FileMissing_ReturnsEmptyNormalized(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	st, err := service.NewJSONStorageService(dir)
	if err != nil {
		t.Fatalf("NewJSONStorageService: %v", err)
	}
	h := &Handler{storageUC: usecase.NewStorageUsecase(st)}

	r := gin.New()
	r.GET("/latest", h.GetLatest)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/latest", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp APIResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Success != true {
		t.Fatalf("expected success=true")
	}
	dataBytes, _ := json.Marshal(resp.Data)
	var latest map[string]any
	_ = json.Unmarshal(dataBytes, &latest)
	if latest["execute_signals"] == nil || latest["watchlist"] == nil || latest["signals"] == nil {
		t.Fatalf("expected arrays present")
	}
}

func TestLatest_NilArraysInFile_NormalizedToEmptySlices(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	st, err := service.NewJSONStorageService(dir)
	if err != nil {
		t.Fatalf("NewJSONStorageService: %v", err)
	}

	// Write a latest_result.json with explicit null arrays.
	raw := []byte(`{
  "scan_id":"20260524000000",
  "generated_at":"2026-05-24T00:00:00Z",
  "execute_signals": null,
  "watchlist": null,
  "signals": null,
  "rejected_summary": null,
  "policy_rejected_summary": null,
  "selected_threshold_profile_summary": null,
  "arbiter_selected_details": null
}`)
	if err := os.WriteFile(filepath.Join(dir, "latest_result.json"), raw, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	h := &Handler{storageUC: usecase.NewStorageUsecase(st)}
	r := gin.New()
	r.GET("/latest", h.GetLatest)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/latest", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp APIResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	dataBytes, _ := json.Marshal(resp.Data)
	var latest map[string]any
	_ = json.Unmarshal(dataBytes, &latest)

	for _, k := range []string{"execute_signals", "watchlist", "signals", "rejected_summary", "policy_rejected_summary", "arbiter_selected_details"} {
		v, ok := latest[k]
		if !ok {
			t.Fatalf("missing key %s", k)
		}
		if _, ok := v.([]any); !ok {
			t.Fatalf("expected %s to be array, got %T", k, v)
		}
	}
	if _, ok := latest["threshold_profile_summary"].(map[string]any); !ok {
		t.Fatalf("expected threshold_profile_summary to be object, got %T", latest["threshold_profile_summary"])
	}
}

func TestLatest_FinalWatch_WatchlistNotNull(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	st, err := service.NewJSONStorageService(dir)
	if err != nil {
		t.Fatalf("NewJSONStorageService: %v", err)
	}

	now := time.Now()
	_ = entity.LatestResult{}

	// Use raw JSON to avoid relying on dto.SignalResponse shape in this test.
	raw := []byte(`{
  "scan_id":"20260524001500",
  "generated_at":"` + now.UTC().Format(time.RFC3339) + `",
  "total_ai_wait": 1,
  "total_final_watch": 1,
  "watchlist": [
    {"symbol":"SOLUSDT","direction":"LONG","timeframe":"M15","trigger_price":100,"stop_loss":95,"take_profit":110,"score":7.5,"strategy":"TREND_PULLBACK","ai_sentiment":"NEUTRAL","is_final_execute":false,"status":"FINAL_WATCH"}
  ]
}`)
	if err := os.WriteFile(filepath.Join(dir, "latest_result.json"), raw, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	h := &Handler{storageUC: usecase.NewStorageUsecase(st)}
	r := gin.New()
	r.GET("/latest", h.GetLatest)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/latest", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp APIResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	dataBytes, _ := json.Marshal(resp.Data)
	var latest map[string]any
	_ = json.Unmarshal(dataBytes, &latest)
	wl := latest["watchlist"].([]any)
	if len(wl) == 0 {
		t.Fatalf("expected watchlist length > 0")
	}
}

func TestLatest_CorruptJSON_Returns500ErrorWrapper(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	st, err := service.NewJSONStorageService(dir)
	if err != nil {
		t.Fatalf("NewJSONStorageService: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "latest_result.json"), []byte("{"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	h := &Handler{storageUC: usecase.NewStorageUsecase(st)}
	r := gin.New()
	r.GET("/latest", h.GetLatest)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/latest", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
	var resp APIResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Success != false || resp.Data != nil || resp.Errors == nil {
		t.Fatalf("expected error wrapper with data=null and errors array")
	}
}

func TestLatest_PolicyRejectedSummary_Deduped(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	st, err := service.NewJSONStorageService(dir)
	if err != nil {
		t.Fatalf("NewJSONStorageService: %v", err)
	}
	raw := []byte(`{
  "scan_id":"20260524003000",
  "policy_rejected_summary":["SOLUSDT: LONG disabled","SOLUSDT: LONG disabled","SOLUSDT: SHORT disabled"]
}`)
	if err := os.WriteFile(filepath.Join(dir, "latest_result.json"), raw, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	h := &Handler{storageUC: usecase.NewStorageUsecase(st)}
	r := gin.New()
	r.GET("/latest", h.GetLatest)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/latest", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp APIResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	dataBytes, _ := json.Marshal(resp.Data)
	var latest map[string]any
	_ = json.Unmarshal(dataBytes, &latest)
	items := latest["policy_rejected_summary"].([]any)
	if len(items) != 2 {
		t.Fatalf("expected deduped length 2, got %d", len(items))
	}
}
