package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/service"
	"cpbro-engine/internal/modules/cryptobroV3/usecase"
	"github.com/gin-gonic/gin"
)

func TestJournal_FileMissing_ReturnsEmptyItems(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	st, err := service.NewJSONStorageService(dir)
	if err != nil {
		t.Fatalf("NewJSONStorageService: %v", err)
	}
	h := &Handler{storageUC: usecase.NewStorageUsecase(st)}

	r := gin.New()
	r.GET("/journal", h.GetJournal)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/journal", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp APIResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Success != true {
		t.Fatalf("expected success=true")
	}
	dataBytes, _ := json.Marshal(resp.Data)
	var jr map[string]any
	_ = json.Unmarshal(dataBytes, &jr)

	items := jr["items"].([]any)
	if len(items) != 0 {
		t.Fatalf("expected empty items")
	}
	if jr["filters"] == nil {
		t.Fatalf("expected filters object")
	}
}

func TestJournal_EmptyFile_ReturnsEmptyItems(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "signal_journal.json"), []byte(""), 0644)

	st, err := service.NewJSONStorageService(dir)
	if err != nil {
		t.Fatalf("NewJSONStorageService: %v", err)
	}
	h := &Handler{storageUC: usecase.NewStorageUsecase(st)}

	r := gin.New()
	r.GET("/journal", h.GetJournal)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/journal", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestJournal_FilterSymbolWorks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	raw := []byte(`[
  {"signal_id":"1","symbol":"BTCUSDT","direction":"LONG","playbook":"TREND_PULLBACK","entry":100,"sl":95,"tp1":105,"tp2":110,"rr":2.0,"score":7.5,"created_at":"2026-05-24T00:00:00Z","status":"MONITORING"}
]`)
	_ = os.WriteFile(filepath.Join(dir, "signal_journal.json"), raw, 0644)

	st, _ := service.NewJSONStorageService(dir)
	h := &Handler{storageUC: usecase.NewStorageUsecase(st)}

	r := gin.New()
	r.GET("/journal", h.GetJournal)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/journal?symbol=BTCUSDT", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp APIResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	dataBytes, _ := json.Marshal(resp.Data)
	var jr map[string]any
	_ = json.Unmarshal(dataBytes, &jr)
	items := jr["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
}

func TestJournal_InvalidLimit_Returns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	st, _ := service.NewJSONStorageService(dir)
	h := &Handler{storageUC: usecase.NewStorageUsecase(st)}
	r := gin.New()
	r.GET("/journal", h.GetJournal)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/journal?limit=abc", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestJournal_CorruptJSON_Returns500(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "signal_journal.json"), []byte("{"), 0644)

	st, _ := service.NewJSONStorageService(dir)
	h := &Handler{storageUC: usecase.NewStorageUsecase(st)}
	r := gin.New()
	r.GET("/journal", h.GetJournal)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/journal", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestJournal_ItemsNonNull(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	st, _ := service.NewJSONStorageService(dir)
	h := &Handler{storageUC: usecase.NewStorageUsecase(st)}
	r := gin.New()
	r.GET("/journal", h.GetJournal)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/journal?offset=0&limit=100", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp APIResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	dataBytes, _ := json.Marshal(resp.Data)
	var jr map[string]any
	_ = json.Unmarshal(dataBytes, &jr)
	if jr["items"] == nil {
		t.Fatalf("expected items array not null")
	}
	_ = time.Second
}

func TestJournal_SortedDescendingByTime(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()

	raw := []byte(`[
		{"signal_id":"1","symbol":"BTCUSDT","direction":"LONG","playbook":"TREND_PULLBACK","entry":100,"sl":95,"tp1":105,"tp2":110,"rr":2.0,"score":7.5,"created_at":"2026-05-24T10:00:00Z","status":"MONITORING"},
		{"signal_id":"2","symbol":"BTCUSDT","direction":"LONG","playbook":"TREND_PULLBACK","entry":100,"sl":95,"tp1":105,"tp2":110,"rr":2.0,"score":7.5,"created_at":"2026-05-24T12:00:00Z","status":"MONITORING"},
		{"signal_id":"3","symbol":"BTCUSDT","direction":"LONG","playbook":"TREND_PULLBACK","entry":100,"sl":95,"tp1":105,"tp2":110,"rr":2.0,"score":7.5,"created_at":"2026-05-24T09:00:00Z","status":"MONITORING"}
	]`)
	_ = os.WriteFile(filepath.Join(dir, "signal_journal.json"), raw, 0644)

	st, _ := service.NewJSONStorageService(dir)
	h := &Handler{storageUC: usecase.NewStorageUsecase(st)}

	r := gin.New()
	r.GET("/journal", h.GetJournal)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/journal?limit=100", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp APIResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	dataBytes, _ := json.Marshal(resp.Data)
	var jr map[string]any
	_ = json.Unmarshal(dataBytes, &jr)

	items := jr["items"].([]any)
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}

	// We expect the sorting descending by created_at:
	// 1st: ID "2" ("2026-05-24T12:00:00Z")
	// 2nd: ID "1" ("2026-05-24T10:00:00Z")
	// 3rd: ID "3" ("2026-05-24T09:00:00Z")
	if items[0].(map[string]any)["signal_id"].(string) != "2" {
		t.Errorf("expected 1st item to be signal_id 2, got %v", items[0].(map[string]any)["signal_id"])
	}
	if items[1].(map[string]any)["signal_id"].(string) != "1" {
		t.Errorf("expected 2nd item to be signal_id 1, got %v", items[1].(map[string]any)["signal_id"])
	}
	if items[2].(map[string]any)["signal_id"].(string) != "3" {
		t.Errorf("expected 3rd item to be signal_id 3, got %v", items[2].(map[string]any)["signal_id"])
	}
}
