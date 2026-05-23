package usecase

import (
	"fmt"
	"math"
	"strings"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
)

type StalenessUsecase struct {
	maxStaleness time.Duration
}

func NewStalenessUsecase(maxStaleness time.Duration) *StalenessUsecase {
	return &StalenessUsecase{
		maxStaleness: maxStaleness,
	}
}

// IsFresh checks if the latest closed candle timestamp is within maxStaleness.
func (uc *StalenessUsecase) IsFresh(m15Candles []dto.Candle) bool {
	if len(m15Candles) == 0 {
		return false
	}

	lastCandle := m15Candles[len(m15Candles)-1]
	return time.Since(lastCandle.Time) <= uc.maxStaleness
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

	regime := strings.ToUpper(policy.Reason)
	isChaos := strings.Contains(regime, "CHAOS")
	isHighVol := strings.Contains(regime, "HIGH_VOL") || isChaos
	isLowVol := strings.Contains(regime, "LOW_VOL")

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

	// Determine Fallback Pct threshold
	var basePct float64
	if isChaos {
		basePct = 0.20
	} else if isHighVol {
		basePct = 0.50
	} else if quant.Tier == TierC {
		basePct = 0.25
	} else {
		basePct = 0.35
	}

	atrVal, hasATR := quant.TechnicalSnapshot.IndicatorValues["ATR"]
	var distanceATR float64
	useATR := hasATR && atrVal > 0

	var status Status
	var isStale bool

	if useATR {
		distanceATR = distance / atrVal
		if distanceATR <= threshold {
			status = FRESH
			isStale = false
		} else if distanceATR <= threshold*1.5 {
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
		} else if distancePct <= basePct*1.5 {
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

	return StalenessResult{
		IsStale:         isStale,
		LastUpdatedTime: time.Now(),
		CurrentTime:     time.Now(),
		Status:          status,
		Reason:          reason,
	}
}
