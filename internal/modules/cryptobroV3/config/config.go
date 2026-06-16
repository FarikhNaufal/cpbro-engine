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
	Env                               string `json:"env"`
	Version                           string `json:"version"`
	Name                              string `json:"name"`
	StartupNotificationTimeoutSeconds int    `json:"startup_notification_timeout_seconds"`
	ShutdownTimeoutSeconds            int    `json:"shutdown_timeout_seconds"`
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
	PollIntervalSeconds      int    `json:"poll_interval_seconds"`
	BoundaryMinutes          int    `json:"boundary_minutes"`
}

// MonitoringConfig virtual position tracker rules
type MonitoringConfig struct {
	Enabled                     bool `json:"enabled"`
	IntervalSeconds             int  `json:"interval_seconds"`
	MaxHoldMinutes              int  `json:"max_hold_minutes"`
	MaxHoldM15Candles           int  `json:"max_hold_m15_candles"`
	TimeoutBufferSeconds        int  `json:"timeout_buffer_seconds"`
	MaxCandleConcurrency        int  `json:"max_candle_concurrency"`
	WatchCooldownMinutes        int  `json:"watch_cooldown_minutes"`
	WatchDedupPriceToleranceBps int  `json:"watch_dedup_price_tolerance_bps"`
}

// WorkerConfig controls runtime worker fanout and concurrency heuristics.
type WorkerConfig struct {
	MaxMarketDataConcurrency        int `json:"max_marketdata_concurrency"`
	MaxCandidatePipelineConcurrency int `json:"max_candidate_pipeline_concurrency"`
	MaxAIConcurrency                int `json:"max_ai_concurrency"`
	MaxMonitoringCandleConcurrency  int `json:"max_monitoring_candle_concurrency"`
}

// UniverseConfig controls tunable universe selection heuristics.
type UniverseConfig struct {
	MaxSymbolsDefault          int      `json:"max_symbols_default"`
	TierAMinQuoteVolume        float64  `json:"tier_a_min_quote_volume"`
	TierBMinQuoteVolume        float64  `json:"tier_b_min_quote_volume"`
	TierCMinVolume             float64  `json:"tier_c_min_volume"`
	DefaultSymbols             []string `json:"default_symbols"`
	DefaultHotBoost            float64  `json:"default_hot_boost"`
	MaxHotBoost                float64  `json:"max_hot_boost"`
	DefaultMinFundingVolume    float64  `json:"default_min_funding_volume"`
	ChaosMinFundingVolume      float64  `json:"chaos_min_funding_volume"`
	LowVolMinVolumeFloor       float64  `json:"low_vol_min_volume_floor"`
	ChaosHighVolMinVolumeFloor float64  `json:"chaos_high_vol_min_volume_floor"`
	WeightLiquidityDefault     float64  `json:"weight_liquidity_default"`
	WeightActivityDefault      float64  `json:"weight_activity_default"`
	WeightHotDefault           float64  `json:"weight_hot_default"`
	WeightLiquidityAlt         float64  `json:"weight_liquidity_alt"`
	WeightActivityAlt          float64  `json:"weight_activity_alt"`
	WeightHotAlt               float64  `json:"weight_hot_alt"`
	WeightLiquidityCompression float64  `json:"weight_liquidity_compression"`
	WeightActivityCompression  float64  `json:"weight_activity_compression"`
	WeightHotCompression       float64  `json:"weight_hot_compression"`
	WeightLiquidityRiskOff     float64  `json:"weight_liquidity_risk_off"`
	WeightActivityRiskOff      float64  `json:"weight_activity_risk_off"`
	WeightHotRiskOff           float64  `json:"weight_hot_risk_off"`
	WeightLiquidityChaos       float64  `json:"weight_liquidity_chaos"`
	WeightActivityChaos        float64  `json:"weight_activity_chaos"`
	WeightHotChaos             float64  `json:"weight_hot_chaos"`
	WeightLiquidityLowVol      float64  `json:"weight_liquidity_low_vol"`
	WeightActivityLowVol       float64  `json:"weight_activity_low_vol"`
	WeightHotLowVol            float64  `json:"weight_hot_low_vol"`
	WeightLiquidityDominance   float64  `json:"weight_liquidity_dominance"`
	WeightActivityDominance    float64  `json:"weight_activity_dominance"`
	WeightHotDominance         float64  `json:"weight_hot_dominance"`
}

// StrategyRuntimeConfig controls runtime gate and debug heuristics.
type StrategyRuntimeConfig struct {
	RequireAIHighForExecute                  bool    `json:"require_ai_high_for_execute"`
	RequireFreshEntryForExecute              bool    `json:"require_fresh_entry_for_execute"`
	WatchCooldownMinutes                     int     `json:"watch_cooldown_minutes"`
	WatchDedupPriceToleranceBps              int     `json:"watch_dedup_price_tolerance_bps"`
	EvaluationMinSampleWarning               int     `json:"min_sample_warning"`
	EvaluationMinSampleMedium                int     `json:"min_sample_medium"`
	EvaluationMinSampleHigh                  int     `json:"min_sample_high"`
	DebugSaveRawKlines                       bool    `json:"debug_save_raw_klines"`
	RawKlinesDebugDir                        string  `json:"raw_klines_debug_dir"`
	MaxMarketDataPrefetchSymbols             int     `json:"max_marketdata_prefetch_symbols"`
	ScanRequestWeightBudget                  int     `json:"scan_request_weight_budget"`
	CompressionNeutralBreadthLower           float64 `json:"compression_neutral_breadth_lower"`
	CompressionNeutralBreadthUpper           float64 `json:"compression_neutral_breadth_upper"`
	CompressionMaxBBWidth                    float64 `json:"compression_max_bb_width"`
	CompressionZeroEligibleFallbackThreshold int     `json:"compression_zero_eligible_fallback_threshold"`
	BroaderVolatilitySampleFloor             int     `json:"broader_volatility_sample_floor"`
	FundingExtremeThreshold                  float64 `json:"funding_extreme_threshold"`
	ATRFallbackPercent                       float64 `json:"atr_fallback_percent"`
	MinSLATRMultiplierBase                   float64 `json:"min_sl_atr_multiplier_base"`
	MinSLATRMultiplierReversal               float64 `json:"min_sl_atr_multiplier_reversal"`
	MinSLATRMultiplierHighVol                float64 `json:"min_sl_atr_multiplier_high_vol"`
	RotationActivityThresholdDefault         float64 `json:"rotation_activity_threshold_default"`
	RotationActivityThresholdAlt             float64 `json:"rotation_activity_threshold_alt"`
	RotationActivityThresholdDefensive       float64 `json:"rotation_activity_threshold_defensive"`
	RotationActivityThresholdLowVol          float64 `json:"rotation_activity_threshold_low_vol"`
	RotationPrefetchRatioDefault             float64 `json:"rotation_prefetch_ratio_default"`
	RotationPrefetchRatioAlt                 float64 `json:"rotation_prefetch_ratio_alt"`
	RotationPrefetchRatioDefensive           float64 `json:"rotation_prefetch_ratio_defensive"`
	StalenessPolicyScaleBase                 float64 `json:"staleness_policy_scale_base"`
	StalenessPolicyScaleMin                  float64 `json:"staleness_policy_scale_min"`
	StalenessPolicyScaleMax                  float64 `json:"staleness_policy_scale_max"`
	StalenessLateThresholdMultiplier         float64 `json:"staleness_late_threshold_multiplier"`
	StalenessBasePctChaos                    float64 `json:"staleness_base_pct_chaos"`
	StalenessBasePctHighVol                  float64 `json:"staleness_base_pct_high_vol"`
	StalenessBasePctTierC                    float64 `json:"staleness_base_pct_tier_c"`
	StalenessBasePctDefault                  float64 `json:"staleness_base_pct_default"`
}

// HotSourceConfig controls external hot-symbol discovery settings.
type HotSourceConfig struct {
	Enabled               bool     `json:"enabled"`
	RequestTimeoutSeconds int      `json:"request_timeout_seconds"`
	CacheTTLSeconds       int      `json:"cache_ttl_seconds"`
	TrendingChains        []string `json:"trending_chains"`
	SocialHypeChains      []string `json:"social_hype_chains"`
	SmartMoneyChains      []string `json:"smart_money_chains"`
}

// EvaluationConfig feedback report thresholds
type EvaluationConfig struct {
	Enabled          bool `json:"enabled"`
	AutoRun          bool `json:"auto_run"`
	AutoApply        bool `json:"auto_apply"`
	IntervalMinutes  int  `json:"interval_minutes"`
	MinSampleWarning int  `json:"min_sample_warning"`
	MinSampleMedium  int  `json:"min_sample_medium"`
	MinSampleHigh    int  `json:"min_sample_high"`
}

// BinanceConfig API settings for read-only connectivity
type BinanceConfig struct {
	APIKey                  string `json:"-"`
	APISecret               string `json:"-"`
	BaseURL                 string `json:"base_url"`
	RequestTimeoutSeconds   int    `json:"request_timeout_seconds"`
	BootstrapTimeoutSeconds int    `json:"bootstrap_timeout_seconds"`
	InitialTimeoutSeconds   int    `json:"initial_timeout_seconds"`
	EnrichTimeoutSeconds    int    `json:"enrich_timeout_seconds"`
	BootstrapCacheSeconds   int    `json:"bootstrap_cache_seconds"`
	MaxRetry                int    `json:"max_retry"`
	RetryBackoffMs          int    `json:"retry_backoff_ms"`
	WebsocketEnabled        bool   `json:"websocket_enabled"`
	WebsocketBaseURL        string `json:"websocket_base_url"`
	WSMaxActiveSymbols      int    `json:"ws_max_active_symbols"`
	WSReconnectSeconds      int    `json:"ws_reconnect_seconds"`
	WSStalePriceSeconds     int    `json:"ws_stale_price_seconds"`
	WSForceRestartHours     int    `json:"ws_force_restart_hours"`
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
	Enabled       bool   `json:"enabled"`
	SignalEnabled bool   `json:"signal_enabled"`
	StatusEnabled bool   `json:"status_enabled"`
	BotToken      string `json:"-"`
	// Backward-compat: TELEGRAM_CHAT_ID maps to SignalChatID if SignalChatID is empty.
	ChatID                        string `json:"-"`
	SignalChatID                  string `json:"-"`
	StatusChatID                  string `json:"-"`
	StatusAllowSignalChatFallback bool   `json:"status_allow_signal_chat_fallback"`
	RequestTimeoutSeconds         int    `json:"request_timeout_seconds"`
	OpsBootEnabled                bool   `json:"ops_boot_enabled"`
	OpsScanEnabled                bool   `json:"ops_scan_enabled"`
	OpsAdminEnabled               bool   `json:"ops_admin_enabled"`
}

// ConcurrencyConfig rate limits for parallel execution loops
type ConcurrencyConfig struct {
	MaxMarketDataConcurrency        int `json:"max_marketdata_concurrency"`
	MaxCandidatePipelineConcurrency int `json:"max_candidate_pipeline_concurrency"`
	MaxMarketDataPrefetchSymbols    int `json:"max_marketdata_prefetch_symbols"`
	ScanRequestWeightBudget         int `json:"scan_request_weight_budget"`
	MaxSymbolsDefault               int `json:"max_symbols_default"`
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

// PocketBaseConfig configures optional PocketBase persistence (journal + evaluation runs).
type PocketBaseConfig struct {
	Enabled               bool   `json:"enabled"`
	URL                   string `json:"url"`
	Token                 string `json:"-"`
	SuperuserEmail        string `json:"-"`
	SuperuserPassword     string `json:"-"`
	AdminEmail            string `json:"-"`
	AdminPassword         string `json:"-"`
	RequestTimeoutSeconds int    `json:"request_timeout_seconds"`
	LoginRetryMax         int    `json:"login_retry_max"`
	JournalSourceMode     string `json:"journal_source_mode"`
}

// SafetyConfig strict runtime overrides (must never be false in production)
type SafetyConfig struct {
	AlertOnly                    bool `json:"alert_only"`
	BinanceReadOnly              bool `json:"binance_readonly"`
	AllowBinanceWrite            bool `json:"allow_binance_write"`
	DisableBinanceOrderEndpoints bool `json:"disable_binance_order_endpoints"`
	DisableAutoExecution         bool `json:"disable_auto_execution"`
	DisableAutoThresholdApply    bool `json:"disable_auto_threshold_apply"`
	RequireAIHighForExecute      bool `json:"require_ai_high_for_execute"`
	RequireFreshEntryForExecute  bool `json:"require_fresh_entry_for_execute"`
	AIAuditEnabled               bool `json:"ai_audit_enabled"`
	DecisionAuditEnabled         bool `json:"decision_audit_enabled"`
	HealthStorageCheck           bool `json:"health_storage_check"`
	HealthCheckTimeoutSeconds    int  `json:"health_check_timeout_seconds"`
}

// LoggingConfig structured logger settings
type LoggingConfig struct {
	LogLevel         string `json:"log_level"`
	LogFormat        string `json:"log_format"`
	LogIncludeScanID bool   `json:"log_include_scan_id"`
	LogFilePath      string `json:"log_file_path"`
	LogMaxSizeMB     int    `json:"log_max_size_mb"`
	LogMaxBackups    int    `json:"log_max_backups"`
	LogMaxAgeDays    int    `json:"log_max_age_days"`
	LogCompress      bool   `json:"log_compress"`
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
	App         AppConfig             `json:"app"`
	HTTP        HTTPConfig            `json:"http"`
	Scanner     ScannerConfig         `json:"scanner"`
	Monitoring  MonitoringConfig      `json:"monitoring"`
	Worker      WorkerConfig          `json:"worker"`
	Universe    UniverseConfig        `json:"universe"`
	Strategy    StrategyRuntimeConfig `json:"strategy"`
	HotSource   HotSourceConfig       `json:"hot_source"`
	Evaluation  EvaluationConfig      `json:"evaluation"`
	Binance     BinanceConfig         `json:"binance"`
	Gemini      GeminiConfig          `json:"gemini"`
	Telegram    TelegramConfig        `json:"telegram"`
	Concurrency ConcurrencyConfig     `json:"concurrency"`
	Storage     StorageConfig         `json:"storage"`
	PocketBase  PocketBaseConfig      `json:"pocketbase"`
	Safety      SafetyConfig          `json:"safety"`
	Logging     LoggingConfig         `json:"logging"`
	Route       RouteConfig           `json:"route"`
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
			Env:                               getEnv("APP_ENV", "local"),
			Version:                           getEnv("APP_VERSION", "0.1.0"),
			Name:                              getEnv("APP_NAME", "cryptobroV3"),
			StartupNotificationTimeoutSeconds: getEnvInt("APP_STARTUP_NOTIFICATION_TIMEOUT_SECONDS", 2),
			ShutdownTimeoutSeconds:            getEnvInt("APP_SHUTDOWN_TIMEOUT_SECONDS", 5),
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
			PollIntervalSeconds:      getEnvInt("SCAN_POLL_INTERVAL_SECONDS", 10),
			BoundaryMinutes:          getEnvInt("SCAN_BOUNDARY_MINUTES", 15),
		},
		Monitoring: MonitoringConfig{
			Enabled:                     getEnvBool("MONITORING_ENABLED", true),
			IntervalSeconds:             getEnvInt("MONITORING_INTERVAL_SECONDS", 60),
			MaxHoldMinutes:              getEnvInt("MONITORING_MAX_HOLD_MINUTES", 120),
			MaxHoldM15Candles:           getEnvInt("MONITORING_MAX_HOLD_M15_CANDLES", 8),
			TimeoutBufferSeconds:        getEnvInt("MONITORING_TIMEOUT_BUFFER_SECONDS", 5),
			MaxCandleConcurrency:        getEnvInt("MAX_MONITORING_CANDLE_CONCURRENCY", 4),
			WatchCooldownMinutes:        getEnvInt("WATCH_COOLDOWN_MINUTES", 30),
			WatchDedupPriceToleranceBps: getEnvInt("WATCH_DEDUP_PRICE_TOLERANCE_BPS", 50),
		},
		Worker: WorkerConfig{
			MaxMarketDataConcurrency:        getEnvIntWithFallback("WORKER_MAX_MARKETDATA_CONCURRENCY", "MAX_MARKETDATA_CONCURRENCY", 10),
			MaxCandidatePipelineConcurrency: getEnvIntWithFallback("WORKER_MAX_CANDIDATE_PIPELINE_CONCURRENCY", "MAX_CANDIDATE_PIPELINE_CONCURRENCY", 0),
			MaxAIConcurrency:                getEnvIntWithFallback("WORKER_MAX_AI_CONCURRENCY", "MAX_AI_CONCURRENCY", 2),
			MaxMonitoringCandleConcurrency:  getEnvIntWithFallback("WORKER_MAX_MONITORING_CANDLE_CONCURRENCY", "MAX_MONITORING_CANDLE_CONCURRENCY", 4),
		},
		Universe: UniverseConfig{
			MaxSymbolsDefault:          getEnvIntWithFallback("UNIVERSE_MAX_SYMBOLS_DEFAULT", "MAX_SYMBOLS_DEFAULT", 75),
			TierAMinQuoteVolume:        getEnvFloat("UNIVERSE_TIER_A_MIN_QUOTE_VOLUME", 150000000.0),
			TierBMinQuoteVolume:        getEnvFloat("UNIVERSE_TIER_B_MIN_QUOTE_VOLUME", 50000000.0),
			TierCMinVolume:             getEnvFloat("UNIVERSE_TIER_C_MIN_VOLUME", 15000000.0),
			DefaultSymbols:             getEnvCSV("UNIVERSE_DEFAULT_SYMBOLS", []string{"BTCUSDT", "ETHUSDT", "SOLUSDT", "BNBUSDT"}),
			DefaultHotBoost:            getEnvFloat("UNIVERSE_DEFAULT_HOT_BOOST", 1.25),
			MaxHotBoost:                getEnvFloat("UNIVERSE_MAX_HOT_BOOST", 1.5),
			DefaultMinFundingVolume:    getEnvFloat("UNIVERSE_DEFAULT_MIN_FUNDING_VOLUME", 50000000.0),
			ChaosMinFundingVolume:      getEnvFloat("UNIVERSE_CHAOS_MIN_FUNDING_VOLUME", 150000000.0),
			LowVolMinVolumeFloor:       getEnvFloat("UNIVERSE_LOW_VOL_MIN_VOLUME_FLOOR", 750000.0),
			ChaosHighVolMinVolumeFloor: getEnvFloat("UNIVERSE_CHAOS_HIGH_VOL_MIN_VOLUME_FLOOR", 10000000.0),
			WeightLiquidityDefault:     getEnvFloat("UNIVERSE_WEIGHT_LIQUIDITY_DEFAULT", 0.65),
			WeightActivityDefault:      getEnvFloat("UNIVERSE_WEIGHT_ACTIVITY_DEFAULT", 0.20),
			WeightHotDefault:           getEnvFloat("UNIVERSE_WEIGHT_HOT_DEFAULT", 0.15),
			WeightLiquidityAlt:         getEnvFloat("UNIVERSE_WEIGHT_LIQUIDITY_ALT", 0.55),
			WeightActivityAlt:          getEnvFloat("UNIVERSE_WEIGHT_ACTIVITY_ALT", 0.25),
			WeightHotAlt:               getEnvFloat("UNIVERSE_WEIGHT_HOT_ALT", 0.20),
			WeightLiquidityCompression: getEnvFloat("UNIVERSE_WEIGHT_LIQUIDITY_COMPRESSION", 0.60),
			WeightActivityCompression:  getEnvFloat("UNIVERSE_WEIGHT_ACTIVITY_COMPRESSION", 0.25),
			WeightHotCompression:       getEnvFloat("UNIVERSE_WEIGHT_HOT_COMPRESSION", 0.15),
			WeightLiquidityRiskOff:     getEnvFloat("UNIVERSE_WEIGHT_LIQUIDITY_RISK_OFF", 0.75),
			WeightActivityRiskOff:      getEnvFloat("UNIVERSE_WEIGHT_ACTIVITY_RISK_OFF", 0.15),
			WeightHotRiskOff:           getEnvFloat("UNIVERSE_WEIGHT_HOT_RISK_OFF", 0.10),
			WeightLiquidityChaos:       getEnvFloat("UNIVERSE_WEIGHT_LIQUIDITY_CHAOS", 0.80),
			WeightActivityChaos:        getEnvFloat("UNIVERSE_WEIGHT_ACTIVITY_CHAOS", 0.15),
			WeightHotChaos:             getEnvFloat("UNIVERSE_WEIGHT_HOT_CHAOS", 0.05),
			WeightLiquidityLowVol:      getEnvFloat("UNIVERSE_WEIGHT_LIQUIDITY_LOW_VOL", 0.70),
			WeightActivityLowVol:       getEnvFloat("UNIVERSE_WEIGHT_ACTIVITY_LOW_VOL", 0.15),
			WeightHotLowVol:            getEnvFloat("UNIVERSE_WEIGHT_HOT_LOW_VOL", 0.15),
			WeightLiquidityDominance:   getEnvFloat("UNIVERSE_WEIGHT_LIQUIDITY_DOMINANCE", 0.72),
			WeightActivityDominance:    getEnvFloat("UNIVERSE_WEIGHT_ACTIVITY_DOMINANCE", 0.13),
			WeightHotDominance:         getEnvFloat("UNIVERSE_WEIGHT_HOT_DOMINANCE", 0.15),
		},
		Strategy: StrategyRuntimeConfig{
			RequireAIHighForExecute:                  getEnvBoolWithFallback("STRATEGY_REQUIRE_AI_HIGH_FOR_EXECUTE", "REQUIRE_AI_HIGH_FOR_EXECUTE", true),
			RequireFreshEntryForExecute:              getEnvBoolWithFallback("STRATEGY_REQUIRE_FRESH_ENTRY_FOR_EXECUTE", "REQUIRE_FRESH_ENTRY_FOR_EXECUTE", true),
			WatchCooldownMinutes:                     getEnvIntWithFallback("STRATEGY_WATCH_COOLDOWN_MINUTES", "WATCH_COOLDOWN_MINUTES", 30),
			WatchDedupPriceToleranceBps:              getEnvIntWithFallback("STRATEGY_WATCH_DEDUP_PRICE_TOLERANCE_BPS", "WATCH_DEDUP_PRICE_TOLERANCE_BPS", 50),
			EvaluationMinSampleWarning:               getEnvIntWithFallback("STRATEGY_EVALUATION_MIN_SAMPLE_WARNING", "EVALUATION_MIN_SAMPLE_WARNING", 10),
			EvaluationMinSampleMedium:                getEnvIntWithFallback("STRATEGY_EVALUATION_MIN_SAMPLE_MEDIUM", "EVALUATION_MIN_SAMPLE_MEDIUM", 20),
			EvaluationMinSampleHigh:                  getEnvIntWithFallback("STRATEGY_EVALUATION_MIN_SAMPLE_HIGH", "EVALUATION_MIN_SAMPLE_HIGH", 50),
			DebugSaveRawKlines:                       getEnvBool("DEBUG_SAVE_RAW_KLINES", false),
			RawKlinesDebugDir:                        getEnv("RAW_KLINES_DEBUG_DIR", "debug/klines"),
			MaxMarketDataPrefetchSymbols:             getEnvIntWithFallback("STRATEGY_MAX_MARKETDATA_PREFETCH_SYMBOLS", "MAX_MARKETDATA_PREFETCH_SYMBOLS", 0),
			ScanRequestWeightBudget:                  getEnvIntWithFallback("STRATEGY_SCAN_REQUEST_WEIGHT_BUDGET", "SCAN_REQUEST_WEIGHT_BUDGET", 0),
			CompressionNeutralBreadthLower:           getEnvFloat("STRATEGY_COMPRESSION_NEUTRAL_BREADTH_LOWER", 0.35),
			CompressionNeutralBreadthUpper:           getEnvFloat("STRATEGY_COMPRESSION_NEUTRAL_BREADTH_UPPER", 0.65),
			CompressionMaxBBWidth:                    getEnvFloat("STRATEGY_COMPRESSION_MAX_BB_WIDTH", 0.10),
			CompressionZeroEligibleFallbackThreshold: getEnvInt("STRATEGY_COMPRESSION_ZERO_ELIGIBLE_FALLBACK_THRESHOLD", 2),
			BroaderVolatilitySampleFloor:             getEnvInt("STRATEGY_BROADER_VOLATILITY_SAMPLE_FLOOR", 6),
			FundingExtremeThreshold:                  getEnvFloat("STRATEGY_FUNDING_EXTREME_THRESHOLD", 0.003),
			ATRFallbackPercent:                       getEnvFloat("STRATEGY_ATR_FALLBACK_PERCENT", 0.01),
			MinSLATRMultiplierBase:                   getEnvFloat("STRATEGY_MIN_SL_ATR_MULTIPLIER_BASE", 1.0),
			MinSLATRMultiplierReversal:               getEnvFloat("STRATEGY_MIN_SL_ATR_MULTIPLIER_REVERSAL", 1.2),
			MinSLATRMultiplierHighVol:                getEnvFloat("STRATEGY_MIN_SL_ATR_MULTIPLIER_HIGH_VOL", 1.5),
			RotationActivityThresholdDefault:         getEnvFloat("STRATEGY_ROTATION_ACTIVITY_THRESHOLD_DEFAULT", 0.55),
			RotationActivityThresholdAlt:             getEnvFloat("STRATEGY_ROTATION_ACTIVITY_THRESHOLD_ALT", 0.45),
			RotationActivityThresholdDefensive:       getEnvFloat("STRATEGY_ROTATION_ACTIVITY_THRESHOLD_DEFENSIVE", 0.65),
			RotationActivityThresholdLowVol:          getEnvFloat("STRATEGY_ROTATION_ACTIVITY_THRESHOLD_LOW_VOL", 0.50),
			RotationPrefetchRatioDefault:             getEnvFloat("STRATEGY_ROTATION_PREFETCH_RATIO_DEFAULT", 0.15),
			RotationPrefetchRatioAlt:                 getEnvFloat("STRATEGY_ROTATION_PREFETCH_RATIO_ALT", 0.20),
			RotationPrefetchRatioDefensive:           getEnvFloat("STRATEGY_ROTATION_PREFETCH_RATIO_DEFENSIVE", 0.10),
			StalenessPolicyScaleBase:                 getEnvFloat("STRATEGY_STALENESS_POLICY_SCALE_BASE", 1.5),
			StalenessPolicyScaleMin:                  getEnvFloat("STRATEGY_STALENESS_POLICY_SCALE_MIN", 0.50),
			StalenessPolicyScaleMax:                  getEnvFloat("STRATEGY_STALENESS_POLICY_SCALE_MAX", 1.20),
			StalenessLateThresholdMultiplier:         getEnvFloat("STRATEGY_STALENESS_LATE_THRESHOLD_MULTIPLIER", 1.5),
			StalenessBasePctChaos:                    getEnvFloat("STRATEGY_STALENESS_BASE_PCT_CHAOS", 0.20),
			StalenessBasePctHighVol:                  getEnvFloat("STRATEGY_STALENESS_BASE_PCT_HIGH_VOL", 0.50),
			StalenessBasePctTierC:                    getEnvFloat("STRATEGY_STALENESS_BASE_PCT_TIER_C", 0.25),
			StalenessBasePctDefault:                  getEnvFloat("STRATEGY_STALENESS_BASE_PCT_DEFAULT", 0.35),
		},
		HotSource: HotSourceConfig{
			Enabled:               getEnvBool("HOT_SOURCE_ENABLED", true),
			RequestTimeoutSeconds: getEnvInt("HOT_SOURCE_REQUEST_TIMEOUT_SECONDS", 10),
			CacheTTLSeconds:       getEnvInt("HOT_SOURCE_CACHE_TTL_SECONDS", 600),
			TrendingChains:        getEnvCSV("HOT_SOURCE_TRENDING_CHAINS", []string{"1", "56", "8453"}),
			SocialHypeChains:      getEnvCSV("HOT_SOURCE_SOCIAL_HYPE_CHAINS", []string{"56", "8453"}),
			SmartMoneyChains:      getEnvCSV("HOT_SOURCE_SMART_MONEY_CHAINS", []string{"56", "8453"}),
		},
		Evaluation: EvaluationConfig{
			Enabled:          getEnvBool("EVALUATION_ENABLED", true),
			AutoRun:          getEnvBool("EVALUATION_AUTO_RUN", false),
			AutoApply:        getEnvBool("EVALUATION_AUTO_APPLY", false),
			IntervalMinutes:  getEnvInt("EVALUATION_INTERVAL_MINUTES", 360),
			MinSampleWarning: getEnvInt("EVALUATION_MIN_SAMPLE_WARNING", 10),
			MinSampleMedium:  getEnvInt("EVALUATION_MIN_SAMPLE_MEDIUM", 20),
			MinSampleHigh:    getEnvInt("EVALUATION_MIN_SAMPLE_HIGH", 50),
		},
		Binance: BinanceConfig{
			APIKey:                  getEnv("BINANCE_API_KEY", ""),
			APISecret:               getEnv("BINANCE_API_SECRET", ""),
			BaseURL:                 getEnv("BINANCE_BASE_URL", "https://fapi.binance.com"),
			RequestTimeoutSeconds:   getEnvInt("BINANCE_REQUEST_TIMEOUT_SECONDS", 15),
			BootstrapTimeoutSeconds: getEnvInt("BINANCE_BOOTSTRAP_TIMEOUT_SECONDS", 20),
			InitialTimeoutSeconds:   getEnvInt("BINANCE_INITIAL_TIMEOUT_SECONDS", 10),
			EnrichTimeoutSeconds:    getEnvInt("BINANCE_ENRICH_TIMEOUT_SECONDS", 15),
			BootstrapCacheSeconds:   getEnvInt("BINANCE_BOOTSTRAP_CACHE_SECONDS", 30),
			MaxRetry:                getEnvInt("BINANCE_MAX_RETRY", 2),
			RetryBackoffMs:          getEnvInt("BINANCE_RETRY_BACKOFF_MS", 300),
			WebsocketEnabled:        getEnvBool("BINANCE_WS_ENABLED", false),
			WebsocketBaseURL:        getEnv("BINANCE_WS_BASE_URL", "wss://fstream.binance.com"),
			WSMaxActiveSymbols:      getEnvInt("BINANCE_WS_MAX_ACTIVE_SYMBOLS", 50),
			WSReconnectSeconds:      getEnvInt("BINANCE_WS_RECONNECT_SECONDS", 5),
			WSStalePriceSeconds:     getEnvInt("BINANCE_WS_STALE_PRICE_SECONDS", 15),
			WSForceRestartHours:     getEnvInt("BINANCE_WS_FORCE_RESTART_HOURS", 23),
		},
		Gemini: GeminiConfig{
			APIKey:                getEnv("GEMINI_API_KEY", ""),
			Model:                 getEnv("GEMINI_MODEL", "gemini-3.1-flash-lite"),
			RequestTimeoutSeconds: getEnvInt("GEMINI_REQUEST_TIMEOUT_SECONDS", 25),
			MaxConcurrency:        getEnvIntWithFallback("WORKER_MAX_AI_CONCURRENCY", "MAX_AI_CONCURRENCY", 2),
			MaxCandidatesDefault:  getEnvInt("MAX_AI_CANDIDATES_DEFAULT", 3),
		},
		Telegram: TelegramConfig{
			Enabled:                       getEnvBool("TELEGRAM_ENABLED", true),
			SignalEnabled:                 getEnvBool("TELEGRAM_SIGNAL_ENABLED", true),
			StatusEnabled:                 getEnvBool("TELEGRAM_STATUS_ENABLED", true),
			BotToken:                      getEnv("TELEGRAM_BOT_TOKEN", ""),
			ChatID:                        getEnv("TELEGRAM_CHAT_ID", ""),
			SignalChatID:                  getEnv("TELEGRAM_SIGNAL_CHAT_ID", ""),
			StatusChatID:                  getEnv("TELEGRAM_STATUS_CHAT_ID", ""),
			StatusAllowSignalChatFallback: getEnvBool("TELEGRAM_STATUS_ALLOW_SIGNAL_CHAT_FALLBACK", false),
			RequestTimeoutSeconds:         getEnvInt("TELEGRAM_REQUEST_TIMEOUT_SECONDS", 10),
			OpsBootEnabled:                getEnvBool("TELEGRAM_OPS_BOOT_ENABLED", true),
			OpsScanEnabled:                getEnvBool("TELEGRAM_OPS_SCAN_ENABLED", false),
			OpsAdminEnabled:               getEnvBool("TELEGRAM_OPS_ADMIN_ENABLED", false),
		},
		Concurrency: ConcurrencyConfig{
			MaxMarketDataConcurrency:        getEnvInt("MAX_MARKETDATA_CONCURRENCY", 10),
			MaxCandidatePipelineConcurrency: getEnvInt("MAX_CANDIDATE_PIPELINE_CONCURRENCY", 0),
			MaxMarketDataPrefetchSymbols:    getEnvInt("MAX_MARKETDATA_PREFETCH_SYMBOLS", 0),
			ScanRequestWeightBudget:         getEnvInt("SCAN_REQUEST_WEIGHT_BUDGET", 0),
			MaxSymbolsDefault:               getEnvInt("MAX_SYMBOLS_DEFAULT", 75),
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
		PocketBase: PocketBaseConfig{
			Enabled:               getEnvBool("POCKETBASE_ENABLED", false),
			URL:                   getEnv("POCKETBASE_URL", ""),
			Token:                 getEnv("POCKETBASE_TOKEN", ""),
			SuperuserEmail:        getEnv("POCKETBASE_SUPERUSER_EMAIL", ""),
			SuperuserPassword:     getEnv("POCKETBASE_SUPERUSER_PASSWORD", ""),
			AdminEmail:            getEnv("POCKETBASE_ADMIN_EMAIL", ""),
			AdminPassword:         getEnv("POCKETBASE_ADMIN_PASSWORD", ""),
			RequestTimeoutSeconds: getEnvInt("POCKETBASE_REQUEST_TIMEOUT_SECONDS", 10),
			LoginRetryMax:         getEnvInt("POCKETBASE_LOGIN_RETRY_MAX", 1),
			JournalSourceMode:     getEnv("POCKETBASE_JOURNAL_SOURCE_MODE", "pocketbase_first"),
		},
		Safety: SafetyConfig{
			AlertOnly:                    getEnvBool("ALERT_ONLY", true),
			BinanceReadOnly:              getEnvBool("BINANCE_READ_ONLY", true),
			AllowBinanceWrite:            getEnvBool("ALLOW_BINANCE_WRITE", false),
			DisableBinanceOrderEndpoints: getEnvBool("DISABLE_BINANCE_ORDER_ENDPOINTS", true),
			DisableAutoExecution:         getEnvBool("DISABLE_AUTO_EXECUTION", true),
			DisableAutoThresholdApply:    getEnvBool("DISABLE_AUTO_THRESHOLD_APPLY", true),
			RequireAIHighForExecute:      getEnvBool("REQUIRE_AI_HIGH_FOR_EXECUTE", true),
			RequireFreshEntryForExecute:  getEnvBool("REQUIRE_FRESH_ENTRY_FOR_EXECUTE", true),
			AIAuditEnabled:               getEnvBool("AI_AUDIT_ENABLED", true),
			DecisionAuditEnabled:         getEnvBool("DECISION_AUDIT_ENABLED", true),
			HealthStorageCheck:           getEnvBool("HEALTH_STORAGE_CHECK", true),
			HealthCheckTimeoutSeconds:    getEnvInt("HEALTH_CHECK_TIMEOUT_SECONDS", 2),
		},
		Logging: LoggingConfig{
			LogLevel:         getEnv("LOG_LEVEL", "info"),
			LogFormat:        getEnv("LOG_FORMAT", "json"),
			LogIncludeScanID: getEnvBool("LOG_INCLUDE_SCAN_ID", true),
			LogFilePath:      getEnv("LOG_FILE_PATH", "storage/logs/app.log"),
			LogMaxSizeMB:     getEnvInt("LOG_MAX_SIZE_MB", 10),
			LogMaxBackups:    getEnvInt("LOG_MAX_BACKUPS", 5),
			LogMaxAgeDays:    getEnvInt("LOG_MAX_AGE_DAYS", 7),
			LogCompress:      getEnvBool("LOG_COMPRESS", true),
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

	// Telegram backward compatibility: TELEGRAM_CHAT_ID acts as TELEGRAM_SIGNAL_CHAT_ID if the new var is empty.
	if strings.TrimSpace(cfg.Telegram.SignalChatID) == "" {
		cfg.Telegram.SignalChatID = cfg.Telegram.ChatID
	}

	normalizeCompatibilityConfig(cfg)

	return cfg, nil
}

func normalizeCompatibilityConfig(cfg *Config) {
	if cfg == nil {
		return
	}

	cfg.Monitoring.MaxCandleConcurrency = cfg.Worker.MaxMonitoringCandleConcurrency
	cfg.Monitoring.WatchCooldownMinutes = cfg.Strategy.WatchCooldownMinutes
	cfg.Monitoring.WatchDedupPriceToleranceBps = cfg.Strategy.WatchDedupPriceToleranceBps

	cfg.Concurrency.MaxMarketDataConcurrency = cfg.Worker.MaxMarketDataConcurrency
	cfg.Concurrency.MaxCandidatePipelineConcurrency = cfg.Worker.MaxCandidatePipelineConcurrency
	cfg.Concurrency.MaxMarketDataPrefetchSymbols = cfg.Strategy.MaxMarketDataPrefetchSymbols
	cfg.Concurrency.ScanRequestWeightBudget = cfg.Strategy.ScanRequestWeightBudget
	cfg.Concurrency.MaxSymbolsDefault = cfg.Universe.MaxSymbolsDefault

	cfg.Gemini.MaxConcurrency = cfg.Worker.MaxAIConcurrency

	cfg.Strategy.RequireAIHighForExecute = cfg.Safety.RequireAIHighForExecute
	cfg.Strategy.RequireFreshEntryForExecute = cfg.Safety.RequireFreshEntryForExecute
}

// ValidateConfig audits config properties for safety and bounds correctness
func ValidateConfig(cfg *Config) error {
	normalizeCompatibilityConfig(cfg)

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

	// PocketBase validation (optional)
	if cfg.PocketBase.Enabled {
		if strings.TrimSpace(cfg.PocketBase.URL) == "" {
			return fmt.Errorf("POCKETBASE_URL cannot be empty when POCKETBASE_ENABLED=true")
		}
		hasToken := strings.TrimSpace(cfg.PocketBase.Token) != ""
		hasSuperuser := strings.TrimSpace(cfg.PocketBase.SuperuserEmail) != "" && strings.TrimSpace(cfg.PocketBase.SuperuserPassword) != ""
		hasAdmin := strings.TrimSpace(cfg.PocketBase.AdminEmail) != "" && strings.TrimSpace(cfg.PocketBase.AdminPassword) != ""
		if !hasToken && !hasSuperuser && !hasAdmin {
			return fmt.Errorf("PocketBase enabled but no auth provided (set POCKETBASE_TOKEN or POCKETBASE_SUPERUSER_EMAIL/POCKETBASE_SUPERUSER_PASSWORD or POCKETBASE_ADMIN_EMAIL/POCKETBASE_ADMIN_PASSWORD)")
		}
		if cfg.PocketBase.RequestTimeoutSeconds <= 0 {
			cfg.PocketBase.RequestTimeoutSeconds = 10
		}
		if cfg.PocketBase.LoginRetryMax < 0 {
			return fmt.Errorf("POCKETBASE_LOGIN_RETRY_MAX must be >= 0")
		}
		if cfg.PocketBase.LoginRetryMax > 3 {
			// prevent accidental long retry loops
			cfg.PocketBase.LoginRetryMax = 3
		}
		switch strings.ToLower(strings.TrimSpace(cfg.PocketBase.JournalSourceMode)) {
		case "", "pocketbase_first":
			cfg.PocketBase.JournalSourceMode = "pocketbase_first"
		case "local_first", "local_mirror_first":
			cfg.PocketBase.JournalSourceMode = "local_first"
		default:
			return fmt.Errorf("POCKETBASE_JOURNAL_SOURCE_MODE must be pocketbase_first or local_first")
		}
	}

	// Timeouts checks
	if cfg.Scanner.ContextTimeoutSeconds <= 0 {
		return fmt.Errorf("SCAN_CONTEXT_TIMEOUT_SECONDS must be greater than zero")
	}
	if cfg.Scanner.PollIntervalSeconds <= 0 {
		return fmt.Errorf("SCAN_POLL_INTERVAL_SECONDS must be greater than zero")
	}
	if cfg.Scanner.BoundaryMinutes <= 0 {
		return fmt.Errorf("SCAN_BOUNDARY_MINUTES must be greater than zero")
	}
	if cfg.App.StartupNotificationTimeoutSeconds <= 0 {
		return fmt.Errorf("APP_STARTUP_NOTIFICATION_TIMEOUT_SECONDS must be greater than zero")
	}
	if cfg.App.ShutdownTimeoutSeconds <= 0 {
		return fmt.Errorf("APP_SHUTDOWN_TIMEOUT_SECONDS must be greater than zero")
	}
	if cfg.Binance.RequestTimeoutSeconds <= 0 {
		return fmt.Errorf("BINANCE_REQUEST_TIMEOUT_SECONDS must be greater than zero")
	}
	if cfg.Binance.BootstrapTimeoutSeconds <= 0 {
		return fmt.Errorf("BINANCE_BOOTSTRAP_TIMEOUT_SECONDS must be greater than zero")
	}
	if cfg.Binance.InitialTimeoutSeconds <= 0 {
		return fmt.Errorf("BINANCE_INITIAL_TIMEOUT_SECONDS must be greater than zero")
	}
	if cfg.Binance.EnrichTimeoutSeconds <= 0 {
		return fmt.Errorf("BINANCE_ENRICH_TIMEOUT_SECONDS must be greater than zero")
	}
	if cfg.Binance.BootstrapCacheSeconds <= 0 {
		return fmt.Errorf("BINANCE_BOOTSTRAP_CACHE_SECONDS must be greater than zero")
	}
	if cfg.Binance.WebsocketEnabled {
		if strings.TrimSpace(cfg.Binance.WebsocketBaseURL) == "" {
			return fmt.Errorf("BINANCE_WS_BASE_URL cannot be empty when BINANCE_WS_ENABLED=true")
		}
		if cfg.Binance.WSMaxActiveSymbols < 1 {
			return fmt.Errorf("BINANCE_WS_MAX_ACTIVE_SYMBOLS must be at least 1")
		}
		if cfg.Binance.WSReconnectSeconds < 1 {
			return fmt.Errorf("BINANCE_WS_RECONNECT_SECONDS must be at least 1")
		}
		if cfg.Binance.WSStalePriceSeconds < 1 {
			return fmt.Errorf("BINANCE_WS_STALE_PRICE_SECONDS must be at least 1")
		}
		if cfg.Binance.WSForceRestartHours < 1 {
			return fmt.Errorf("BINANCE_WS_FORCE_RESTART_HOURS must be at least 1")
		}
	}
	if cfg.Gemini.RequestTimeoutSeconds <= 0 {
		return fmt.Errorf("GEMINI_REQUEST_TIMEOUT_SECONDS must be greater than zero")
	}
	if cfg.Telegram.RequestTimeoutSeconds <= 0 {
		return fmt.Errorf("TELEGRAM_REQUEST_TIMEOUT_SECONDS must be greater than zero")
	}
	if cfg.Worker.MaxMarketDataConcurrency < 1 {
		return fmt.Errorf("WORKER_MAX_MARKETDATA_CONCURRENCY must be at least 1")
	}
	if cfg.Worker.MaxCandidatePipelineConcurrency < 0 {
		return fmt.Errorf("WORKER_MAX_CANDIDATE_PIPELINE_CONCURRENCY must be >= 0")
	}
	if cfg.Worker.MaxAIConcurrency < 1 {
		return fmt.Errorf("WORKER_MAX_AI_CONCURRENCY must be at least 1")
	}
	if cfg.Worker.MaxMonitoringCandleConcurrency < 1 {
		return fmt.Errorf("WORKER_MAX_MONITORING_CANDLE_CONCURRENCY must be at least 1")
	}
	if cfg.Universe.MaxSymbolsDefault < 1 {
		return fmt.Errorf("UNIVERSE_MAX_SYMBOLS_DEFAULT must be at least 1")
	}
	if cfg.Universe.TierAMinQuoteVolume <= 0 || cfg.Universe.TierBMinQuoteVolume <= 0 {
		return fmt.Errorf("universe tier quote volume thresholds must be greater than zero")
	}
	if cfg.Universe.TierAMinQuoteVolume <= cfg.Universe.TierBMinQuoteVolume {
		return fmt.Errorf("UNIVERSE_TIER_A_MIN_QUOTE_VOLUME must be greater than UNIVERSE_TIER_B_MIN_QUOTE_VOLUME")
	}
	if cfg.Universe.TierCMinVolume < 0 {
		return fmt.Errorf("UNIVERSE_TIER_C_MIN_VOLUME must be >= 0")
	}
	if len(cfg.Universe.DefaultSymbols) == 0 {
		return fmt.Errorf("UNIVERSE_DEFAULT_SYMBOLS must contain at least one symbol")
	}
	for _, value := range []float64{
		cfg.Universe.DefaultHotBoost,
		cfg.Universe.MaxHotBoost,
		cfg.Universe.DefaultMinFundingVolume,
		cfg.Universe.ChaosMinFundingVolume,
		cfg.Universe.LowVolMinVolumeFloor,
		cfg.Universe.ChaosHighVolMinVolumeFloor,
	} {
		if value <= 0 {
			return fmt.Errorf("universe boost/volume floors must be greater than zero")
		}
	}
	if cfg.Universe.MaxHotBoost < 1.0 || cfg.Universe.DefaultHotBoost < 1.0 || cfg.Universe.DefaultHotBoost > cfg.Universe.MaxHotBoost {
		return fmt.Errorf("universe hot boost must satisfy 1.0 <= default <= max")
	}
	for _, weights := range [][]float64{
		{cfg.Universe.WeightLiquidityDefault, cfg.Universe.WeightActivityDefault, cfg.Universe.WeightHotDefault},
		{cfg.Universe.WeightLiquidityAlt, cfg.Universe.WeightActivityAlt, cfg.Universe.WeightHotAlt},
		{cfg.Universe.WeightLiquidityCompression, cfg.Universe.WeightActivityCompression, cfg.Universe.WeightHotCompression},
		{cfg.Universe.WeightLiquidityRiskOff, cfg.Universe.WeightActivityRiskOff, cfg.Universe.WeightHotRiskOff},
		{cfg.Universe.WeightLiquidityChaos, cfg.Universe.WeightActivityChaos, cfg.Universe.WeightHotChaos},
		{cfg.Universe.WeightLiquidityLowVol, cfg.Universe.WeightActivityLowVol, cfg.Universe.WeightHotLowVol},
		{cfg.Universe.WeightLiquidityDominance, cfg.Universe.WeightActivityDominance, cfg.Universe.WeightHotDominance},
	} {
		sum := 0.0
		for _, value := range weights {
			if value < 0 || value > 1 {
				return fmt.Errorf("universe ranking weights must be within [0,1]")
			}
			sum += value
		}
		if sum < 0.99 || sum > 1.01 {
			return fmt.Errorf("universe ranking weights per regime must sum to 1")
		}
	}
	if cfg.Strategy.WatchCooldownMinutes < 1 {
		return fmt.Errorf("STRATEGY_WATCH_COOLDOWN_MINUTES must be at least 1")
	}
	if cfg.Strategy.WatchDedupPriceToleranceBps < 1 {
		return fmt.Errorf("STRATEGY_WATCH_DEDUP_PRICE_TOLERANCE_BPS must be at least 1")
	}
	if cfg.Strategy.EvaluationMinSampleWarning < 1 || cfg.Strategy.EvaluationMinSampleMedium < 1 || cfg.Strategy.EvaluationMinSampleHigh < 1 {
		return fmt.Errorf("strategy evaluation sample thresholds must be at least 1")
	}
	if cfg.Strategy.EvaluationMinSampleWarning > cfg.Strategy.EvaluationMinSampleMedium || cfg.Strategy.EvaluationMinSampleMedium > cfg.Strategy.EvaluationMinSampleHigh {
		return fmt.Errorf("strategy evaluation sample thresholds must be ordered warning <= medium <= high")
	}
	if cfg.Strategy.MaxMarketDataPrefetchSymbols < 0 {
		return fmt.Errorf("STRATEGY_MAX_MARKETDATA_PREFETCH_SYMBOLS must be >= 0")
	}
	if cfg.Strategy.ScanRequestWeightBudget < 0 {
		return fmt.Errorf("STRATEGY_SCAN_REQUEST_WEIGHT_BUDGET must be >= 0")
	}
	if cfg.Strategy.CompressionNeutralBreadthLower < 0 || cfg.Strategy.CompressionNeutralBreadthUpper > 1 || cfg.Strategy.CompressionNeutralBreadthLower >= cfg.Strategy.CompressionNeutralBreadthUpper {
		return fmt.Errorf("strategy compression neutral breadth must satisfy 0 <= lower < upper <= 1")
	}
	if cfg.Strategy.CompressionMaxBBWidth <= 0 || cfg.Strategy.CompressionMaxBBWidth > 1 {
		return fmt.Errorf("STRATEGY_COMPRESSION_MAX_BB_WIDTH must be within (0,1]")
	}
	if cfg.Strategy.CompressionZeroEligibleFallbackThreshold < 1 {
		return fmt.Errorf("STRATEGY_COMPRESSION_ZERO_ELIGIBLE_FALLBACK_THRESHOLD must be at least 1")
	}
	if cfg.Strategy.BroaderVolatilitySampleFloor < 1 {
		return fmt.Errorf("STRATEGY_BROADER_VOLATILITY_SAMPLE_FLOOR must be at least 1")
	}
	if cfg.Strategy.FundingExtremeThreshold <= 0 {
		return fmt.Errorf("STRATEGY_FUNDING_EXTREME_THRESHOLD must be greater than zero")
	}
	if cfg.Strategy.ATRFallbackPercent <= 0 {
		return fmt.Errorf("STRATEGY_ATR_FALLBACK_PERCENT must be greater than zero")
	}
	if cfg.Strategy.MinSLATRMultiplierBase <= 0 || cfg.Strategy.MinSLATRMultiplierReversal <= 0 || cfg.Strategy.MinSLATRMultiplierHighVol <= 0 {
		return fmt.Errorf("strategy SL ATR multipliers must be greater than zero")
	}
	if cfg.Strategy.MinSLATRMultiplierBase > cfg.Strategy.MinSLATRMultiplierReversal || cfg.Strategy.MinSLATRMultiplierReversal > cfg.Strategy.MinSLATRMultiplierHighVol {
		return fmt.Errorf("strategy SL ATR multipliers must satisfy base <= reversal <= high_vol")
	}
	for _, value := range []float64{
		cfg.Strategy.RotationActivityThresholdDefault,
		cfg.Strategy.RotationActivityThresholdAlt,
		cfg.Strategy.RotationActivityThresholdDefensive,
		cfg.Strategy.RotationActivityThresholdLowVol,
		cfg.Strategy.RotationPrefetchRatioDefault,
		cfg.Strategy.RotationPrefetchRatioAlt,
		cfg.Strategy.RotationPrefetchRatioDefensive,
	} {
		if value < 0 || value > 1 {
			return fmt.Errorf("strategy rotation thresholds/ratios must be within [0,1]")
		}
	}
	for _, value := range []float64{
		cfg.Strategy.StalenessPolicyScaleBase,
		cfg.Strategy.StalenessPolicyScaleMin,
		cfg.Strategy.StalenessPolicyScaleMax,
		cfg.Strategy.StalenessLateThresholdMultiplier,
		cfg.Strategy.StalenessBasePctChaos,
		cfg.Strategy.StalenessBasePctHighVol,
		cfg.Strategy.StalenessBasePctTierC,
		cfg.Strategy.StalenessBasePctDefault,
	} {
		if value <= 0 {
			return fmt.Errorf("strategy staleness settings must be greater than zero")
		}
	}
	if cfg.Strategy.StalenessPolicyScaleMin > cfg.Strategy.StalenessPolicyScaleMax {
		return fmt.Errorf("STRATEGY_STALENESS_POLICY_SCALE_MIN must be <= STRATEGY_STALENESS_POLICY_SCALE_MAX")
	}
	if cfg.HotSource.RequestTimeoutSeconds <= 0 {
		return fmt.Errorf("HOT_SOURCE_REQUEST_TIMEOUT_SECONDS must be greater than zero")
	}
	if cfg.HotSource.CacheTTLSeconds <= 0 {
		return fmt.Errorf("HOT_SOURCE_CACHE_TTL_SECONDS must be greater than zero")
	}
	if len(cfg.HotSource.TrendingChains) == 0 {
		return fmt.Errorf("HOT_SOURCE_TRENDING_CHAINS must contain at least one chain")
	}
	if len(cfg.HotSource.SocialHypeChains) == 0 {
		return fmt.Errorf("HOT_SOURCE_SOCIAL_HYPE_CHAINS must contain at least one chain")
	}
	if len(cfg.HotSource.SmartMoneyChains) == 0 {
		return fmt.Errorf("HOT_SOURCE_SMART_MONEY_CHAINS must contain at least one chain")
	}

	// Concurrencies checks
	if cfg.Gemini.MaxConcurrency < 1 {
		return fmt.Errorf("MAX_AI_CONCURRENCY must be at least 1")
	}
	if cfg.Concurrency.MaxMarketDataConcurrency < 1 {
		return fmt.Errorf("MAX_MARKETDATA_CONCURRENCY must be at least 1")
	}
	if cfg.Concurrency.MaxCandidatePipelineConcurrency < 0 {
		return fmt.Errorf("MAX_CANDIDATE_PIPELINE_CONCURRENCY must be >= 0")
	}
	if cfg.Concurrency.MaxMarketDataPrefetchSymbols < 0 {
		return fmt.Errorf("MAX_MARKETDATA_PREFETCH_SYMBOLS must be >= 0")
	}
	if cfg.Concurrency.ScanRequestWeightBudget < 0 {
		return fmt.Errorf("SCAN_REQUEST_WEIGHT_BUDGET must be >= 0")
	}
	if cfg.Monitoring.IntervalSeconds <= 0 {
		return fmt.Errorf("MONITORING_INTERVAL_SECONDS must be greater than zero")
	}
	if cfg.Monitoring.TimeoutBufferSeconds < 0 {
		return fmt.Errorf("MONITORING_TIMEOUT_BUFFER_SECONDS must be >= 0")
	}
	if cfg.Monitoring.MaxCandleConcurrency < 1 {
		return fmt.Errorf("MAX_MONITORING_CANDLE_CONCURRENCY must be at least 1")
	}
	if cfg.Monitoring.WatchCooldownMinutes < 1 {
		return fmt.Errorf("WATCH_COOLDOWN_MINUTES must be at least 1")
	}
	if cfg.Monitoring.WatchDedupPriceToleranceBps < 1 {
		return fmt.Errorf("WATCH_DEDUP_PRICE_TOLERANCE_BPS must be at least 1")
	}
	if cfg.Safety.HealthCheckTimeoutSeconds <= 0 {
		return fmt.Errorf("HEALTH_CHECK_TIMEOUT_SECONDS must be greater than zero")
	}

	// CRITICAL SAFETY BOUNDS (Must NEVER be false)
	if !cfg.Safety.AlertOnly {
		return fmt.Errorf("CRITICAL SAFETY VIOLATION: ALERT_ONLY must be true")
	}
	if !cfg.Safety.BinanceReadOnly {
		return fmt.Errorf("CRITICAL SAFETY VIOLATION: BINANCE_READ_ONLY must be true")
	}
	if cfg.Safety.AllowBinanceWrite {
		return fmt.Errorf("CRITICAL SAFETY VIOLATION: ALLOW_BINANCE_WRITE must be false")
	}
	if !cfg.Safety.DisableBinanceOrderEndpoints {
		return fmt.Errorf("CRITICAL SAFETY VIOLATION: DISABLE_BINANCE_ORDER_ENDPOINTS must be true")
	}
	if !cfg.Safety.DisableAutoExecution {
		return fmt.Errorf("CRITICAL SAFETY VIOLATION: DISABLE_AUTO_EXECUTION must be true")
	}
	if !cfg.Safety.DisableAutoThresholdApply {
		return fmt.Errorf("CRITICAL SAFETY VIOLATION: DISABLE_AUTO_THRESHOLD_APPLY must be true")
	}
	if !cfg.Safety.RequireAIHighForExecute {
		return fmt.Errorf("CRITICAL SAFETY VIOLATION: REQUIRE_AI_HIGH_FOR_EXECUTE must be true")
	}
	if !cfg.Safety.RequireFreshEntryForExecute {
		return fmt.Errorf("CRITICAL SAFETY VIOLATION: REQUIRE_FRESH_ENTRY_FOR_EXECUTE must be true")
	}
	if cfg.Evaluation.AutoApply {
		return fmt.Errorf("CRITICAL SAFETY VIOLATION: EVALUATION_AUTO_APPLY must be false")
	}

	// Production hardening: if AI audit is disabled while execute requires AI HIGH, fail fast.
	if strings.EqualFold(cfg.App.Env, "production") && !cfg.Safety.AIAuditEnabled && cfg.Safety.RequireAIHighForExecute {
		return fmt.Errorf("CRITICAL SAFETY VIOLATION: AI_AUDIT_ENABLED must be true in production when REQUIRE_AI_HIGH_FOR_EXECUTE=true")
	}

	// Telegram validation (best-effort; do not panic)
	if cfg.Telegram.Enabled {
		if (cfg.Telegram.SignalEnabled || cfg.Telegram.StatusEnabled) && strings.TrimSpace(cfg.Telegram.BotToken) == "" {
			return fmt.Errorf("TELEGRAM_BOT_TOKEN must be set when Telegram is enabled")
		}
		if cfg.Telegram.SignalEnabled && strings.TrimSpace(cfg.Telegram.SignalChatID) == "" {
			return fmt.Errorf("TELEGRAM_SIGNAL_CHAT_ID (or TELEGRAM_CHAT_ID) must be set when TELEGRAM_SIGNAL_ENABLED=true")
		}

		if cfg.Telegram.StatusEnabled && strings.TrimSpace(cfg.Telegram.StatusChatID) == "" && !cfg.Telegram.StatusAllowSignalChatFallback {
			// Status sender is disabled silently at runtime, but we emit a warning and disable here as well.
			fmt.Fprintln(os.Stderr, "WARN: TELEGRAM_STATUS_ENABLED=true but TELEGRAM_STATUS_CHAT_ID is empty and TELEGRAM_STATUS_ALLOW_SIGNAL_CHAT_FALLBACK=false; disabling status notifications")
			cfg.Telegram.StatusEnabled = false
		}
	}

	return nil
}

// SafeConfigView exports config details while redacting sensitive API keys or credentials
func SafeConfigView(cfg *Config) map[string]any {
	normalizeCompatibilityConfig(cfg)

	return map[string]any{
		"app": map[string]any{
			"env":                               cfg.App.Env,
			"version":                           cfg.App.Version,
			"name":                              cfg.App.Name,
			"startup_notification_timeout_secs": cfg.App.StartupNotificationTimeoutSeconds,
			"shutdown_timeout_seconds":          cfg.App.ShutdownTimeoutSeconds,
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
			"poll_interval_seconds":            cfg.Scanner.PollIntervalSeconds,
			"boundary_minutes":                 cfg.Scanner.BoundaryMinutes,
		},
		"monitoring": map[string]any{
			"enabled":                cfg.Monitoring.Enabled,
			"interval_seconds":       cfg.Monitoring.IntervalSeconds,
			"max_hold_minutes":       cfg.Monitoring.MaxHoldMinutes,
			"timeout_buffer_seconds": cfg.Monitoring.TimeoutBufferSeconds,
		},
		"worker": map[string]any{
			"max_marketdata_concurrency":         cfg.Worker.MaxMarketDataConcurrency,
			"max_candidate_pipeline_concurrency": cfg.Worker.MaxCandidatePipelineConcurrency,
			"max_ai_concurrency":                 cfg.Worker.MaxAIConcurrency,
			"max_monitoring_candle_concurrency":  cfg.Worker.MaxMonitoringCandleConcurrency,
		},
		"universe": map[string]any{
			"max_symbols_default":             cfg.Universe.MaxSymbolsDefault,
			"tier_a_min_quote_volume":         cfg.Universe.TierAMinQuoteVolume,
			"tier_b_min_quote_volume":         cfg.Universe.TierBMinQuoteVolume,
			"tier_c_min_volume":               cfg.Universe.TierCMinVolume,
			"default_symbols":                 cfg.Universe.DefaultSymbols,
			"default_hot_boost":               cfg.Universe.DefaultHotBoost,
			"max_hot_boost":                   cfg.Universe.MaxHotBoost,
			"default_min_funding_volume":      cfg.Universe.DefaultMinFundingVolume,
			"chaos_min_funding_volume":        cfg.Universe.ChaosMinFundingVolume,
			"low_vol_min_volume_floor":        cfg.Universe.LowVolMinVolumeFloor,
			"chaos_high_vol_min_volume_floor": cfg.Universe.ChaosHighVolMinVolumeFloor,
			"weight_liquidity_default":        cfg.Universe.WeightLiquidityDefault,
			"weight_activity_default":         cfg.Universe.WeightActivityDefault,
			"weight_hot_default":              cfg.Universe.WeightHotDefault,
			"weight_liquidity_alt":            cfg.Universe.WeightLiquidityAlt,
			"weight_activity_alt":             cfg.Universe.WeightActivityAlt,
			"weight_hot_alt":                  cfg.Universe.WeightHotAlt,
			"weight_liquidity_compression":    cfg.Universe.WeightLiquidityCompression,
			"weight_activity_compression":     cfg.Universe.WeightActivityCompression,
			"weight_hot_compression":          cfg.Universe.WeightHotCompression,
			"weight_liquidity_risk_off":       cfg.Universe.WeightLiquidityRiskOff,
			"weight_activity_risk_off":        cfg.Universe.WeightActivityRiskOff,
			"weight_hot_risk_off":             cfg.Universe.WeightHotRiskOff,
			"weight_liquidity_chaos":          cfg.Universe.WeightLiquidityChaos,
			"weight_activity_chaos":           cfg.Universe.WeightActivityChaos,
			"weight_hot_chaos":                cfg.Universe.WeightHotChaos,
			"weight_liquidity_low_vol":        cfg.Universe.WeightLiquidityLowVol,
			"weight_activity_low_vol":         cfg.Universe.WeightActivityLowVol,
			"weight_hot_low_vol":              cfg.Universe.WeightHotLowVol,
			"weight_liquidity_dominance":      cfg.Universe.WeightLiquidityDominance,
			"weight_activity_dominance":       cfg.Universe.WeightActivityDominance,
			"weight_hot_dominance":            cfg.Universe.WeightHotDominance,
		},
		"strategy": map[string]any{
			"require_ai_high_for_execute":                  cfg.Strategy.RequireAIHighForExecute,
			"require_fresh_entry_for_execute":              cfg.Strategy.RequireFreshEntryForExecute,
			"watch_cooldown_minutes":                       cfg.Strategy.WatchCooldownMinutes,
			"watch_dedup_price_tolerance_bps":              cfg.Strategy.WatchDedupPriceToleranceBps,
			"min_sample_warning":                           cfg.Strategy.EvaluationMinSampleWarning,
			"min_sample_medium":                            cfg.Strategy.EvaluationMinSampleMedium,
			"min_sample_high":                              cfg.Strategy.EvaluationMinSampleHigh,
			"debug_save_raw_klines":                        cfg.Strategy.DebugSaveRawKlines,
			"raw_klines_debug_dir":                         cfg.Strategy.RawKlinesDebugDir,
			"max_marketdata_prefetch_symbols":              cfg.Strategy.MaxMarketDataPrefetchSymbols,
			"scan_request_weight_budget":                   cfg.Strategy.ScanRequestWeightBudget,
			"compression_neutral_breadth_lower":            cfg.Strategy.CompressionNeutralBreadthLower,
			"compression_neutral_breadth_upper":            cfg.Strategy.CompressionNeutralBreadthUpper,
			"compression_max_bb_width":                     cfg.Strategy.CompressionMaxBBWidth,
			"compression_zero_eligible_fallback_threshold": cfg.Strategy.CompressionZeroEligibleFallbackThreshold,
			"broader_volatility_sample_floor":              cfg.Strategy.BroaderVolatilitySampleFloor,
			"funding_extreme_threshold":                    cfg.Strategy.FundingExtremeThreshold,
			"atr_fallback_percent":                         cfg.Strategy.ATRFallbackPercent,
			"min_sl_atr_multiplier_base":                   cfg.Strategy.MinSLATRMultiplierBase,
			"min_sl_atr_multiplier_reversal":               cfg.Strategy.MinSLATRMultiplierReversal,
			"min_sl_atr_multiplier_high_vol":               cfg.Strategy.MinSLATRMultiplierHighVol,
			"rotation_activity_threshold_default":          cfg.Strategy.RotationActivityThresholdDefault,
			"rotation_activity_threshold_alt":              cfg.Strategy.RotationActivityThresholdAlt,
			"rotation_activity_threshold_defensive":        cfg.Strategy.RotationActivityThresholdDefensive,
			"rotation_activity_threshold_low_vol":          cfg.Strategy.RotationActivityThresholdLowVol,
			"rotation_prefetch_ratio_default":              cfg.Strategy.RotationPrefetchRatioDefault,
			"rotation_prefetch_ratio_alt":                  cfg.Strategy.RotationPrefetchRatioAlt,
			"rotation_prefetch_ratio_defensive":            cfg.Strategy.RotationPrefetchRatioDefensive,
			"staleness_policy_scale_base":                  cfg.Strategy.StalenessPolicyScaleBase,
			"staleness_policy_scale_min":                   cfg.Strategy.StalenessPolicyScaleMin,
			"staleness_policy_scale_max":                   cfg.Strategy.StalenessPolicyScaleMax,
			"staleness_late_threshold_multiplier":          cfg.Strategy.StalenessLateThresholdMultiplier,
			"staleness_base_pct_chaos":                     cfg.Strategy.StalenessBasePctChaos,
			"staleness_base_pct_high_vol":                  cfg.Strategy.StalenessBasePctHighVol,
			"staleness_base_pct_tier_c":                    cfg.Strategy.StalenessBasePctTierC,
			"staleness_base_pct_default":                   cfg.Strategy.StalenessBasePctDefault,
		},
		"hot_source": map[string]any{
			"enabled":                 cfg.HotSource.Enabled,
			"request_timeout_seconds": cfg.HotSource.RequestTimeoutSeconds,
			"cache_ttl_seconds":       cfg.HotSource.CacheTTLSeconds,
			"trending_chains":         cfg.HotSource.TrendingChains,
			"social_hype_chains":      cfg.HotSource.SocialHypeChains,
			"smart_money_chains":      cfg.HotSource.SmartMoneyChains,
		},
		"evaluation": map[string]any{
			"enabled":            cfg.Evaluation.Enabled,
			"auto_run":           cfg.Evaluation.AutoRun,
			"auto_apply":         cfg.Evaluation.AutoApply,
			"interval_minutes":   cfg.Evaluation.IntervalMinutes,
			"min_sample_warning": cfg.Evaluation.MinSampleWarning,
			"min_sample_medium":  cfg.Evaluation.MinSampleMedium,
			"min_sample_high":    cfg.Evaluation.MinSampleHigh,
		},
		"binance": map[string]any{
			"base_url":                  cfg.Binance.BaseURL,
			"request_timeout_seconds":   cfg.Binance.RequestTimeoutSeconds,
			"bootstrap_timeout_seconds": cfg.Binance.BootstrapTimeoutSeconds,
			"initial_timeout_seconds":   cfg.Binance.InitialTimeoutSeconds,
			"enrich_timeout_seconds":    cfg.Binance.EnrichTimeoutSeconds,
			"bootstrap_cache_seconds":   cfg.Binance.BootstrapCacheSeconds,
			"max_retry":                 cfg.Binance.MaxRetry,
			"retry_backoff_ms":          cfg.Binance.RetryBackoffMs,
			"api_key_set":               cfg.Binance.APIKey != "",
			"websocket_enabled":         cfg.Binance.WebsocketEnabled,
			"websocket_base_url":        cfg.Binance.WebsocketBaseURL,
			"ws_max_active_symbols":     cfg.Binance.WSMaxActiveSymbols,
			"ws_reconnect_seconds":      cfg.Binance.WSReconnectSeconds,
			"ws_stale_price_seconds":    cfg.Binance.WSStalePriceSeconds,
			"ws_force_restart_hours":    cfg.Binance.WSForceRestartHours,
		},
		"gemini": map[string]any{
			"model":                   cfg.Gemini.Model,
			"request_timeout_seconds": cfg.Gemini.RequestTimeoutSeconds,
			"api_key_set":             cfg.Gemini.APIKey != "",
		},
		"telegram": map[string]any{
			"enabled":                           cfg.Telegram.Enabled,
			"signal_enabled":                    cfg.Telegram.SignalEnabled,
			"status_enabled":                    cfg.Telegram.StatusEnabled,
			"request_timeout_seconds":           cfg.Telegram.RequestTimeoutSeconds,
			"bot_token_set":                     cfg.Telegram.BotToken != "",
			"signal_chat_id_set":                cfg.Telegram.SignalChatID != "" || cfg.Telegram.ChatID != "",
			"status_chat_id_set":                cfg.Telegram.StatusChatID != "",
			"status_allow_signal_chat_fallback": cfg.Telegram.StatusAllowSignalChatFallback,
			"ops_boot_enabled":                  cfg.Telegram.OpsBootEnabled,
			"ops_scan_enabled":                  cfg.Telegram.OpsScanEnabled,
			"ops_admin_enabled":                 cfg.Telegram.OpsAdminEnabled,
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
		"pocketbase": map[string]any{
			"enabled":                 cfg.PocketBase.Enabled,
			"url":                     cfg.PocketBase.URL,
			"token_configured":        strings.TrimSpace(cfg.PocketBase.Token) != "",
			"superuser_configured":    strings.TrimSpace(cfg.PocketBase.SuperuserEmail) != "" && strings.TrimSpace(cfg.PocketBase.SuperuserPassword) != "",
			"admin_configured":        strings.TrimSpace(cfg.PocketBase.AdminEmail) != "" && strings.TrimSpace(cfg.PocketBase.AdminPassword) != "",
			"request_timeout_seconds": cfg.PocketBase.RequestTimeoutSeconds,
			"login_retry_max":         cfg.PocketBase.LoginRetryMax,
		},
		"safety": map[string]any{
			"alert_only":                      cfg.Safety.AlertOnly,
			"binance_readonly":                cfg.Safety.BinanceReadOnly,
			"allow_binance_write":             cfg.Safety.AllowBinanceWrite,
			"disable_binance_order_endpoints": cfg.Safety.DisableBinanceOrderEndpoints,
			"disable_auto_execution":          cfg.Safety.DisableAutoExecution,
			"disable_auto_threshold_apply":    cfg.Safety.DisableAutoThresholdApply,
			"require_ai_high_for_execute":     cfg.Safety.RequireAIHighForExecute,
			"require_fresh_entry_for_execute": cfg.Safety.RequireFreshEntryForExecute,
			"ai_audit_enabled":                cfg.Safety.AIAuditEnabled,
			"decision_audit_enabled":          cfg.Safety.DecisionAuditEnabled,
			"health_storage_check":            cfg.Safety.HealthStorageCheck,
			"health_check_timeout_seconds":    cfg.Safety.HealthCheckTimeoutSeconds,
		},
		"logging": map[string]any{
			"log_level":           cfg.Logging.LogLevel,
			"log_format":          cfg.Logging.LogFormat,
			"log_include_scan_id": cfg.Logging.LogIncludeScanID,
			"log_file_path":       cfg.Logging.LogFilePath,
			"log_max_size_mb":     cfg.Logging.LogMaxSizeMB,
			"log_max_backups":     cfg.Logging.LogMaxBackups,
			"log_max_age_days":    cfg.Logging.LogMaxAgeDays,
			"log_compress":        cfg.Logging.LogCompress,
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

func getEnvFloat(key string, defaultVal float64) float64 {
	if val := os.Getenv(key); val != "" {
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f
		}
	}
	return defaultVal
}

func getEnvIntWithFallback(primaryKey, fallbackKey string, defaultVal int) int {
	if val := os.Getenv(primaryKey); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return getEnvInt(fallbackKey, defaultVal)
}

func getEnvBoolWithFallback(primaryKey, fallbackKey string, defaultVal bool) bool {
	if val := os.Getenv(primaryKey); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			return b
		}
	}
	return getEnvBool(fallbackKey, defaultVal)
}

func getEnvCSV(key string, defaultVal []string) []string {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return append([]string(nil), defaultVal...)
	}
	parts := strings.Split(val, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return append([]string(nil), defaultVal...)
	}
	return out
}
