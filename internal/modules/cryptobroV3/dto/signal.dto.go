package dto

import "time"

type SignalResponse struct {
	Symbol         string    `json:"symbol"`
	Direction      string    `json:"direction"` // BUY, SELL, HOLD
	Timeframe      string    `json:"timeframe"` // M15, H1, H4
	TriggerPrice   float64   `json:"trigger_price"`
	StopLoss       float64   `json:"stop_loss"`
	TakeProfit     float64   `json:"take_profit"`
	Score          float64   `json:"score"`
	Strategy       string    `json:"strategy"`
	AISentiment    string    `json:"ai_sentiment"`
	IsFinalExecute bool      `json:"is_final_execute"`
	ReconciledTime time.Time `json:"reconciled_time"`
	Status         string    `json:"status"` // PENDING, ACTIVE, COMPLETED, CANCELLED
}
