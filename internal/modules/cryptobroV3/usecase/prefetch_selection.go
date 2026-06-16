package usecase

import "math"

type prefetchSelectionDebug struct {
	HotSlots      int
	RotationSlots int
}

func selectPrefetchCandidates(candidates []UniverseCandidate, prefetchLimit int, policy MarketPolicy) ([]UniverseCandidate, []UniverseCandidate, prefetchSelectionDebug) {
	debug := prefetchSelectionDebug{}
	if prefetchLimit <= 0 || prefetchLimit >= len(candidates) {
		return candidates, nil, debug
	}

	var hotCandidates []UniverseCandidate
	var rotationCandidates []UniverseCandidate

	rotationThreshold := resolveRotationActivityThreshold(policy)
	for _, candidate := range candidates {
		if candidate.IsHot {
			hotCandidates = append(hotCandidates, candidate)
			continue
		}
		if candidate.ActivityScore >= rotationThreshold {
			rotationCandidates = append(rotationCandidates, candidate)
		}
	}

	selected := make([]UniverseCandidate, 0, prefetchLimit)
	selectedMap := make(map[string]struct{}, prefetchLimit)

	appendIfMissing := func(candidate UniverseCandidate) {
		if len(selected) >= prefetchLimit {
			return
		}
		if _, exists := selectedMap[candidate.Symbol]; exists {
			return
		}
		selected = append(selected, candidate)
		selectedMap[candidate.Symbol] = struct{}{}
	}

	ratio := policy.HotPrefetchSlotRatio
	if ratio <= 0 {
		ratio = 0.25
	}
	debug.HotSlots = int(math.Round(float64(prefetchLimit) * ratio))
	if debug.HotSlots < 1 && len(hotCandidates) > 0 && prefetchLimit >= 3 {
		debug.HotSlots = 1
	}
	if debug.HotSlots > len(hotCandidates) {
		debug.HotSlots = len(hotCandidates)
	}

	debug.RotationSlots = resolveRotationPrefetchSlots(policy, prefetchLimit, len(rotationCandidates))

	for i := 0; i < debug.HotSlots; i++ {
		candidate := hotCandidates[i]
		candidate.HotOverlaySelected = true
		appendIfMissing(candidate)
	}

	for i := 0; i < debug.RotationSlots; i++ {
		appendIfMissing(rotationCandidates[i])
	}

	for _, candidate := range candidates {
		appendIfMissing(candidate)
	}

	deferred := make([]UniverseCandidate, 0, len(candidates)-len(selected))
	for _, candidate := range candidates {
		if _, exists := selectedMap[candidate.Symbol]; exists {
			continue
		}
		deferred = append(deferred, candidate)
	}

	return selected, deferred, debug
}

func resolveRotationActivityThreshold(policy MarketPolicy) float64 {
	settings := getRuntimeSettings()
	switch policy.EffectiveRegime() {
	case ALT_SUPPORTIVE, COMPRESSION:
		if settings.RotationActivityThresholdAlt > 0 {
			return settings.RotationActivityThresholdAlt
		}
		return 0.45
	case RISK_OFF, BTC_CHAOS, HIGH_VOL:
		if settings.RotationActivityThresholdDefensive > 0 {
			return settings.RotationActivityThresholdDefensive
		}
		return 0.65
	case CHOP_RANGE, LOW_VOL:
		if settings.RotationActivityThresholdLowVol > 0 {
			return settings.RotationActivityThresholdLowVol
		}
		return 0.50
	default:
		if settings.RotationActivityThresholdDefault > 0 {
			return settings.RotationActivityThresholdDefault
		}
		return 0.55
	}
}

func resolveRotationPrefetchSlots(policy MarketPolicy, prefetchLimit int, rotationCount int) int {
	if prefetchLimit < 6 || rotationCount == 0 {
		return 0
	}

	settings := getRuntimeSettings()
	ratio := settings.RotationPrefetchRatioDefault
	if ratio <= 0 {
		ratio = 0.15
	}
	switch policy.EffectiveRegime() {
	case ALT_SUPPORTIVE, COMPRESSION:
		if settings.RotationPrefetchRatioAlt > 0 {
			ratio = settings.RotationPrefetchRatioAlt
		} else {
			ratio = 0.20
		}
	case RISK_OFF, BTC_CHAOS, HIGH_VOL:
		if settings.RotationPrefetchRatioDefensive > 0 {
			ratio = settings.RotationPrefetchRatioDefensive
		} else {
			ratio = 0.10
		}
	}

	slots := int(math.Round(float64(prefetchLimit) * ratio))
	if slots < 1 && prefetchLimit >= 8 {
		slots = 1
	}
	if maxSlots := maxInt(1, prefetchLimit/3); slots > maxSlots {
		slots = maxSlots
	}
	if slots > rotationCount {
		slots = rotationCount
	}
	return slots
}
