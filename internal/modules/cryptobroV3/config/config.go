package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// AppConfig environment settings
type AppConfig struct {
	Env     string `json:"env"`
	Version string `json:"version"`
	Name    string `json:"name"`
}

// HTTPConfig routing port configuration
type HTTPConfig struct {
	Port string `json:"port"`
}

// ScannerConfig setup parameters for scanner daemon run loops
type ScannerConfig struct {
	Enabled                  bool   `json:"enabled"`
	IntervalMode             string `json:"interval_mode"`
	StartupDelaySeconds      int    `json:"startup_delay_seconds"`
	ContextTimeoutSeconds    int    `json:"context_timeout_seconds"`
	PreventOverlap           bool   `json:"prevent_overlap"`
	CloseCandleBufferSeconds int    `json:"scan_close_candle_buffer_seconds"`
}

// MonitoringConfig virtual position tracker rules
type MonitoringConfig struct {
	Enabled           bool `json:"enabled"`
	IntervalSeconds   int  `json:"interval_seconds"`
	MaxHoldMinutes    int  `json:"max_hold_minutes"`
	MaxHoldM15Candles int  `json:"max_hold_m15_candles"`
}

// EvaluationConfig feedback report thresholds
type EvaluationConfig struct {
	Enabled          bool `json:"enabled"`
	AutoRun          bool `json:"auto_run"`
	IntervalMinutes  int  `json:"interval_minutes"`
	MinSampleWarning int  `json:"min_sample_warning"`
	MinSampleMedium  int  `json:"min_sample_medium"`
	MinSampleHigh    int  `json:"min_sample_high"`
}

// BinanceConfig API settings for read-only connectivity
type BinanceConfig struct {
	APIKey                string `json:"-"`
	APISecret             string `json:"-"`
	BaseURL               string `json:"base_url"`
	RequestTimeoutSeconds int    `json:"request_timeout_seconds"`
	MaxRetry              int    `json:"max_retry"`
	RetryBackoffMs        int    `json:"retry_backoff_ms"`
}

// GeminiConfig API settings for Gemini AI Candles auditor
type GeminiConfig struct {
	APIKey                string `json:"-"`
	Model                 string `json:"model"`
	RequestTimeoutSeconds int    `json:"request_timeout_seconds"`
	MaxConcurrency        int    `json:"max_concurrency"`
	MaxCandidatesDefault  int    `json:"max_candidates_default"`
}

// TelegramConfig dispatch details for trade execution alerts
type TelegramConfig struct {
	Enabled               bool   `json:"enabled"`
	BotToken              string `json:"-"`
	ChatID                string `json:"-"`
	RequestTimeoutSeconds int    `json:"request_timeout_seconds"`
}

// ConcurrencyConfig rate limits for parallel execution loops
type ConcurrencyConfig struct {
	MaxMarketDataConcurrency int `json:"max_marketdata_concurrency"`
	MaxSymbolsDefault        int `json:"max_symbols_default"`
}

// StorageConfig JSON file names and directories
type StorageConfig struct {
	StoragePath          string `json:"storage_path"`
	LatestResultFile     string `json:"latest_result_file"`
	SignalHistoryFile    string `json:"signal_history_file"`
	SignalJournalFile    string `json:"signal_journal_file"`
	AIAuditCacheFile     string `json:"ai_audit_cache_file"`
	EvaluationReportFile string `json:"evaluation_report_file"`
	DecisionAuditFile    string `json:"decision_audit_file"`
	HealthSnapshotFile   string `json:"health_snapshot_file"`
}

// SafetyConfig strict runtime overrides (must never be false in production)
type SafetyConfig struct {
	AlertOnly                    bool `json:"alert_only"`
	BinanceReadOnly              bool `json:"binance_readonly"`
	DisableBinanceOrderEndpoints bool `json:"disable_binance_order_endpoints"`
	DisableAutoExecution         bool `json:"disable_auto_execution"`
	DisableAutoThresholdApply    bool `json:"disable_auto_threshold_apply"`
	RequireAIHighForExecute      bool `json:"require_ai_high_for_execute"`
	RequireFreshEntryForExecute  bool `json:"require_fresh_entry_for_execute"`
	AIAuditEnabled               bool `json:"ai_audit_enabled"`
	DecisionAuditEnabled         bool `json:"decision_audit_enabled"`
	HealthStorageCheck           bool `json:"health_storage_check"`
}

// LoggingConfig structured logger settings
type LoggingConfig struct {
	LogLevel         string `json:"log_level"`
	LogFormat        string `json:"log_format"`
	LogIncludeScanID bool   `json:"log_include_scan_id"`
}

// RouteConfig prefix settings for exposing REST endpoints
type RouteConfig struct {
	APIPrefix                   string `json:"api_prefix"`
	EnableDecisionAuditEndpoint bool   `json:"enable_decision_audit_endpoint"`
	EnableEvaluationRunEndpoint bool   `json:"enable_evaluation_run_endpoint"`
	SwaggerEnabled              bool   `json:"swagger_enabled"`
	SwaggerHost                 string `json:"swagger_host"`
	SwaggerBasePath             string `json:"swagger_base_path"`
}

// Config wraps all engine configurations
type Config struct {
	App         AppConfig         `json:"app"`
	HTTP        HTTPConfig        `json:"http"`
	Scanner     ScannerConfig     `json:"scanner"`
	Monitoring  MonitoringConfig  `json:"monitoring"`
	Evaluation  EvaluationConfig  `json:"evaluation"`
	Binance     BinanceConfig     `json:"binance"`
	Gemini      GeminiConfig      `json:"gemini"`
	Telegram    TelegramConfig    `json:"telegram"`
	Concurrency ConcurrencyConfig `json:"concurrency"`
	Storage     StorageConfig     `json:"storage"`
	Safety      SafetyConfig      `json:"safety"`
	Logging     LoggingConfig     `json:"logging"`
	Route       RouteConfig       `json:"route"`
}

// LoadConfig parses a local .env file (if exists) and populates Config from env variables
func LoadConfig() (*Config, error) {
	// Attempt to load .env file from root
	_ = LoadEnvFile(".env")

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		return nil, err
	}

	if err := ValidateConfig(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// LoadConfigFromEnv parses Config fields using environment variables with safe defaults
func LoadConfigFromEnv() (*Config, error) {
	cfg := &Config{
		App: AppConfig{
			Env:     getEnv("APP_ENV", "local"),
			Version: getEnv("APP_VERSION", "0.1.0"),
			Name:    getEnv("APP_NAME", "cryptobroV3"),
		},
		HTTP: HTTPConfig{
			Port: getEnv("HTTP_PORT", "8080"),
		},
		Scanner: ScannerConfig{
			Enabled:                  getEnvBool("SCAN_ENABLED", true),
			IntervalMode:             getEnv("SCAN_INTERVAL_MODE", "m15_close"),
			StartupDelaySeconds:      getEnvInt("SCAN_STARTUP_DELAY_SECONDS", 5),
			ContextTimeoutSeconds:    getEnvInt("SCAN_CONTEXT_TIMEOUT_SECONDS", 120),
			PreventOverlap:           getEnvBool("PREVENT_SCAN_OVERLAP", true),
			CloseCandleBufferSeconds: getEnvInt("SCAN_CLOSE_CANDLE_BUFFER_SECONDS", 3),
		},
		Monitoring: MonitoringConfig{
			Enabled:           getEnvBool("MONITORING_ENABLED", true),
			IntervalSeconds:   getEnvInt("MONITORING_INTERVAL_SECONDS", 60),
			MaxHoldMinutes:    getEnvInt("MONITORING_MAX_HOLD_MINUTES", 120),
			MaxHoldM15Candles: getEnvInt("MONITORING_MAX_HOLD_M15_CANDLES", 8),
		},
		Evaluation: EvaluationConfig{
			Enabled:          getEnvBool("EVALUATION_ENABLED", true),
			AutoRun:          getEnvBool("EVALUATION_AUTO_RUN", false),
			IntervalMinutes:  getEnvInt("EVALUATION_INTERVAL_MINUTES", 360),
			MinSampleWarning: getEnvInt("EVALUATION_MIN_SAMPLE_WARNING", 10),
			MinSampleMedium:  getEnvInt("EVALUATION_MIN_SAMPLE_MEDIUM", 20),
			MinSampleHigh:    getEnvInt("EVALUATION_MIN_SAMPLE_HIGH", 50),
		},
		Binance: BinanceConfig{
			APIKey:                getEnv("BINANCE_API_KEY", ""),
			APISecret:             getEnv("BINANCE_API_SECRET", ""),
			BaseURL:               getEnv("BINANCE_BASE_URL", "https://fapi.binance.com"),
			RequestTimeoutSeconds: getEnvInt("BINANCE_REQUEST_TIMEOUT_SECONDS", 15),
			MaxRetry:              getEnvInt("BINANCE_MAX_RETRY", 2),
			RetryBackoffMs:        getEnvInt("BINANCE_RETRY_BACKOFF_MS", 300),
		},
		Gemini: GeminiConfig{
			APIKey:                getEnv("GEMINI_API_KEY", ""),
			Model:                 getEnv("GEMINI_MODEL", "gemini-3.1-flash-lite"),
			RequestTimeoutSeconds: getEnvInt("GEMINI_REQUEST_TIMEOUT_SECONDS", 25),
			MaxConcurrency:        getEnvInt("MAX_AI_CONCURRENCY", 2),
			MaxCandidatesDefault:  getEnvInt("MAX_AI_CANDIDATES_DEFAULT", 3),
		},
		Telegram: TelegramConfig{
			Enabled:               getEnvBool("TELEGRAM_ENABLED", true),
			BotToken:              getEnv("TELEGRAM_BOT_TOKEN", ""),
			ChatID:                getEnv("TELEGRAM_CHAT_ID", ""),
			RequestTimeoutSeconds: getEnvInt("TELEGRAM_REQUEST_TIMEOUT_SECONDS", 10),
		},
		Concurrency: ConcurrencyConfig{
			MaxMarketDataConcurrency: getEnvInt("MAX_MARKETDATA_CONCURRENCY", 10),
			MaxSymbolsDefault:        getEnvInt("MAX_SYMBOLS_DEFAULT", 75),
		},
		Storage: StorageConfig{
			StoragePath:          getEnv("STORAGE_PATH", "storage"),
			LatestResultFile:     getEnv("LATEST_RESULT_FILE", "latest_result.json"),
			SignalHistoryFile:    getEnv("SIGNAL_HISTORY_FILE", "signal_history.json"),
			SignalJournalFile:    getEnv("SIGNAL_JOURNAL_FILE", "signal_journal.json"),
			AIAuditCacheFile:     getEnv("AI_AUDIT_CACHE_FILE", "ai_audit_cache.json"),
			EvaluationReportFile: getEnv("EVALUATION_REPORT_FILE", "evaluation_report.json"),
			DecisionAuditFile:    getEnv("DECISION_AUDIT_FILE", "decision_audit.json"),
			HealthSnapshotFile:   getEnv("HEALTH_SNAPSHOT_FILE", "health_snapshot.json"),
		},
		Safety: SafetyConfig{
			AlertOnly:                    getEnvBool("ALERT_ONLY", true),
			BinanceReadOnly:              getEnvBool("BINANCE_READ_ONLY", true),
			DisableBinanceOrderEndpoints: getEnvBool("DISABLE_BINANCE_ORDER_ENDPOINTS", true),
			DisableAutoExecution:         getEnvBool("DISABLE_AUTO_EXECUTION", true),
			DisableAutoThresholdApply:    getEnvBool("DISABLE_AUTO_THRESHOLD_APPLY", true),
			RequireAIHighForExecute:      getEnvBool("REQUIRE_AI_HIGH_FOR_EXECUTE", true),
			RequireFreshEntryForExecute:  getEnvBool("REQUIRE_FRESH_ENTRY_FOR_EXECUTE", true),
			AIAuditEnabled:               getEnvBool("AI_AUDIT_ENABLED", true),
			DecisionAuditEnabled:         getEnvBool("DECISION_AUDIT_ENABLED", true),
			HealthStorageCheck:           getEnvBool("HEALTH_STORAGE_CHECK", true),
		},
		Logging: LoggingConfig{
			LogLevel:         getEnv("LOG_LEVEL", "info"),
			LogFormat:        getEnv("LOG_FORMAT", "json"),
			LogIncludeScanID: getEnvBool("LOG_INCLUDE_SCAN_ID", true),
		},
		Route: RouteConfig{
			APIPrefix:                   getEnv("API_PREFIX", "/api/v3"),
			EnableDecisionAuditEndpoint: getEnvBool("ENABLE_DECISION_AUDIT_ENDPOINT", true),
			EnableEvaluationRunEndpoint: getEnvBool("ENABLE_EVALUATION_RUN_ENDPOINT", true),
			SwaggerEnabled:              getEnvBool("SWAGGER_ENABLED", true),
			SwaggerHost:                 getEnv("SWAGGER_HOST", "localhost:"+getEnv("HTTP_PORT", "8080")),
			SwaggerBasePath:             getEnv("SWAGGER_BASE_PATH", "/api/v3"),
		},
	}

	return cfg, nil
}

// ValidateConfig audits config properties for safety and bounds correctness
func ValidateConfig(cfg *Config) error {
	// HTTP Port check
	if strings.TrimSpace(cfg.HTTP.Port) == "" {
		return fmt.Errorf("HTTP_PORT cannot be empty")
	}
	if p, err := strconv.Atoi(cfg.HTTP.Port); err != nil || p <= 0 || p > 65535 {
		return fmt.Errorf("HTTP_PORT must be a valid port number (1-65535): %s", cfg.HTTP.Port)
	}

	// Storage Path check
	if strings.TrimSpace(cfg.Storage.StoragePath) == "" {
		return fmt.Errorf("STORAGE_PATH cannot be empty")
	}

	// Timeouts checks
	if cfg.Scanner.ContextTimeoutSeconds <= 0 {
		return fmt.Errorf("SCAN_CONTEXT_TIMEOUT_SECONDS must be greater than zero")
	}
	if cfg.Binance.RequestTimeoutSeconds <= 0 {
		return fmt.Errorf("BINANCE_REQUEST_TIMEOUT_SECONDS must be greater than zero")
	}
	if cfg.Gemini.RequestTimeoutSeconds <= 0 {
		return fmt.Errorf("GEMINI_REQUEST_TIMEOUT_SECONDS must be greater than zero")
	}
	if cfg.Telegram.RequestTimeoutSeconds <= 0 {
		return fmt.Errorf("TELEGRAM_REQUEST_TIMEOUT_SECONDS must be greater than zero")
	}

	// Concurrencies checks
	if cfg.Gemini.MaxConcurrency < 1 {
		return fmt.Errorf("MAX_AI_CONCURRENCY must be at least 1")
	}
	if cfg.Concurrency.MaxMarketDataConcurrency < 1 {
		return fmt.Errorf("MAX_MARKETDATA_CONCURRENCY must be at least 1")
	}

	// CRITICAL SAFETY BOUNDS (Must NEVER be false)
	if !cfg.Safety.AlertOnly {
		return fmt.Errorf("CRITICAL SAFETY VIOLATION: ALERT_ONLY must be true")
	}
	if !cfg.Safety.BinanceReadOnly {
		return fmt.Errorf("CRITICAL SAFETY VIOLATION: BINANCE_READ_ONLY must be true")
	}
	if !cfg.Safety.DisableBinanceOrderEndpoints {
		return fmt.Errorf("CRITICAL SAFETY VIOLATION: DISABLE_BINANCE_ORDER_ENDPOINTS must be true")
	}
	if !cfg.Safety.DisableAutoExecution {
		return fmt.Errorf("CRITICAL SAFETY VIOLATION: DISABLE_AUTO_EXECUTION must be true")
	}

	return nil
}

// SafeConfigView exports config details while redacting sensitive API keys or credentials
func SafeConfigView(cfg *Config) map[string]any {
	return map[string]any{
		"app": map[string]any{
			"env":     cfg.App.Env,
			"version": cfg.App.Version,
			"name":    cfg.App.Name,
		},
		"http": map[string]any{
			"port": cfg.HTTP.Port,
		},
		"scanner": map[string]any{
			"enabled":                          cfg.Scanner.Enabled,
			"interval_mode":                    cfg.Scanner.IntervalMode,
			"startup_delay_seconds":            cfg.Scanner.StartupDelaySeconds,
			"context_timeout_seconds":          cfg.Scanner.ContextTimeoutSeconds,
			"prevent_overlap":                  cfg.Scanner.PreventOverlap,
			"scan_close_candle_buffer_seconds": cfg.Scanner.CloseCandleBufferSeconds,
		},
		"monitoring": map[string]any{
			"enabled":              cfg.Monitoring.Enabled,
			"interval_seconds":     cfg.Monitoring.IntervalSeconds,
			"max_hold_minutes":     cfg.Monitoring.MaxHoldMinutes,
			"max_hold_m15_candles": cfg.Monitoring.MaxHoldM15Candles,
		},
		"evaluation": map[string]any{
			"enabled":            cfg.Evaluation.Enabled,
			"auto_run":           cfg.Evaluation.AutoRun,
			"interval_minutes":   cfg.Evaluation.IntervalMinutes,
			"min_sample_warning": cfg.Evaluation.MinSampleWarning,
			"min_sample_medium":  cfg.Evaluation.MinSampleMedium,
			"min_sample_high":    cfg.Evaluation.MinSampleHigh,
		},
		"binance": map[string]any{
			"base_url":                cfg.Binance.BaseURL,
			"request_timeout_seconds": cfg.Binance.RequestTimeoutSeconds,
			"max_retry":               cfg.Binance.MaxRetry,
			"retry_backoff_ms":        cfg.Binance.RetryBackoffMs,
			"api_key_set":             cfg.Binance.APIKey != "",
		},
		"gemini": map[string]any{
			"model":                   cfg.Gemini.Model,
			"request_timeout_seconds": cfg.Gemini.RequestTimeoutSeconds,
			"max_concurrency":         cfg.Gemini.MaxConcurrency,
			"max_candidates_default":  cfg.Gemini.MaxCandidatesDefault,
			"api_key_set":             cfg.Gemini.APIKey != "",
		},
		"telegram": map[string]any{
			"enabled":                 cfg.Telegram.Enabled,
			"request_timeout_seconds": cfg.Telegram.RequestTimeoutSeconds,
			"bot_token_set":           cfg.Telegram.BotToken != "",
			"chat_id_set":             cfg.Telegram.ChatID != "",
		},
		"concurrency": map[string]any{
			"max_marketdata_concurrency": cfg.Concurrency.MaxMarketDataConcurrency,
			"max_symbols_default":        cfg.Concurrency.MaxSymbolsDefault,
		},
		"storage": map[string]any{
			"storage_path":           cfg.Storage.StoragePath,
			"latest_result_file":     cfg.Storage.LatestResultFile,
			"signal_history_file":    cfg.Storage.SignalHistoryFile,
			"signal_journal_file":    cfg.Storage.SignalJournalFile,
			"ai_audit_cache_file":    cfg.Storage.AIAuditCacheFile,
			"evaluation_report_file": cfg.Storage.EvaluationReportFile,
			"decision_audit_file":    cfg.Storage.DecisionAuditFile,
			"health_snapshot_file":   cfg.Storage.HealthSnapshotFile,
		},
		"safety": map[string]any{
			"alert_only":                      cfg.Safety.AlertOnly,
			"binance_readonly":                cfg.Safety.BinanceReadOnly,
			"disable_binance_order_endpoints": cfg.Safety.DisableBinanceOrderEndpoints,
			"disable_auto_execution":          cfg.Safety.DisableAutoExecution,
			"disable_auto_threshold_apply":    cfg.Safety.DisableAutoThresholdApply,
			"require_ai_high_for_execute":     cfg.Safety.RequireAIHighForExecute,
			"require_fresh_entry_for_execute": cfg.Safety.RequireFreshEntryForExecute,
			"ai_audit_enabled":                cfg.Safety.AIAuditEnabled,
			"decision_audit_enabled":          cfg.Safety.DecisionAuditEnabled,
			"health_storage_check":            cfg.Safety.HealthStorageCheck,
		},
		"logging": map[string]any{
			"log_level":           cfg.Logging.LogLevel,
			"log_format":          cfg.Logging.LogFormat,
			"log_include_scan_id": cfg.Logging.LogIncludeScanID,
		},
		"route": map[string]any{
			"api_prefix":                     cfg.Route.APIPrefix,
			"enable_decision_audit_endpoint": cfg.Route.EnableDecisionAuditEndpoint,
			"enable_evaluation_run_endpoint": cfg.Route.EnableEvaluationRunEndpoint,
			"swagger_enabled":                cfg.Route.SwaggerEnabled,
			"swagger_host":                   cfg.Route.SwaggerHost,
			"swagger_base_path":              cfg.Route.SwaggerBasePath,
		},
	}
}

// LoadEnvFile custom parser for local environment configuration files
func LoadEnvFile(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		// Trim surrounding quotes
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}

		if os.Getenv(key) == "" {
			_ = os.Setenv(key, val)
		}
	}
	return scanner.Err()
}

// Helpers
func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			return b
		}
	}
	return defaultVal
}
