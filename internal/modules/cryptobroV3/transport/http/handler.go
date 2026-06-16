package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
	"cpbro-engine/internal/modules/cryptobroV3/entity"
	"cpbro-engine/internal/modules/cryptobroV3/service"
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
	runtime         HandlerRuntimeConfig
}

type HandlerRuntimeConfig struct {
	AppName         string
	AppVersion      string
	AppEnv          string
	SwaggerEnabled  bool
	BinanceReadOnly bool
	LogFilePath     string
	SafeConfig      map[string]any
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
	configs ...HandlerRuntimeConfig,
) *Handler {
	runtimeCfg := HandlerRuntimeConfig{
		AppName:         "cpbro-engine",
		AppVersion:      "3.0.0",
		AppEnv:          "development",
		SwaggerEnabled:  false,
		BinanceReadOnly: true,
		LogFilePath:     "storage/logs/app.log",
		SafeConfig:      map[string]any{},
	}
	if len(configs) > 0 {
		runtimeCfg = configs[0]
		if strings.TrimSpace(runtimeCfg.AppName) == "" {
			runtimeCfg.AppName = "cpbro-engine"
		}
		if strings.TrimSpace(runtimeCfg.AppVersion) == "" {
			runtimeCfg.AppVersion = "3.0.0"
		}
		if strings.TrimSpace(runtimeCfg.AppEnv) == "" {
			runtimeCfg.AppEnv = "development"
		}
		if strings.TrimSpace(runtimeCfg.LogFilePath) == "" {
			runtimeCfg.LogFilePath = "storage/logs/app.log"
		}
		if runtimeCfg.SafeConfig == nil {
			runtimeCfg.SafeConfig = map[string]any{}
		}
	}
	return &Handler{
		scannerUC:       scannerUC,
		feedbackUC:      feedbackUC,
		storageUC:       storageUC,
		backtestUC:      backtestUC,
		observabilityUC: observabilityUC,
		storageDir:      storageDir,
		scannerRunning:  scannerRunning,
		startTime:       time.Now(),
		runtime:         runtimeCfg,
	}
}

func (h *Handler) mapHealthResponse(status usecase.HealthStatus) dto.HealthResponse {
	var lastScanStr, lastEvalStr string
	var websocketLastMessageStr string
	if !status.LastScanTime.IsZero() {
		lastScanStr = status.LastScanTime.Format(time.RFC3339)
	}
	if !status.LastEvaluationTime.IsZero() {
		lastEvalStr = status.LastEvaluationTime.Format(time.RFC3339)
	}
	if !status.RealtimePrice.LastMessageTime.IsZero() {
		websocketLastMessageStr = status.RealtimePrice.LastMessageTime.Format(time.RFC3339)
	}

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
	if status.RealtimePrice.Enabled && status.RealtimePrice.ActiveSymbols > 0 && !status.RealtimePrice.Connected {
		warnings = append(warnings, "binance_websocket=disconnected")
	}

	healthStatus := "healthy"
	if len(warnings) > 0 {
		healthStatus = "degraded"
	}

	scannerRunning := status.ScanWorkerRunning
	if h.scannerRunning != nil && h.scannerRunning.Load() {
		scannerRunning = true
	}

	return dto.HealthResponse{
		AppName:                  h.runtime.AppName,
		AppVersion:               h.runtime.AppVersion,
		AppEnv:                   h.runtime.AppEnv,
		Mode:                     status.Mode,
		AlertOnly:                status.Mode == "alert-only",
		BinanceReadOnly:          h.runtime.BinanceReadOnly,
		ScannerRunning:           scannerRunning,
		LastScanTime:             lastScanStr,
		LastEvaluationTime:       lastEvalStr,
		StorageAvailable:         storageAvailable,
		SwaggerEnabled:           h.runtime.SwaggerEnabled,
		UptimeSeconds:            uptime,
		Status:                   healthStatus,
		Warnings:                 warnings,
		WebsocketEnabled:         status.RealtimePrice.Enabled,
		WebsocketConnected:       status.RealtimePrice.Connected,
		WebsocketActiveSymbols:   status.RealtimePrice.ActiveSymbols,
		WebsocketLastMessageTime: websocketLastMessageStr,
		SafeConfig:               h.runtime.SafeConfig,
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

	// Dynamically enrich execute/watch statuses using their dedicated journals.
	if journal, err := h.storageUC.LoadSignalJournal(); err == nil {
		latestJournal := make(map[string]usecase.SignalJournal)
		for _, item := range journal {
			if _, exists := latestJournal[item.Symbol]; !exists {
				latestJournal[item.Symbol] = item
			}
		}
		for i, sig := range res.ExecuteSignals {
			if item, exists := latestJournal[sig.Symbol]; exists {
				diff := sig.ReconciledTime.Sub(item.CreatedAt)
				if diff < 0 {
					diff = -diff
				}
				if diff < 2*time.Hour {
					res.ExecuteSignals[i].Status = string(item.Status)
				}
			}
		}
	}
	if watchJournal, err := h.storageUC.LoadWatchJournal(); err == nil {
		latestWatchJournal := make(map[string]usecase.WatchJournal)
		for _, item := range watchJournal {
			if _, exists := latestWatchJournal[item.Symbol]; !exists {
				latestWatchJournal[item.Symbol] = item
			}
		}
		for i, sig := range res.Watchlist {
			if item, exists := latestWatchJournal[sig.Symbol]; exists {
				diff := sig.ReconciledTime.Sub(item.CreatedAt)
				if diff < 0 {
					diff = -diff
				}
				if diff < 2*time.Hour {
					res.Watchlist[i].Status = string(item.Status)
				}
			}
		}
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
		if playbookFilter != "" {
			matched := false
			parts := strings.Split(playbookFilter, ",")
			for _, p := range parts {
				if strings.EqualFold(string(item.Playbook), strings.TrimSpace(p)) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		if directionFilter != "" && !strings.EqualFold(string(item.Direction), directionFilter) {
			continue
		}
		filtered = append(filtered, item)
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})

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

// GetWatchJournal godoc
// @Summary      Get virtual watch signal journal
// @Description  Reads watch_journal.json. Contains virtual watchlist monitoring outcomes only.
// @Tags         journal
// @Produce      json
// @Param        symbol query string false "Filter by symbol"
// @Param        playbook query string false "Filter by playbook"
// @Param        status query string false "Filter by status"
// @Param        limit query int false "Limit rows"
// @Success      200 {object} dto.JournalAPIResponse
// @Failure      400 {object} dto.ErrorAPIResponse
// @Failure      500 {object} dto.ErrorAPIResponse
// @Router       /watch-journal [get]
func (h *Handler) GetWatchJournal(c *gin.Context) {
	journal, err := h.storageUC.LoadWatchJournal()
	if err != nil {
		c.JSON(http.StatusInternalServerError, fail("failed to read watch journal", sanitizeErr(err.Error())))
		return
	}

	filtered := []usecase.WatchJournal{}
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
		if playbookFilter != "" {
			matched := false
			parts := strings.Split(playbookFilter, ",")
			for _, p := range parts {
				if strings.EqualFold(string(item.Playbook), strings.TrimSpace(p)) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		if directionFilter != "" && !strings.EqualFold(string(item.Direction), directionFilter) {
			continue
		}
		filtered = append(filtered, item)
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})

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

	c.JSON(http.StatusOK, ok("watch journal retrieved successfully", resp))
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
		if playbookFilter != "" {
			matched := false
			parts := strings.Split(playbookFilter, ",")
			for _, p := range parts {
				if strings.EqualFold(string(item.Playbook), strings.TrimSpace(p)) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		if directionFilter != "" && !strings.EqualFold(string(item.Direction), directionFilter) {
			continue
		}
		filtered = append(filtered, item)
	}

	sort.Slice(filtered, func(i, j int) bool {
		tI := filtered[i].CreatedAt
		if tI.IsZero() {
			tI = filtered[i].GeneratedAt
		}
		tJ := filtered[j].CreatedAt
		if tJ.IsZero() {
			tJ = filtered[j].GeneratedAt
		}
		return tI.After(tJ)
	})

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

	sort.Slice(reports, func(i, j int) bool {
		return reports[i].GeneratedAt.After(reports[j].GeneratedAt)
	})

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

// LogEntry represents a single parsed structured JSON log line
type LogEntry struct {
	Time       string         `json:"time"`
	Level      string         `json:"level"`
	Message    string         `json:"msg"`
	ScanID     string         `json:"scan_id,omitempty"`
	Module     string         `json:"module,omitempty"`
	Tag        string         `json:"tag,omitempty"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

// LogFilters wraps query filters for filtering logs
type LogFilters struct {
	Level  string
	ScanID string
	Module string
	Search string
}

// Matches evaluates if a LogEntry matches the specified LogFilters
func (e *LogEntry) Matches(f LogFilters) bool {
	if f.Level != "" {
		matched := false
		parts := strings.Split(f.Level, ",")
		for _, p := range parts {
			if strings.EqualFold(e.Level, strings.TrimSpace(p)) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if f.ScanID != "" && e.ScanID != f.ScanID {
		return false
	}
	if f.Module != "" {
		matched := false
		parts := strings.Split(f.Module, ",")
		for _, p := range parts {
			trimmed := strings.TrimSpace(p)
			if strings.EqualFold(e.Module, trimmed) || strings.EqualFold(e.Tag, trimmed) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if f.Search != "" {
		sLower := strings.ToLower(f.Search)
		if strings.Contains(strings.ToLower(e.Message), sLower) {
			return true
		}
		for k, v := range e.Attributes {
			if strings.Contains(strings.ToLower(k), sLower) {
				return true
			}
			if strings.Contains(strings.ToLower(fmt.Sprintf("%v", v)), sLower) {
				return true
			}
		}
		return false
	}
	return true
}

// ParseLogJSON parses a JSON log line into a structured LogEntry
func ParseLogJSON(line string) (*LogEntry, error) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return nil, err
	}
	e := &LogEntry{Attributes: make(map[string]any)}
	for k, v := range raw {
		switch k {
		case "time":
			if s, ok := v.(string); ok {
				e.Time = s
			}
		case "level":
			if s, ok := v.(string); ok {
				e.Level = s
			}
		case "msg":
			if s, ok := v.(string); ok {
				e.Message = s
			}
		case "scan_id":
			if s, ok := v.(string); ok {
				e.ScanID = s
			}
		case "module":
			if s, ok := v.(string); ok {
				e.Module = s
			}
		case "tag":
			if s, ok := v.(string); ok {
				e.Tag = s
			}
		default:
			e.Attributes[k] = v
		}
	}
	if e.Module == "" && e.Tag != "" {
		e.Module = e.Tag
	} else if e.Tag == "" && e.Module != "" {
		e.Tag = e.Module
	}
	return e, nil
}

// GetLogs godoc
// @Summary      Get system logs
// @Description  Retrieves historical system logs or establishes a real-time SSE logs stream.
// @Tags         observability
// @Produce      json
// @Produce      text/event-stream
// @Param        stream query bool false "Establish real-time SSE stream"
// @Param        limit query int false "Maximum number of history logs (default 100)"
// @Param        offset query int false "Pagination offset for history logs"
// @Param        level query string false "Filter by level (INFO, WARN, ERROR, DEBUG)"
// @Param        scan_id query string false "Filter by specific scan ID"
// @Param        module query string false "Filter by specific module/tag (e.g. market, gemini, pocketbase, storage)"
// @Param        search query string false "Free text search in messages and attributes"
// @Success      200 {object} APIResponse
// @Router       /observability/logs [get]
func (h *Handler) GetLogs(c *gin.Context) {
	// Parse common filters
	filters := LogFilters{
		Level:  c.Query("level"),
		ScanID: c.Query("scan_id"),
		Module: c.Query("module"),
		Search: c.Query("search"),
	}

	// Resolve log file path
	logFilePath := h.runtime.LogFilePath

	// Check if streaming is requested
	stream := c.Query("stream") == "true" || c.GetHeader("Accept") == "text/event-stream"

	limit := 100
	if s := c.Query("limit"); s != "" {
		if val, err := strconv.Atoi(s); err == nil && val > 0 {
			limit = val
		}
	}

	if stream {
		c.Writer.Header().Set("Content-Type", "text/event-stream")
		c.Writer.Header().Set("Cache-Control", "no-cache")
		c.Writer.Header().Set("Connection", "keep-alive")
		c.Writer.Header().Set("Transfer-Encoding", "chunked")

		// 1. Send initial logs history (chronological order)
		lines, err := service.ReadLastNLines(logFilePath, limit)
		if err == nil {
			for _, line := range lines {
				entry, err := ParseLogJSON(line)
				if err == nil && entry.Matches(filters) {
					c.SSEvent("log", entry)
				}
			}
			c.Writer.Flush()
		}

		// 2. Subscribe to new logs
		logChan := service.GlobalLogBroadcaster.Subscribe()
		defer service.GlobalLogBroadcaster.Unsubscribe(logChan)

		// 3. Loop and push
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-c.Request.Context().Done():
				return
			case logStr, ok := <-logChan:
				if !ok {
					return
				}
				entry, err := ParseLogJSON(logStr)
				if err == nil && entry.Matches(filters) {
					c.SSEvent("log", entry)
					c.Writer.Flush()
				}
			case <-ticker.C:
				c.SSEvent("ping", "keep-alive")
				c.Writer.Flush()
			}
		}
	} else {
		// Non-streaming history fetch (reversed chronological order: newest first)
		offset := 0
		if s := c.Query("offset"); s != "" {
			if val, err := strconv.Atoi(s); err == nil && val >= 0 {
				offset = val
			}
		}

		// Read up to 1000 lines from log file
		lines, err := service.ReadLastNLines(logFilePath, 1000)
		if err != nil {
			c.JSON(http.StatusInternalServerError, fail("failed to read log file", err.Error()))
			return
		}

		var entries []*LogEntry
		for i := len(lines) - 1; i >= 0; i-- {
			entry, err := ParseLogJSON(lines[i])
			if err == nil && entry.Matches(filters) {
				entries = append(entries, entry)
			}
		}

		total := len(entries)
		start := offset
		if start > total {
			start = total
		}
		end := start + limit
		if end > total {
			end = total
		}

		paged := entries[start:end]

		c.JSON(http.StatusOK, ok("logs retrieved successfully", map[string]any{
			"items":  paged,
			"total":  total,
			"limit":  limit,
			"offset": offset,
		}))
	}
}
