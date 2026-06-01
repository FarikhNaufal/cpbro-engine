package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type PocketBaseAuthMode string

const (
	PocketBaseAuthModeToken     PocketBaseAuthMode = "token"
	PocketBaseAuthModeSuperuser PocketBaseAuthMode = "superuser"
	PocketBaseAuthModeAdmin     PocketBaseAuthMode = "admin"
)

type PocketBaseClient struct {
	baseURL    string
	httpClient *http.Client

	authMode PocketBaseAuthMode
	token    string
	identity string
	password string

	loginRetryMax int

	mu sync.Mutex
}

type pocketBaseAuthResponse struct {
	Token string `json:"token"`
}

func NewPocketBaseClient(baseURL string, timeout time.Duration, authMode PocketBaseAuthMode, token, identity, password string) (*PocketBaseClient, error) {
	return NewPocketBaseClientWithHTTPClient(baseURL, nil, timeout, authMode, token, identity, password, 1)
}

func NewPocketBaseClientWithHTTPClient(baseURL string, httpClient *http.Client, timeout time.Duration, authMode PocketBaseAuthMode, token, identity, password string, loginRetryMax int) (*PocketBaseClient, error) {
	if strings.TrimSpace(baseURL) == "" {
		return nil, errors.New("pocketbase baseURL is empty")
	}
	_, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid pocketbase baseURL: %w", err)
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if strings.TrimSpace(token) == "" && (authMode == PocketBaseAuthModeToken) {
		return nil, errors.New("pocketbase auth mode token requires token")
	}
	if (authMode == PocketBaseAuthModeSuperuser || authMode == PocketBaseAuthModeAdmin) && (strings.TrimSpace(identity) == "" || strings.TrimSpace(password) == "") {
		return nil, errors.New("pocketbase auth mode requires identity and password")
	}
	if loginRetryMax < 0 {
		return nil, errors.New("pocketbase loginRetryMax must be >= 0")
	}
	if loginRetryMax > 3 {
		loginRetryMax = 3
	}

	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	} else if httpClient.Timeout == 0 {
		httpClient.Timeout = timeout
	}

	return &PocketBaseClient{
		baseURL:       baseURL,
		httpClient:    httpClient,
		authMode:      authMode,
		token:         token,
		identity:      identity,
		password:      password,
		loginRetryMax: loginRetryMax,
	}, nil
}

func (c *PocketBaseClient) ensureToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if strings.TrimSpace(c.token) != "" {
		return c.token, nil
	}
	switch c.authMode {
	case PocketBaseAuthModeToken:
		return c.token, nil
	case PocketBaseAuthModeSuperuser:
		return c.login(ctx, "/api/collections/_superusers/auth-with-password")
	case PocketBaseAuthModeAdmin:
		return c.login(ctx, "/api/admins/auth-with-password")
	default:
		return "", fmt.Errorf("unknown pocketbase auth mode: %s", c.authMode)
	}
}

func (c *PocketBaseClient) clearToken() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.authMode != PocketBaseAuthModeToken {
		c.token = ""
	}
}

func (c *PocketBaseClient) login(ctx context.Context, path string) (string, error) {
	body := map[string]any{
		"identity": c.identity,
		"password": c.password,
	}
	b, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.baseURL, "/")+path, bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("pocketbase auth failed: status=%d body=%s", resp.StatusCode, sanitizePBBody(raw))
	}

	var out pocketBaseAuthResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("pocketbase auth parse failed: %w", err)
	}
	if strings.TrimSpace(out.Token) == "" {
		return "", errors.New("pocketbase auth returned empty token")
	}
	c.token = out.Token
	return c.token, nil
}

func (c *PocketBaseClient) doJSON(ctx context.Context, method, path string, query url.Values, body any, out any) error {
	u := strings.TrimRight(c.baseURL, "/") + path
	if query != nil && len(query) > 0 {
		u = u + "?" + query.Encode()
	}

	var bodyBytes []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		bodyBytes = b
	}

	maxAttempts := 1 + c.loginRetryMax
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		var r io.Reader
		if bodyBytes != nil {
			r = bytes.NewReader(bodyBytes)
		}

		req, err := http.NewRequestWithContext(ctx, method, u, r)
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")

		token, err := c.ensureToken(ctx)
		if err != nil {
			return err
		}
		if strings.TrimSpace(token) != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return err
		}
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			if attempt < maxAttempts && c.authMode != PocketBaseAuthModeToken {
				c.clearToken()
				time.Sleep(time.Duration(attempt*100) * time.Millisecond)
				continue
			}
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("pocketbase request failed: method=%s path=%s status=%d body=%s", method, path, resp.StatusCode, sanitizePBBody(raw))
		}

		if out == nil || len(raw) == 0 {
			return nil
		}
		return json.Unmarshal(raw, out)
	}

	return errors.New("pocketbase request failed after auth retries")
}

func sanitizePBBody(b []byte) string {
	// Best-effort truncation to avoid logging secrets.
	s := strings.TrimSpace(string(b))
	if len(s) > 300 {
		s = s[:300] + "...(truncated)"
	}
	return s
}
