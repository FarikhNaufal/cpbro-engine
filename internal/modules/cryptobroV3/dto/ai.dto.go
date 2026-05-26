package dto

import "time"

type AIAuditRequest struct {
	Symbol            string             `json:"symbol"`
	Direction         string             `json:"direction"`
	Playbook          string             `json:"playbook"`
	SetupType         string             `json:"setup_type"`
	QuantScore        float64            `json:"quant_score"`
	PolicySummary     string             `json:"policy_summary"`
	H4Trend           string             `json:"h4_trend"`
	H1Trend           string             `json:"h1_trend"`
	M15H1Structure    string             `json:"m15_h1_structure"`
	SupportResistance string             `json:"support_resistance"`
	BotEntry          float64            `json:"bot_entry"`
	BotSL             float64            `json:"bot_sl"`
	BotTP             float64            `json:"bot_tp"`
	BotRR             float64            `json:"bot_rr"`
	RsiMfiAdxAtr      string             `json:"rsi_mfi_adx_atr"`
	M15Candles        []Candle           `json:"m15_candles"`
	H1Candles         []Candle           `json:"h1_candles"`
	H4Candles         []Candle           `json:"h4_candles"`
	MarketRegime      string             `json:"market_regime"`
	Timestamp         time.Time          `json:"timestamp"`
	Payload           GeminiAuditPayload `json:"payload"`
}

type Candle struct {
	Time  time.Time `json:"time"`
	Open  float64   `json:"open"`
	High  float64   `json:"high"`
	Low   float64   `json:"low"`
	Close float64   `json:"close"`
	Vol   float64   `json:"vol"`
}

type GeminiCandidateContext struct {
	Symbol    string  `json:"symbol"`
	Direction string  `json:"direction"`
	Playbook  string  `json:"playbook"`
	SetupType string  `json:"setup_type"`
	Tier      string  `json:"tier"`
	Score     float64 `json:"score"`
	Grade     string  `json:"grade"`
}

type GeminiPolicyContext struct {
	Regime           string   `json:"regime"`
	BtcTrend         string   `json:"btc_trend"`
	BtcScore         float64  `json:"btc_score"`
	BtcChaos         float64  `json:"btc_chaos"`
	LongMode         string   `json:"long_mode"`
	ShortMode        string   `json:"short_mode"`
	AllowedPlaybooks []string `json:"allowed_playbooks"`
	AllowedTiers     []string `json:"allowed_tiers"`
	MinScoreExecute  float64  `json:"min_score_execute"`
	MinRRExecute     float64  `json:"min_rr_execute"`
	MinADXExecute    float64  `json:"min_adx_execute"`
}

type GeminiTechnicalContext struct {
	RSI            float64 `json:"rsi"`
	RSISlope       float64 `json:"rsi_slope"`
	MFI            float64 `json:"mfi"`
	MFISlope       float64 `json:"mfi_slope"`
	ADX            float64 `json:"adx"`
	ADXSlope       float64 `json:"adx_slope"`
	ATR            float64 `json:"atr"`
	ATRPercent     float64 `json:"atr_percent"`
	VolumeRatio    float64 `json:"volume_ratio"`
	OIChange       float64 `json:"oi_change"`
	FundingRate    float64 `json:"funding_rate"`
	PriceChange24h float64 `json:"price_change_24h"`
}

type GeminiStructureContext struct {
	H4Trend               string  `json:"h4_trend"`
	H1Trend               string  `json:"h1_trend"`
	M15Structure          string  `json:"m15_structure"`
	H1Structure           string  `json:"h1_structure"`
	Support               float64 `json:"support"`
	Resistance            float64 `json:"resistance"`
	SessionHigh           float64 `json:"session_high"`
	SessionLow            float64 `json:"session_low"`
	LiquidityUpper        float64 `json:"liquidity_upper"`
	LiquidityLower        float64 `json:"liquidity_lower"`
	SweepSide             string  `json:"sweep_side"`
	HasLiquiditySweep     bool    `json:"has_liquidity_sweep"`
	HasVolumeConfirmation bool    `json:"has_volume_confirmation"`
	Bos                   bool    `json:"bos"`
	Choch                 bool    `json:"choch"`
}

type GeminiTradePlanContext struct {
	ProposedEntry      float64 `json:"proposed_entry"`
	ProposedSL         float64 `json:"proposed_sl"`
	ProposedTP1        float64 `json:"proposed_tp1"`
	ProposedTP2        float64 `json:"proposed_tp2"`
	RR                 float64 `json:"rr"`
	InvalidationReason string  `json:"invalidation_reason"`
}

type GeminiKlineContext struct {
	M15Candles []Candle `json:"m15_candles"`
	H1Candles  []Candle `json:"h1_candles"`
	H4Candles  []Candle `json:"h4_candles"`
}

type GeminiAuditPayload struct {
	Candidate GeminiCandidateContext `json:"candidate"`
	Policy    GeminiPolicyContext    `json:"policy"`
	Technical GeminiTechnicalContext `json:"technical"`
	Structure GeminiStructureContext `json:"structure"`
	TradePlan GeminiTradePlanContext `json:"trade_plan"`
	Klines    GeminiKlineContext     `json:"klines"`
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
