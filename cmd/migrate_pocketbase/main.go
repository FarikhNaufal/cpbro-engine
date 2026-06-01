package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/config"
	"cpbro-engine/internal/modules/cryptobroV3/service"
)

func main() {
	var (
		dryRun     = flag.Bool("dry-run", false, "Print what would be migrated without writing to PocketBase")
		storageDir = flag.String("storage-path", "", "Override STORAGE_PATH (optional)")
		timeoutSec = flag.Int("timeout-seconds", 30, "Overall migration timeout seconds")
	)
	flag.Parse()

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
	if *storageDir != "" {
		cfg.Storage.StoragePath = *storageDir
	}
	if !cfg.PocketBase.Enabled {
		log.Fatalf("POCKETBASE_ENABLED must be true for migration")
	}

	timeout := time.Duration(*timeoutSec) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	jsonStorage, err := service.NewJSONStorageService(cfg.Storage.StoragePath)
	if err != nil {
		log.Fatalf("failed to init JSON storage: %v", err)
	}

	pbClient, err := buildPBClient(cfg)
	if err != nil {
		log.Fatalf("failed to init PocketBase client: %v", err)
	}
	pbStorage, err := service.NewPocketBaseStorageService(jsonStorage, pbClient)
	if err != nil {
		log.Fatalf("failed to init PocketBase storage: %v", err)
	}

	slog.Info("Starting PocketBase migration", "storage_path", cfg.Storage.StoragePath, "dry_run", *dryRun)

	if err := migrateSignalJournal(ctx, cfg, jsonStorage, pbStorage, *dryRun); err != nil {
		log.Fatalf("signal_journal migration failed: %v", err)
	}
	if err := migrateEvaluationReport(ctx, cfg, jsonStorage, pbClient, *dryRun); err != nil {
		log.Fatalf("evaluation_report migration failed: %v", err)
	}

	slog.Info("PocketBase migration completed successfully")
}

func buildPBClient(cfg *config.Config) (*service.PocketBaseClient, error) {
	timeout := time.Duration(cfg.PocketBase.RequestTimeoutSeconds) * time.Second
	retryMax := cfg.PocketBase.LoginRetryMax

	switch {
	case cfg.PocketBase.Token != "":
		return service.NewPocketBaseClientWithHTTPClient(cfg.PocketBase.URL, nil, timeout, service.PocketBaseAuthModeToken, cfg.PocketBase.Token, "", "", retryMax)
	case cfg.PocketBase.SuperuserEmail != "" && cfg.PocketBase.SuperuserPassword != "":
		return service.NewPocketBaseClientWithHTTPClient(cfg.PocketBase.URL, nil, timeout, service.PocketBaseAuthModeSuperuser, "", cfg.PocketBase.SuperuserEmail, cfg.PocketBase.SuperuserPassword, retryMax)
	case cfg.PocketBase.AdminEmail != "" && cfg.PocketBase.AdminPassword != "":
		return service.NewPocketBaseClientWithHTTPClient(cfg.PocketBase.URL, nil, timeout, service.PocketBaseAuthModeAdmin, "", cfg.PocketBase.AdminEmail, cfg.PocketBase.AdminPassword, retryMax)
	default:
		return nil, errors.New("no pocketbase auth configured")
	}
}

func migrateSignalJournal(ctx context.Context, cfg *config.Config, jsonStorage *service.JSONStorageService, pbStorage *service.PocketBaseStorageService, dryRun bool) error {
	journalPath := filepath.Join(cfg.Storage.StoragePath, cfg.Storage.SignalJournalFile)
	_, statErr := os.Stat(journalPath)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			slog.Info("signal_journal.json not found; skipping", "path", journalPath)
			return nil
		}
		return statErr
	}

	journal, err := jsonStorage.LoadSignalJournal()
	if err != nil {
		return err
	}
	if len(journal) == 0 {
		slog.Info("signal_journal is empty; nothing to migrate")
		return nil
	}

	slog.Info("Migrating signal_journals", "count", len(journal))
	if dryRun {
		return nil
	}

	// Upsert by signal_id (unique index).
	for i := range journal {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if err := pbStorage.AppendSignalJournal(journal[i]); err != nil {
			return fmt.Errorf("append signal_journal failed at idx=%d signal_id=%s: %w", i, journal[i].ID, err)
		}
	}

	slog.Info("signal_journals migrated", "count", len(journal))
	return nil
}

func migrateEvaluationReport(ctx context.Context, cfg *config.Config, jsonStorage *service.JSONStorageService, pbClient *service.PocketBaseClient, dryRun bool) error {
	reportPath := filepath.Join(cfg.Storage.StoragePath, cfg.Storage.EvaluationReportFile)
	_, statErr := os.Stat(reportPath)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			slog.Info("evaluation_report.json not found; skipping", "path", reportPath)
			return nil
		}
		return statErr
	}

	report, err := jsonStorage.LoadEvaluationReport()
	if err != nil {
		return err
	}
	if report == nil || report.GeneratedAt.IsZero() {
		slog.Info("evaluation_report empty or missing generated_at; skipping")
		return nil
	}

	evaluationID := fmt.Sprintf("migrate_%s", report.GeneratedAt.UTC().Format("20060102150405"))
	slog.Info("Migrating evaluation_report into evaluation_runs", "evaluation_id", evaluationID, "generated_at", report.GeneratedAt.UTC().Format(time.RFC3339))
	if dryRun {
		return nil
	}

	payload := map[string]any{
		"evaluation_id":                              evaluationID,
		"generated_at":                               report.GeneratedAt.UTC().Format(time.RFC3339Nano),
		"status":                                     string(report.Status),
		"total_signals":                              report.TotalSignals,
		"data_completeness_json":                     report.DataCompleteness,
		"metrics_json":                               report.Metrics,
		"playbook_stats_json":                        report.PlaybookStats,
		"regime_stats_json":                          report.RegimeStats,
		"tier_stats_json":                            report.TierStats,
		"direction_stats_json":                       report.DirectionStats,
		"ai_stats_json":                              report.AIStats,
		"staleness_stats_json":                       report.StalenessStats,
		"conflict_stats_json":                        report.ConflictStats,
		"cooldown_stats_json":                        report.CooldownStats,
		"gate_bug_findings_json":                     report.GateBugFindings,
		"recommendations_json":                       report.Recommendations,
		"best_playbook":                              report.BestPlaybook,
		"worst_playbook":                             report.WorstPlaybook,
		"setup_yang_sering_langsung_sl":              report.SetupYangSeringLangsungSL,
		"setup_yang_sering_expired":                  report.SetupYangSeringExpired,
		"setup_yang_sering_stale":                    report.SetupYangSeringStale,
		"regime_yang_paling_buruk":                   report.RegimeYangPalingBuruk,
		"tier_yang_paling_buruk":                     report.TierYangPalingBuruk,
		"direction_yang_paling_buruk":                report.DirectionYangPalingBuruk,
		"playbook_dengan_mae_terbesar":               report.PlaybookDenganMAETerbesar,
		"playbook_dengan_expired_rate_terbesar":      report.PlaybookDenganExpiredRate,
		"playbook_dengan_tp1_rate_terbaik":           report.PlaybookDenganTP1Terbaik,
		"playbook_dengan_tp2_follow_through_terbaik": report.PlaybookDenganTP2Follow,
		"notes_json": map[string]any{
			"schema_version": report.SchemaVersion,
			"config_version": report.ConfigVersion,
			"notes":          report.Notes,
			"source_files":   report.SourceFilesUsed,
		},
	}

	// Idempotent: if evaluation_id already exists, patch it.
	recID, err := findEvaluationRunIDByEvaluationID(ctx, pbClient, evaluationID)
	if err != nil {
		return err
	}
	if recID == "" {
		return pbClient.DoJSON(ctx, "POST", "/api/collections/evaluation_runs/records", nil, payload, nil)
	}
	return pbClient.DoJSON(ctx, "PATCH", "/api/collections/evaluation_runs/records/"+recID, nil, payload, nil)
}

func findEvaluationRunIDByEvaluationID(ctx context.Context, pb *service.PocketBaseClient, evaluationID string) (string, error) {
	q := url.Values{}
	q.Set("perPage", "1")
	q.Set("page", "1")
	q.Set("filter", fmt.Sprintf("evaluation_id='%s'", strings.ReplaceAll(evaluationID, "'", "\\'")))
	var resp struct {
		Items []map[string]any `json:"items"`
	}
	if err := pb.DoJSON(ctx, "GET", "/api/collections/evaluation_runs/records", q, nil, &resp); err != nil {
		return "", err
	}
	if len(resp.Items) == 0 {
		return "", nil
	}
	recID, _ := resp.Items[0]["id"].(string)
	return recID, nil
}
