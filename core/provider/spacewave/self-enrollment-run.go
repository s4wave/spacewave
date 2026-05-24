package provider_spacewave

import (
	"context"
	"slices"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/s4wave/spacewave/core/provider/spacewave/selfenrollmentrun"
	"github.com/s4wave/spacewave/core/sobject"
)

// SelfEnrollmentRunSnapshot is a snapshot of the visible self-enrollment run.
type SelfEnrollmentRunSnapshot struct {
	Running               bool
	CurrentSharedObjectID string
	CompletedIDs          []string
	Failures              []*SelfEnrollmentRunFailure
}

// SelfEnrollmentRunFailure describes a failed per-object enrollment.
type SelfEnrollmentRunFailure struct {
	SharedObjectID string
	Err            error
}

type selfEnrollmentRunState struct {
	acc *ProviderAccount

	// bcast guards run state fields below.
	bcast                 broadcast.Broadcast
	running               bool
	currentSharedObjectID string
	completedIDs          []string
	failures              []*SelfEnrollmentRunFailure
}

func newSelfEnrollmentRunState(acc *ProviderAccount) *selfEnrollmentRunState {
	return &selfEnrollmentRunState{acc: acc}
}

// GetSelfEnrollmentRunBroadcast returns the visible run broadcast.
func (a *ProviderAccount) GetSelfEnrollmentRunBroadcast() *broadcast.Broadcast {
	return &a.getSelfEnrollmentRun().bcast
}

// GetSelfEnrollmentRunSnapshot returns the visible run state.
func (a *ProviderAccount) GetSelfEnrollmentRunSnapshot() *SelfEnrollmentRunSnapshot {
	return a.getSelfEnrollmentRun().snapshot()
}

// WatchSelfEnrollmentRunSnapshot returns the visible run state and wait channel.
func (a *ProviderAccount) WatchSelfEnrollmentRunSnapshot() (*SelfEnrollmentRunSnapshot, <-chan struct{}) {
	return a.getSelfEnrollmentRun().watchSnapshot()
}

// StartSelfEnrollmentRun runs self-enrollment for the current pending set.
func (a *ProviderAccount) StartSelfEnrollmentRun(ctx context.Context) error {
	return a.getSelfEnrollmentRun().start(ctx)
}

func (a *ProviderAccount) getSelfEnrollmentRun() *selfEnrollmentRunState {
	if a.selfEnrollmentRun == nil {
		a.selfEnrollmentRun = newSelfEnrollmentRunState(a)
	}
	return a.selfEnrollmentRun
}

func (r *selfEnrollmentRunState) snapshot() *SelfEnrollmentRunSnapshot {
	var snapshot *SelfEnrollmentRunSnapshot
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		snapshot = &SelfEnrollmentRunSnapshot{
			Running:               r.running,
			CurrentSharedObjectID: r.currentSharedObjectID,
			CompletedIDs:          slices.Clone(r.completedIDs),
			Failures:              cloneSelfEnrollmentFailures(r.failures),
		}
	})
	return snapshot
}

func (r *selfEnrollmentRunState) watchSnapshot() (*SelfEnrollmentRunSnapshot, <-chan struct{}) {
	var snapshot *SelfEnrollmentRunSnapshot
	var ch <-chan struct{}
	r.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		ch = getWaitCh()
		snapshot = &SelfEnrollmentRunSnapshot{
			Running:               r.running,
			CurrentSharedObjectID: r.currentSharedObjectID,
			CompletedIDs:          slices.Clone(r.completedIDs),
			Failures:              cloneSelfEnrollmentFailures(r.failures),
		}
	})
	return snapshot, ch
}

func (r *selfEnrollmentRunState) start(ctx context.Context) error {
	req, err := r.buildRequest(ctx)
	if err != nil || req == nil {
		return err
	}
	if r.acc.selfEnrollmentRunRoutine == nil {
		return r.run(ctx, req)
	}
	r.acc.selfEnrollmentRunRoutine.SetState(req)
	return nil
}

func (a *ProviderAccount) shouldAutoStartSelfEnrollmentRunLocked(summary *SelfEnrollmentSummary) bool {
	var unlockedKeys int
	if store := a.GetEntityKeyStore(); store != nil {
		unlockedKeys = store.GetUnlockedCount()
	}
	return selfenrollmentrun.ShouldAutoStart(
		summary,
		a.state.selfEnrollmentSkippedGenerationKey,
		a.selfEnrollmentRunRoutine != nil,
		unlockedKeys,
	)
}

func (a *ProviderAccount) startSelfEnrollmentRunFromSummary(summary *SelfEnrollmentSummary) {
	if a.selfEnrollmentRunRoutine == nil {
		return
	}
	req := selfenrollmentrun.NewRequest(summary)
	if req != nil {
		a.selfEnrollmentRunRoutine.SetState(req)
	}
}

func (r *selfEnrollmentRunState) buildRequest(ctx context.Context) (*selfenrollmentrun.Request, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	store := r.acc.GetEntityKeyStore()
	if store == nil || store.GetUnlockedCount() == 0 {
		return nil, sobject.ErrSharedObjectRecoveryCredentialRequired
	}
	var summary *SelfEnrollmentSummary
	accountBcast := r.acc.GetAccountBroadcast()
	accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		summary = r.acc.GetSelfEnrollmentSummary()
	})
	if summary == nil || summary.GetCount() == 0 {
		return nil, nil
	}
	return selfenrollmentrun.NewRequest(summary), nil
}

func (r *selfEnrollmentRunState) run(ctx context.Context, req *selfenrollmentrun.Request) error {
	if req == nil {
		return nil
	}
	store := r.acc.GetEntityKeyStore()
	if store == nil || store.GetUnlockedCount() == 0 {
		return sobject.ErrSharedObjectRecoveryCredentialRequired
	}
	ref := r.acc.RetainEntityKeypairStepUp()
	defer ref.Release()

	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		r.running = true
		r.currentSharedObjectID = ""
		r.completedIDs = nil
		r.failures = nil
		broadcast()
	})
	defer r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		r.running = false
		r.currentSharedObjectID = ""
		broadcast()
	})

	for _, soID := range req.IDs {
		if err := ctx.Err(); err != nil {
			return err
		}
		r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			r.currentSharedObjectID = soID
			broadcast()
		})
		ref := sobject.NewSharedObjectRef(
			r.acc.GetProviderID(),
			r.acc.GetAccountID(),
			soID,
			SobjectBlockStoreID(soID),
		)
		_, rel, err := r.acc.MountSharedObject(ctx, ref, nil)
		if rel != nil {
			rel()
		}
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			failure := &SelfEnrollmentRunFailure{
				SharedObjectID: soID,
				Err:            err,
			}
			r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				r.failures = append(r.failures, failure)
				broadcast()
			})
			continue
		}
		r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			r.completedIDs = append(r.completedIDs, soID)
			broadcast()
		})
	}
	return r.acc.RefreshSelfEnrollmentSummary(ctx)
}

func cloneSelfEnrollmentFailures(
	failures []*SelfEnrollmentRunFailure,
) []*SelfEnrollmentRunFailure {
	if len(failures) == 0 {
		return nil
	}
	next := make([]*SelfEnrollmentRunFailure, len(failures))
	for i, failure := range failures {
		if failure != nil {
			next[i] = &SelfEnrollmentRunFailure{
				SharedObjectID: failure.SharedObjectID,
				Err:            failure.Err,
			}
		}
	}
	return next
}
