package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
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
// @termsOfService  https://example.com/terms

// @contact.name   cryptobroV3 Maintainer

// @license.name  Private

// @host      localhost:8080
// @BasePath  /api/v3
// @schemes   http

func main() {
	log.Println("Starting Cryptobro V3 Engine...")

	// 1. Load Configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("failed to load configuration: %v", err)
	}
	slog.Info("Configuration loaded successfully", "env", cfg.App.Env, "version", cfg.App.Version)

	// 2. Initialize Storage from config
	storageService, err := service.NewJSONStorageService(cfg.Storage.StoragePath)
	if err != nil {
		log.Fatalf("failed to initialize json storage: %v", err)
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
	binanceService := service.NewBinanceReadonlyService(cfg.Binance.APIKey, cfg.Binance.APISecret)

	var geminiService *service.GeminiService
	if cfg.Gemini.APIKey != "" {
		geminiService, err = service.NewGeminiService(cfg.Gemini.Model)
		if err != nil {
			log.Printf("warning: Gemini service failed to initialize: %v (AI audits will fail)", err)
		}
	}

	telegramService := service.NewTelegramService(cfg.Telegram.Enabled)

	// 4. Initialize Usecases
	storageUC := usecase.NewStorageUsecase(storageService)
	marketDataUC := usecase.NewMarketDataUsecase(binanceService)
	marketPolicyUC := usecase.NewMarketPolicyUsecase()
	universeUC := usecase.NewUniverseUsecase()
	strategySelectorUC := usecase.NewStrategySelectorUsecase()
	playbookEligibilityUC := usecase.NewPlaybookEligibilityUsecase()
	playbookQuantEngineUC := usecase.NewPlaybookQuantEngineUsecase()
	scoringUC := usecase.NewScoringUsecase()
	candidateArbiterUC := usecase.NewCandidateArbiterUsecase()
	localGateUC := usecase.NewLocalGateUsecase()
	aiCandidateSelectorUC := usecase.NewAICandidateSelectorUsecase(60.0)

	var aiService usecase.AIAuditorService
	if geminiService != nil {
		aiService = geminiService
	} else {
		aiService = &mockAIAuditor{}
	}

	aiAuditorUC := usecase.NewAIAuditorUsecase(aiService, storageUC)
	planReconciliationUC := usecase.NewPlanReconciliationUsecase()
	stalenessUC := usecase.NewStalenessUsecase(30 * time.Minute)
	finalGateUC := usecase.NewFinalGateUsecase()
	conflictResolverUC := usecase.NewConflictResolverUsecase()
	notificationUC := usecase.NewNotificationUsecase(telegramService)
	monitoringUC := usecase.NewMonitoringUsecase(binanceService, storageUC)
	feedbackUC := usecase.NewFeedbackUsecase(storageUC)

	{
		startCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		notificationUC.SendStatus(
			startCtx,
			"🤖 *[CRYPTOBRO V3 STARTED]*\n\n"+
				"*Env:* "+cfg.App.Env+"\n"+
				"*Version:* "+cfg.App.Version+"\n"+
				"*HTTP Port:* "+cfg.HTTP.Port+"\n"+
				"*Time:* "+notificationTime(time.Now()),
		)
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
		notificationUC,
		monitoringUC,
		feedbackUC,
		storageUC,
	)

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

	// Start Background Scan, Monitoring & Evaluation Workers
	go startStartupScan(ctx, cfg, scannerUC, storageUC, notificationUC)
	go startBackgroundWorker(ctx, cfg, scannerUC, storageUC, feedbackUC, notificationUC)
	go startMonitoringWorker(ctx, cfg, monitoringUC)
	go startEvaluationWorker(ctx, cfg, feedbackUC)

	// 7. Setup HTTP transport Handler and Router
	observabilityUC := usecase.NewObservabilityUsecase(binanceService, aiService, telegramService, cfg.Storage.StoragePath)
	handler := transhttp.NewHandler(scannerUC, feedbackUC, storageUC, backtestUC, observabilityUC, cfg.Storage.StoragePath, &scannerRunning)
	router := transhttp.SetupRouter(cfg, handler)

	server := &http.Server{
		Addr:    ":" + cfg.HTTP.Port,
		Handler: router,
	}

	// Server shutdown listener
	go func() {
		<-ctx.Done()
		slog.Info("Shutting down HTTP server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.Error("HTTP server shutdown error", "error", err)
		}
	}()

	slog.Info("Server listening...", "port", cfg.HTTP.Port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("failed to start server: %v", err)
	}
}

func startStartupScan(ctx context.Context, cfg *config.Config, scannerUC *usecase.ScannerUsecase, storageUC *usecase.StorageUsecase, notificationUC *usecase.NotificationUsecase) {
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

		slog.Info("Startup scan trigger: executing initial scan")
		notificationUC.SendStatus(
			ctx,
			"🟦 *[SCAN STARTED]* (startup)\n"+
				"*At:* "+notificationTime(time.Now()),
		)

		if !scannerRunning.CompareAndSwap(false, true) {
			slog.Warn("Startup scan skipped: scan already in progress")
			return
		}
		defer scannerRunning.Store(false)

		scanCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.Scanner.ContextTimeoutSeconds)*time.Second)
		defer cancel()

		boundary := time.Now().Truncate(15 * time.Minute)
		_, err := scannerUC.Run(scanCtx, dto.ScanRequest{
			TriggerTime: boundary,
		})
		if err != nil {
			slog.Error("Startup scan failed", "error", err)
			notificationUC.SendStatus(
				ctx,
				"🟥 *[SCAN FAILED]* (startup)\n"+
					"*Error:* "+err.Error()+"\n"+
					"*At:* "+notificationTime(time.Now()),
			)
		} else {
			usecase.GetGlobalMetrics().SetLastScanTime(time.Now())
			usecase.GetGlobalMetrics().SetLastSuccessScan(time.Now())

			latest, loadErr := storageUC.LoadLatestResult()
			if loadErr == nil && latest != nil && latest.ScanID != "" {
				notificationUC.SendStatus(
					ctx,
					"✅ *[SCAN DONE]* (startup)\n"+
						"*ScanID:* "+latest.ScanID+"\n"+
						fmt.Sprintf("*AI:* confirm=%d wait=%d reject=%d error=%d\n", latest.TotalAIConfirm, latest.TotalAIWait, latest.TotalAIReject, latest.TotalAIError)+
						fmt.Sprintf("*Final:* execute=%d watch=%d reject=%d\n", latest.TotalFinalExecute, latest.TotalFinalWatch, latest.TotalFinalReject)+
						fmt.Sprintf("*Signals:* %d\n", len(latest.Signals))+
						"*At:* "+notificationTime(time.Now()),
				)
			}
		}
	}()
}

func startBackgroundWorker(ctx context.Context, cfg *config.Config, scannerUC *usecase.ScannerUsecase, storageUC *usecase.StorageUsecase, feedbackUC *usecase.FeedbackUsecase, notificationUC *usecase.NotificationUsecase) {
	if !cfg.Scanner.Enabled {
		slog.Info("Scan worker disabled by config")
		return
	}

	slog.Info("Starting background scan worker...")
	usecase.ScanWorkerRunning.Store(true)
	defer usecase.ScanWorkerRunning.Store(false)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	lastRun := time.Now().Truncate(15 * time.Minute)

	for {
		select {
		case <-ctx.Done():
			slog.Info("Background scan worker stopped.")
			return
		case now := <-ticker.C:
			boundary := now.Truncate(15 * time.Minute)
			bufferSec := cfg.Scanner.CloseCandleBufferSeconds
			if bufferSec <= 0 {
				bufferSec = 3
			}
			if boundary.After(lastRun) && now.Sub(boundary) >= time.Duration(bufferSec)*time.Second {
				lastRun = boundary

				go func() {
					defer func() {
						if r := recover(); r != nil {
							slog.Error("PANIC RECOVERY in background scan worker", "panic", r)
						}
					}()

					slog.Info("Background worker trigger: executing M15 scan", "boundary", boundary.Format("15:04:05"))
					notificationUC.SendStatus(
						ctx,
						"🟦 *[SCAN STARTED]* (M15 close)\n"+
							"*Boundary:* "+notificationTime(boundary)+"\n"+
							"*At:* "+notificationTime(time.Now()),
					)

					if !scannerRunning.CompareAndSwap(false, true) {
						slog.Warn("Scan worker skipped: scan already in progress")
						return
					}
					defer scannerRunning.Store(false)

					scanCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.Scanner.ContextTimeoutSeconds)*time.Second)
					defer cancel()

					_, err := scannerUC.Run(scanCtx, dto.ScanRequest{
						TriggerTime: boundary,
					})
					if err != nil {
						slog.Error("Background scan failed", "error", err)
						notificationUC.SendStatus(
							ctx,
							"🟥 *[SCAN FAILED]* (M15 close)\n"+
								"*Boundary:* "+notificationTime(boundary)+"\n"+
								"*Error:* "+err.Error()+"\n"+
								"*At:* "+notificationTime(time.Now()),
						)
					} else {
						usecase.GetGlobalMetrics().SetLastScanTime(time.Now())
						usecase.GetGlobalMetrics().SetLastSuccessScan(time.Now())

						latest, loadErr := storageUC.LoadLatestResult()
						if loadErr == nil && latest != nil && latest.ScanID != "" {
							notificationUC.SendStatus(
								ctx,
								"✅ *[SCAN DONE]* (M15 close)\n"+
									"*Boundary:* "+notificationTime(boundary)+"\n"+
									"*ScanID:* "+latest.ScanID+"\n"+
									fmt.Sprintf("*AI:* confirm=%d wait=%d reject=%d error=%d\n", latest.TotalAIConfirm, latest.TotalAIWait, latest.TotalAIReject, latest.TotalAIError)+
									fmt.Sprintf("*Final:* execute=%d watch=%d reject=%d\n", latest.TotalFinalExecute, latest.TotalFinalWatch, latest.TotalFinalReject)+
									fmt.Sprintf("*Signals:* %d\n", len(latest.Signals))+
									"*At:* "+notificationTime(time.Now()),
							)
						}
					}

					if cfg.Evaluation.Enabled && cfg.Evaluation.AutoRun {
						evalErr := feedbackUC.GenerateEvaluationReport()
						if evalErr != nil {
							slog.Error("Background evaluation failed", "error", evalErr)
						} else {
							usecase.GetGlobalMetrics().SetLastEvaluationTime(time.Now())
						}
					}
				}()
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

	for {
		select {
		case <-ctx.Done():
			slog.Info("Monitoring worker stopped.")
			return
		case <-ticker.C:
			go func() {
				defer func() {
					if r := recover(); r != nil {
						slog.Error("PANIC RECOVERY in monitoring worker", "panic", r)
					}
				}()

				timeoutSec := cfg.Monitoring.IntervalSeconds - 5
				if timeoutSec <= 0 {
					timeoutSec = 25
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

	for {
		select {
		case <-ctx.Done():
			slog.Info("Evaluation worker stopped.")
			return
		case <-ticker.C:
			go func() {
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
