package config

import (
	"os"
	"testing"
)

func TestConfig_LoadDefaultsEmptyEnv(t *testing.T) {
	// Clear relevant env variables to test defaults
	os.Clearenv()

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadConfigFromEnv failed: %v", err)
	}

	// Verify defaults
	if cfg.App.Env != "local" {
		t.Errorf("Expected App.Env=local, got %s", cfg.App.Env)
	}
	if cfg.HTTP.Port != "8080" {
		t.Errorf("Expected HTTP.Port=8080, got %s", cfg.HTTP.Port)
	}
	if !cfg.Scanner.Enabled {
		t.Errorf("Expected Scanner.Enabled=true")
	}
	if cfg.Storage.StoragePath != "storage" {
		t.Errorf("Expected StoragePath=storage, got %s", cfg.Storage.StoragePath)
	}
	if !cfg.Safety.AlertOnly {
		t.Errorf("Expected Safety.AlertOnly=true")
	}
	if cfg.Scanner.CloseCandleBufferSeconds != 3 {
		t.Errorf("Expected CloseCandleBufferSeconds=3, got %d", cfg.Scanner.CloseCandleBufferSeconds)
	}
	if !cfg.Safety.AIAuditEnabled {
		t.Errorf("Expected AIAuditEnabled=true")
	}
	if !cfg.Safety.DecisionAuditEnabled {
		t.Errorf("Expected DecisionAuditEnabled=true")
	}
	if !cfg.Safety.HealthStorageCheck {
		t.Errorf("Expected HealthStorageCheck=true")
	}
	if cfg.Binance.WebsocketEnabled {
		t.Errorf("Expected Binance.WebsocketEnabled=false")
	}
	if cfg.Universe.TierAMinQuoteVolume != 150000000.0 || cfg.Universe.TierBMinQuoteVolume != 50000000.0 {
		t.Errorf("unexpected universe tier defaults: %+v", cfg.Universe)
	}
	if cfg.Universe.DefaultHotBoost != 1.25 || cfg.Universe.MaxHotBoost != 1.5 {
		t.Errorf("unexpected universe hot boost defaults: %+v", cfg.Universe)
	}
	if cfg.Strategy.CompressionMaxBBWidth != 0.10 || cfg.Strategy.FundingExtremeThreshold != 0.003 {
		t.Errorf("unexpected strategy heuristic defaults: %+v", cfg.Strategy)
	}
	if cfg.Strategy.StalenessBasePctDefault != 0.35 || cfg.Strategy.StalenessLateThresholdMultiplier != 1.5 {
		t.Errorf("unexpected staleness defaults: %+v", cfg.Strategy)
	}
	if cfg.Concurrency.MaxMarketDataConcurrency != cfg.Worker.MaxMarketDataConcurrency {
		t.Errorf("expected concurrency mirror to follow worker max market data concurrency")
	}
	if cfg.Monitoring.MaxCandleConcurrency != cfg.Worker.MaxMonitoringCandleConcurrency {
		t.Errorf("expected monitoring max candle concurrency mirror to follow worker")
	}
	if cfg.Monitoring.WatchCooldownMinutes != cfg.Strategy.WatchCooldownMinutes {
		t.Errorf("expected monitoring watch cooldown mirror to follow strategy")
	}
}

func TestConfig_NormalizeCompatibilityConfigAuthoritativeSectionsWin(t *testing.T) {
	t.Setenv("STRATEGY_REQUIRE_AI_HIGH_FOR_EXECUTE", "false")
	t.Setenv("STRATEGY_REQUIRE_FRESH_ENTRY_FOR_EXECUTE", "false")
	t.Setenv("REQUIRE_AI_HIGH_FOR_EXECUTE", "true")
	t.Setenv("REQUIRE_FRESH_ENTRY_FOR_EXECUTE", "true")
	t.Setenv("WORKER_MAX_MARKETDATA_CONCURRENCY", "17")
	t.Setenv("MAX_MARKETDATA_CONCURRENCY", "3")
	t.Setenv("WORKER_MAX_MONITORING_CANDLE_CONCURRENCY", "9")
	t.Setenv("MAX_MONITORING_CANDLE_CONCURRENCY", "2")
	t.Setenv("STRATEGY_WATCH_COOLDOWN_MINUTES", "44")
	t.Setenv("WATCH_COOLDOWN_MINUTES", "11")
	t.Setenv("UNIVERSE_MAX_SYMBOLS_DEFAULT", "66")
	t.Setenv("MAX_SYMBOLS_DEFAULT", "22")
	t.Setenv("MONITORING_MAX_HOLD_MINUTES", "135")
	t.Setenv("MONITORING_MAX_HOLD_M15_CANDLES", "2")
	t.Setenv("SCAN_INTERVAL_MODE", "candle_close")
	t.Setenv("STRATEGY_EVALUATION_MIN_SAMPLE_WARNING", "77")
	t.Setenv("EVALUATION_MIN_SAMPLE_WARNING", "11")

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadConfigFromEnv failed: %v", err)
	}

	if !cfg.Strategy.RequireAIHighForExecute || !cfg.Strategy.RequireFreshEntryForExecute {
		t.Fatalf("expected strategy safety toggles to be normalized from safety layer: %+v", cfg.Strategy)
	}
	if cfg.Concurrency.MaxMarketDataConcurrency != 17 {
		t.Fatalf("expected concurrency mirror to follow worker authoritative value, got %d", cfg.Concurrency.MaxMarketDataConcurrency)
	}
	if cfg.Monitoring.MaxCandleConcurrency != 9 {
		t.Fatalf("expected monitoring concurrency mirror to follow worker authoritative value, got %d", cfg.Monitoring.MaxCandleConcurrency)
	}
	if cfg.Monitoring.WatchCooldownMinutes != 44 {
		t.Fatalf("expected monitoring watch cooldown mirror to follow strategy authoritative value, got %d", cfg.Monitoring.WatchCooldownMinutes)
	}
	if cfg.Concurrency.MaxSymbolsDefault != 66 {
		t.Fatalf("expected concurrency max symbols mirror to follow universe authoritative value, got %d", cfg.Concurrency.MaxSymbolsDefault)
	}
	if cfg.Monitoring.MaxHoldM15Candles != 9 {
		t.Fatalf("expected monitoring max hold candles to mirror authoritative minutes, got %d", cfg.Monitoring.MaxHoldM15Candles)
	}
	if cfg.Scanner.IntervalMode != "m15_close" {
		t.Fatalf("expected scan interval mode alias to normalize to m15_close, got %s", cfg.Scanner.IntervalMode)
	}
	if cfg.Evaluation.MinSampleWarning != 77 {
		t.Fatalf("expected evaluation min sample mirror to follow strategy authoritative value, got %d", cfg.Evaluation.MinSampleWarning)
	}
}

func TestConfig_ValidateConfigNormalizesLegacyMirrors(t *testing.T) {
	cfg, _ := LoadConfigFromEnv()
	cfg.Telegram.Enabled = false
	cfg.Worker.MaxMonitoringCandleConcurrency = 12
	cfg.Worker.MaxMarketDataConcurrency = 13
	cfg.Strategy.WatchCooldownMinutes = 55
	cfg.Universe.MaxSymbolsDefault = 77
	cfg.Safety.RequireAIHighForExecute = true
	cfg.Safety.RequireFreshEntryForExecute = true
	cfg.Strategy.RequireAIHighForExecute = false
	cfg.Strategy.RequireFreshEntryForExecute = false
	cfg.Monitoring.MaxHoldMinutes = 150
	cfg.Monitoring.MaxHoldM15Candles = 1
	cfg.Monitoring.MaxCandleConcurrency = 1
	cfg.Monitoring.WatchCooldownMinutes = 1
	cfg.Concurrency.MaxMarketDataConcurrency = 1
	cfg.Concurrency.MaxSymbolsDefault = 1

	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("expected ValidateConfig to pass after normalizing legacy mirrors, got: %v", err)
	}
	if cfg.Monitoring.MaxCandleConcurrency != 12 {
		t.Fatalf("expected monitoring max candle concurrency to normalize to worker value, got %d", cfg.Monitoring.MaxCandleConcurrency)
	}
	if cfg.Monitoring.WatchCooldownMinutes != 55 {
		t.Fatalf("expected monitoring watch cooldown to normalize to strategy value, got %d", cfg.Monitoring.WatchCooldownMinutes)
	}
	if cfg.Concurrency.MaxMarketDataConcurrency != 13 {
		t.Fatalf("expected concurrency mirror to normalize to worker value, got %d", cfg.Concurrency.MaxMarketDataConcurrency)
	}
	if cfg.Concurrency.MaxSymbolsDefault != 77 {
		t.Fatalf("expected concurrency max symbols mirror to normalize to universe value, got %d", cfg.Concurrency.MaxSymbolsDefault)
	}
	if !cfg.Strategy.RequireAIHighForExecute || !cfg.Strategy.RequireFreshEntryForExecute {
		t.Fatalf("expected strategy safety flags to normalize to true, got %+v", cfg.Strategy)
	}
	if cfg.Monitoring.MaxHoldM15Candles != 10 {
		t.Fatalf("expected monitoring max hold candles to normalize from authoritative minutes, got %d", cfg.Monitoring.MaxHoldM15Candles)
	}
}

func TestConfig_NormalizeCompatibilityConfigDerivesMinutesFromCandles(t *testing.T) {
	cfg, _ := LoadConfigFromEnv()
	cfg.Monitoring.MaxHoldMinutes = 0
	cfg.Monitoring.MaxHoldM15Candles = 6

	normalizeCompatibilityConfig(cfg)

	if cfg.Monitoring.MaxHoldMinutes != 90 {
		t.Fatalf("expected monitoring max hold minutes derived from candle count, got %d", cfg.Monitoring.MaxHoldMinutes)
	}
}

func TestConfig_ValidationRejectsUnsupportedScanIntervalMode(t *testing.T) {
	cfg, _ := LoadConfigFromEnv()
	cfg.Scanner.IntervalMode = "poll_interval"

	if err := ValidateConfig(cfg); err == nil {
		t.Fatalf("expected validation error for unsupported scan interval mode")
	}
}

func TestConfig_ValidationUniverseWeightsAndThresholds(t *testing.T) {
	cfg, _ := LoadConfigFromEnv()
	cfg.Universe.TierAMinQuoteVolume = 40000000.0
	cfg.Universe.TierBMinQuoteVolume = 50000000.0

	if err := ValidateConfig(cfg); err == nil {
		t.Fatalf("expected validation error when tier A threshold <= tier B threshold")
	}

	cfg, _ = LoadConfigFromEnv()
	cfg.Universe.WeightLiquidityDefault = 0.9
	cfg.Universe.WeightActivityDefault = 0.2
	cfg.Universe.WeightHotDefault = 0.2
	if err := ValidateConfig(cfg); err == nil {
		t.Fatalf("expected validation error when universe weights do not sum to 1")
	}
}

func TestConfig_ValidationStrategyStalenessTuning(t *testing.T) {
	cfg, _ := LoadConfigFromEnv()
	cfg.Strategy.StalenessPolicyScaleMin = 1.3
	cfg.Strategy.StalenessPolicyScaleMax = 1.1
	if err := ValidateConfig(cfg); err == nil {
		t.Fatalf("expected validation error when staleness scale min > max")
	}

	cfg, _ = LoadConfigFromEnv()
	cfg.Strategy.StalenessLateThresholdMultiplier = 0
	if err := ValidateConfig(cfg); err == nil {
		t.Fatalf("expected validation error when staleness late multiplier <= 0")
	}
}

func TestConfig_SafeConfigViewIncludesNewRuntimeSurfaces(t *testing.T) {
	cfg, _ := LoadConfigFromEnv()
	view := SafeConfigView(cfg)

	universeMap := view["universe"].(map[string]any)
	if universeMap["tier_a_min_quote_volume"] != cfg.Universe.TierAMinQuoteVolume {
		t.Fatalf("expected safe universe view to expose tier_a_min_quote_volume")
	}
	if universeMap["weight_hot_chaos"] != cfg.Universe.WeightHotChaos {
		t.Fatalf("expected safe universe view to expose chaos hot weight")
	}

	strategyMap := view["strategy"].(map[string]any)
	if strategyMap["funding_extreme_threshold"] != cfg.Strategy.FundingExtremeThreshold {
		t.Fatalf("expected safe strategy view to expose funding_extreme_threshold")
	}
	if strategyMap["staleness_base_pct_default"] != cfg.Strategy.StalenessBasePctDefault {
		t.Fatalf("expected safe strategy view to expose staleness base pct default")
	}
	if _, ok := view["concurrency"]; ok {
		t.Fatalf("expected legacy concurrency section to be hidden from safe config view")
	}
	monitoringMap := view["monitoring"].(map[string]any)
	if _, ok := monitoringMap["watch_cooldown_minutes"]; ok {
		t.Fatalf("expected mirrored monitoring watch fields to be hidden from safe config view")
	}
	geminiMap := view["gemini"].(map[string]any)
	if _, ok := geminiMap["max_concurrency"]; ok {
		t.Fatalf("expected worker-owned gemini concurrency mirror to be hidden from safe config view")
	}
	evaluationMap := view["evaluation"].(map[string]any)
	if _, ok := evaluationMap["min_sample_warning"]; ok {
		t.Fatalf("expected strategy-owned evaluation min sample mirrors to be hidden from safe config view")
	}
	if _, ok := evaluationMap["min_sample_medium"]; ok {
		t.Fatalf("expected strategy-owned evaluation min sample medium mirror to be hidden from safe config view")
	}
	if _, ok := evaluationMap["min_sample_high"]; ok {
		t.Fatalf("expected strategy-owned evaluation min sample high mirror to be hidden from safe config view")
	}
}

func TestConfig_ValidationSafetyAlertOnly(t *testing.T) {
	cfg, _ := LoadConfigFromEnv()
	cfg.Safety.AlertOnly = false // Safety violation

	err := ValidateConfig(cfg)
	if err == nil {
		t.Errorf("Expected validation error for ALERT_ONLY=false, got nil")
	}
}

func TestConfig_ValidationSafetyBinanceReadOnly(t *testing.T) {
	cfg, _ := LoadConfigFromEnv()
	cfg.Safety.BinanceReadOnly = false // Safety violation

	err := ValidateConfig(cfg)
	if err == nil {
		t.Errorf("Expected validation error for BINANCE_READ_ONLY=false, got nil")
	}
}

func TestConfig_ValidationSafetyDisableAutoExecution(t *testing.T) {
	cfg, _ := LoadConfigFromEnv()
	cfg.Safety.DisableAutoExecution = false // Safety violation

	err := ValidateConfig(cfg)
	if err == nil {
		t.Errorf("Expected validation error for DISABLE_AUTO_EXECUTION=false, got nil")
	}
}

func TestConfig_ValidationSafetyDisableBinanceOrderEndpoints(t *testing.T) {
	cfg, _ := LoadConfigFromEnv()
	cfg.Safety.DisableBinanceOrderEndpoints = false // Safety violation

	err := ValidateConfig(cfg)
	if err == nil {
		t.Errorf("Expected validation error for DISABLE_BINANCE_ORDER_ENDPOINTS=false, got nil")
	}
}

func TestConfig_ValidationSafetyDisableAutoThresholdApply(t *testing.T) {
	cfg, _ := LoadConfigFromEnv()
	cfg.Safety.DisableAutoThresholdApply = false
	if err := ValidateConfig(cfg); err == nil {
		t.Errorf("Expected validation error for DISABLE_AUTO_THRESHOLD_APPLY=false, got nil")
	}
}

func TestConfig_ValidationSafetyRequireAIHighForExecute(t *testing.T) {
	cfg, _ := LoadConfigFromEnv()
	cfg.Safety.RequireAIHighForExecute = false
	if err := ValidateConfig(cfg); err == nil {
		t.Errorf("Expected validation error for REQUIRE_AI_HIGH_FOR_EXECUTE=false, got nil")
	}
}

func TestConfig_ValidationSafetyRequireFreshEntryForExecute(t *testing.T) {
	cfg, _ := LoadConfigFromEnv()
	cfg.Safety.RequireFreshEntryForExecute = false
	if err := ValidateConfig(cfg); err == nil {
		t.Errorf("Expected validation error for REQUIRE_FRESH_ENTRY_FOR_EXECUTE=false, got nil")
	}
}

func TestConfig_ValidationSafetyAllowBinanceWriteMustBeFalse(t *testing.T) {
	cfg, _ := LoadConfigFromEnv()
	cfg.Safety.AllowBinanceWrite = true
	if err := ValidateConfig(cfg); err == nil {
		t.Errorf("Expected validation error for ALLOW_BINANCE_WRITE=true, got nil")
	}
}

func TestConfig_ValidationSafetyEvaluationAutoApplyMustBeFalse(t *testing.T) {
	cfg, _ := LoadConfigFromEnv()
	cfg.Evaluation.AutoApply = true
	if err := ValidateConfig(cfg); err == nil {
		t.Errorf("Expected validation error for EVALUATION_AUTO_APPLY=true, got nil")
	}
}

func TestConfig_SafeConfigViewRedaction(t *testing.T) {
	cfg := &Config{}
	cfg.Binance.APIKey = "binance-secret-key"
	cfg.Binance.APISecret = "binance-secret"
	cfg.Gemini.APIKey = "gemini-secret-key"
	cfg.Telegram.BotToken = "telegram-secret-token"
	cfg.Telegram.ChatID = "telegram-chat-id"

	view := SafeConfigView(cfg)

	// Check that secrets are not present
	for _, secKey := range []string{"api_key", "api_secret", "bot_token", "chat_id"} {
		if _, ok := view[secKey]; ok {
			t.Errorf("Sensitive key %s exposed at root level of SafeConfigView", secKey)
		}
	}

	binMap := view["binance"].(map[string]any)
	if _, ok := binMap["api_key"]; ok {
		t.Errorf("Sensitive key api_key exposed inside binance view")
	}
	if _, ok := binMap["api_secret"]; ok {
		t.Errorf("Sensitive key api_secret exposed inside binance view")
	}
	if binMap["api_key_set"] != true {
		t.Errorf("Expected api_key_set to be true indicators")
	}

	gemMap := view["gemini"].(map[string]any)
	if _, ok := gemMap["api_key"]; ok {
		t.Errorf("Sensitive key api_key exposed inside gemini view")
	}

	tgMap := view["telegram"].(map[string]any)
	if _, ok := tgMap["bot_token"]; ok {
		t.Errorf("Sensitive key bot_token exposed inside telegram view")
	}
	if _, ok := tgMap["chat_id"]; ok {
		t.Errorf("Sensitive key chat_id exposed inside telegram view")
	}

	safetyMap := view["safety"].(map[string]any)
	if safetyMap["ai_audit_enabled"] != cfg.Safety.AIAuditEnabled {
		t.Errorf("Expected safety view ai_audit_enabled equal to struct")
	}
	if safetyMap["decision_audit_enabled"] != cfg.Safety.DecisionAuditEnabled {
		t.Errorf("Expected safety view decision_audit_enabled equal to struct")
	}
	if safetyMap["health_storage_check"] != cfg.Safety.HealthStorageCheck {
		t.Errorf("Expected safety view health_storage_check equal to struct")
	}
}

func TestConfig_ValidationConcurrencyLimits(t *testing.T) {
	cfg, _ := LoadConfigFromEnv()
	cfg.Gemini.MaxConcurrency = 0

	err := ValidateConfig(cfg)
	if err == nil {
		t.Errorf("Expected validation error for MaxConcurrency < 1, got nil")
	}
}

func TestConfig_ValidationEmptyStoragePath(t *testing.T) {
	cfg, _ := LoadConfigFromEnv()
	cfg.Storage.StoragePath = ""

	err := ValidateConfig(cfg)
	if err == nil {
		t.Errorf("Expected validation error for empty StoragePath, got nil")
	}
}

func TestConfig_TelegramBackwardCompatChatIDActsAsSignalChatID(t *testing.T) {
	t.Setenv("TELEGRAM_CHAT_ID", "123")
	t.Setenv("TELEGRAM_SIGNAL_CHAT_ID", "")

	cfg, _ := LoadConfigFromEnv()
	cfg.Telegram.Enabled = true
	cfg.Telegram.SignalEnabled = true
	cfg.Telegram.BotToken = "token"

	if cfg.Telegram.SignalChatID == "" {
		t.Fatalf("expected SignalChatID to be derived from ChatID for backward compatibility")
	}
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("expected telegram backward compat config to validate, got: %v", err)
	}
}

func TestConfig_TelegramStatusMissingChatIDDisablesStatus(t *testing.T) {
	cfg, _ := LoadConfigFromEnv()
	cfg.Telegram.Enabled = true
	cfg.Telegram.SignalEnabled = false
	cfg.Telegram.StatusEnabled = true
	cfg.Telegram.BotToken = "token"
	cfg.Telegram.StatusChatID = ""
	cfg.Telegram.StatusAllowSignalChatFallback = false

	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg.Telegram.StatusEnabled {
		t.Fatalf("expected StatusEnabled to be disabled when StatusChatID is empty and fallback is false")
	}
}

func TestConfig_ProductionDisallowsAIDisabledWhenAIHighRequired(t *testing.T) {
	cfg, _ := LoadConfigFromEnv()
	cfg.App.Env = "production"
	cfg.Safety.AIAuditEnabled = false
	cfg.Safety.RequireAIHighForExecute = true
	if err := ValidateConfig(cfg); err == nil {
		t.Fatalf("expected validation error in production when AI audit disabled but AI HIGH required")
	}
}

func TestConfig_BinanceWebsocketEnvParsingAndValidation(t *testing.T) {
	t.Setenv("TELEGRAM_ENABLED", "false")
	t.Setenv("BINANCE_WS_ENABLED", "true")
	t.Setenv("BINANCE_WS_BASE_URL", "wss://fstream.binance.com")
	t.Setenv("BINANCE_WS_MAX_ACTIVE_SYMBOLS", "25")
	t.Setenv("BINANCE_WS_RECONNECT_SECONDS", "4")
	t.Setenv("BINANCE_WS_STALE_PRICE_SECONDS", "12")
	t.Setenv("BINANCE_WS_FORCE_RESTART_HOURS", "22")

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadConfigFromEnv failed: %v", err)
	}
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("ValidateConfig failed: %v", err)
	}
	if !cfg.Binance.WebsocketEnabled || cfg.Binance.WSMaxActiveSymbols != 25 {
		t.Fatalf("unexpected websocket config: %+v", cfg.Binance)
	}
}
