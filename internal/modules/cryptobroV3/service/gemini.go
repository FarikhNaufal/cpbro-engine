package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"time"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
	"google.golang.org/genai"
)

type GeminiService struct {
	client *genai.Client
	model  string
}

func NewGeminiService(modelName string) (*GeminiService, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY environment variable is not set")
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: apiKey,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create gemini client: %w", err)
	}

	if modelName == "" {
		modelName = "gemini-2.5-flash"
	}

	return &GeminiService{
		client: client,
		model:  modelName,
	}, nil
}

func formatCompactCandles(candles []dto.Candle, count int) string {
	if len(candles) == 0 {
		return "N/A"
	}
	start := len(candles) - count
	if start < 0 {
		start = 0
	}
	var lines []string
	for i := start; i < len(candles); i++ {
		c := candles[i]
		utcTimeStr := c.Time.UTC().Format(time.RFC3339)
		ms := c.Time.UTC().UnixMilli()
		lines = append(lines, fmt.Sprintf("[%s | %d] O=%0.5f H=%0.5f L=%0.5f C=%0.5f V=%0.2f",
			utcTimeStr, ms, c.Open, c.High, c.Low, c.Close, c.Vol))
	}
	return strings.Join(lines, "\n")
}

// AuditCandidate runs the structured AI Candle Auditor on raw kline structures.
func (s *GeminiService) AuditCandidate(ctx context.Context, req dto.AIAuditRequest) (*dto.AIAuditResponse, error) {
	p := req.Payload

	// Format allowed playbooks/tiers
	allowedPlaybooksStr := "N/A"
	if len(p.Policy.AllowedPlaybooks) > 0 {
		allowedPlaybooksStr = strings.Join(p.Policy.AllowedPlaybooks, ", ")
	}
	allowedTiersStr := "N/A"
	if len(p.Policy.AllowedTiers) > 0 {
		allowedTiersStr = strings.Join(p.Policy.AllowedTiers, ", ")
	}

	// Format Support/Resistance/Levels
	supportVal := "N/A"
	if p.Structure.Support > 0 {
		supportVal = fmt.Sprintf("%0.5f", p.Structure.Support)
	}
	resistanceVal := "N/A"
	if p.Structure.Resistance > 0 {
		resistanceVal = fmt.Sprintf("%0.5f", p.Structure.Resistance)
	}
	sessionHighVal := "N/A"
	if p.Structure.SessionHigh > 0 {
		sessionHighVal = fmt.Sprintf("%0.5f", p.Structure.SessionHigh)
	}
	sessionLowVal := "N/A"
	if p.Structure.SessionLow > 0 {
		sessionLowVal = fmt.Sprintf("%0.5f", p.Structure.SessionLow)
	}
	liqUpperVal := "N/A"
	if p.Structure.LiquidityUpper > 0 {
		liqUpperVal = fmt.Sprintf("%0.5f", p.Structure.LiquidityUpper)
	}
	liqLowerVal := "N/A"
	if p.Structure.LiquidityLower > 0 {
		liqLowerVal = fmt.Sprintf("%0.5f", p.Structure.LiquidityLower)
	}

	// Double check if all structure levels are N/A
	structureIncompleteReason := ""
	if supportVal == "N/A" && resistanceVal == "N/A" && sessionHighVal == "N/A" && sessionLowVal == "N/A" && liqUpperVal == "N/A" && liqLowerVal == "N/A" {
		structureIncompleteReason = " | WARNING: Structure context incomplete (all support/resistance levels are N/A)"
	}

	// Format Technical Indicators deterministically
	rsiValStr := fmt.Sprintf("%0.2f", p.Technical.RSI)
	rsiSlopeStr := fmt.Sprintf("%0.2f", p.Technical.RSISlope)
	mfiValStr := fmt.Sprintf("%0.2f", p.Technical.MFI)
	mfiSlopeStr := fmt.Sprintf("%0.2f", p.Technical.MFISlope)
	adxValStr := fmt.Sprintf("%0.2f", p.Technical.ADX)
	adxSlopeStr := fmt.Sprintf("%0.2f", p.Technical.ADXSlope)
	atrValStr := fmt.Sprintf("%0.5f", p.Technical.ATR)
	atrPercentStr := fmt.Sprintf("%0.2f%%", p.Technical.ATRPercent)
	volRatioStr := fmt.Sprintf("%0.2f", p.Technical.VolumeRatio)

	oiChangeStr := fmt.Sprintf("%0.2f%%", p.Technical.OIChange)
	if !p.Technical.HasCrowdingEvidence && p.Technical.OIChange == 0.0 {
		oiChangeStr += " (limited derivatives context)"
	}

	crowdingScoreStr := fmt.Sprintf("%0.2f", p.Technical.CrowdingScore)
	breakoutLevelStr := "N/A"
	if p.Technical.BreakoutLevel > 0 {
		breakoutLevelStr = fmt.Sprintf("%0.5f", p.Technical.BreakoutLevel)
	}

	fundingRateStr := fmt.Sprintf("%0.5f", p.Technical.FundingRate)
	priceChange24hStr := fmt.Sprintf("%0.2f%%", p.Technical.PriceChange24h)

	// Limit candle payload size deterministically (closed candles only are expected upstream).
	payloadForAI := p
	if len(payloadForAI.Klines.M15Candles) > 30 {
		payloadForAI.Klines.M15Candles = payloadForAI.Klines.M15Candles[len(payloadForAI.Klines.M15Candles)-30:]
	}
	if len(payloadForAI.Klines.H1Candles) > 5 {
		payloadForAI.Klines.H1Candles = payloadForAI.Klines.H1Candles[len(payloadForAI.Klines.H1Candles)-5:]
	}
	if len(payloadForAI.Klines.H4Candles) > 5 {
		payloadForAI.Klines.H4Candles = payloadForAI.Klines.H4Candles[len(payloadForAI.Klines.H4Candles)-5:]
	}

	// Format Candles (human-readable mirror of the same closed candles payload, for robustness).
	m15CandlesText := formatCompactCandles(payloadForAI.Klines.M15Candles, 30)
	h1CandlesText := formatCompactCandles(payloadForAI.Klines.H1Candles, 5)
	h4CandlesText := formatCompactCandles(payloadForAI.Klines.H4Candles, 5)

	payloadJSON, err := json.MarshalIndent(payloadForAI, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal ai payload: %w", err)
	}

	prompt := fmt.Sprintf(`You are CryptobroV3 Candle Narrative Auditor.

Your job is ONLY to audit whether the raw CLOSED M15 candles support the bot’s chosen direction + playbook narrative, and whether the bot’s proposed entry timing is fresh/late/missed.
You are NOT a trading executor and NOT a calculator.

Hard Rules (must follow):
1) Do NOT change bot direction (LONG/SHORT). Do NOT propose reversing it.
2) Do NOT change bot numeric trade plan (entry, SL, TP1, TP2, RR). Treat them as read-only.
3) Do NOT output trade instructions, order instructions, or FINAL_EXECUTE. You only output an audit verdict.
4) Use ONLY closed candles from payload.klines.m15_candles. Ignore any "latest price".
5) If data is missing / inconsistent / insufficient, choose WAIT or REJECT (never rubber-stamp CONFIRM).

What you will receive:
- A single JSON object: "payload" (see below).
- The payload includes: candidate identity, policy context, technical context, structure context, bot trade plan, and raw closed candles.

Your tasks:
A) Audit last 5 closed M15 candles:
- Determine candle_narrative: REJECTION / CONTINUATION / CHOP / EXHAUSTED / UNCLEAR
- Determine last_5_candles_bias: BULLISH / BEARISH / NEUTRAL
- Determine has_rejection and has_confirmation
- Determine if narrative conflicts with bot direction (conflict_with_bot)

B) Audit entry_timing vs narrative:
- FRESH / ACCEPTABLE / LATE / MISSED

C) Validate playbook narrative vs candles (playbook-aware):
- TREND_PULLBACK: needs pullback stabilization + resumption evidence; reject/wait if exhaustion against direction.
- LIQUIDITY_SWEEP_REVERSAL: requires sweep-like wick + reclaim/close back in range + confirmation; else WAIT/REJECT.
- COMPRESSION_BREAKOUT_RETEST: do not rubber-stamp first breakout candle; prefer breakout + retest + hold/reject; else WAIT_RETEST.
- RANGE_EDGE_REVERSAL: requires range-edge rejection; if strong expansion through edge, REJECT/WATCH_ONLY.
- CROWDED_POSITIONING_SQUEEZE: derivatives may be incomplete; require price-action evidence of failed continuation + reclaim/rejection + confirmation.

Output requirements (STRICT):
- Output ONLY one JSON object. No markdown. No code fences. No extra keys. No trailing text.
- All schema fields are REQUIRED.

Schema:
{
  "decision": "CONFIRM|WAIT|REJECT",
  "confidence": "HIGH|MEDIUM|LOW",
  "candle_narrative": "REJECTION|CONTINUATION|CHOP|EXHAUSTED|UNCLEAR",
  "last_5_candles_bias": "BULLISH|BEARISH|NEUTRAL",
  "has_rejection": true,
  "has_confirmation": true,
  "entry_timing": "FRESH|ACCEPTABLE|LATE|MISSED",
  "conflict_with_bot": false,
  "suggested_action": "EXECUTE_IF_NOT_STALE|WAIT_RETEST|REJECT|WATCH_ONLY",
  "plan_feedback": "text",
  "reason": "text",
  "risk": "text"
}

Consistency rules (must hold):
- If decision=REJECT => suggested_action=REJECT
- If decision=WAIT => suggested_action is WAIT_RETEST or WATCH_ONLY
- suggested_action=EXECUTE_IF_NOT_STALE ONLY IF ALL true:
  - decision=CONFIRM, confidence=HIGH, conflict_with_bot=false
  - entry_timing is FRESH or ACCEPTABLE
  - has_confirmation=true
  - candle_narrative is NOT UNCLEAR and NOT CHOP
- If uncertain, prefer WAIT with MEDIUM/LOW confidence.

Now audit the provided payload JSON.

Trading Candidate Context:
- Symbol: %s
- Direction: %s
- Playbook: %s
- Setup Type: %s
- Tier: %s
- Quant Score: %0.2f
- Grade: %s

Market Policy Context:
- Regime: %s
- BTC Trend: %s
- BTC Score: %0.2f
- BTC Chaos: %0.2f
- Long Mode: %s
- Short Mode: %s
- Allowed Playbooks: %s
- Allowed Tiers: %s
- Min Score Execute: %0.2f
- Min RR Execute: %0.2f
- Min ADX Execute: %0.2f

Technical Context:
- RSI: %s
- RSI Slope: %s
- MFI: %s
- MFI Slope: %s
- ADX: %s
- ADX Slope: %s
- ATR: %s
- ATR Percent: %s
- Volume Ratio: %s
- OI Change: %s
- Crowding Score: %s
- Has Crowding Evidence: %v
- Funding Rate: %s
- Price Change 24h: %s
- Breakout Level (if applicable): %s
- Retest Hold (if applicable): %v
- Retest Touches (if applicable): %d

Structure Context:
- H4 Trend: %s
- H1 Trend: %s
- M15 Structure: %s
- H1 Structure: %s
- Support: %s
- Resistance: %s
- Session High: %s
- Session Low: %s
- Liquidity Upper: %s
- Liquidity Lower: %s
- Sweep Side: %s
- Has Liquidity Sweep: %v
- Has Volume Confirmation: %v
- BOS: %v
- CHOCH: %v%s

Trade Plan:
- Proposed Entry Price: %0.5f
- Proposed Stop Loss: %0.5f
- Proposed Take Profit 1: %0.5f
- Proposed Take Profit 2: %0.5f
- Risk-to-Reward: %0.2f
- Invalidation Reason: %s

M15 Candles (Last 30 closed):
%s

HIGHER TIMEFRAME CONTEXT:
- H4 Trend: %s
- H1 Trend: %s
- H4 last closed candles summary:
%s
- H1 last closed candles summary:
%s

Address these specific evaluation questions:
1. Are the last 5 candles rejection or continuation?
2. Is there a confirmation candle present?
3. Is the setup already exhausted?
4. Is the bot entry timing fresh, acceptable, late, or missed?
5. Does the candle narrative conflict with the bot direction?
6. Does the selected playbook fit the raw klines?
7. Is the suggested action to execute-if-not-stale, wait retest, watch only, or reject?

payload (JSON):
%s`,
		p.Candidate.Symbol, p.Candidate.Direction, p.Candidate.Playbook, p.Candidate.SetupType, p.Candidate.Tier, p.Candidate.Score, p.Candidate.Grade,
		p.Policy.Regime, p.Policy.BtcTrend, p.Policy.BtcScore, p.Policy.BtcChaos, p.Policy.LongMode, p.Policy.ShortMode, allowedPlaybooksStr, allowedTiersStr, p.Policy.MinScoreExecute, p.Policy.MinRRExecute, p.Policy.MinADXExecute,
		rsiValStr, rsiSlopeStr, mfiValStr, mfiSlopeStr, adxValStr, adxSlopeStr, atrValStr, atrPercentStr, volRatioStr, oiChangeStr, crowdingScoreStr, p.Technical.HasCrowdingEvidence, fundingRateStr, priceChange24hStr, breakoutLevelStr, p.Technical.RetestHold, p.Technical.RetestTouches,
		p.Structure.H4Trend, p.Structure.H1Trend, p.Structure.M15Structure, p.Structure.H1Structure, supportVal, resistanceVal, sessionHighVal, sessionLowVal, liqUpperVal, liqLowerVal, p.Structure.SweepSide, p.Structure.HasLiquiditySweep, p.Structure.HasVolumeConfirmation, p.Structure.Bos, p.Structure.Choch, structureIncompleteReason,
		p.TradePlan.ProposedEntry, p.TradePlan.ProposedSL, p.TradePlan.ProposedTP1, p.TradePlan.ProposedTP2, p.TradePlan.RR, p.TradePlan.InvalidationReason,
		m15CandlesText,
		p.Structure.H4Trend, p.Structure.H1Trend,
		h4CandlesText,
		h1CandlesText,
		string(payloadJSON))

	// Configure structured JSON schema
	config := &genai.GenerateContentConfig{
		ResponseMIMEType: "application/json",
		ResponseSchema: &genai.Schema{
			Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{
				"decision": {
					Type: genai.TypeString,
					Enum: []string{"CONFIRM", "WAIT", "REJECT"},
				},
				"confidence": {
					Type: genai.TypeString,
					Enum: []string{"HIGH", "MEDIUM", "LOW"},
				},
				"candle_narrative": {
					Type: genai.TypeString,
					Enum: []string{"REJECTION", "CONTINUATION", "CHOP", "EXHAUSTED", "UNCLEAR"},
				},
				"last_5_candles_bias": {
					Type: genai.TypeString,
					Enum: []string{"BULLISH", "BEARISH", "NEUTRAL"},
				},
				"has_rejection": {
					Type: genai.TypeBoolean,
				},
				"has_confirmation": {
					Type: genai.TypeBoolean,
				},
				"entry_timing": {
					Type: genai.TypeString,
					Enum: []string{"FRESH", "ACCEPTABLE", "LATE", "MISSED"},
				},
				"conflict_with_bot": {
					Type: genai.TypeBoolean,
				},
				"suggested_action": {
					Type: genai.TypeString,
					Enum: []string{"EXECUTE_IF_NOT_STALE", "WAIT_RETEST", "REJECT", "WATCH_ONLY"},
				},
				"plan_feedback": {
					Type: genai.TypeString,
				},
				"reason": {
					Type: genai.TypeString,
				},
				"risk": {
					Type: genai.TypeString,
				},
			},
			Required: []string{
				"decision", "confidence", "candle_narrative", "last_5_candles_bias",
				"has_rejection", "has_confirmation", "entry_timing", "conflict_with_bot",
				"suggested_action", "plan_feedback", "reason", "risk",
			},
		},
	}

	resp, err := s.client.Models.GenerateContent(ctx, s.model, genai.Text(prompt), config)
	if err != nil {
		return nil, fmt.Errorf("failed to call gemini api: %w", err)
	}

	var rawText string
	if resp != nil && len(resp.Candidates) > 0 && resp.Candidates[0].Content != nil && len(resp.Candidates[0].Content.Parts) > 0 {
		rawText = resp.Candidates[0].Content.Parts[0].Text
	}

	if rawText == "" {
		return nil, fmt.Errorf("AI_ERROR: empty response from Gemini API")
	}

	// Strict JSON parsing
	var auditOut dto.AIAuditResponse
	if err := json.Unmarshal([]byte(rawText), &auditOut); err != nil {
		return nil, fmt.Errorf("AI_ERROR: failed to parse JSON: %w", err)
	}

	// Strict enum validation
	if err := validateAuditResponse(auditOut); err != nil {
		return nil, fmt.Errorf("AI_ERROR: %w", err)
	}

	// Map traditional fields
	auditOut.Symbol = req.Symbol
	auditOut.IsApproved = (auditOut.Decision == "CONFIRM")
	auditOut.Reasoning = auditOut.Reason

	// Derive Sentiment
	if auditOut.Decision == "CONFIRM" {
		if req.Direction == "LONG" {
			auditOut.Sentiment = "BULLISH"
		} else {
			auditOut.Sentiment = "BEARISH"
		}
	} else {
		auditOut.Sentiment = "NEUTRAL"
	}

	// Derive Confidence Score
	switch auditOut.Confidence {
	case "HIGH":
		auditOut.ConfidenceScore = 0.9
	case "MEDIUM":
		auditOut.ConfidenceScore = 0.6
	case "LOW":
		auditOut.ConfidenceScore = 0.3
	default:
		auditOut.ConfidenceScore = 0.5
	}

	// Force suggested stop loss and take profit to zero to ignore AI overrides
	auditOut.SuggestedStopLoss = 0
	auditOut.SuggestedTakeProfit = 0

	return &auditOut, nil
}

// Ping runs a fast 1-token query to verify Gemini API connection and credentials.
func (s *GeminiService) Ping(ctx context.Context) error {
	config := &genai.GenerateContentConfig{
		MaxOutputTokens: 1,
	}
	_, err := s.client.Models.GenerateContent(ctx, s.model, genai.Text("ping"), config)
	return err
}

func validateAuditResponse(res dto.AIAuditResponse) error {
	validDecisions := map[string]bool{"CONFIRM": true, "WAIT": true, "REJECT": true}
	if !validDecisions[res.Decision] {
		return fmt.Errorf("missing or invalid field 'decision': %s", res.Decision)
	}

	validConfidence := map[string]bool{"HIGH": true, "MEDIUM": true, "LOW": true}
	if !validConfidence[res.Confidence] {
		return fmt.Errorf("missing or invalid field 'confidence': %s", res.Confidence)
	}

	validNarratives := map[string]bool{"REJECTION": true, "CONTINUATION": true, "CHOP": true, "EXHAUSTED": true, "UNCLEAR": true}
	if !validNarratives[res.CandleNarrative] {
		return fmt.Errorf("missing or invalid field 'candle_narrative': %s", res.CandleNarrative)
	}

	validBiases := map[string]bool{"BULLISH": true, "BEARISH": true, "NEUTRAL": true}
	if !validBiases[res.Last5CandlesBias] {
		return fmt.Errorf("missing or invalid field 'last_5_candles_bias': %s", res.Last5CandlesBias)
	}

	validTimings := map[string]bool{"FRESH": true, "ACCEPTABLE": true, "LATE": true, "MISSED": true}
	if !validTimings[res.EntryTiming] {
		return fmt.Errorf("missing or invalid field 'entry_timing': %s", res.EntryTiming)
	}

	validActions := map[string]bool{"EXECUTE_IF_NOT_STALE": true, "WAIT_RETEST": true, "REJECT": true, "WATCH_ONLY": true}
	if !validActions[res.SuggestedAction] {
		return fmt.Errorf("missing or invalid field 'suggested_action': %s", res.SuggestedAction)
	}

	if strings.TrimSpace(res.PlanFeedback) == "" {
		return fmt.Errorf("missing required field 'plan_feedback'")
	}
	if strings.TrimSpace(res.Reason) == "" {
		return fmt.Errorf("missing required field 'reason'")
	}
	if strings.TrimSpace(res.Risk) == "" {
		return fmt.Errorf("missing required field 'risk'")
	}

	return nil
}
