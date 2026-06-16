package usecase

import (
	"context"
	"fmt"
	"math"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
)

type StalenessUsecase struct {
	maxStaleness     time.Duration
	latestPriceFeed  LatestPriceFeed
	fallbackProvider MarketDataProvider
}

func NewStalenessUsecase(maxStaleness time.Duration) *StalenessUsecase {
	return &StalenessUsecase{
		maxStaleness: maxStaleness,
	}
}

func (uc *StalenessUsecase) SetLatestPriceFeed(feed LatestPriceFeed) {
	if uc == nil {
		return
	}
	uc.latestPriceFeed = feed
}

func (uc *StalenessUsecase) SetFallbackProvider(provider MarketDataProvider) {
	if uc == nil {
		return
	}
	uc.fallbackProvider = provider
}

func (uc *StalenessUsecase) SyncSymbols(symbols []string) error {
	if uc == nil || uc.latestPriceFeed == nil {
		return nil
	}
	return uc.latestPriceFeed.SyncSymbols(symbols)
}

func (uc *StalenessUsecase) ResolveLatestPrice(ctx context.Context, symbol string) (float64, bool) {
	resolver := latestPriceResolver{
		realtime: uc.latestPriceFeed,
		fallback: uc.fallbackProvider,
	}
	return resolver.Resolve(ctx, symbol)
}

// IsFresh checks if the latest closed candle timestamp is within maxStaleness.
func (uc *StalenessUsecase) IsFresh(m15Candles []dto.Candle) bool {
	return uc.IsFreshAt(m15Candles, time.Now(), 15*time.Minute)
}

// IsFreshAt checks closed-candle freshness against a supplied clock.
// This keeps live scans tied to wall-clock time while allowing historical backtests
// to validate gaps without comparing old candles to the current real time.
func (uc *StalenessUsecase) IsFreshAt(m15Candles []dto.Candle, now time.Time, timeframe time.Duration) bool {
	if len(m15Candles) == 0 {
		return false
	}
	if now.IsZero() {
		now = time.Now()
	}
	if timeframe <= 0 {
		timeframe = 15 * time.Minute
	}

	lastCandle := m15Candles[len(m15Candles)-1]
	// dto.Candle.Time is treated as candle open-time in this project.
	// For closed-candle freshness, approximate close-time as open+timeframe.
	lastCloseTime := lastCandle.Time.Add(timeframe)
	return !lastCloseTime.After(now) && now.Sub(lastCloseTime) <= uc.maxStaleness
}

// Evaluate performs the ATR-based or percentage-based live price staleness validation.
func (uc *StalenessUsecase) Evaluate(quant QuantResult, review PlanReview, policy MarketPolicy, latestPrice float64) StalenessResult {
	entry := quant.TradePlan.EntryPrice
	if entry <= 0 {
		entry = quant.TriggerPrice
	}

	if entry <= 0 || latestPrice <= 0 {
		return StalenessResult{
			IsStale:         true,
			LastUpdatedTime: time.Now(),
			CurrentTime:     time.Now(),
			Status:          MISSED,
			Reason:          "Invalid price or entry parameter",
		}
	}

	distance := math.Abs(latestPrice - entry)
	distancePct := (distance / latestPrice) * 100

	regime := policy.EffectiveRegime()
	isChaos := regime == BTC_CHAOS
	isHighVol := regime == HIGH_VOL || isChaos
	isLowVol := regime == LOW_VOL

	// Get profile
	profile := GetPlaybookThresholdProfile(quant.Playbook, policy, quant.Tier)
	baseThreshold := profile.StalenessATR
	if baseThreshold <= 0 {
		baseThreshold = 0.30
	}

	// Adjust by Tier
	tierAdjustment := 0.0
	switch quant.Tier {
	case TierA:
		tierAdjustment = 0.05
	case TierC:
		tierAdjustment = -0.05
	}
	baseThreshold += tierAdjustment

	// Adjust by Volatility / Market conditions
	threshold := baseThreshold
	if isChaos || isHighVol {
		threshold -= 0.05
	} else if isLowVol {
		threshold = math.Min(threshold+0.05, 0.50)
	}
	settings := getRuntimeSettings()
	if policy.StalenessATRMultiplier > 0 {
		scaleBase := settings.StalenessPolicyScaleBase
		if scaleBase <= 0 {
			scaleBase = 1.5
		}
		policyScale := policy.StalenessATRMultiplier / scaleBase
		scaleMin := settings.StalenessPolicyScaleMin
		if scaleMin <= 0 {
			scaleMin = 0.50
		}
		scaleMax := settings.StalenessPolicyScaleMax
		if scaleMax <= 0 {
			scaleMax = 1.20
		}
		policyScale = math.Max(scaleMin, math.Min(policyScale, scaleMax))
		threshold *= policyScale
	}
	threshold = math.Max(0.15, threshold)

	// Determine Fallback Pct threshold
	var basePct float64
	if isChaos {
		basePct = settings.StalenessBasePctChaos
	} else if isHighVol {
		basePct = settings.StalenessBasePctHighVol
	} else if quant.Tier == TierC {
		basePct = settings.StalenessBasePctTierC
	} else {
		basePct = settings.StalenessBasePctDefault
	}
	if basePct <= 0 {
		basePct = 0.35
	}
	if policy.StalenessATRMultiplier > 0 {
		scaleBase := settings.StalenessPolicyScaleBase
		if scaleBase <= 0 {
			scaleBase = 1.5
		}
		policyScale := policy.StalenessATRMultiplier / scaleBase
		scaleMin := settings.StalenessPolicyScaleMin
		if scaleMin <= 0 {
			scaleMin = 0.50
		}
		scaleMax := settings.StalenessPolicyScaleMax
		if scaleMax <= 0 {
			scaleMax = 1.20
		}
		policyScale = math.Max(scaleMin, math.Min(policyScale, scaleMax))
		basePct *= policyScale
	}

	atrVal, hasATR := quant.TechnicalSnapshot.IndicatorValues[IndicatorATR]
	var distanceATR float64
	useATR := hasATR && atrVal > 0

	var status Status
	var isStale bool

	lateMultiplier := settings.StalenessLateThresholdMultiplier
	if lateMultiplier <= 1.0 {
		lateMultiplier = 1.5
	}
	if useATR {
		distanceATR = distance / atrVal
		if distanceATR <= threshold {
			status = FRESH
			isStale = false
		} else if distanceATR <= threshold*lateMultiplier {
			status = LATE
			isStale = true
		} else {
			status = MISSED
			isStale = true
		}
	} else {
		if distancePct <= basePct {
			status = FRESH
			isStale = false
		} else if distancePct <= basePct*lateMultiplier {
			status = LATE
			isStale = true
		} else {
			status = MISSED
			isStale = true
		}
	}

	reason := fmt.Sprintf("Staleness evaluated: latestPrice=%0.5f, entryPrice=%0.5f, distance=%0.5f. ", latestPrice, entry, distance)
	if useATR {
		reason += fmt.Sprintf("ATR=%0.5f, distanceATR=%0.4f, threshold=%0.4f, status=%s", atrVal, distanceATR, threshold, status)
	} else {
		reason += fmt.Sprintf("distancePct=%0.3f%%, thresholdPct=%0.3f%%, status=%s", distancePct, basePct, status)
	}
	reason += fmt.Sprintf(" | plan_status=%s plan_need_retest=%v", review.Status, review.NeedRetest)

	return StalenessResult{
		IsStale:         isStale,
		LastUpdatedTime: time.Now(),
		CurrentTime:     time.Now(),
		Status:          status,
		Reason:          reason,
	}
}
