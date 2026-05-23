package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
	"cpbro-engine/internal/modules/cryptobroV3/entity"
	"cpbro-engine/internal/modules/cryptobroV3/usecase"
	"github.com/gin-gonic/gin"
)

// Handler handles all HTTP requests for the cryptobroV3 module
type Handler struct {
	scannerUC       *usecase.ScannerUsecase
	feedbackUC      *usecase.FeedbackUsecase
	storageUC       *usecase.StorageUsecase
	backtestUC      *usecase.BacktestEngineUsecase
	observabilityUC *usecase.ObservabilityUsecase
	storageDir      string
	scannerRunning  *atomic.Bool
}

// NewHandler initializes a Handler structure with all dependencies
func NewHandler(
	scannerUC *usecase.ScannerUsecase,
	feedbackUC *usecase.FeedbackUsecase,
	storageUC *usecase.StorageUsecase,
	backtestUC *usecase.BacktestEngineUsecase,
	observabilityUC *usecase.ObservabilityUsecase,
	storageDir string,
	scannerRunning *atomic.Bool,
) *Handler {
	return &Handler{
		scannerUC:       scannerUC,
		feedbackUC:      feedbackUC,
		storageUC:       storageUC,
		backtestUC:      backtestUC,
		observabilityUC: observabilityUC,
		storageDir:      storageDir,
		scannerRunning:  scannerRunning,
	}
}

// GetHealth godoc
// @Summary      Health check
// @Description  Returns application health status, connectivity checks, and SRE metrics. Alert-only mode.
// @Tags         health
// @Produce      json
// @Success      200 {object} APIResponse
// @Failure      500 {object} APIResponse
// @Router       /health [get]
func (h *Handler) GetHealth(c *gin.Context) {
	status, err := h.observabilityUC.PerformHealthAudit(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{
			Success: false,
			Message: "failed to perform health audit",
			Errors:  []string{err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, status)
}

// GetLatest godoc
// @Summary      Get latest scanner result
// @Description  Returns latest cryptobroV3 scanner summary from storage/latest_result.json.
// @Tags         scanner
// @Produce      json
// @Success      200 {object} APIResponse
// @Failure      500 {object} APIResponse
// @Router       /latest [get]
func (h *Handler) GetLatest(c *gin.Context) {
	res, err := h.storageUC.LoadLatestResult()
	if err != nil || res == nil || res.ScanID == "" {
		c.JSON(http.StatusOK, APIResponse{
			Success: true,
			Message: "latest result not found, returning empty summary",
			Data:    entity.LatestResult{},
		})
		return
	}

	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Message: "latest result retrieved successfully",
		Data:    res,
	})
}

// PostRun godoc
// @Summary      Run scanner manually
// @Description  Triggers AnalyzeMarketV3 manually. Does not execute Binance orders.
// @Tags         scanner
// @Produce      json
// @Success      200 {object} APIResponse
// @Failure      409 {object} APIResponse
// @Failure      500 {object} APIResponse
// @Router       /run [post]
func (h *Handler) PostRun(c *gin.Context) {
	if !h.scannerRunning.CompareAndSwap(false, true) {
		c.JSON(http.StatusConflict, APIResponse{
			Success: false,
			Message: "another scan is currently running",
		})
		return
	}
	defer h.scannerRunning.Store(false)

	scanCtx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Minute)
	defer cancel()

	var req dto.ScanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req.TriggerTime = time.Now()
	}

	res, err := h.scannerUC.Run(scanCtx, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{
			Success: false,
			Message: "scan execution failed",
			Errors:  []string{err.Error()},
		})
		return
	}

	latest, loadErr := h.storageUC.LoadLatestResult()
	if loadErr == nil && latest != nil && latest.ScanID != "" {
		c.JSON(http.StatusOK, APIResponse{
			Success: true,
			Message: "scan executed successfully",
			Data:    latest,
		})
	} else {
		c.JSON(http.StatusOK, APIResponse{
			Success: true,
			Message: "scan executed successfully",
			Data:    res,
		})
	}
}

// GetJournal godoc
// @Summary      Get virtual signal journal
// @Description  Reads signal_journal.json. Contains virtual monitoring outcomes only.
// @Tags         journal
// @Produce      json
// @Param        symbol query string false "Filter by symbol"
// @Param        playbook query string false "Filter by playbook"
// @Param        status query string false "Filter by status"
// @Param        limit query int false "Limit rows"
// @Success      200 {object} APIResponse
// @Failure      400 {object} APIResponse
// @Failure      500 {object} APIResponse
// @Router       /journal [get]
func (h *Handler) GetJournal(c *gin.Context) {
	journal, err := h.storageUC.LoadSignalJournal()
	if err != nil {
		c.JSON(http.StatusOK, APIResponse{
			Success: true,
			Message: "journal not found, returning empty array",
			Data:    []usecase.SignalJournal{},
		})
		return
	}

	filtered := []usecase.SignalJournal{}
	symbolFilter := c.Query("symbol")
	statusFilter := c.Query("status")
	playbookFilter := c.Query("playbook")

	for _, item := range journal {
		if symbolFilter != "" && !strings.EqualFold(item.Symbol, symbolFilter) {
			continue
		}
		if statusFilter != "" && !strings.EqualFold(string(item.Status), statusFilter) {
			continue
		}
		if playbookFilter != "" && !strings.EqualFold(string(item.Playbook), playbookFilter) {
			continue
		}
		filtered = append(filtered, item)
	}

	limitStr := c.Query("limit")
	limit := 100 // default limit
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}
	if limit > 500 {
		limit = 500 // enforce max bounds
	}
	if limit > len(filtered) {
		limit = len(filtered)
	}

	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Message: "journal retrieved successfully",
		Data:    filtered[:limit],
	})
}

// GetEvaluation godoc
// @Summary      Get feedback evaluation report
// @Description  Reads evaluation_report.json. Does not auto-apply recommendations.
// @Tags         evaluation
// @Produce      json
// @Success      200 {object} APIResponse
// @Failure      500 {object} APIResponse
// @Router       /evaluation [get]
func (h *Handler) GetEvaluation(c *gin.Context) {
	report, err := h.storageUC.LoadEvaluationReport()
	if err != nil || report == nil || report.GeneratedAt.IsZero() {
		var completeness usecase.DataCompleteness

		journal, _ := h.storageUC.LoadSignalJournal()
		if len(journal) > 0 {
			completeness.HasSignalJournal = true
			completeness.CanEvaluateExecutedOutcome = true
		}
		latestRes, _ := h.storageUC.LoadLatestResult()
		if latestRes != nil && len(latestRes.Signals) > 0 {
			completeness.HasLatestResult = true
			completeness.CanEvaluateWatchMissedOpportunity = true
		}
		audits, _ := h.storageUC.LoadDecisionAudits()
		if len(audits) > 0 {
			completeness.HasDecisionAudit = true
			completeness.CanEvaluateAIWait = true
			completeness.CanEvaluateConflictDowngrade = true
		}

		report = &usecase.EvaluationReport{
			DataCompleteness:          completeness,
			Metrics:                   map[string]float64{},
			PlaybookStats:             map[string]usecase.PlaybookStats{},
			RegimeStats:               map[string]usecase.RegimeStats{},
			TierStats:                 map[string]usecase.TierStats{},
			DirectionStats:            map[string]usecase.DirectionStats{},
			AIStats:                   map[string]usecase.AIStats{},
			StalenessStats:            map[string]usecase.StalenessStats{},
			GateBugFindings:           []string{},
			Recommendations:           []usecase.ThresholdRecommendation{},
			BestPlaybook:              "",
			WorstPlaybook:             "",
			SetupYangSeringLangsungSL: "",
			SetupYangSeringExpired:    "",
			SetupYangSeringStale:      "",
			RegimeYangPalingBuruk:     "",
			TierYangPalingBuruk:       "",
			DirectionYangPalingBuruk:  "",
			PlaybookDenganMAETerbesar: "",
			PlaybookDenganExpiredRate: "",
			PlaybookDenganTP1Terbaik:  "",
			PlaybookDenganTP2Follow:   "",
		}
	}

	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Message: "evaluation report retrieved successfully",
		Data:    report,
	})
}

// PostEvaluationRun godoc
// @Summary      Run feedback evaluation
// @Description  Runs Feedback Evaluation and writes evaluation_report.json. Does not auto-apply threshold or policy changes.
// @Tags         evaluation
// @Produce      json
// @Success      200 {object} APIResponse
// @Failure      500 {object} APIResponse
// @Router       /evaluation/run [post]
func (h *Handler) PostEvaluationRun(c *gin.Context) {
	err := h.feedbackUC.GenerateEvaluationReport()
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{
			Success: false,
			Message: "failed to generate evaluation report",
			Errors:  []string{err.Error()},
		})
		return
	}

	report, loadErr := h.storageUC.LoadEvaluationReport()
	if loadErr != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{
			Success: false,
			Message: "report generated but load failed",
			Errors:  []string{loadErr.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Message: "evaluation report generated successfully",
		Data:    report,
	})
}

// GetDecisionAudit godoc
// @Summary      Get decision audit trail
// @Description  Reads decision_audit.json for audit/evaluation only. Not used for trade decisions.
// @Tags         audit
// @Produce      json
// @Param        scan_id query string false "Filter by scan id"
// @Param        symbol query string false "Filter by symbol"
// @Param        final_status query string false "Filter by final status"
// @Param        playbook query string false "Filter by playbook"
// @Param        limit query int false "Limit rows"
// @Success      200 {object} APIResponse
// @Failure      400 {object} APIResponse
// @Failure      500 {object} APIResponse
// @Router       /decision-audit [get]
func (h *Handler) GetDecisionAudit(c *gin.Context) {
	audits, err := h.storageUC.LoadDecisionAudits()
	if err != nil {
		c.JSON(http.StatusOK, APIResponse{
			Success: true,
			Message: "decision audits not found, returning empty array",
			Data:    []usecase.DecisionAudit{},
		})
		return
	}

	filtered := []usecase.DecisionAudit{}
	scanIDFilter := c.Query("scan_id")
	symbolFilter := c.Query("symbol")
	statusFilter := c.Query("final_status")
	playbookFilter := c.Query("playbook")

	for _, item := range audits {
		if scanIDFilter != "" && item.ScanID != scanIDFilter {
			continue
		}
		if symbolFilter != "" && !strings.EqualFold(item.Symbol, symbolFilter) {
			continue
		}
		if statusFilter != "" && !strings.EqualFold(string(item.FinalStatus), statusFilter) {
			continue
		}
		if playbookFilter != "" && !strings.EqualFold(string(item.Playbook), playbookFilter) {
			continue
		}
		filtered = append(filtered, item)
	}

	limitStr := c.Query("limit")
	limit := 100 // default limit
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}
	if limit > 500 {
		limit = 500 // enforce max bounds
	}
	if limit > len(filtered) {
		limit = len(filtered)
	}

	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Message: "decision audits retrieved successfully",
		Data:    filtered[:limit],
	})
}

// PostBacktestRun godoc
// @Summary      Run backtest simulation
// @Description  Runs a read-only backtest replay over historic candles. Does not execute real orders.
// @Tags         backtest
// @Accept       json
// @Produce      json
// @Success      200 {object} APIResponse
// @Failure      400 {object} APIResponse
// @Failure      500 {object} APIResponse
// @Router       /backtest/run [post]
func (h *Handler) PostBacktestRun(c *gin.Context) {
	var req usecase.BacktestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse{
			Success: false,
			Message: "invalid request payload",
			Errors:  []string{err.Error()},
		})
		return
	}

	report, err := h.backtestUC.RunBacktest(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{
			Success: false,
			Message: "backtest run failed",
			Errors:  []string{err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Message: "backtest run completed successfully",
		Data:    report,
	})
}

// GetBacktestReports godoc
// @Summary      List backtest reports
// @Description  Lists all historical backtest simulation reports from storage.
// @Tags         backtest
// @Produce      json
// @Success      200 {object} APIResponse
// @Failure      500 {object} APIResponse
// @Router       /backtest/reports [get]
func (h *Handler) GetBacktestReports(c *gin.Context) {
	summaryFile := filepath.Join(h.storageDir, "backtest_report.json")
	var reports []usecase.BacktestReportSummary
	data, err := os.ReadFile(summaryFile)
	if err != nil {
		c.JSON(http.StatusOK, APIResponse{
			Success: true,
			Message: "no backtest reports found",
			Data:    []usecase.BacktestReportSummary{},
		})
		return
	}
	if err := json.Unmarshal(data, &reports); err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{
			Success: false,
			Message: "failed to parse backtest reports summary",
			Errors:  []string{err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Message: "backtest reports retrieved successfully",
		Data:    reports,
	})
}

// GetBacktestReportByID godoc
// @Summary      Get backtest report by run ID
// @Description  Retrieves details for a single backtest run by its run_id.
// @Tags         backtest
// @Produce      json
// @Param        run_id path string true "Backtest Run ID"
// @Success      200 {object} APIResponse
// @Failure      404 {object} APIResponse
// @Failure      500 {object} APIResponse
// @Router       /backtest/reports/{run_id} [get]
func (h *Handler) GetBacktestReportByID(c *gin.Context) {
	runID := c.Param("run_id")
	runID = filepath.Base(runID) // prevent path traversal
	runFile := filepath.Join(h.storageDir, "backtest_runs", fmt.Sprintf("backtest_%s.json", runID))

	data, err := os.ReadFile(runFile)
	if err != nil {
		c.JSON(http.StatusNotFound, APIResponse{
			Success: false,
			Message: "backtest run report not found",
		})
		return
	}

	var report usecase.BacktestReport
	if err := json.Unmarshal(data, &report); err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{
			Success: false,
			Message: "failed to parse backtest run report",
			Errors:  []string{err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Message: "backtest run report retrieved successfully",
		Data:    report,
	})
}
