package dto

import "time"

type ScanRequest struct {
	TriggerTime time.Time `json:"trigger_time"`
}

type ScanResult struct {
	Timestamp time.Time        `json:"timestamp"`
	Duration  string           `json:"duration"`
	Found     int              `json:"found"`
	Signals   []SignalResponse `json:"signals"`
}

type Ticker24h struct {
	Symbol             string  `json:"symbol"`
	PriceChangePercent float64 `json:"price_change_percent"`
	LastPrice          float64 `json:"last_price"`
	Volume             float64 `json:"volume"`
	QuoteVolume        float64 `json:"quote_volume"`
}
