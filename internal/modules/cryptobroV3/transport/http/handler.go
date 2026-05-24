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
	scannerUC       scannerRunner
	feedbackUC      *usecase.FeedbackUsecase
	storageUC       *usecase.StorageUsecase
	backtestUC      *usecase.BacktestEngineUsecase
	observabilityUC *usecase.ObservabilityUsecase
	storageDir      string
	scannerRunning  *atomic.Bool
	startTime       time.Time
}

type scannerRunner interface {
	Run(ctx context.Context, req dto.ScanRequest) (dto.ScanResult, error)
}

// NewHandler initializes a Handler structure with all dependencies
func NewHandler(
	scannerUC scannerRunner,
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
		startTime:       time.Now(),
	}
}

func (h *Handler) mapHealthResponse(status usecase.HealthStatus) dto.HealthResponse {
	var lastScanStr, lastEvalStr string
	if !status.LastScanTime.IsZero() {
		lastScanStr = status.LastScanTime.Format(time.RFC3339)
	}
	if !status.LastEvaluationTime.IsZero() {
		lastEvalStr = status.LastEvaluationTime.Format(time.RFC3339)
	}

	appName := os.Getenv("APP_NAME")
	if appName == "" {
		appName = "cpbro-engine"
	}
	appVersion := os.Getenv("APP_VERSION")
	if appVersion == "" {
		appVersion = "3.0.0"
	}
	appEnv := os.Getenv("APP_ENV")
	if appEnv == "" {
		appEnv = "development"
	}

	swaggerEnabled := os.Getenv("SWAGGER_ENABLED") == "true"
	uptime := time.Since(h.startTime).Seconds()

	storageAvailable := status.StorageWritable == "OK" || status.StorageWritable == "OK (SKIPPED)"

	warnings := make([]string, 0)
	if status.BinanceConnectivity != "" && !strings.HasPrefix(status.BinanceConnectivity, "OK") {
		warnings = append(warnings, "binance_connectivity="+sanitizeErr(status.BinanceConnectivity))
	}
	if status.GeminiAvailability != "" && !strings.HasPrefix(status.GeminiAvailability, "OK") {
		warnings = append(warnings, "gemini_availability="+sanitizeErr(status.GeminiAvailability))
	}
	if status.TelegramAvailability != "" && !strings.HasPrefix(status.TelegramAvailability, "OK") && status.TelegramAvailability != "NOT_CONFIGURED" {
		warnings = append(warnings, "telegram_availability="+sanitizeErr(status.TelegramAvailability))
	}
	if !storageAvailable {
		warnings = append(warnings, "storage_writable="+sanitizeErr(status.StorageWritable))
	}

	healthStatus := "healthy"
	if len(warnings) > 0 {
		healthStatus = "degraded"
	}

	safeCfg := map[string]any{
		"binance_api_key_set":         strings.TrimSpace(os.Getenv("BINANCE_API_KEY")) != "",
		"binance_api_secret_set":      strings.TrimSpace(os.Getenv("BINANCE_API_SECRET")) != "",
		"gemini_api_key_set":          strings.TrimSpace(os.Getenv("GEMINI_API_KEY")) != "",
		"telegram_bot_token_set":      strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")) != "",
		"telegram_signal_chat_id_set": strings.TrimSpace(os.Getenv("TELEGRAM_SIGNAL_CHAT_ID")) != "" || strings.TrimSpace(os.Getenv("TELEGRAM_CHAT_ID")) != "",
		"telegram_status_chat_id_set": strings.TrimSpace(os.Getenv("TELEGRAM_STATUS_CHAT_ID")) != "",
	}

	scannerRunning := status.ScanWorkerRunning
	if h.scannerRunning != nil && h.scannerRunning.Load() {
		scannerRunning = true
	}

	return dto.HealthResponse{
		AppName:            appName,
		AppVersion:         appVersion,
		AppEnv:             appEnv,
		Mode:               status.Mode,
		AlertOnly:          status.Mode == "alert-only",
		BinanceReadOnly:    os.Getenv("BINANCE_READ_ONLY") != "false",
		ScannerRunning:     scannerRunning,
		LastScanTime:       lastScanStr,
		LastEvaluationTime: lastEvalStr,
		StorageAvailable:   storageAvailable,
		SwaggerEnabled:     swaggerEnabled,
		UptimeSeconds:      uptime,
		Status:             healthStatus,
		Warnings:           warnings,
		SafeConfig:         safeCfg,
	}
}

// GetHealth godoc
// @Summary      Health check
// @Description  Returns application health status, connectivity checks, and SRE metrics. Alert-only mode.
// @Tags         health
// @Produce      json
// @Success      200 {object} dto.HealthAPIResponse
// @Failure      500 {object} dto.ErrorAPIResponse
// @Router       /health [get]
func (h *Handler) GetHealth(c *gin.Context) {
	status, err := h.observabilityUC.PerformHealthAudit(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, fail("failed to perform health audit", err.Error()))
		return
	}
	c.JSON(http.StatusOK, ok("ok", h.mapHealthResponse(status)))
}

// GetLatest godoc
// @Summary      Get latest scanner result
// @Description  Returns latest cryptobroV3 scanner summary from storage/latest_result.json.
// @Tags         scanner
// @Produce      json
// @Success      200 {object} dto.LatestAPIResponse
// @Failure      500 {object} dto.ErrorAPIResponse
// @Router       /latest [get]
func (h *Handler) GetLatest(c *gin.Context) {
	res, err := h.storageUC.LoadLatestResult()
	if err != nil {
		c.JSON(http.StatusInternalServerError, fail("failed to read latest result", sanitizeErr(err.Error())))
		return
	}
	if res == nil || res.ScanID == "" {
		emptyNorm := usecase.NormalizeLatestResultForFrontend(nil)
		c.JSON(http.StatusOK, ok("latest result not found, returning empty summary", emptyNorm))
		return
	}

	c.JSON(http.StatusOK, ok("latest result retrieved successfully", usecase.NormalizeLatestResultForFrontend(res)))
}

// PostRun godoc
// @Summary      Run scanner manually
// @Description  Triggers AnalyzeMarketV3 manually. Does not execute Binance orders.
// @Tags         scanner
// @Produce      json
// @Success      200 {object} dto.LatestAPIResponse
// @Failure      409 {object} dto.ErrorAPIResponse
// @Failure      500 {object} dto.ErrorAPIResponse
// @Router       /run [post]
func (h *Handler) PostRun(c *gin.Context) {
	if h.scannerRunning != nil && !h.scannerRunning.CompareAndSwap(false, true) {
		c.JSON(http.StatusConflict, fail("scan already running", "scan already running"))
		return
	}
	if h.scannerRunning != nil {
		defer h.scannerRunning.Store(false)
	}

	defer func() {
		if r := recover(); r != nil {
			c.JSON(http.StatusInternalServerError, fail("scan panicked", "panic recovered"))
		}
	}()

	scanCtx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Minute)
	defer cancel()

	var req dto.ScanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req.TriggerTime = time.Now()
	}

	res, err := h.scannerUC.Run(scanCtx, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, fail("scan execution failed", sanitizeErr(err.Error())))
		return
	}

	latest, loadErr := h.storageUC.LoadLatestResult()
	if loadErr == nil && latest != nil && latest.ScanID != "" {
		norm := usecase.NormalizeLatestResultForFrontend(latest)
		c.JSON(http.StatusOK, ok("scan executed successfully", norm))
	} else {
		mockLatest := &entity.LatestResult{
			GeneratedAt: res.Timestamp,
			Duration:    res.Duration,
			Signals:     res.Signals,
		}
		norm := usecase.NormalizeLatestResultForFrontend(mockLatest)
		if loadErr != nil {
			norm.Warnings = append(norm.Warnings, "latest_result_load_failed")
			norm.PartialErrors = append(norm.PartialErrors, sanitizeErr(loadErr.Error()))
		}
		c.JSON(http.StatusOK, ok("scan executed successfully", norm))
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
// @Success      200 {object} dto.JournalAPIResponse
// @Failure      400 {object} dto.ErrorAPIResponse
// @Failure      500 {object} dto.ErrorAPIResponse
// @Router       /journal [get]
func (h *Handler) GetJournal(c *gin.Context) {
	journal, err := h.storageUC.LoadSignalJournal()
	if err != nil {
		c.JSON(http.StatusInternalServerError, fail("failed to read journal", sanitizeErr(err.Error())))
		return
	}

	filtered := []usecase.SignalJournal{}
	symbolFilter := c.Query("symbol")
	statusFilter := c.Query("status")
	playbookFilter := c.Query("playbook")
	directionFilter := c.Query("direction")

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
		if directionFilter != "" && !strings.EqualFold(string(item.Direction), directionFilter) {
			continue
		}
		filtered = append(filtered, item)
	}

	limitStr := c.Query("limit")
	limit := 100 // default limit
	if limitStr != "" {
		l, err := strconv.Atoi(limitStr)
		if err != nil || l <= 0 {
			c.JSON(http.StatusBadRequest, fail("invalid limit"))
			return
		}
		limit = l
	}
	if limit > 500 {
		limit = 500 // enforce max bounds
	}

	offsetStr := c.Query("offset")
	offset := 0
	if offsetStr != "" {
		o, err := strconv.Atoi(offsetStr)
		if err != nil || o < 0 {
			c.JSON(http.StatusBadRequest, fail("invalid offset"))
			return
		}
		offset = o
	}

	if offset > len(filtered) {
		offset = len(filtered)
	}
	end := offset + limit
	if end > len(filtered) {
		end = len(filtered)
	}

	resp := usecase.NormalizeJournalForFrontend(
		filtered[offset:end],
		limit,
		offset,
		dto.JournalFilters{
			Symbol:    symbolFilter,
			Playbook:  playbookFilter,
			Status:    statusFilter,
			Direction: directionFilter,
		},
	)

	c.JSON(http.StatusOK, ok("journal retrieved successfully", resp))
}

// GetEvaluation godoc
// @Summary      Get feedback evaluation report
// @Description  Reads evaluation_report.json. Does not auto-apply recommendations.
// @Tags         evaluation
// @Produce      json
// @Success      200 {object} dto.EvaluationAPIResponse
// @Failure      500 {object} dto.ErrorAPIResponse
// @Router       /evaluation [get]
func (h *Handler) GetEvaluation(c *gin.Context) {
	report, err := h.storageUC.LoadEvaluationReport()
	if err != nil {
		c.JSON(http.StatusInternalServerError, fail("failed to read evaluation report", sanitizeErr(err.Error())))
		return
	}
	if report == nil || report.GeneratedAt.IsZero() {
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
			DataCompleteness: completeness,
			Metrics:          map[string]float64{},
			PlaybookStats:    map[string]usecase.PlaybookStats{},
			RegimeStats:      map[string]usecase.RegimeStats{},
			TierStats:        map[string]usecase.TierStats{},
			DirectionStats:   map[string]usecase.DirectionStats{},
			AIStats:          map[string]usecase.AIStats{},
			StalenessStats:   map[string]usecase.StalenessStats{},
			ConflictStats:    map[string]int{},
			CooldownStats:    map[string]int{},
			GateBugFindings:  []string{},
			Recommendations:  []usecase.ThresholdRecommendation{},
		}

		c.JSON(http.StatusOK, ok("evaluation report not found, returning empty report", usecase.NormalizeEvaluationForFrontend(report)))
		return
	}

	c.JSON(http.StatusOK, ok("evaluation report retrieved successfully", usecase.NormalizeEvaluationForFrontend(report)))
}

// PostEvaluationRun godoc
// @Summary      Run feedback evaluation
// @Description  Runs Feedback Evaluation and writes evaluation_report.json. Does not auto-apply threshold or policy changes.
// @Tags         evaluation
// @Produce      json
// @Success      200 {object} dto.EvaluationAPIResponse
// @Failure      500 {object} dto.ErrorAPIResponse
// @Router       /evaluation/run [post]
func (h *Handler) PostEvaluationRun(c *gin.Context) {
	err := h.feedbackUC.GenerateEvaluationReport()
	if err != nil {
		c.JSON(http.StatusInternalServerError, fail("failed to generate evaluation report", err.Error()))
		return
	}

	report, loadErr := h.storageUC.LoadEvaluationReport()
	if loadErr != nil {
		c.JSON(http.StatusInternalServerError, fail("report generated but load failed", loadErr.Error()))
		return
	}

	c.JSON(http.StatusOK, ok("evaluation report generated successfully", usecase.NormalizeEvaluationForFrontend(report)))
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
// @Param        direction query string false "Filter by direction"
// @Param        limit query int false "Limit rows"
// @Param        offset query int false "Offset rows"
// @Success      200 {object} dto.DecisionAuditAPIResponse
// @Failure      400 {object} dto.ErrorAPIResponse
// @Failure      500 {object} dto.ErrorAPIResponse
// @Router       /decision-audit [get]
func (h *Handler) GetDecisionAudit(c *gin.Context) {
	audits, err := h.storageUC.LoadDecisionAudits()
	if err != nil {
		c.JSON(http.StatusInternalServerError, fail("failed to read decision audit", sanitizeErr(err.Error())))
		return
	}

	filtered := []usecase.DecisionAudit{}
	scanIDFilter := c.Query("scan_id")
	symbolFilter := c.Query("symbol")
	statusFilter := c.Query("final_status")
	playbookFilter := c.Query("playbook")
	directionFilter := c.Query("direction")

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
		if directionFilter != "" && !strings.EqualFold(string(item.Direction), directionFilter) {
			continue
		}
		filtered = append(filtered, item)
	}

	limit := 100
	if s := c.Query("limit"); s != "" {
		v, convErr := strconv.Atoi(s)
		if convErr != nil || v <= 0 || v > 1000 {
			c.JSON(http.StatusBadRequest, fail("invalid limit", "limit must be between 1 and 1000"))
			return
		}
		limit = v
	}
	offset := 0
	if s := c.Query("offset"); s != "" {
		v, convErr := strconv.Atoi(s)
		if convErr != nil || v < 0 {
			c.JSON(http.StatusBadRequest, fail("invalid offset", "offset must be >= 0"))
			return
		}
		offset = v
	}

	total := len(filtered)
	start := offset
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}
	paged := filtered[start:end]

	resp := usecase.NormalizeDecisionAuditForFrontend(
		paged,
		limit,
		offset,
		dto.DecisionAuditFilters{
			ScanID:      scanIDFilter,
			Symbol:      symbolFilter,
			FinalStatus: statusFilter,
			Playbook:    playbookFilter,
			Direction:   directionFilter,
		},
	)
	resp.Total = total

	c.JSON(http.StatusOK, ok("decision audit retrieved successfully", resp))
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
		c.JSON(http.StatusBadRequest, fail("invalid request payload", err.Error()))
		return
	}

	report, err := h.backtestUC.RunBacktest(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, fail("backtest run failed", err.Error()))
		return
	}

	c.JSON(http.StatusOK, ok("backtest run completed successfully", report))
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
		c.JSON(http.StatusOK, ok("no backtest reports found", []usecase.BacktestReportSummary{}))
		return
	}
	if err := json.Unmarshal(data, &reports); err != nil {
		c.JSON(http.StatusInternalServerError, fail("failed to parse backtest reports summary", err.Error()))
		return
	}

	c.JSON(http.StatusOK, ok("backtest reports retrieved successfully", reports))
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
		c.JSON(http.StatusNotFound, fail("backtest run report not found"))
		return
	}

	var report usecase.BacktestReport
	if err := json.Unmarshal(data, &report); err != nil {
		c.JSON(http.StatusInternalServerError, fail("failed to parse backtest run report", err.Error()))
		return
	}

	c.JSON(http.StatusOK, ok("backtest run report retrieved successfully", report))
}
