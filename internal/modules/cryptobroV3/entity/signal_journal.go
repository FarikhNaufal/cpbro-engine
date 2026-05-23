package entity

import "time"

type SignalJournal struct {
	ID            string    `json:"id"`
	Symbol        string    `json:"symbol"`
	Direction     string    `json:"direction"` // BUY, SELL, HOLD
	Timeframe     string    `json:"timeframe"` // M15, H1, H4
	EntryPrice    float64   `json:"entry_price"`
	StopLoss      float64   `json:"stop_loss"`
	TakeProfit    float64   `json:"take_profit"`
	LatestPrice   float64   `json:"latest_price"`
	Status        string    `json:"status"` // ACTIVE, TP_HIT, SL_HIT, STALE, MANUAL_CLOSED
	QuantScore    float64   `json:"quant_score"`
	AISentiment   string    `json:"ai_sentiment"`
	AIReasoning   string    `json:"ai_reasoning"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	ClosedAt      time.Time `json:"closed_at,omitempty"`
	PnlPercentage float64   `json:"pnl_percentage"`
}
