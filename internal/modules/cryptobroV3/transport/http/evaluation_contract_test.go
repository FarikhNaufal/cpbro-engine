package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/entity"
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
	for _, k := range []string{"source_files_used", "freshness_markers", "playbook_stats", "regime_stats", "tier_stats", "direction_stats", "ai_stats", "staleness_stats", "long_regime_playbook_stats", "weak_long_setups", "conflict_stats", "cooldown_stats", "gate_bug_findings", "recommendations", "notes"} {
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
	for _, k := range []string{"source_files_used", "freshness_markers", "playbook_stats", "regime_stats", "tier_stats", "direction_stats", "ai_stats", "staleness_stats", "long_regime_playbook_stats", "weak_long_setups", "conflict_stats", "cooldown_stats", "gate_bug_findings", "recommendations", "notes"} {
		if ev[k] == nil {
			t.Fatalf("expected %s not null", k)
		}
	}
}

func TestEvaluation_ReportOlderThanLatestResult_AppendsStaleNote(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	st, _ := service.NewJSONStorageService(dir)
	storageUC := usecase.NewStorageUsecase(st)

	reportTime := time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Second)
	latestTime := reportTime.Add(90 * time.Minute)

	if err := storageUC.SaveEvaluationReport(usecase.EvaluationReport{
		GeneratedAt: reportTime,
		Notes:       "base note",
	}); err != nil {
		t.Fatalf("SaveEvaluationReport: %v", err)
	}
	if err := storageUC.SaveLatestResult(&entity.LatestResult{
		GeneratedAt: latestTime,
		ScanID:      "scan-newer",
	}); err != nil {
		t.Fatalf("SaveLatestResult: %v", err)
	}

	h := &Handler{storageUC: storageUC}
	r := gin.New()
	r.GET("/evaluation", h.GetEvaluation)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/evaluation", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp APIResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	dataBytes, _ := json.Marshal(resp.Data)
	var ev map[string]any
	if err := json.Unmarshal(dataBytes, &ev); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}

	notes, ok := ev["notes"].([]any)
	if !ok || len(notes) == 0 {
		t.Fatalf("expected notes array, got %T %v", ev["notes"], ev["notes"])
	}

	found := false
	for _, note := range notes {
		if s, _ := note.(string); strings.Contains(s, "older than latest_result") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected stale latest_result note, got %v", notes)
	}

	markers, ok := ev["freshness_markers"].(map[string]any)
	if !ok {
		t.Fatalf("expected freshness_markers object, got %T", ev["freshness_markers"])
	}
	if markers["latest_result"] == nil {
		t.Fatalf("expected latest_result freshness marker, got %v", markers)
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
