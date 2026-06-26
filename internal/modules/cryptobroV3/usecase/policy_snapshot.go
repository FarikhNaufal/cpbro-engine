package usecase

import (
	"strings"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/entity"
)

func policyAllowedPlaybookStrings(policy MarketPolicy) []string {
	if len(policy.AllowedPlaybooks) == 0 {
		return nil
	}
	out := make([]string, 0, len(policy.AllowedPlaybooks))
	for _, playbook := range policy.AllowedPlaybooks {
		out = append(out, string(playbook))
	}
	return out
}

func bootstrapCacheAgeSeconds(meta bootstrapFetchMeta) uint64 {
	if meta.CacheAge <= 0 {
		return 0
	}
	return uint64(meta.CacheAge / time.Second)
}

func applyPolicySnapshotToDecisionAudit(audit *DecisionAudit, policy MarketPolicy, compressionFallbackActive bool) {
	if audit == nil {
		return
	}
	profile := GetPlaybookThresholdProfile(audit.Playbook, policy, audit.Tier)
	audit.PolicyLongMode = string(policy.LongMode)
	audit.PolicyShortMode = string(policy.ShortMode)
	audit.PolicyRequireAIConfidence = string(effectiveRequiredAIConfidence(policy, profile))
	audit.PolicyRequireFreshEntry = effectiveRequireFreshEntry(policy)
	audit.PolicyAllowedPlaybooks = policyAllowedPlaybookStrings(policy)
	audit.PolicyReason = strings.TrimSpace(policy.Reason)
	audit.CompressionLowVolFallbackActive = compressionFallbackActive
}

func applyPolicySnapshotToSignalJournal(journal *SignalJournal, policy MarketPolicy) {
	if journal == nil {
		return
	}
	profile := GetPlaybookThresholdProfile(journal.Playbook, policy, journal.Tier)
	journal.PolicyLongMode = string(policy.LongMode)
	journal.PolicyShortMode = string(policy.ShortMode)
	journal.PolicyRequireAIConfidence = string(effectiveRequiredAIConfidence(policy, profile))
	journal.PolicyRequireFreshEntry = effectiveRequireFreshEntry(policy)
	journal.PolicyAllowedPlaybooks = policyAllowedPlaybookStrings(policy)
	journal.PolicyReason = strings.TrimSpace(policy.Reason)
}

func applyBootstrapProvenanceToDecisionAudit(audit *DecisionAudit, tickersMeta, fundingMeta bootstrapFetchMeta) {
	if audit == nil {
		return
	}
	audit.BootstrapTickerSource = string(tickersMeta.Source)
	audit.BootstrapTickerCacheAgeSeconds = bootstrapCacheAgeSeconds(tickersMeta)
	audit.BootstrapFundingSource = string(fundingMeta.Source)
	audit.BootstrapFundingCacheAgeSeconds = bootstrapCacheAgeSeconds(fundingMeta)
}

func applyPolicySnapshotToLatestResult(latest *entity.LatestResult, policy MarketPolicy) {
	if latest == nil {
		return
	}
	latest.ActivePolicyLongMode = string(policy.LongMode)
	latest.ActivePolicyShortMode = string(policy.ShortMode)
	latest.ActivePolicyRequireAIConfidence = string(effectiveRequiredAIConfidenceForPolicy(policy))
	latest.ActivePolicyRequireFreshEntry = effectiveRequireFreshEntry(policy)
	latest.ActivePolicyAllowedPlaybooks = policyAllowedPlaybookStrings(policy)
}

func applyBootstrapProvenanceToLatestResult(latest *entity.LatestResult, tickersMeta, fundingMeta bootstrapFetchMeta) {
	if latest == nil {
		return
	}
	latest.BootstrapTickerSource = string(tickersMeta.Source)
	latest.BootstrapTickerCacheAgeSeconds = bootstrapCacheAgeSeconds(tickersMeta)
	latest.BootstrapFundingSource = string(fundingMeta.Source)
	latest.BootstrapFundingCacheAgeSeconds = bootstrapCacheAgeSeconds(fundingMeta)
}
