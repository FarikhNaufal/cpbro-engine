package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPocketBaseClient_RetryReLoginOn401(t *testing.T) {
	loginCount := 0
	resourceCount := 0

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/collections/_superusers/auth-with-password":
			loginCount++
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "newtoken"})
			return
		case r.Method == http.MethodGet && r.URL.Path == "/api/collections/signal_journals/records":
			resourceCount++
			auth := r.Header.Get("Authorization")
			// First attempt with stale token -> 401, then must retry after relogin.
			if auth == "Bearer staletoken" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			if auth != "Bearer newtoken" {
				http.Error(w, "bad token", http.StatusForbidden)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"page":       1,
				"perPage":    1,
				"totalItems": 0,
				"totalPages": 1,
				"items":      []any{},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	})

	httpClient := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, r)
			return rr.Result(), nil
		}),
	}

	c, err := NewPocketBaseClientWithHTTPClient("http://pocketbase.local", httpClient, 2*time.Second, PocketBaseAuthModeSuperuser, "staletoken", "admin@example.com", "pass", 1)
	if err != nil {
		t.Fatalf("NewPocketBaseClientWithHTTPClient: %v", err)
	}

	var out map[string]any
	err = c.doJSON(
		context.Background(),
		http.MethodGet,
		"/api/collections/signal_journals/records",
		nil,
		nil,
		&out,
	)
	if err != nil {
		t.Fatalf("doJSON: %v", err)
	}
	if loginCount != 1 {
		t.Fatalf("expected 1 login, got %d", loginCount)
	}
	if resourceCount < 2 {
		t.Fatalf("expected at least 2 resource calls (401 then retry), got %d", resourceCount)
	}
}

func TestPocketBaseClient_NoRetryWhenRetryMaxZero(t *testing.T) {
	loginCount := 0
	resourceCount := 0

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "auth-with-password"):
			loginCount++
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "newtoken"})
			return
		case r.Method == http.MethodGet && r.URL.Path == "/api/collections/signal_journals/records":
			resourceCount++
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		default:
			http.NotFound(w, r)
			return
		}
	})

	httpClient := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, r)
			return rr.Result(), nil
		}),
	}

	c, err := NewPocketBaseClientWithHTTPClient("http://pocketbase.local", httpClient, 2*time.Second, PocketBaseAuthModeSuperuser, "staletoken", "admin@example.com", "pass", 0)
	if err != nil {
		t.Fatalf("NewPocketBaseClientWithHTTPClient: %v", err)
	}

	err = c.doJSON(context.Background(), http.MethodGet, "/api/collections/signal_journals/records", nil, nil, nil)
	if err == nil {
		t.Fatalf("expected error")
	}
	if loginCount != 0 {
		t.Fatalf("expected 0 login (no retry), got %d", loginCount)
	}
	if resourceCount != 1 {
		t.Fatalf("expected 1 resource call, got %d", resourceCount)
	}
}
