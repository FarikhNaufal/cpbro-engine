package dto

type APIResponse struct {
	Success bool     `json:"success"`
	Message string   `json:"message"`
	Data    any      `json:"data"`
	Errors  []string `json:"errors"`
}

type HealthAPIResponse struct {
	Success bool           `json:"success"`
	Message string         `json:"message"`
	Data    HealthResponse `json:"data"`
	Errors  []string       `json:"errors"`
}

type LatestAPIResponse struct {
	Success bool                 `json:"success"`
	Message string               `json:"message"`
	Data    LatestResultResponse `json:"data"`
	Errors  []string             `json:"errors"`
}

type JournalAPIResponse struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    JournalResponse `json:"data"`
	Errors  []string        `json:"errors"`
}

type EvaluationAPIResponse struct {
	Success bool               `json:"success"`
	Message string             `json:"message"`
	Data    EvaluationResponse `json:"data"`
	Errors  []string           `json:"errors"`
}

type DecisionAuditAPIResponse struct {
	Success bool                  `json:"success"`
	Message string                `json:"message"`
	Data    DecisionAuditResponse `json:"data"`
	Errors  []string              `json:"errors"`
}

type EmptyArrayResponse struct {
	Success bool     `json:"success"`
	Message string   `json:"message"`
	Data    []any    `json:"data"`
	Errors  []string `json:"errors"`
}

type ErrorAPIResponse struct {
	Success bool     `json:"success"`
	Message string   `json:"message"`
	Data    any      `json:"data"`
	Errors  []string `json:"errors"`
}
