package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/config"
	"cpbro-engine/internal/modules/cryptobroV3/dto"
	"cpbro-engine/internal/modules/cryptobroV3/service"
	transhttp "cpbro-engine/internal/modules/cryptobroV3/transport/http"
	"cpbro-engine/internal/modules/cryptobroV3/usecase"
)

var (
	scannerRunning atomic.Bool
)

const (
	defaultWorkerBoundaryBufferSeconds   = 3
	defaultMonitoringTimeoutFloorSeconds = 25
	defaultRecheckBoundaryMinutes        = 5
)

type watchRecheckTickDecision struct {
	Boundary            time.Time
	ShouldRun           bool
	SkipReason          string
	NextPrimaryBoundary time.Time
}

func notificationTime(t time.Time) string {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		loc = time.Local
	}
	return t.In(loc).Format("2006-01-02 15:04:05 MST")
}

// @title           cryptobroV3 API
// @version         0.1.0
// @description     cryptobroV3 is an alert-only crypto scanner, AI candle auditor, virtual monitoring, and feedback evaluation API.
// @description     This API is read-only / alert-only. It does not provide Binance order execution.
//

// @contact.name   cryptobroV3 Maintainer

// @license.name  Private

// @host      localhost:8080
// @BasePath  /api/v3
// @schemes   http

func main() {
	slog.Info("Starting Cryptobro V3 Engine...")

	// 1. Load Configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Initialize Custom Structured Logger
	if err := service.InitLogger(cfg); err != nil {
		slog.Error("failed to initialize logger", "error", err)
		os.Exit(1)
	}

	slog.Info("Configuration loaded successfully", "env", cfg.App.Env, "version", cfg.App.Version)

	usecase.SetRuntimeSettings(usecase.BuildRuntimeSettings(cfg))

	// 2. Initialize Storage from config
	jsonStorage, err := service.NewJSONStorageServiceWithFiles(cfg.Storage.StoragePath, service.JSONStorageFiles{
		LatestResultFile:     cfg.Storage.LatestResultFile,
		SignalHistoryFile:    cfg.Storage.SignalHistoryFile,
		SignalJournalFile:    cfg.Storage.SignalJournalFile,
		WatchJournalFile:     cfg.Storage.WatchJournalFile,
		AIAuditCacheFile:     cfg.Storage.AIAuditCacheFile,
		EvaluationReportFile: cfg.Storage.EvaluationReportFile,
		DecisionAuditFile:    cfg.Storage.DecisionAuditFile,
	})
	if err != nil {
		slog.Error("failed to initialize json storage", "error", err)
		os.Exit(1)
	}
	var storageRepo usecase.StorageRepository = jsonStorage

	if cfg.PocketBase.Enabled {
		timeout := time.Duration(cfg.PocketBase.RequestTimeoutSeconds) * time.Second
		var pbClient *service.PocketBaseClient
		retryMax := cfg.PocketBase.LoginRetryMax
		switch {
		case cfg.PocketBase.Token != "":
			pbClient, err = service.NewPocketBaseClientWithHTTPClient(cfg.PocketBase.URL, nil, timeout, service.PocketBaseAuthModeToken, cfg.PocketBase.Token, "", "", retryMax)
		case cfg.PocketBase.SuperuserEmail != "" && cfg.PocketBase.SuperuserPassword != "":
			pbClient, err = service.NewPocketBaseClientWithHTTPClient(cfg.PocketBase.URL, nil, timeout, service.PocketBaseAuthModeSuperuser, "", cfg.PocketBase.SuperuserEmail, cfg.PocketBase.SuperuserPassword, retryMax)
		case cfg.PocketBase.AdminEmail != "" && cfg.PocketBase.AdminPassword != "":
			pbClient, err = service.NewPocketBaseClientWithHTTPClient(cfg.PocketBase.URL, nil, timeout, service.PocketBaseAuthModeAdmin, "", cfg.PocketBase.AdminEmail, cfg.PocketBase.AdminPassword, retryMax)
		default:
			err = nil
		}
		if err != nil {
			slog.Error("failed to initialize pocketbase client", "error", err)
			os.Exit(1)
		}
		if pbClient != nil {
			pbStorage, err := service.NewPocketBaseStorageService(jsonStorage, pbClient, cfg.PocketBase.JournalSourceMode)
			if err != nil {
				slog.Error("failed to initialize pocketbase storage", "error", err)
				os.Exit(1)
			}
			storageRepo = pbStorage
			slog.Info("PocketBase storage enabled for signal_journals + watch_journals + evaluation_runs", "journal_source_mode", cfg.PocketBase.JournalSourceMode)
		}
	}

	// Load configuration registry
	policyPath := filepath.Join(".", "config", "policy_profile.json")
	playbookPath := filepath.Join(".", "config", "playbook_threshold_profile.json")
	reg, err := usecase.LoadConfigRegistry(policyPath, playbookPath)
	if err != nil {
		slog.Error("Failed to load configuration registry, using default code config", "error", err)
	} else {
		usecase.SetGlobalConfigRegistry(reg)
		slog.Info("Configuration registry loaded successfully", "version", reg.GetVersion())
	}

	// 3. Initialize Services (Binance, Gemini, Telegram)
	binanceService := service.NewBinanceReadonlyServiceWithOptions(
		cfg.Binance.APIKey,
		cfg.Binance.APISecret,
		cfg.Binance.BaseURL,
		time.Duration(cfg.Binance.RequestTimeoutSeconds)*time.Second,
		cfg.Binance.MaxRetry,
		time.Duration(cfg.Binance.RetryBackoffMs)*time.Millisecond,
	)

	var geminiService *service.GeminiService
	if cfg.Gemini.APIKey != "" {
		geminiService, err = service.NewGeminiServiceWithAPIKey(cfg.Gemini.APIKey, cfg.Gemini.Model, time.Duration(cfg.Gemini.RequestTimeoutSeconds)*time.Second)
		if err != nil {
			slog.Warn("Gemini service failed to initialize", "error", err, "context", "AI audits will fail")
		}
	}

	telegramService := service.NewTelegramService(service.TelegramConfig{
		Enabled:                       cfg.Telegram.Enabled,
		SignalEnabled:                 cfg.Telegram.SignalEnabled,
		StatusEnabled:                 cfg.Telegram.StatusEnabled,
		BotToken:                      cfg.Telegram.BotToken,
		SignalChatID:                  cfg.Telegram.SignalChatID,
		StatusChatID:                  cfg.Telegram.StatusChatID,
		StatusAllowSignalChatFallback: cfg.Telegram.StatusAllowSignalChatFallback,
		RequestTimeoutSeconds:         cfg.Telegram.RequestTimeoutSeconds,
	})

	var realtimePriceFeed *service.BinanceRealtimePriceStream
	if cfg.Binance.WebsocketEnabled {
		realtimePriceFeed = service.NewBinanceRealtimePriceStream(service.BinanceRealtimePriceConfig{
			Enabled:          cfg.Binance.WebsocketEnabled,
			BaseURL:          cfg.Binance.WebsocketBaseURL,
			MaxActiveSymbols: cfg.Binance.WSMaxActiveSymbols,
			ReconnectDelay:   time.Duration(cfg.Binance.WSReconnectSeconds) * time.Second,
			StaleAfter:       time.Duration(cfg.Binance.WSStalePriceSeconds) * time.Second,
			ForceRestart:     time.Duration(cfg.Binance.WSForceRestartHours) * time.Hour,
		})
	}

	// 4. Initialize Usecases
	storageUC := usecase.NewStorageUsecase(storageRepo)
	marketDataUC := usecase.NewMarketDataUsecase(binanceService, usecase.MarketDataUsecaseConfig{
		BootstrapTimeout: time.Duration(cfg.Binance.BootstrapTimeoutSeconds) * time.Second,
		InitialTimeout:   time.Duration(cfg.Binance.InitialTimeoutSeconds) * time.Second,
		EnrichTimeout:    time.Duration(cfg.Binance.EnrichTimeoutSeconds) * time.Second,
		GlobalCacheTTL:   time.Duration(cfg.Binance.BootstrapCacheSeconds) * time.Second,
	})
	marketPolicyUC := usecase.NewMarketPolicyUsecase()
	universeUC := usecase.NewUniverseUsecase()
	strategySelectorUC := usecase.NewStrategySelectorUsecase()
	playbookEligibilityUC := usecase.NewPlaybookEligibilityUsecase()
	playbookQuantEngineUC := usecase.NewPlaybookQuantEngineUsecase()
	scoringUC := usecase.NewScoringUsecase()
	candidateArbiterUC := usecase.NewCandidateArbiterUsecase()
	localGateUC := usecase.NewLocalGateUsecase()
	localGateUC.SetMarketData(marketDataUC)
	aiCandidateSelectorUC := usecase.NewAICandidateSelectorUsecase(7.5)

	var aiService usecase.AIAuditorService
	if geminiService != nil {
		aiService = geminiService
	} else {
		aiService = &mockAIAuditor{}
	}

	aiAuditorUC := usecase.NewAIAuditorUsecase(aiService, storageUC)
	planReconciliationUC := usecase.NewPlanReconciliationUsecase()
	stalenessUC := usecase.NewStalenessUsecase(usecase.GetClosedCandleFreshnessDurationForExternal())
	stalenessUC.SetLatestPriceFeed(realtimePriceFeed)
	stalenessUC.SetFallbackProvider(binanceService)
	finalGateUC := usecase.NewFinalGateUsecase()
	conflictResolverUC := usecase.NewConflictResolverUsecase()
	signalNotificationUC := usecase.NewSignalNotificationUsecase(telegramService, storageUC)
	opsNotificationUC := usecase.NewOpsNotificationUsecase(telegramService)
	opsNotificationUC.SetAdminEnabled(cfg.Telegram.OpsAdminEnabled)
	monitoringUC := usecase.NewMonitoringUsecase(binanceService, storageUC)
	monitoringUC.SetLatestPriceFeed(realtimePriceFeed)
	feedbackUC := usecase.NewFeedbackUsecase(storageUC)

	{
		startCtx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.App.StartupNotificationTimeoutSeconds)*time.Second)
		defer cancel()
		if cfg.Telegram.OpsBootEnabled {
			opsNotificationUC.SendBootStatus(
				startCtx,
				cfg.App.Name,
				cfg.App.Env,
				cfg.App.Version,
				cfg.HTTP.Port,
				cfg.Safety.AlertOnly,
				cfg.Safety.BinanceReadOnly,
				cfg.Scanner.Enabled,
				cfg.Monitoring.Enabled,
			)
		}
	}

	scannerUC := usecase.NewScannerUsecase(
		marketDataUC,
		marketPolicyUC,
		universeUC,
		strategySelectorUC,
		playbookEligibilityUC,
		playbookQuantEngineUC,
		scoringUC,
		candidateArbiterUC,
		localGateUC,
		aiCandidateSelectorUC,
		aiAuditorUC,
		planReconciliationUC,
		stalenessUC,
		finalGateUC,
		conflictResolverUC,
		signalNotificationUC,
		opsNotificationUC,
		monitoringUC,
		feedbackUC,
		storageUC,
	)
	if cfg.HotSource.Enabled {
		hotRankService := service.NewBinanceHotRankServiceWithConfig(service.BinanceHotRankConfig{
			RequestTimeout:   time.Duration(cfg.HotSource.RequestTimeoutSeconds) * time.Second,
			CacheTTL:         time.Duration(cfg.HotSource.CacheTTLSeconds) * time.Second,
			TrendingChains:   cfg.HotSource.TrendingChains,
			SocialChains:     cfg.HotSource.SocialHypeChains,
			SmartMoneyChains: cfg.HotSource.SmartMoneyChains,
		})
		scannerUC.SetHotSymbolProvider(hotRankService)
	} else {
		slog.Info("Hot source overlay disabled by config")
	}

	backtestUC := usecase.NewBacktestEngineUsecase(
		binanceService,
		marketPolicyUC,
		universeUC,
		strategySelectorUC,
		playbookEligibilityUC,
		playbookQuantEngineUC,
		scoringUC,
		candidateArbiterUC,
		localGateUC,
		aiCandidateSelectorUC,
		aiAuditorUC,
		planReconciliationUC,
		stalenessUC,
		finalGateUC,
		conflictResolverUC,
		storageUC,
		cfg.Storage.StoragePath,
	)

	// 5. Initialize last scan and evaluation times from storage if files exist
	latestRes, err := storageUC.LoadLatestResult()
	if err == nil && latestRes != nil && !latestRes.GeneratedAt.IsZero() {
		usecase.GetGlobalMetrics().SetLastScanTime(latestRes.GeneratedAt)
		usecase.GetGlobalMetrics().SetLastSuccessScan(latestRes.GeneratedAt)
	}
	report, err := storageUC.LoadEvaluationReport()
	if err == nil && report != nil && !report.GeneratedAt.IsZero() {
		usecase.GetGlobalMetrics().SetLastEvaluationTime(report.GeneratedAt)
	}

	// 6. Context for graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if realtimePriceFeed != nil {
		realtimePriceFeed.Start(ctx)
		slog.Info("Binance realtime price websocket enabled", "base_url", cfg.Binance.WebsocketBaseURL, "max_active_symbols", cfg.Binance.WSMaxActiveSymbols)
	}

	// Start Background Scan, Monitoring & Evaluation Workers
	go startStartupScan(ctx, cfg, scannerUC, storageUC, opsNotificationUC)
	go startBackgroundWorker(ctx, cfg, scannerUC, storageUC, feedbackUC, opsNotificationUC)
	go startWatchRecheckWorker(ctx, cfg, scannerUC)
	go startMonitoringWorker(ctx, cfg, monitoringUC)
	go startEvaluationWorker(ctx, cfg, feedbackUC)

	// 7. Setup HTTP transport Handler and Router
	observabilityUC := usecase.NewObservabilityUsecase(binanceService, aiService, telegramService, cfg.Storage.StoragePath)
	observabilityUC.SetStorageFiles(cfg.Storage.SignalJournalFile, cfg.Storage.WatchJournalFile, cfg.Storage.HealthSnapshotFile)
	observabilityUC.SetRealtimeStatusProvider(realtimePriceFeed)
	handler := transhttp.NewHandler(scannerUC, feedbackUC, storageUC, backtestUC, observabilityUC, cfg.Storage.StoragePath, &scannerRunning, transhttp.HandlerRuntimeConfig{
		AppName:         cfg.App.Name,
		AppVersion:      cfg.App.Version,
		AppEnv:          cfg.App.Env,
		SwaggerEnabled:  cfg.Route.SwaggerEnabled,
		BinanceReadOnly: cfg.Safety.BinanceReadOnly,
		LogFilePath:     cfg.Logging.LogFilePath,
		SafeConfig: map[string]any{
			"binance_api_key_set":         strings.TrimSpace(cfg.Binance.APIKey) != "",
			"binance_api_secret_set":      strings.TrimSpace(cfg.Binance.APISecret) != "",
			"gemini_api_key_set":          strings.TrimSpace(cfg.Gemini.APIKey) != "",
			"telegram_bot_token_set":      strings.TrimSpace(cfg.Telegram.BotToken) != "",
			"telegram_signal_chat_id_set": strings.TrimSpace(cfg.Telegram.SignalChatID) != "" || strings.TrimSpace(cfg.Telegram.ChatID) != "",
			"telegram_status_chat_id_set": strings.TrimSpace(cfg.Telegram.StatusChatID) != "",
		},
	})
	router := transhttp.SetupRouter(cfg, handler)

	server := &http.Server{
		Addr:    ":" + cfg.HTTP.Port,
		Handler: router,
	}

	// Server shutdown listener
	go func() {
		<-ctx.Done()
		slog.Info("Shutting down HTTP server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.App.ShutdownTimeoutSeconds)*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.Error("HTTP server shutdown error", "error", err)
		}
	}()

	slog.Info("Server listening...", "port", cfg.HTTP.Port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("failed to start server", "error", err)
		os.Exit(1)
	}
}

func startStartupScan(ctx context.Context, cfg *config.Config, scannerUC *usecase.ScannerUsecase, storageUC *usecase.StorageUsecase, opsUC *usecase.OpsNotificationUsecase) {
	if !cfg.Scanner.Enabled {
		return
	}

	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Duration(cfg.Scanner.StartupDelaySeconds) * time.Second):
		}

		defer func() {
			if r := recover(); r != nil {
				slog.Error("PANIC RECOVERY in startup scan worker", "panic", r)
			}
		}()

		slog.Info("Startup scan trigger: executing initial scan", "interval_mode", cfg.Scanner.IntervalMode)
		boundaryDuration := time.Duration(cfg.Scanner.BoundaryMinutes) * time.Minute
		boundary := time.Now().Truncate(boundaryDuration)
		scanID := usecase.BuildScanIDForExternal("startup", boundary)

		if cfg.Scanner.PreventOverlap {
			if !scannerRunning.CompareAndSwap(false, true) {
				slog.Warn("Startup scan skipped: scan already in progress")
				usecase.GetGlobalMetrics().IncrementScanOverlapSkip()
				return
			}
			defer scannerRunning.Store(false)
		}

		if cfg.Telegram.OpsScanEnabled {
			opsUC.SendScanStarted(ctx, scanID, boundary, "startup M15 close scan")
		}

		scanCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.Scanner.ContextTimeoutSeconds)*time.Second)
		defer cancel()

		_, err := scannerUC.Run(scanCtx, dto.ScanRequest{
			TriggerTime:   boundary,
			TriggerSource: "startup",
		})
		if err != nil {
			slog.Error("Startup scan failed", "error", err)
			if cfg.Telegram.OpsScanEnabled {
				opsUC.SendScanFailed(ctx, scanID, boundary, err)
			}
		} else {
			usecase.GetGlobalMetrics().SetLastScanTime(time.Now())
			usecase.GetGlobalMetrics().SetLastSuccessScan(time.Now())

			latest, loadErr := storageUC.LoadLatestResult()
			if loadErr == nil && latest != nil && latest.ScanID != "" {
				if cfg.Telegram.OpsScanEnabled {
					opsUC.SendScanDone(ctx, latest)
				}
			}
		}
	}()
}

func startBackgroundWorker(ctx context.Context, cfg *config.Config, scannerUC *usecase.ScannerUsecase, storageUC *usecase.StorageUsecase, feedbackUC *usecase.FeedbackUsecase, opsUC *usecase.OpsNotificationUsecase) {
	if !cfg.Scanner.Enabled {
		slog.Info("Scan worker disabled by config")
		return
	}

	slog.Info("Starting background scan worker...", "interval_mode", cfg.Scanner.IntervalMode)
	usecase.ScanWorkerRunning.Store(true)
	defer usecase.ScanWorkerRunning.Store(false)
	ticker := time.NewTicker(time.Duration(cfg.Scanner.PollIntervalSeconds) * time.Second)
	defer ticker.Stop()

	boundaryDuration := time.Duration(cfg.Scanner.BoundaryMinutes) * time.Minute
	lastRun := time.Now().Truncate(boundaryDuration)

	for {
		select {
		case <-ctx.Done():
			slog.Info("Background scan worker stopped.")
			return
		case now := <-ticker.C:
			boundary := now.Truncate(boundaryDuration)
			bufferSec := cfg.Scanner.CloseCandleBufferSeconds
			if bufferSec <= 0 {
				bufferSec = defaultWorkerBoundaryBufferSeconds
			}
			if boundary.After(lastRun) && now.Sub(boundary) >= time.Duration(bufferSec)*time.Second {
				lastRun = boundary

				go func(boundary time.Time) {
					defer func() {
						if r := recover(); r != nil {
							slog.Error("PANIC RECOVERY in background scan worker", "panic", r)
						}
					}()

					slog.Info("Background worker trigger: executing M15 scan", "boundary", boundary.Format("15:04:05"), "interval_mode", cfg.Scanner.IntervalMode)
					scanID := usecase.BuildScanIDForExternal("scheduled", boundary)

					if cfg.Scanner.PreventOverlap {
						if !scannerRunning.CompareAndSwap(false, true) {
							slog.Warn("Scan worker skipped: scan already in progress")
							usecase.GetGlobalMetrics().IncrementScanOverlapSkip()
							return
						}
						defer scannerRunning.Store(false)
					}

					if cfg.Telegram.OpsScanEnabled {
						opsUC.SendScanStarted(ctx, scanID, boundary, "M15 close scan")
					}

					scanCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.Scanner.ContextTimeoutSeconds)*time.Second)
					defer cancel()

					_, err := scannerUC.Run(scanCtx, dto.ScanRequest{
						TriggerTime:   boundary,
						TriggerSource: "scheduled",
					})
					if err != nil {
						slog.Error("Background scan failed", "error", err)
						if cfg.Telegram.OpsScanEnabled {
							opsUC.SendScanFailed(ctx, scanID, boundary, err)
						}
					} else {
						usecase.GetGlobalMetrics().SetLastScanTime(time.Now())
						usecase.GetGlobalMetrics().SetLastSuccessScan(time.Now())

						latest, loadErr := storageUC.LoadLatestResult()
						if loadErr == nil && latest != nil && latest.ScanID != "" {
							if cfg.Telegram.OpsScanEnabled {
								opsUC.SendScanDone(ctx, latest)
							}
						}
					}

				}(boundary)
			}
		}
	}
}

func startMonitoringWorker(ctx context.Context, cfg *config.Config, monitoringUC *usecase.MonitoringUsecase) {
	if !cfg.Monitoring.Enabled {
		slog.Info("Monitoring worker disabled by config")
		return
	}

	slog.Info("Starting background monitoring worker...")
	usecase.MonitoringWorkerRunning.Store(true)
	defer usecase.MonitoringWorkerRunning.Store(false)
	ticker := time.NewTicker(time.Duration(cfg.Monitoring.IntervalSeconds) * time.Second)
	defer ticker.Stop()
	var monitoringTickRunning atomic.Bool

	for {
		select {
		case <-ctx.Done():
			slog.Info("Monitoring worker stopped.")
			return
		case <-ticker.C:
			if !monitoringTickRunning.CompareAndSwap(false, true) {
				slog.Warn("Monitoring worker skipped: previous tick still running")
				continue
			}
			go func() {
				defer monitoringTickRunning.Store(false)
				defer func() {
					if r := recover(); r != nil {
						slog.Error("PANIC RECOVERY in monitoring worker", "panic", r)
					}
				}()

				timeoutSec := cfg.Monitoring.IntervalSeconds - cfg.Monitoring.TimeoutBufferSeconds
				if timeoutSec <= 0 {
					timeoutSec = defaultMonitoringTimeoutFloorSeconds
				}

				monitorCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
				defer cancel()

				if err := monitoringUC.MonitorVirtualPositions(monitorCtx); err != nil {
					slog.Error("Monitoring worker execution failed", "error", err)
				}
			}()
		}
	}
}

func startWatchRecheckWorker(ctx context.Context, cfg *config.Config, scannerUC *usecase.ScannerUsecase) {
	if !cfg.Monitoring.Enabled || !cfg.Monitoring.RecheckEnabled {
		slog.Info("Watch recheck worker disabled by config")
		return
	}

	slog.Info("Starting watch recheck worker...")
	usecase.RecheckWorkerRunning.Store(true)
	defer usecase.RecheckWorkerRunning.Store(false)

	ticker := time.NewTicker(time.Duration(cfg.Scanner.PollIntervalSeconds) * time.Second)
	defer ticker.Stop()

	recheckBoundaryMinutes := usecase.SnapshotRuntimeSettings().WatchRecheckBoundaryMinutes
	if recheckBoundaryMinutes <= 0 {
		recheckBoundaryMinutes = defaultRecheckBoundaryMinutes
	}
	recheckBoundaryDuration := time.Duration(recheckBoundaryMinutes) * time.Minute
	primaryBoundaryDuration := time.Duration(cfg.Scanner.BoundaryMinutes) * time.Minute
	lastRun := time.Now().Truncate(recheckBoundaryDuration)

	for {
		select {
		case <-ctx.Done():
			slog.Info("Watch recheck worker stopped.")
			return
		case now := <-ticker.C:
			bufferSec := cfg.Scanner.CloseCandleBufferSeconds
			if bufferSec <= 0 {
				bufferSec = defaultWorkerBoundaryBufferSeconds
			}
			decision := evaluateWatchRecheckTick(now, lastRun, recheckBoundaryDuration, primaryBoundaryDuration, time.Duration(bufferSec)*time.Second)
			if !decision.ShouldRun {
				if decision.SkipReason == "primary_boundary" {
					lastRun = decision.Boundary
					slog.Info("Watch recheck skipped at primary boundary", "worker", "watch_recheck", "boundary", decision.Boundary.Format("15:04:05"), "primary_boundary_minutes", cfg.Scanner.BoundaryMinutes, "recheck_boundary_minutes", recheckBoundaryMinutes)
				}
				continue
			}
			boundary := decision.Boundary
			lastRun = boundary
			slog.Info("Watch recheck boundary reached", "worker", "watch_recheck", "boundary", boundary.Format("15:04:05"), "next_primary_boundary", decision.NextPrimaryBoundary.Format("15:04:05"), "boundary_minutes", recheckBoundaryMinutes, "primary_boundary_minutes", cfg.Scanner.BoundaryMinutes)

			go func(boundary time.Time) {
				defer func() {
					if r := recover(); r != nil {
						slog.Error("PANIC RECOVERY in watch recheck worker", "panic", r)
					}
				}()

				if !scannerRunning.CompareAndSwap(false, true) {
					slog.Warn("Watch recheck skipped: primary scan pipeline already in progress", "worker", "watch_recheck", "boundary", boundary.Format("15:04:05"), "primary_boundary_minutes", cfg.Scanner.BoundaryMinutes, "recheck_boundary_minutes", recheckBoundaryMinutes)
					usecase.GetGlobalMetrics().IncrementRecheckOverlapSkip()
					return
				}
				defer scannerRunning.Store(false)

				recheckCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.Scanner.ContextTimeoutSeconds)*time.Second)
				defer cancel()

				if _, err := scannerUC.RunWatchRecheck(recheckCtx, dto.ScanRequest{TriggerTime: boundary, TriggerSource: "recheck"}); err != nil {
					slog.Error("Watch recheck worker execution failed", "boundary", boundary.Format("15:04:05"), "error", err)
					usecase.GetGlobalMetrics().IncrementRecheckFail()
					return
				}
			}(boundary)
		}
	}
}

func evaluateWatchRecheckTick(now time.Time, lastRun time.Time, recheckBoundaryDuration time.Duration, primaryBoundaryDuration time.Duration, closeBuffer time.Duration) watchRecheckTickDecision {
	if recheckBoundaryDuration <= 0 {
		return watchRecheckTickDecision{ShouldRun: false, SkipReason: "invalid_recheck_boundary"}
	}
	boundary := now.Truncate(recheckBoundaryDuration)
	nextPrimaryBoundary := time.Time{}
	if primaryBoundaryDuration > 0 {
		nextPrimaryBoundary = boundary.Truncate(primaryBoundaryDuration).Add(primaryBoundaryDuration)
	}
	if primaryBoundaryDuration > 0 && boundary.Equal(boundary.Truncate(primaryBoundaryDuration)) {
		return watchRecheckTickDecision{Boundary: boundary, NextPrimaryBoundary: nextPrimaryBoundary, ShouldRun: false, SkipReason: "primary_boundary"}
	}
	if !boundary.After(lastRun) {
		return watchRecheckTickDecision{Boundary: boundary, NextPrimaryBoundary: nextPrimaryBoundary, ShouldRun: false, SkipReason: "already_processed_boundary"}
	}
	if closeBuffer > 0 && now.Sub(boundary) < closeBuffer {
		return watchRecheckTickDecision{Boundary: boundary, NextPrimaryBoundary: nextPrimaryBoundary, ShouldRun: false, SkipReason: "close_buffer_not_elapsed"}
	}
	if primaryBoundaryDuration > 0 && !nextPrimaryBoundary.IsZero() && nextPrimaryBoundary.After(now) && nextPrimaryBoundary.Sub(now) <= closeBuffer {
		return watchRecheckTickDecision{Boundary: boundary, NextPrimaryBoundary: nextPrimaryBoundary, ShouldRun: false, SkipReason: "primary_guard_window"}
	}
	return watchRecheckTickDecision{Boundary: boundary, NextPrimaryBoundary: nextPrimaryBoundary, ShouldRun: true}
}

func tryStartBackgroundRun(flag *atomic.Bool, workerName string) bool {
	if flag == nil {
		return true
	}
	if !flag.CompareAndSwap(false, true) {
		slog.Warn(workerName + " skipped: previous tick still running")
		return false
	}
	return true
}

func startEvaluationWorker(ctx context.Context, cfg *config.Config, feedbackUC *usecase.FeedbackUsecase) {
	if !cfg.Evaluation.Enabled || !cfg.Evaluation.AutoRun {
		slog.Info("Evaluation background worker disabled by config (Evaluation.Enabled=false or AutoRun=false)")
		return
	}

	slog.Info("Starting background evaluation worker...", "interval_minutes", cfg.Evaluation.IntervalMinutes)
	usecase.EvaluationWorkerRunning.Store(true)
	defer usecase.EvaluationWorkerRunning.Store(false)
	ticker := time.NewTicker(time.Duration(cfg.Evaluation.IntervalMinutes) * time.Minute)
	defer ticker.Stop()
	var evaluationTickRunning atomic.Bool

	for {
		select {
		case <-ctx.Done():
			slog.Info("Evaluation worker stopped.")
			return
		case <-ticker.C:
			if !tryStartBackgroundRun(&evaluationTickRunning, "Evaluation worker") {
				continue
			}
			go func() {
				defer evaluationTickRunning.Store(false)
				defer func() {
					if r := recover(); r != nil {
						slog.Error("PANIC RECOVERY in evaluation worker", "panic", r)
					}
				}()

				if err := feedbackUC.GenerateEvaluationReport(); err != nil {
					slog.Error("Evaluation worker execution failed", "error", err)
				} else {
					usecase.GetGlobalMetrics().SetLastEvaluationTime(time.Now())
					slog.Info("Background feedback evaluation completed and report saved.")
				}
			}()
		}
	}
}

type mockAIAuditor struct{}

func (m *mockAIAuditor) AuditCandidate(ctx context.Context, req dto.AIAuditRequest) (*dto.AIAuditResponse, error) {
	return &dto.AIAuditResponse{
		Symbol:     req.Symbol,
		Decision:   "REJECT",
		Sentiment:  "NEUTRAL",
		IsApproved: false,
		Reasoning:  "Mock AI Auditor placeholder active — Gemini service unavailable.",
	}, nil
}
