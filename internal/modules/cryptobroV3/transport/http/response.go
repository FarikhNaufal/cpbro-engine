package http

// APIResponse defines a consistent payload structure for all HTTP endpoints
type APIResponse struct {
	Success bool     `json:"success" example:"true"`
	Message string   `json:"message" example:"ok"`
	Data    any      `json:"data,omitempty"`
	Errors  []string `json:"errors,omitempty"`
}
