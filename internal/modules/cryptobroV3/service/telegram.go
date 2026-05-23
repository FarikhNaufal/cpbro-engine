package service

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type TelegramService struct {
	enabled bool

	signalEnabled bool
	statusEnabled bool

	token string

	signalChatID string
	statusChatID string

	statusAllowSignalChatFallback bool

	client *http.Client
}

type TelegramConfig struct {
	Enabled                       bool
	SignalEnabled                 bool
	StatusEnabled                 bool
	BotToken                      string
	SignalChatID                  string
	StatusChatID                  string
	StatusAllowSignalChatFallback bool
	RequestTimeoutSeconds         int
}

func NewTelegramService(cfg TelegramConfig) *TelegramService {
	timeout := 10 * time.Second
	if cfg.RequestTimeoutSeconds > 0 {
		timeout = time.Duration(cfg.RequestTimeoutSeconds) * time.Second
	}
	return &TelegramService{
		enabled:                       cfg.Enabled,
		signalEnabled:                 cfg.SignalEnabled,
		statusEnabled:                 cfg.StatusEnabled,
		token:                         strings.TrimSpace(cfg.BotToken),
		signalChatID:                  strings.TrimSpace(cfg.SignalChatID),
		statusChatID:                  strings.TrimSpace(cfg.StatusChatID),
		statusAllowSignalChatFallback: cfg.StatusAllowSignalChatFallback,
		client:                        &http.Client{Timeout: timeout},
	}
}

func (t *TelegramService) SendSignalMessage(ctx context.Context, msg string) error {
	if !t.enabled || !t.signalEnabled {
		return nil
	}
	if t.token == "" {
		return nil
	}
	if t.signalChatID == "" {
		return nil
	}
	return t.send(ctx, t.signalChatID, msg)
}

func (t *TelegramService) SendOpsMessage(ctx context.Context, msg string) error {
	if !t.enabled || !t.statusEnabled {
		return nil
	}
	if t.token == "" {
		return nil
	}

	chatID := t.statusChatID
	if chatID == "" {
		if !t.statusAllowSignalChatFallback {
			return nil
		}
		chatID = t.signalChatID
	}
	if chatID == "" {
		return nil
	}
	return t.send(ctx, chatID, msg)
}

func (t *TelegramService) send(ctx context.Context, chatID string, msg string) error {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.token)
	data := url.Values{}
	data.Set("chat_id", chatID)
	data.Set("text", msg)
	data.Set("parse_mode", "Markdown")

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, nil)
	if err != nil {
		return err
	}
	req.URL.RawQuery = data.Encode()

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send telegram message request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram api returned non-ok status: %d", resp.StatusCode)
	}
	return nil
}

// Ping checks Telegram Bot API availability. Used by health endpoint.
func (t *TelegramService) Ping(ctx context.Context) error {
	if !t.enabled {
		return fmt.Errorf("TELEGRAM_ENABLED is false")
	}
	if t.token == "" {
		return fmt.Errorf("TELEGRAM_BOT_TOKEN is empty")
	}
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/getMe", t.token)
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return err
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram getMe returned status: %d", resp.StatusCode)
	}
	return nil
}
