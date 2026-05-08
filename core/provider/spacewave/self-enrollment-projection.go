package provider_spacewave

import "slices"

// SelfEnrollmentProjection is the backend state projection for Session
// Self-Enrollment. It is built only from Provider Account-owned state.
type SelfEnrollmentProjection struct {
	SharedObjectIDs          []string
	GenerationKey            string
	Count                    uint32
	PendingLoaded            bool
	CredentialRequired       bool
	Running                  bool
	CurrentSharedObjectID    string
	CompletedSharedObjectIDs []string
	Skipped                  bool
	SkippedGenerationKey     string
	Failures                 []*SelfEnrollmentRunFailure
}

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
	proj := &SelfEnrollmentProjection{
		SkippedGenerationKey: skippedKey,
	}
	if run != nil {
		proj.Running = run.Running
		proj.CurrentSharedObjectID = run.CurrentSharedObjectID
		proj.CompletedSharedObjectIDs = slices.Clone(run.CompletedIDs)
		proj.Failures = cloneSelfEnrollmentFailures(run.Failures)
	}
	if summary == nil {
		return proj
	}
	proj.SharedObjectIDs = summary.GetIDs()
	proj.GenerationKey = summary.GetGenerationKey()
	proj.Count = summary.GetCount()
	proj.PendingLoaded = summary.GetLoaded()
	proj.CredentialRequired = summary.GetCount() != 0 &&
		(!hasEntityKeyStore || unlockedCount == 0)
	proj.Skipped = skippedKey != "" && skippedKey == summary.GetGenerationKey()
	return proj
}
