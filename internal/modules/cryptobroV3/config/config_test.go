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
