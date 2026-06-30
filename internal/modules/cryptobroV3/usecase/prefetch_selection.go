package usecase

import (
	"math"
	"sort"
)

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

	// Hot-first: sort hot candidates by HotScore descending (highest hot score first)
	sort.SliceStable(hotCandidates, func(i, j int) bool {
		return hotCandidates[i].HotScore > hotCandidates[j].HotScore
	})

	// Sort rotation by activity score descending
	sort.SliceStable(rotationCandidates, func(i, j int) bool {
		return rotationCandidates[i].ActivityScore > rotationCandidates[j].ActivityScore
	})

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

	// Hot-first pipeline: allocate more slots to hot symbols (45% by default instead of 25%)
	ratio := policy.HotPrefetchSlotRatio
	if ratio <= 0 {
		ratio = 0.45 // Increased from 0.25 to prioritize hot coins
	}
	debug.HotSlots = int(math.Round(float64(prefetchLimit) * ratio))
	if debug.HotSlots < 1 && len(hotCandidates) > 0 {
		debug.HotSlots = 1
	}
	if debug.HotSlots > len(hotCandidates) {
		debug.HotSlots = len(hotCandidates)
	}

	debug.RotationSlots = resolveRotationPrefetchSlots(policy, prefetchLimit, len(rotationCandidates))

	// Hot-first: allocate hot slots BEFORE rotation slots
	for i := 0; i < debug.HotSlots; i++ {
		candidate := hotCandidates[i]
		candidate.HotOverlaySelected = true
		appendIfMissing(candidate)
	}

	for i := 0; i < debug.RotationSlots; i++ {
		appendIfMissing(rotationCandidates[i])
	}

	// Fill remaining with general candidates sorted by composite score (already sorted from universe)
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
		return defaultRotationActivityThresholdAlt
	case RISK_OFF, BTC_CHAOS, HIGH_VOL:
		if settings.RotationActivityThresholdDefensive > 0 {
			return settings.RotationActivityThresholdDefensive
		}
		return defaultRotationActivityThresholdDefensive
	case CHOP_RANGE, LOW_VOL:
		if settings.RotationActivityThresholdLowVol > 0 {
			return settings.RotationActivityThresholdLowVol
		}
		return defaultRotationActivityThresholdLowVol
	default:
		if settings.RotationActivityThresholdDefault > 0 {
			return settings.RotationActivityThresholdDefault
		}
		return defaultRotationActivityThresholdDefault
	}
}

func resolveRotationPrefetchSlots(policy MarketPolicy, prefetchLimit int, rotationCount int) int {
	if prefetchLimit < 6 || rotationCount == 0 {
		return 0
	}
	if policy.EffectiveRegime() == RISK_OFF || policy.EffectiveRegime() == HIGH_VOL || policy.EffectiveRegime() == BTC_CHAOS {
		return 0
	}

	settings := getRuntimeSettings()
	ratio := settings.RotationPrefetchRatioDefault
	if ratio <= 0 {
		ratio = defaultRotationPrefetchRatio
	}
	switch policy.EffectiveRegime() {
	case ALT_SUPPORTIVE, COMPRESSION:
		if settings.RotationPrefetchRatioAlt > 0 {
			ratio = settings.RotationPrefetchRatioAlt
		} else {
			ratio = defaultRotationPrefetchRatioAlt
		}
	case RISK_OFF, BTC_CHAOS, HIGH_VOL:
		if settings.RotationPrefetchRatioDefensive > 0 {
			ratio = settings.RotationPrefetchRatioDefensive
		} else {
			ratio = defaultRotationPrefetchRatioDefensive
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
