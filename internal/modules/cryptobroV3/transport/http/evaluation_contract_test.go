package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"cpbro-engine/internal/modules/cryptobroV3/service"
	"cpbro-engine/internal/modules/cryptobroV3/usecase"
	"github.com/gin-gonic/gin"
)

func TestEvaluation_MissingReport_ReturnsEmptyReportValid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	st, _ := service.NewJSONStorageService(dir)
	h := &Handler{storageUC: usecase.NewStorageUsecase(st)}

	r := gin.New()
	r.GET("/evaluation", h.GetEvaluation)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/evaluation", nil)
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
	var ev map[string]any
	_ = json.Unmarshal(dataBytes, &ev)
	if ev["generated_at"] != "" {
		t.Fatalf("expected generated_at empty")
	}
	if ev["metrics"] == nil {
		t.Fatalf("expected metrics object")
	}
	for _, k := range []string{"playbook_stats", "regime_stats", "tier_stats", "direction_stats", "ai_stats", "staleness_stats", "conflict_stats", "cooldown_stats", "gate_bug_findings", "recommendations", "notes"} {
		if ev[k] == nil {
			t.Fatalf("expected %s not null", k)
		}
	}
}

func TestEvaluation_CorruptReport_Returns500(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "evaluation_report.json"), []byte("{"), 0644)

	st, _ := service.NewJSONStorageService(dir)
	h := &Handler{storageUC: usecase.NewStorageUsecase(st)}
	r := gin.New()
	r.GET("/evaluation", h.GetEvaluation)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/evaluation", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestEvaluationRun_EmptyStorage_ReturnsValidReport(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	st, _ := service.NewJSONStorageService(dir)
	storageUC := usecase.NewStorageUsecase(st)
	feedbackUC := usecase.NewFeedbackUsecase(storageUC)

	h := &Handler{storageUC: storageUC, feedbackUC: feedbackUC}
	r := gin.New()
	r.POST("/evaluation/run", h.PostEvaluationRun)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/evaluation/run", nil)
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
	var ev map[string]any
	_ = json.Unmarshal(dataBytes, &ev)
	for _, k := range []string{"playbook_stats", "regime_stats", "tier_stats", "direction_stats", "ai_stats", "staleness_stats", "conflict_stats", "cooldown_stats", "gate_bug_findings", "recommendations", "notes"} {
		if ev[k] == nil {
			t.Fatalf("expected %s not null", k)
		}
	}
}

func TestEvaluationRun_CorruptJournal_Returns500(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "signal_journal.json"), []byte("{"), 0644)

	st, _ := service.NewJSONStorageService(dir)
	storageUC := usecase.NewStorageUsecase(st)
	feedbackUC := usecase.NewFeedbackUsecase(storageUC)
	h := &Handler{storageUC: storageUC, feedbackUC: feedbackUC}

	r := gin.New()
	r.POST("/evaluation/run", h.PostEvaluationRun)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/evaluation/run", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}
