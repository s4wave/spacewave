package provider_spacewave

import (
	"context"

	protobuf_go_lite "github.com/aperturerobotics/protobuf-go-lite"
	"github.com/s4wave/spacewave/core/session"
)

// selfRejoinSweepState captures the meaningful sweep inputs so the routine only
// reruns when bootstrap/mutation eligibility is met and one of the underlying
// cloud/session triggers materially changes.
type selfRejoinSweepState struct {
	// fetchedEpoch is the last successfully fetched cloud epoch.
	fetchedEpoch uint64
	// generation increments on new local session registration and WS reconnect.
	generation uint64
	// keypairs is the last fetched entity keypair snapshot.
	keypairs []*session.EntityKeypair
}

func equalSelfRejoinSweepState(
	v1, v2 *selfRejoinSweepState,
) bool {
	if v1 == nil || v2 == nil {
		return v1 == v2
	}
	return v1.fetchedEpoch == v2.fetchedEpoch &&
		v1.generation == v2.generation &&
		protobuf_go_lite.IsEqualVTSlice(v1.keypairs, v2.keypairs)
}

// buildSelfRejoinSweepStateLocked builds the desired sweep state.
// Must be called within an accountBcast HoldLock scope.
func (a *ProviderAccount) buildSelfRejoinSweepStateLocked() *selfRejoinSweepState {
	if !a.state.accountBootstrapFetched || a.state.selfRejoinSweepGeneration == 0 {
		return nil
	}
	if !providerAccountStatusAllowsCloudMutation(a.state.status) ||
		!cloudSelfEnrollmentAllowed(a.state.info) {
		return nil
	}

	cli := a.sessionClient
	if cli == nil || cli.priv == nil || cli.peerID == "" {
		return nil
	}

	return &selfRejoinSweepState{
		fetchedEpoch: a.state.lastFetchedEpoch,
		generation:   a.state.selfRejoinSweepGeneration,
		keypairs:     cloneEntityKeypairs(a.KeypairsSnapshot()),
	}
}

// setSelfRejoinSweepState updates the desired sweep target.
func (a *ProviderAccount) setSelfRejoinSweepState(state *selfRejoinSweepState) {
	if a.selfRejoinSweep == nil {
		return
	}
	a.setSelfRejoinSweepRunning(state != nil)
	_, _, _, running := a.selfRejoinSweep.SetState(state)
	if !running {
		a.setSelfRejoinSweepRunning(false)
	}
}

// GetSelfRejoinSweepRunning returns true while automatic post-login rejoin is active.
// Must be called within an accountBcast HoldLock scope.
func (a *ProviderAccount) GetSelfRejoinSweepRunning() bool {
	return a.state.selfRejoinSweepRunning
}

func (a *ProviderAccount) setSelfRejoinSweepRunning(running bool) {
	a.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if a.state.selfRejoinSweepRunning == running {
			return
		}
		a.state.selfRejoinSweepRunning = running
		broadcast()
	})
}

// refreshSelfRejoinSweepState rebuilds the desired sweep target from current
// cached account/session state.
func (a *ProviderAccount) refreshSelfRejoinSweepState() {
	var state *selfRejoinSweepState
	a.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		state = a.buildSelfRejoinSweepStateLocked()
	})
	a.setSelfRejoinSweepState(state)
}

// bumpSelfRejoinSweepGeneration records a new sweep-worthy trigger and updates
// the desired sweep target.
func (a *ProviderAccount) bumpSelfRejoinSweepGeneration() {
	var state *selfRejoinSweepState
	a.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		a.state.selfRejoinSweepGeneration++
		state = a.buildSelfRejoinSweepStateLocked()
		broadcast()
	})
	a.setSelfRejoinSweepState(state)
}

func (a *ProviderAccount) primeSelfRejoinSweepFromUnlockedEntityKeys() {
	if a.entityKeyStore == nil || a.entityKeyStore.GetUnlockedCount() == 0 {
		return
	}
	a.bumpSelfRejoinSweepGeneration()
}

// runSelfRejoinSweep opportunistically mounts all accessible SOs so the
// mount-time recovery path can repair missing same-entity peers ahead of UI
// navigation.
func (a *ProviderAccount) runSelfRejoinSweep(
	ctx context.Context,
	state *selfRejoinSweepState,
) error {
	a.setSelfRejoinSweepRunning(true)

	if state == nil || !a.canSelfEnrollCloudObjects() {
		a.setSelfRejoinSweepRunning(false)
		return nil
	}

	if err := a.EnsureSharedObjectListLoaded(ctx); err != nil {
		return err
	}

	list := a.soListCtr.GetValue()
	if list == nil {
		return nil
	}

	for _, entry := range list.GetSharedObjects() {
		if err := ctx.Err(); err != nil {
			return err
		}
		ref := entry.GetRef()
		if ref == nil {
			continue
		}
		soID := ref.GetProviderResourceRef().GetId()
		_, rel, err := a.MountSharedObject(ctx, ref, nil)
		if rel != nil {
			rel()
		}
		if err != nil {
			a.le.WithError(err).
				WithField("sobject-id", soID).
				Debug("self-rejoin sweep skipped shared object")
		}
		if err := a.processPendingMailboxEntries(ctx, soID); err != nil {
			a.le.WithError(err).
				WithField("sobject-id", soID).
				Debug("self-rejoin sweep mailbox processing skipped shared object")
		}
	}
	a.setSelfRejoinSweepRunning(false)
	return nil
}

func cloneEntityKeypairs(
	keypairs []*session.EntityKeypair,
) []*session.EntityKeypair {
	if len(keypairs) == 0 {
		return nil
	}
	next := make([]*session.EntityKeypair, len(keypairs))
	for i, keypair := range keypairs {
		if keypair != nil {
			next[i] = keypair.CloneVT()
		}
	}
	return next
}
