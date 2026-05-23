package service

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
)

type TelegramService struct {
	enabled bool
	token  string
	chatID string
	client *http.Client
}

func NewTelegramService(enabled bool) *TelegramService {
	return &TelegramService{
		enabled: enabled,
		token:  os.Getenv("TELEGRAM_BOT_TOKEN"),
		chatID: os.Getenv("TELEGRAM_CHAT_ID"),
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// SendFinalExecuteAlert broadcasts a signal after all gates are satisfied.
// This is the ONLY layer allowed to send signals to Telegram.
func (t *TelegramService) SendFinalExecuteAlert(ctx context.Context, signal dto.SignalResponse) error {
	if !signal.IsFinalExecute {
		return fmt.Errorf("refusing to send Telegram alert: signal must have IsFinalExecute = true")
	}

	if !t.enabled {
		return nil
	}
	if t.token == "" || t.chatID == "" {
		// Log or return early if not configured
		return nil
	}

	message := fmt.Sprintf(
		"🔔 *FINAL EXECUTE SIGNAL*\n\n"+
			"*Symbol:* %s\n"+
			"*Direction:* %s\n"+
			"*Strategy:* %s\n"+
			"*Trigger Price:* %.4f\n"+
			"*Take Profit:* %.4f\n"+
			"*Stop Loss:* %.4f\n"+
			"*Quant Score:* %.2f\n"+
			"*AI Sentiment:* %s\n"+
			"*Time:* %s",
		signal.Symbol,
		signal.Direction,
		signal.Strategy,
		signal.TriggerPrice,
		signal.TakeProfit,
		signal.StopLoss,
		signal.Score,
		signal.AISentiment,
		signal.ReconciledTime.Format(time.RFC3339),
	)

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.token)
	data := url.Values{}
	data.Set("chat_id", t.chatID)
	data.Set("text", message)
	data.Set("parse_mode", "Markdown")

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
		return fmt.Errorf("telegram api returned non-ok status: %d", resp.StatusCode)
	}

	return nil
}

func (t *TelegramService) SendTelegramMessage(ctx context.Context, msg string) error {
	if !t.enabled {
		return nil
	}
	if t.token == "" || t.chatID == "" {
		return nil
	}

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.token)
	data := url.Values{}
	data.Set("chat_id", t.chatID)
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

// Ping queries getMe endpoint of the Telegram Bot API to check connectivity and validation.
func (t *TelegramService) Ping(ctx context.Context) error {
	if t.token == "" {
		return fmt.Errorf("TELEGRAM_BOT_TOKEN is empty")
	}
	if !t.enabled {
		return fmt.Errorf("TELEGRAM_ENABLED is false")
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
