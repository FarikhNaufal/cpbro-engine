package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

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

// AuditCandidate runs the structured AI Candle Auditor on raw kline structures.
func (s *GeminiService) AuditCandidate(ctx context.Context, req dto.AIAuditRequest) (*dto.AIAuditResponse, error) {
	// Build candle data representation (last 30 closed M15 candles)
	var klineStr []string
	for idx, c := range req.M15Candles {
		klineStr = append(klineStr, fmt.Sprintf("Candle %d: Time=%s, Open=%0.5f, High=%0.5f, Low=%0.5f, Close=%0.5f, Vol=%0.1f",
			idx, c.Time.Format("15:04"), c.Open, c.High, c.Low, c.Close, c.Vol))
	}
	candlesText := strings.Join(klineStr, "\n")

	prompt := fmt.Sprintf(`You are a narrative candle structure auditor.
Role: Analyze the raw candle patterns, market narrative, and timing.
CRITICAL: You are NOT a calculator or execution engine.
- You CANNOT change entry/SL/TP prices under any circumstances.
- You CANNOT flip LONG to SHORT or vice-versa.
- Provide a strict evaluation of the candle structures.

Trading Candidate Context:
- Symbol: %s
- Direction: %s
- Playbook: %s
- Setup Type: %s
- Quant Score: %0.2f
- Policy Summary: %s
- H4 Trend: %s
- H1 Trend: %s
- M15/H1 Structure: %s
- Support/Resistance: %s
- Bot Entry Price: %0.5f
- Bot Stop Loss: %0.5f
- Bot Take Profit: %0.5f
- Bot Risk-to-Reward: %0.2f
- Indicator Context (RSI/MFI/ADX/ATR/OI/Funding): %s

M15 Candles (Last 30 closed):
%s

Address these specific evaluation questions:
1. Are the last 5 candles rejection or continuation?
2. Is there a confirmation candle present?
3. Is the setup already exhausted?
4. Is the bot entry timing fresh, acceptable, late, or missed?
5. Does the candle narrative conflict with the bot direction?
6. Does the selected playbook fit the raw klines?
7. Is the suggested action to execute-if-not-stale, wait retest, watch only, or reject?`,
		req.Symbol, req.Direction, req.Playbook, req.SetupType, req.QuantScore, req.PolicySummary,
		req.H4Trend, req.H1Trend, req.M15H1Structure, req.SupportResistance, req.BotEntry, req.BotSL,
		req.BotTP, req.BotRR, req.RsiMfiAdxAtr, candlesText)

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
