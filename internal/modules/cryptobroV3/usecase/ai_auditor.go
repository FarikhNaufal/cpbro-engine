package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
	"cpbro-engine/internal/modules/cryptobroV3/entity"
)

type AIAuditorUsecase struct {
	aiService      AIAuditorService
	storageUsecase *StorageUsecase
}

const (
	aiAuditCacheTTL     = defaultClosedCandleTimeframe
	aiAuditCacheMaxSize = 2000
)

func normalizeSuggestedActionDecision(res *dto.AIAuditResponse) (bool, string) {
	if res == nil {
		return false, ""
	}

	switch res.SuggestedAction {
	case "WAIT_RETEST", "WATCH_ONLY":
		if res.Decision == "CONFIRM" {
			res.Decision = "WAIT"
			return true, "non-executable suggested_action requires WAIT decision"
		}
	case "REJECT":
		if res.Decision != "REJECT" {
			res.Decision = "REJECT"
			return true, "reject suggested_action requires REJECT decision"
		}
	}

	return false, ""
}

func NewAIAuditorUsecase(aiService AIAuditorService, storageUsecase *StorageUsecase) *AIAuditorUsecase {
	return &AIAuditorUsecase{
		aiService:      aiService,
		storageUsecase: storageUsecase,
	}
}

func pruneAIAuditCacheEntries(cacheMap map[string]entity.CachedAudit, now time.Time) {
	if cacheMap == nil {
		return
	}
	for key, cached := range cacheMap {
		if now.Sub(cached.CachedAt) >= aiAuditCacheTTL {
			delete(cacheMap, key)
		}
	}
	if len(cacheMap) <= aiAuditCacheMaxSize {
		return
	}

	type cacheEntry struct {
		key      string
		cachedAt time.Time
	}
	entries := make([]cacheEntry, 0, len(cacheMap))
	for key, cached := range cacheMap {
		entries = append(entries, cacheEntry{key: key, cachedAt: cached.CachedAt})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].cachedAt.Before(entries[j].cachedAt)
	})
	removeCount := len(cacheMap) - aiAuditCacheMaxSize
	for index := 0; index < removeCount && index < len(entries); index++ {
		delete(cacheMap, entries[index].key)
	}
}

func BuildAIAuditCacheKey(candidate QuantResult, payload dto.GeminiAuditPayload) string {
	lastClosedTime := "N/A"
	m15Closed := GetClosedCandlesOnly(candidate.RawKlines, defaultClosedCandleTimeframe)
	if len(m15Closed) > 0 {
		lastClosedTime = m15Closed[len(m15Closed)-1].Time.Format(time.RFC3339)
	}

	payloadHash := "N/A"
	payloadBytes, err := json.Marshal(payload)
	if err == nil {
		hash := sha256.Sum256(payloadBytes)
		payloadHash = hex.EncodeToString(hash[:])
	}

	return fmt.Sprintf("ai:v3:%s:%s:%s:%s:%s:%s",
		candidate.Symbol,
		candidate.Direction,
		candidate.Playbook,
		candidate.SetupType,
		lastClosedTime,
		payloadHash,
	)
}

// Audit queries cached audits or calls GeminiService API if no cache hit exists.
func (uc *AIAuditorUsecase) Audit(ctx context.Context, quant QuantResult, policy MarketPolicy, m15, h1, h4 []dto.Candle, m5Summary *M5ConfirmationSummary) (dto.AIAuditResponse, error) {
	symbol := quant.Symbol

	// Limit RawKlines to last 30 closed M15 candles
	m15Closed := GetClosedCandlesOnly(m15, defaultClosedCandleTimeframe)
	if len(m15Closed) > 30 {
		m15Closed = m15Closed[len(m15Closed)-30:]
	}

	h1Closed := GetClosedCandlesOnly(h1, time.Hour)
	if len(h1Closed) > 10 {
		h1Closed = h1Closed[len(h1Closed)-10:]
	}

	h4Closed := GetClosedCandlesOnly(h4, 4*time.Hour)
	if len(h4Closed) > 10 {
		h4Closed = h4Closed[len(h4Closed)-10:]
	}

	// Technical Indicator details
	rsiVal := quant.TechnicalSnapshot.RSI
	rsiSlope := quant.TechnicalSnapshot.RSISlope
	mfiVal := quant.TechnicalSnapshot.MFI
	mfiSlope := quant.TechnicalSnapshot.MFISlope
	adxVal := quant.TechnicalSnapshot.IndicatorValues[IndicatorADX]
	adxSlope := quant.TechnicalSnapshot.ADXSlope
	atrVal := quant.TechnicalSnapshot.IndicatorValues[IndicatorATR]
	atrPercent := quant.TechnicalSnapshot.ATRPercent
	volumeRatio := quant.TechnicalSnapshot.VolumeRatio
	oiChange := quant.TechnicalSnapshot.OIChange
	fundingRate := quant.TechnicalSnapshot.FundingRate
	priceChange24h := quant.TechnicalSnapshot.PriceChange24h
	crowdingScore := quant.TechnicalSnapshot.IndicatorValues[IndicatorCrowdingScore]
	hasCrowdingEvidence := quant.TechnicalSnapshot.IndicatorValues[IndicatorHasCrowdingEvidence] == 1.0
	breakoutLevel := quant.TechnicalSnapshot.IndicatorValues[IndicatorBreakoutLevel]
	retestHold := quant.TechnicalSnapshot.IndicatorValues[IndicatorRetestHold] == 1.0
	retestTouches := int(quant.TechnicalSnapshot.IndicatorValues[IndicatorRetestTouches])

	// Policy summary representation
	var allowedPlaybooks []string
	for _, p := range policy.AllowedPlaybooks {
		allowedPlaybooks = append(allowedPlaybooks, string(p))
	}
	var allowedTiers []string
	for _, t := range policy.AllowedTiers {
		allowedTiers = append(allowedTiers, string(t))
	}

	// Sweep details
	sweepSide := "NONE"
	if quant.TechnicalSnapshot.IndicatorValues[IndicatorSweepLow] == 1.0 {
		sweepSide = "LOWER"
	} else if quant.TechnicalSnapshot.IndicatorValues[IndicatorSweepHigh] == 1.0 {
		sweepSide = "UPPER"
	}

	// Trade Plan Details
	entry := quant.TradePlan.EntryPrice
	tp := quant.TradePlan.TakeProfit
	sl := quant.TradePlan.StopLoss
	rr := 0.0
	tp1 := 0.0
	if entry > 0 && tp > 0 && sl > 0 {
		if quant.Direction == LONG && entry > sl {
			rr = (tp - entry) / (entry - sl)
			tp1 = entry + (tp-entry)*0.5
		} else if quant.Direction == SHORT && sl > entry {
			rr = (entry - tp) / (sl - entry)
			tp1 = entry - (entry-tp)*0.5
		}
	}

	var m5Context *dto.GeminiM5ConfirmationContext
	if m5Summary != nil {
		m5Context = &dto.GeminiM5ConfirmationContext{
			Used:              m5Summary.Used,
			Mode:              string(m5Summary.Mode),
			Status:            string(m5Summary.Status),
			Reason:            m5Summary.Reason,
			ConfirmationType:  m5Summary.ConfirmationType,
			Confirmed:         m5Summary.Confirmed,
			EarlyInvalidation: m5Summary.EarlyInvalidation,
			LastClose:         m5Summary.LastClose,
			EMA9:              m5Summary.EMA9,
			WickRatio:         m5Summary.WickRatio,
			Threshold:         m5Summary.Threshold,
		}
	}

	// Build the canonical GeminiAuditPayload
	payload := dto.GeminiAuditPayload{
		Candidate: dto.GeminiCandidateContext{
			Symbol:    symbol,
			Direction: string(quant.Direction),
			Playbook:  string(quant.Playbook),
			SetupType: quant.SetupType,
			Tier:      string(quant.Tier),
			Score:     quant.Score,
			Grade:     GetGrade(quant.Score),
		},
		Policy: dto.GeminiPolicyContext{
			Regime:           string(policy.Regime),
			BtcTrend:         policy.BtcTrend,
			BtcScore:         policy.BtcScore,
			BtcChaos:         policy.BtcChaos,
			LongMode:         string(policy.LongMode),
			ShortMode:        string(policy.ShortMode),
			AllowedPlaybooks: allowedPlaybooks,
			AllowedTiers:     allowedTiers,
			MinScoreExecute:  policy.MinScoreExecute,
			MinRRExecute:     policy.MinRRExecute,
			MinADXExecute:    policy.MinADXExecute,
		},
		Technical: dto.GeminiTechnicalContext{
			RSI:                 rsiVal,
			RSISlope:            rsiSlope,
			MFI:                 mfiVal,
			MFISlope:            mfiSlope,
			ADX:                 adxVal,
			ADXSlope:            adxSlope,
			ATR:                 atrVal,
			ATRPercent:          atrPercent,
			VolumeRatio:         volumeRatio,
			OIChange:            oiChange,
			CrowdingScore:       crowdingScore,
			HasCrowdingEvidence: hasCrowdingEvidence,
			FundingRate:         fundingRate,
			PriceChange24h:      priceChange24h,
			BreakoutLevel:       breakoutLevel,
			RetestHold:          retestHold,
			RetestTouches:       retestTouches,
		},
		Structure: dto.GeminiStructureContext{
			H4Trend:               quant.H4Trend,
			H1Trend:               quant.H1Trend,
			M15Structure:          quant.MarketStructure,
			H1Structure:           quant.StructureSnapshot.H1Structure,
			Support:               quant.StructureSnapshot.Support,
			Resistance:            quant.StructureSnapshot.Resistance,
			SessionHigh:           quant.StructureSnapshot.SessionHigh,
			SessionLow:            quant.StructureSnapshot.SessionLow,
			LiquidityUpper:        quant.StructureSnapshot.LiquidityUpper,
			LiquidityLower:        quant.StructureSnapshot.LiquidityLower,
			SweepSide:             sweepSide,
			HasLiquiditySweep:     quant.TechnicalSnapshot.IndicatorValues[IndicatorSweepLow] == 1.0 || quant.TechnicalSnapshot.IndicatorValues[IndicatorSweepHigh] == 1.0,
			HasVolumeConfirmation: quant.TechnicalSnapshot.IndicatorValues[IndicatorVolumeSpike] == 1.0,
			Bos:                   quant.StructureSnapshot.BOS,
			Choch:                 quant.StructureSnapshot.CHOCH,
		},
		TradePlan: dto.GeminiTradePlanContext{
			ProposedEntry:      entry,
			ProposedSL:         sl,
			ProposedTP1:        tp1,
			ProposedTP2:        tp,
			RR:                 rr,
			InvalidationReason: quant.TradePlan.Reason,
		},
		M5Confirmation: m5Context,
		Klines: dto.GeminiKlineContext{
			M15Candles: m15Closed,
			H1Candles:  h1Closed,
			H4Candles:  h4Closed,
		},
	}

	// Build cache key
	cacheKey := BuildAIAuditCacheKey(quant, payload)

	// Try loading cache
	cache, err := uc.storageUsecase.LoadAIAuditCache()
	if err == nil && cache != nil && cache.CacheMap != nil {
		pruneAIAuditCacheEntries(cache.CacheMap, time.Now())
		if cached, ok := cache.CacheMap[cacheKey]; ok {
			// Cache validity duration is 15 minutes
			if time.Since(cached.CachedAt) < aiAuditCacheTTL {
				slog.Info("AI Audit Cache HIT", "module", "gemini", "key", cacheKey, "symbol", symbol, "cached_at", cached.CachedAt)
				cachedRes := cached.Response
				if changed, reason := normalizeSuggestedActionDecision(&cachedRes); changed {
					slog.Warn("Normalized cached AI audit response for operational consistency",
						"module", "gemini",
						"symbol", symbol,
						"suggested_action", cachedRes.SuggestedAction,
						"decision", cachedRes.Decision,
						"reason", reason,
					)
				}
				cachedRes.IsApproved = (cachedRes.Decision == "CONFIRM")
				if strings.TrimSpace(cachedRes.Reasoning) == "" {
					cachedRes.Reasoning = cachedRes.Reason
				}
				if cachedRes.Decision == "CONFIRM" {
					if quant.Direction == LONG {
						cachedRes.Sentiment = "BULLISH"
					} else {
						cachedRes.Sentiment = "BEARISH"
					}
				} else {
					cachedRes.Sentiment = "NEUTRAL"
				}
				return cachedRes, nil
			}
		}
	}

	// Build indicators context string (ordered for prompt)
	orderedIndicators := []string{
		fmt.Sprintf("RSI=%0.2f", rsiVal),
		fmt.Sprintf("RSI_SLOPE=%0.2f", rsiSlope),
		fmt.Sprintf("MFI=%0.2f", mfiVal),
		fmt.Sprintf("MFI_SLOPE=%0.2f", mfiSlope),
		fmt.Sprintf("ADX=%0.2f", adxVal),
		fmt.Sprintf("ADX_SLOPE=%0.2f", adxSlope),
		fmt.Sprintf("ATR=%0.5f", atrVal),
		fmt.Sprintf("ATR_PERCENT=%0.2f%%", atrPercent),
		fmt.Sprintf("VOLUME_RATIO=%0.2f", volumeRatio),
		fmt.Sprintf("OI_CHANGE=%0.2f%%", oiChange),
		fmt.Sprintf("FUNDING_RATE=%0.5f", fundingRate),
		fmt.Sprintf("PRICE_CHANGE_24H=%0.2f%%", priceChange24h),
	}
	indicatorCtx := strings.Join(orderedIndicators, ", ")

	// Support/resistance context fallback string
	srCtx := "N/A"
	if quant.StructureSnapshot.Support > 0 && quant.StructureSnapshot.Resistance > 0 {
		srCtx = fmt.Sprintf("Support=%0.5f, Resistance=%0.5f", quant.StructureSnapshot.Support, quant.StructureSnapshot.Resistance)
	} else if quant.StructureSnapshot.Support > 0 {
		srCtx = fmt.Sprintf("Support=%0.5f, Resistance=N/A", quant.StructureSnapshot.Support)
	} else if quant.StructureSnapshot.Resistance > 0 {
		srCtx = fmt.Sprintf("Support=N/A, Resistance=%0.5f", quant.StructureSnapshot.Resistance)
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
		H1Candles:         h1Closed,
		H4Candles:         h4Closed,
		MarketRegime:      policy.Reason,
		Timestamp:         time.Now(),
		Payload:           payload,
	}

	// Call Gemini Auditor service
	slog.Info("AI Audit Cache MISS; calling Gemini API...", "module", "gemini", "symbol", symbol, "direction", req.Direction, "playbook", req.Playbook)
	res, err := uc.aiService.AuditCandidate(ctx, req)
	if err != nil {
		slog.Warn("AI Audit failed with error", "module", "gemini", "symbol", symbol, "error", err)
		// AI failure must return IsApproved = false (no execution allowed!)
		return dto.AIAuditResponse{
			Symbol:     symbol,
			IsApproved: false,
			Sentiment:  "NEUTRAL",
			Reasoning:  "AI_ERROR: " + err.Error(),
			Decision:   "WAIT",
			Confidence: "LOW",
			Reason:     "AI_ERROR",
			Risk:       err.Error(),
		}, err
	}

	// Work on a local copy to avoid mutating shared pointers returned by the service implementation.
	if res != nil {
		resVal := *res
		res = &resVal
	}

	if changed, reason := normalizeSuggestedActionDecision(res); changed {
		slog.Warn("Normalized AI audit response for operational consistency",
			"module", "gemini",
			"symbol", symbol,
			"suggested_action", res.SuggestedAction,
			"decision", res.Decision,
			"reason", reason,
		)
	}

	slog.Info("AI Audit verdict generated by Gemini",
		"module", "gemini",
		"symbol", symbol,
		"decision", res.Decision,
		"confidence", res.Confidence,
		"suggested_action", res.SuggestedAction,
		"reason", res.Reason,
	)

	// Perform robust field mappings & derivations at the usecase layer
	res.Symbol = symbol
	res.IsApproved = (res.Decision == "CONFIRM")
	if strings.TrimSpace(res.Reasoning) == "" {
		res.Reasoning = res.Reason
	}

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
		res.Sentiment = "NEUTRAL"
		res.Reasoning = "AI rejected due to direction conflict with bot: " + res.Reasoning
	}

	// 2. Ignore any suggested target levels from AI to protect the quant bot plan parameters
	res.SuggestedStopLoss = 0
	res.SuggestedTakeProfit = 0

	res.AuditedAt = time.Now()

	_ = uc.storageUsecase.UpdateAIAuditCache(func(c *entity.AIAuditCache) error {
		if c.CacheMap == nil {
			c.CacheMap = make(map[string]entity.CachedAudit)
		}
		now := time.Now()
		pruneAIAuditCacheEntries(c.CacheMap, now)
		c.CacheMap[cacheKey] = entity.CachedAudit{
			Response: *res,
			CachedAt: now,
		}
		pruneAIAuditCacheEntries(c.CacheMap, now)
		return nil
	})

	return *res, nil
}
