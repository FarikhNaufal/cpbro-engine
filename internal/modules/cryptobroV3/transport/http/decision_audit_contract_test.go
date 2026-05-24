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

func TestDecisionAudit_MissingFile_ReturnsEmptyValid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	st, _ := service.NewJSONStorageService(dir)
	h := &Handler{storageUC: usecase.NewStorageUsecase(st)}

	r := gin.New()
	r.GET("/decision-audit", h.GetDecisionAudit)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/decision-audit", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp APIResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if !resp.Success {
		t.Fatalf("expected success=true")
	}
	dataBytes, _ := json.Marshal(resp.Data)
	var d map[string]any
	_ = json.Unmarshal(dataBytes, &d)

	if d["items"] == nil || d["filters"] == nil {
		t.Fatalf("expected items and filters not null")
	}
	if d["offset"] == nil {
		t.Fatalf("expected offset present")
	}
}

func TestDecisionAudit_FilterScanID_Works(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()

	now := time.Now().UTC()
	audits := []usecase.DecisionAudit{
		{ScanID: "A", Symbol: "BTCUSDT", GeneratedAt: now},
		{ScanID: "B", Symbol: "ETHUSDT", GeneratedAt: now},
	}
	b, _ := json.Marshal(audits)
	_ = os.WriteFile(filepath.Join(dir, "decision_audit.json"), b, 0644)

	st, _ := service.NewJSONStorageService(dir)
	h := &Handler{storageUC: usecase.NewStorageUsecase(st)}

	r := gin.New()
	r.GET("/decision-audit", h.GetDecisionAudit)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/decision-audit?scan_id=A", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp APIResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	dataBytes, _ := json.Marshal(resp.Data)
	var d map[string]any
	_ = json.Unmarshal(dataBytes, &d)
	items := d["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
}

func TestDecisionAudit_InvalidLimit_Returns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	st, _ := service.NewJSONStorageService(dir)
	h := &Handler{storageUC: usecase.NewStorageUsecase(st)}

	r := gin.New()
	r.GET("/decision-audit", h.GetDecisionAudit)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/decision-audit?limit=0", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestDecisionAudit_CorruptJSON_Returns500(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "decision_audit.json"), []byte("{"), 0644)

	st, _ := service.NewJSONStorageService(dir)
	h := &Handler{storageUC: usecase.NewStorageUsecase(st)}

	r := gin.New()
	r.GET("/decision-audit", h.GetDecisionAudit)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/decision-audit", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}
