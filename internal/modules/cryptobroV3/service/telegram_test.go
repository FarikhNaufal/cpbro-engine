package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type mockRoundTripper struct {
	roundTripFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.roundTripFunc(req)
}

func TestTelegramService_SendSignalMessage_FallbackOnParseError(t *testing.T) {
	callCount := 0
	rt := &mockRoundTripper{
		roundTripFunc: func(req *http.Request) (*http.Response, error) {
			callCount++
			if callCount == 1 {
				// Verify parseMode was HTML
				query := req.URL.Query()
				if query.Get("parse_mode") != "HTML" {
					t.Errorf("expected parse_mode HTML on first call, got %s", query.Get("parse_mode"))
				}
				// Return 400 Entity Parse Error
				body := `{"ok":false,"error_code":400,"description":"Bad Request: can't parse entities in message text"}`
				return &http.Response{
					StatusCode: http.StatusBadRequest,
					Body:       io.NopCloser(bytes.NewBufferString(body)),
					Header:     make(http.Header),
				}, nil
			}
			// Fallback call should have empty parseMode (plain text)
			query := req.URL.Query()
			if query.Get("parse_mode") != "" {
				t.Errorf("expected empty parse_mode on fallback call, got %s", query.Get("parse_mode"))
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(`{"ok":true}`)),
				Header:     make(http.Header),
			}, nil
		},
	}

	cfg := TelegramConfig{
		Enabled:       true,
		SignalEnabled: true,
		BotToken:      "123456:fake_token_for_testing_purposes_12345",
		SignalChatID:  "-10012345678",
	}
	svc := NewTelegramService(cfg)
	svc.client.Transport = rt

	err := svc.SendSignalMessage(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("expected SendSignalMessage to succeed with fallback, got err: %v", err)
	}

	if callCount != 2 {
		t.Errorf("expected 2 HTTP calls (original + fallback), got %d", callCount)
	}
}

func TestTelegramService_Sanitization(t *testing.T) {
	token := "123456:fake_token_for_testing_purposes_12345"
	rt := &mockRoundTripper{
		roundTripFunc: func(req *http.Request) (*http.Response, error) {
			// Return network error containing the bot token
			return nil, errors.New("connection failed to api.telegram.org/bot" + token + "/sendMessage")
		},
	}

	cfg := TelegramConfig{
		Enabled:       true,
		SignalEnabled: true,
		BotToken:      token,
		SignalChatID:  "-10012345678",
	}
	svc := NewTelegramService(cfg)
	svc.client.Transport = rt

	err := svc.SendSignalMessage(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected connection error, got nil")
	}

	errStr := err.Error()
	if strings.Contains(errStr, token) {
		t.Errorf("leak detected! error string contains bot token: %s", errStr)
	}
	if !strings.Contains(errStr, "[REDACTED_TOKEN]") {
		t.Errorf("expected error string to contain [REDACTED_TOKEN], got: %s", errStr)
	}
}

func TestTelegramAPIError_Getters(t *testing.T) {
	apiErr := &TelegramAPIError{
		StatusCode:  400,
		ErrorCode:   400,
		Description: "can't parse entities",
	}

	if apiErr.GetStatusCode() != 400 {
		t.Errorf("expected StatusCode 400, got %d", apiErr.GetStatusCode())
	}
	if apiErr.GetErrorCode() != 400 {
		t.Errorf("expected ErrorCode 400, got %d", apiErr.GetErrorCode())
	}
	if apiErr.GetDescription() != "can't parse entities" {
		t.Errorf("expected Description 'can't parse entities', got %s", apiErr.GetDescription())
	}
}
