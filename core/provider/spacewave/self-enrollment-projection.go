package provider_spacewave

import "github.com/s4wave/spacewave/core/provider/spacewave/selfenrollmentprojection"

// SelfEnrollmentProjection is the backend state projection for Session Self-Enrollment.
type SelfEnrollmentProjection = selfenrollmentprojection.Projection

// SelfEnrollmentRunSnapshot is a snapshot of the visible self-enrollment run.
type SelfEnrollmentRunSnapshot = selfenrollmentprojection.RunSnapshot

// SelfEnrollmentRunFailure describes a failed per-object enrollment.
type SelfEnrollmentRunFailure = selfenrollmentprojection.RunFailure

// BuildSelfEnrollmentProjection builds a Session Self-Enrollment projection
// from cached backend state without fetching from the cloud.
func (a *ProviderAccount) BuildSelfEnrollmentProjection() *SelfEnrollmentProjection {
	var summary *SelfEnrollmentSummary
	var skippedKey string
	a.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if a.state.selfEnrollmentSummary != nil {
			summary = a.state.selfEnrollmentSummary.clone()
		}
		skippedKey = a.state.selfEnrollmentSkippedGenerationKey
	})

	run := a.GetSelfEnrollmentRunSnapshot()
	store := a.GetEntityKeyStore()
	unlockedCount := 0
	if store != nil {
		unlockedCount = store.GetUnlockedCount()
	}
	return buildSelfEnrollmentProjection(summary, run, skippedKey, store != nil, unlockedCount)
}

// WatchSelfEnrollmentProjection returns the current projection and the backend
// wait channels that can change it.
func (a *ProviderAccount) WatchSelfEnrollmentProjection() (
	*SelfEnrollmentProjection,
	SelfEnrollmentProjectionWatch,
) {
	var summary *SelfEnrollmentSummary
	var skippedKey string
	var watch SelfEnrollmentProjectionWatch
	a.accountBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		watch.AccountCh = getWaitCh()
		if a.state.selfEnrollmentSummary != nil {
			summary = a.state.selfEnrollmentSummary.clone()
		}
		skippedKey = a.state.selfEnrollmentSkippedGenerationKey
	})

	run, runCh := a.WatchSelfEnrollmentRunSnapshot()
	watch.RunCh = runCh

	store := a.GetEntityKeyStore()
	unlockedCount := 0
	if store != nil {
		unlockedCount, watch.EntityKeyCh = store.WatchUnlockedCount()
	}
	return buildSelfEnrollmentProjection(summary, run, skippedKey, store != nil, unlockedCount), watch
}

func buildSelfEnrollmentProjection(
	summary *SelfEnrollmentSummary,
	run *SelfEnrollmentRunSnapshot,
	skippedKey string,
	hasEntityKeyStore bool,
	unlockedCount int,
) *SelfEnrollmentProjection {
	return selfenrollmentprojection.Build(selfenrollmentprojection.Snapshot{
		Summary:                 selfEnrollmentProjectionSummary(summary),
		Run:                     run,
		SkippedGenerationKey:    skippedKey,
		HasCredentialStore:      hasEntityKeyStore,
		UnlockedCredentialCount: unlockedCount,
	})
}

func selfEnrollmentProjectionSummary(
	summary *SelfEnrollmentSummary,
) selfenrollmentprojection.Summary {
	if summary == nil {
		return selfenrollmentprojection.Summary{}
	}
	return selfenrollmentprojection.Summary{
		Present:       true,
		SharedObjects: summary.GetIDs(),
		GenerationKey: summary.GetGenerationKey(),
		Count:         summary.GetCount(),
		Loaded:        summary.GetLoaded(),
	}
}
