package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
	"cpbro-engine/internal/modules/cryptobroV3/service"
	"cpbro-engine/internal/modules/cryptobroV3/usecase"
	"github.com/gin-gonic/gin"
)

type fakeScannerRunner struct {
	result dto.ScanResult
	err    error
}

func (f *fakeScannerRunner) Run(ctx context.Context, req dto.ScanRequest) (dto.ScanResult, error) {
	return f.result, f.err
}

func TestRun_Success_ReturnsLatestResultResponseShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	st, _ := service.NewJSONStorageService(dir)
	storageUC := usecase.NewStorageUsecase(st)

	var running atomic.Bool
	h := &Handler{
		scannerUC:      &fakeScannerRunner{result: dto.ScanResult{Timestamp: time.Now(), Duration: "1s", Signals: []dto.SignalResponse{}}},
		storageUC:      storageUC,
		scannerRunning: &running,
	}

	r := gin.New()
	r.POST("/run", h.PostRun)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/run", nil)
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
	for _, k := range []string{"execute_signals", "watchlist", "signals", "rejected_summary", "policy_rejected_summary", "arbiter_selected_details", "warnings", "partial_errors"} {
		if d[k] == nil {
			t.Fatalf("expected %s not null", k)
		}
	}
}

func TestRun_Overlap_Returns409(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	st, _ := service.NewJSONStorageService(dir)

	var running atomic.Bool
	running.Store(true)

	h := &Handler{
		scannerUC:      &fakeScannerRunner{},
		storageUC:      usecase.NewStorageUsecase(st),
		scannerRunning: &running,
	}

	r := gin.New()
	r.POST("/run", h.PostRun)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/run", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w.Code)
	}
}
