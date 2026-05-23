package dto

import "time"

type AIAuditRequest struct {
	Symbol            string    `json:"symbol"`
	Direction         string    `json:"direction"`
	Playbook          string    `json:"playbook"`
	SetupType         string    `json:"setup_type"`
	QuantScore        float64   `json:"quant_score"`
	PolicySummary     string    `json:"policy_summary"`
	H4Trend           string    `json:"h4_trend"`
	H1Trend           string    `json:"h1_trend"`
	M15H1Structure    string    `json:"m15_h1_structure"`
	SupportResistance string    `json:"support_resistance"`
	BotEntry          float64   `json:"bot_entry"`
	BotSL             float64   `json:"bot_sl"`
	BotTP             float64   `json:"bot_tp"`
	BotRR             float64   `json:"bot_rr"`
	RsiMfiAdxAtr      string    `json:"rsi_mfi_adx_atr"`
	M15Candles        []Candle  `json:"m15_candles"`
	H1Candles         []Candle  `json:"h1_candles"`
	H4Candles         []Candle  `json:"h4_candles"`
	MarketRegime      string    `json:"market_regime"`
	Timestamp         time.Time `json:"timestamp"`
}

type Candle struct {
	Time  time.Time `json:"time"`
	Open  float64   `json:"open"`
	High  float64   `json:"high"`
	Low   float64   `json:"low"`
	Close float64   `json:"close"`
	Vol   float64   `json:"vol"`
}

type AIAuditResponse struct {
	Symbol              string    `json:"symbol"`
	Sentiment           string    `json:"sentiment"` // BULLISH, BEARISH, NEUTRAL
	ConfidenceScore     float64   `json:"confidence_score"`
	Reasoning           string    `json:"reasoning"`
	SuggestedStopLoss   float64   `json:"suggested_stop_loss"`
	SuggestedTakeProfit float64   `json:"suggested_take_profit"`
	IsApproved          bool      `json:"is_approved"`
	AuditedAt           time.Time `json:"audited_at"`

	// Strictly mapped narrative fields
	Decision         string `json:"decision"`
	Confidence       string `json:"confidence"`
	CandleNarrative  string `json:"candle_narrative"`
	Last5CandlesBias string `json:"last_5_candles_bias"`
	HasRejection     bool   `json:"has_rejection"`
	HasConfirmation  bool   `json:"has_confirmation"`
	EntryTiming      string `json:"entry_timing"`
	ConflictWithBot  bool   `json:"conflict_with_bot"`
	SuggestedAction  string `json:"suggested_action"`
	PlanFeedback     string `json:"plan_feedback"`
	Reason           string `json:"reason"`
	Risk             string `json:"risk"`
}
