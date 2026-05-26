package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
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
	return t.send(ctx, t.signalChatID, msg, "HTML")
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
	return t.send(ctx, chatID, msg, "HTML")
}

type TelegramAPIError struct {
	StatusCode  int
	ErrorCode   int
	Description string
}

func (e *TelegramAPIError) Error() string {
	return fmt.Sprintf("telegram api returned non-ok status: %d (error_code: %d, description: %s)", e.StatusCode, e.ErrorCode, e.Description)
}

func (e *TelegramAPIError) GetStatusCode() int {
	return e.StatusCode
}

func (e *TelegramAPIError) GetErrorCode() int {
	return e.ErrorCode
}

func (e *TelegramAPIError) GetDescription() string {
	return e.Description
}

type TelegramErrorResponse struct {
	Ok          bool   `json:"ok"`
	ErrorCode   int    `json:"error_code"`
	Description string `json:"description"`
}

func isEntityParseError(err error) bool {
	if err == nil {
		return false
	}
	if apiErr, ok := err.(*TelegramAPIError); ok {
		return apiErr.StatusCode == 400 && strings.Contains(strings.ToLower(apiErr.Description), "can't parse entities")
	}
	return strings.Contains(strings.ToLower(err.Error()), "can't parse entities")
}

func (t *TelegramService) sanitizeErr(err error) error {
	if err == nil {
		return nil
	}
	if apiErr, ok := err.(*TelegramAPIError); ok {
		return apiErr
	}
	errStr := err.Error()
	if t.token != "" {
		errStr = strings.ReplaceAll(errStr, t.token, "[REDACTED_TOKEN]")
	}
	return fmt.Errorf("%s", errStr)
}

func (t *TelegramService) send(ctx context.Context, chatID string, msg string, parseMode string) error {
	err := t.sendRequest(ctx, chatID, msg, parseMode)
	if err != nil {
		if parseMode != "" && isEntityParseError(err) {
			slog.Warn("Telegram HTML parse failed, trying plain text fallback...", "chat_id", chatID, "error", t.sanitizeErr(err).Error())
			fallbackErr := t.sendRequest(ctx, chatID, msg, "")
			if fallbackErr == nil {
				slog.Info("Telegram plain text fallback message sent successfully", "chat_id", chatID)
				return nil
			}
			return t.sanitizeErr(fallbackErr)
		}
		return t.sanitizeErr(err)
	}
	return nil
}

func (t *TelegramService) sendRequest(ctx context.Context, chatID string, msg string, parseMode string) error {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.token)
	data := url.Values{}
	data.Set("chat_id", chatID)
	data.Set("text", msg)
	if parseMode != "" {
		data.Set("parse_mode", parseMode)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, nil)
	if err != nil {
		return err
	}
	req.URL.RawQuery = data.Encode()

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send telegram request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp TelegramErrorResponse
		bodyBytes, readErr := io.ReadAll(resp.Body)
		if readErr == nil {
			_ = json.Unmarshal(bodyBytes, &errResp)
		}

		slog.Error("Telegram API error",
			"status_code", resp.StatusCode,
			"telegram_error_code", errResp.ErrorCode,
			"telegram_description", errResp.Description,
			"message_length", len(msg),
			"parse_mode", parseMode,
		)

		return &TelegramAPIError{
			StatusCode:  resp.StatusCode,
			ErrorCode:   errResp.ErrorCode,
			Description: errResp.Description,
		}
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
