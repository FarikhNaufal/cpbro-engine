package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
	"cpbro-engine/internal/modules/cryptobroV3/entity"
)

type AIAuditorUsecase struct {
	aiService      AIAuditorService
	storageUsecase *StorageUsecase
}

func NewAIAuditorUsecase(aiService AIAuditorService, storageUsecase *StorageUsecase) *AIAuditorUsecase {
	return &AIAuditorUsecase{
		aiService:      aiService,
		storageUsecase: storageUsecase,
	}
}

// Audit queries cached audits or calls GeminiService API if no cache hit exists.
func (uc *AIAuditorUsecase) Audit(ctx context.Context, quant QuantResult, policy MarketPolicy, m15, h1, h4 []dto.Candle) (dto.AIAuditResponse, error) {
	symbol := quant.Symbol

	// Try loading cache
	cache, err := uc.storageUsecase.LoadAIAuditCache()
	if err == nil && cache.CacheMap != nil {
		if cached, ok := cache.CacheMap[symbol]; ok {
			// Cache validity duration is 15 minutes
			if time.Since(cached.CachedAt) < 15*time.Minute {
				return cached.Response, nil
			}
		}
	}

	// Build indicators context string
	var indicators []string
	for k, v := range quant.TechnicalSnapshot.IndicatorValues {
		indicators = append(indicators, fmt.Sprintf("%s=%0.2f", k, v))
	}
	indicatorCtx := strings.Join(indicators, ", ")

	// Support/resistance context
	var highLowStrs []string
	if len(quant.StructureSnapshot.Highs) > 0 {
		highLowStrs = append(highLowStrs, fmt.Sprintf("Highs=%v", quant.StructureSnapshot.Highs))
	}
	if len(quant.StructureSnapshot.Lows) > 0 {
		highLowStrs = append(highLowStrs, fmt.Sprintf("Lows=%v", quant.StructureSnapshot.Lows))
	}
	srCtx := strings.Join(highLowStrs, ", ")
	if srCtx == "" {
		srCtx = "N/A"
	}

	// Calculate Bot Risk-to-Reward ratio
	entry := quant.TradePlan.EntryPrice
	tp := quant.TradePlan.TakeProfit
	sl := quant.TradePlan.StopLoss
	rr := 0.0
	if entry > 0 && tp > 0 && sl > 0 {
		if quant.Direction == LONG && entry > sl {
			rr = (tp - entry) / (entry - sl)
		} else if quant.Direction == SHORT && sl > entry {
			rr = (entry - tp) / (sl - entry)
		}
	}

	// Limit RawKlines to last 30 closed M15 candles
	m15Closed := m15
	if len(m15Closed) > 30 {
		m15Closed = m15Closed[len(m15Closed)-30:]
	}

	// Prepare audit request payload with complete context
	req := dto.AIAuditRequest{
		Symbol:            symbol,
		Direction:         string(quant.Direction),
		Playbook:          string(quant.Playbook),
		SetupType:         quant.SetupType,
		QuantScore:        quant.Score,
		PolicySummary:     fmt.Sprintf("AllowLong=%v, AllowShort=%v, LongMode=%s, ShortMode=%s, Reason=%s", policy.AllowLong, policy.AllowShort, policy.LongMode, policy.ShortMode, policy.Reason),
		H4Trend:           quant.H4Trend,
		H1Trend:           quant.H1Trend,
		M15H1Structure:    quant.MarketStructure,
		SupportResistance: srCtx,
		BotEntry:          entry,
		BotSL:             sl,
		BotTP:             tp,
		BotRR:             rr,
		RsiMfiAdxAtr:      indicatorCtx,
		M15Candles:        m15Closed,
		H1Candles:         h1,
		H4Candles:         h4,
		MarketRegime:      policy.Reason,
		Timestamp:         time.Now(),
	}

	// Call Gemini Auditor service
	res, err := uc.aiService.AuditCandidate(ctx, req)
	if err != nil {
		// AI failure must return IsApproved = false (no execution allowed!)
		return dto.AIAuditResponse{
			Symbol:     symbol,
			IsApproved: false,
			Sentiment:  "NEUTRAL",
			Reasoning:  "AI_ERROR: " + err.Error(),
		}, err
	}

	// Perform robust field mappings & derivations at the usecase layer
	res.Symbol = symbol
	res.IsApproved = (res.Decision == "CONFIRM")
	res.Reasoning = res.Reason

	// Derive Sentiment
	if res.Decision == "CONFIRM" {
		if quant.Direction == LONG {
			res.Sentiment = "BULLISH"
		} else {
			res.Sentiment = "BEARISH"
		}
	} else {
		res.Sentiment = "NEUTRAL"
	}

	// Derive Confidence Score
	switch res.Confidence {
	case "HIGH":
		res.ConfidenceScore = 0.9
	case "MEDIUM":
		res.ConfidenceScore = 0.6
	case "LOW":
		res.ConfidenceScore = 0.3
	default:
		res.ConfidenceScore = 0.5
	}

	// Force validation rules:
	// 1. Conflict checking: reject if AI indicates a conflict or is inconsistent
	if res.ConflictWithBot {
		res.IsApproved = false
		res.Decision = "REJECT"
		res.SuggestedAction = "REJECT"
		res.Reasoning = "AI rejected due to direction conflict with bot: " + res.Reason
	}

	// 2. Ignore any suggested target levels from AI to protect the quant bot plan parameters
	res.SuggestedStopLoss = 0
	res.SuggestedTakeProfit = 0

	res.AuditedAt = time.Now()

	// Update cache
	if cache == nil || cache.CacheMap == nil {
		cache = &entity.AIAuditCache{CacheMap: make(map[string]entity.CachedAudit)}
	}
	cache.CacheMap[symbol] = entity.CachedAudit{
		Response: *res,
		CachedAt: time.Now(),
	}
	_ = uc.storageUsecase.SaveAIAuditCache(*cache)

	return *res, nil
}
