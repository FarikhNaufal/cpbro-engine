package http

import (
	"net/http"
	"strings"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
)

// APIResponse defines a consistent payload structure for all HTTP endpoints
type APIResponse = dto.APIResponse

func ok(message string, data any) APIResponse {
	if message == "" {
		message = "ok"
	}
	return APIResponse{
		Success: true,
		Message: message,
		Data:    data,
		Errors:  []string{},
	}
}

func fail(message string, errs ...string) APIResponse {
	if message == "" {
		message = "error"
	}
	resp := APIResponse{
		Success: false,
		Message: message,
		Data:    nil,
		Errors:  []string{},
	}
	for _, e := range errs {
		if e == "" {
			continue
		}
		resp.Errors = append(resp.Errors, e)
	}
	if resp.Errors == nil {
		resp.Errors = []string{}
	}
	return resp
}

func statusFor(resp APIResponse, defaultStatus int) int {
	if resp.Success {
		return defaultStatus
	}
	if defaultStatus != http.StatusOK {
		return defaultStatus
	}
	return http.StatusInternalServerError
}

func sanitizeErr(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.TrimSpace(s)
	if len(s) > 300 {
		return s[:300] + "…"
	}
	return s
}
