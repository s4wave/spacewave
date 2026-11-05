package space_world_replica_controller

import (
	"context"
	"sync"
	"time"

	"github.com/aperturerobotics/util/ccontainer"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	space_world_replica "github.com/s4wave/spacewave/core/space/world/replica"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/sirupsen/logrus"
)

// reconciler observes and acts for one run of a replica controller. Only the
// run goroutine touches it.
type reconciler struct {
	// c is the controller.
	c *Controller
	// acc is the account hosting this Device's Session.
	acc *provider_local.ProviderAccount
	// key is the key of the Session this Device runs as.
	key crypto.PrivKey
	// list watches the account's SharedObjects.
	list ccontainer.Watchable[*sobject.SharedObjectList]
	// mount is the replicated Space while this Device participates, or nil.
	mount *mount
	// rechecked is set once this node asked for a recheck of the current copy.
	rechecked bool
	// recheckBase is the backfill outcome current when the recheck was asked
	// for. Only a later outcome comes from the recheck.
	recheckBase *sobject_world_engine.BackfillState
	// step is the last step taken, logged when it changes.
	step space_world_replica.ReplicaStep
}

// newReconciler constructs a reconciler for acc acting with the Session key.
func newReconciler(c *Controller, acc *provider_local.ProviderAccount, key crypto.PrivKey) *reconciler {
	return &reconciler{c: c, acc: acc, key: key, step: -1}
}

// run observes and acts after each wake until ctx ends.
func (r *reconciler) run(ctx context.Context) error {
	// Watch the account's SharedObjects.
	list, releaseList, err := r.acc.AccessSharedObjectList(ctx, nil)
	if err != nil {
		return err
	}
	defer releaseList()
	r.list = list

	// Wake on each change of the list until run returns.
	listCtx, stopList := context.WithCancel(ctx)
	var wg sync.WaitGroup
	initial := list.GetValue()
	wg.Go(func() { wakeOnChange(listCtx, list, initial, r.c.wake) })
	defer func() { stopList(); wg.Wait() }()

	// Take the next step after each change of either node's state, the list,
	// the Space or the copy, retrying a failed step after retryDelay.
	for {
		var waitCh <-chan struct{}
		r.c.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			waitCh = getWaitCh()
		})
		var retry <-chan time.Time
		if err := r.pass(ctx); err != nil && ctx.Err() == nil {
			r.c.le.WithError(err).WithField("step", r.step.String()).Warn("replica step failed")
			retry = time.After(retryDelay)
		}
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-waitCh:
		case <-retry:
		}
	}
}

// pass observes, picks the step and takes it.
func (r *reconciler) pass(ctx context.Context) error {
	// Observe and pick the step, logging a change of step.
	obs, err := r.observe(ctx)
	if err != nil {
		return err
	}
	step := space_world_replica.NextStep(obs)
	if step != r.step {
		var owners []string
		if obs.Ownership != nil {
			owners = obs.Ownership.Owners()
		}
		r.c.le.WithFields(logrus.Fields{
			"shared-object": obs.Spec.GetSharedObjectId(),
			"step":          step.String(),
			"owners":        owners,
		}).Info("replica step")
		r.step = step
	}

	// Take the step.
	return r.act(ctx, step, obs)
}

// observe returns what this node sees: both specs, the ownership, the invite,
// the copy's outcome and whether the other node's proof holds.
func (r *reconciler) observe(ctx context.Context) (*space_world_replica.Observation, error) {
	// Read the specs and the invite.
	spec := r.c.conf.GetSpec()
	peerState := r.c.getPeerState()
	obs := &space_world_replica.Observation{
		LocalPeerID:    r.c.local.String(),
		Spec:           spec,
		PeerSpec:       peerState.GetSpec(),
		InviteReceived: peerState.GetInvite() != nil,
	}

	// A Device that does not participate sees nothing more.
	if err := r.syncMount(ctx); err != nil || r.mount == nil {
		return obs, err
	}

	// Read the ownership and whether the desired owner participates.
	snap := r.mount.state.GetValue()
	own, err := snap.GetOwnership(ctx)
	if err != nil {
		return nil, err
	}
	obs.Ownership = own
	obs.StoragePeerID = r.mount.so.GetPeerID().String()
	desired, err := snap.GetParticipantConfigForPeer(ctx, spec.GetDesiredOwnerPeerId())
	obs.DesiredOwnerParticipates = err == nil && desired != nil

	// Read the copy's outcome and check the other node's proof.
	err = r.mount.accessBackfill(ctx, func(be sobject_world_engine.BackfillEngine) error {
		obs.Backfill = r.freshBackfill(be.GetBackfillState().GetValue())
		if obs.Owns() && peerState.GetProof() != nil {
			var perr error
			obs.ProofAccepted, perr = r.proofAccepted(ctx, be, peerState.GetProof())
			return perr
		}
		return nil
	})
	return obs, err
}

// syncMount mounts the Space while the account lists the SharedObject, and
// releases it once the account no longer does.
func (r *reconciler) syncMount(ctx context.Context) error {
	// Find the SharedObject in the account's list.
	id := r.c.conf.GetSpec().GetSharedObjectId()
	var ref *sobject.SharedObjectRef
	for _, entry := range r.list.GetValue().GetSharedObjects() {
		if entry.GetRef().GetProviderResourceRef().GetId() == id {
			ref = entry.GetRef()
		}
	}

	// Release a mount the account no longer lists, and mount one it does.
	if ref == nil {
		r.releaseMount()
		return nil
	}
	if r.mount != nil {
		return nil
	}
	m, err := openMount(ctx, r, ref)
	if err != nil {
		return err
	}
	r.mount = m
	return nil
}

// releaseMount releases the Space and forgets the recheck of its copy.
func (r *reconciler) releaseMount() {
	if r.mount != nil {
		r.mount.release()
		r.mount = nil
	}
	r.rechecked, r.recheckBase = false, nil
}

// freshBackfill returns the copy's outcome when it came from a recheck this
// node asked for, or a later copy, and nil otherwise.
func (r *reconciler) freshBackfill(st *sobject_world_engine.BackfillState) *sobject_world_engine.BackfillState {
	if !r.rechecked || st == nil || st == r.recheckBase {
		return nil
	}
	return st
}

// proofAccepted reports whether the other node's proof names the desired
// owner on the stream and covers the World this Device has installed.
func (r *reconciler) proofAccepted(ctx context.Context, be sobject_world_engine.BackfillEngine, signed *space_world_replica.SignedSOReplicaProof) (bool, error) {
	// Check who signed it and for what.
	spec := r.c.conf.GetSpec()
	proof, err := signed.Verify(spec.GetSharedObjectId(), spec.GetDesiredOwnerPeerId(), r.c.remote.String())
	if err != nil {
		r.c.le.WithError(err).Debug("replica proof refused")
		return false, nil
	}

	// Accept only a copy of the installed World.
	root, seqno, err := be.GetHead(ctx)
	if err != nil {
		return false, err
	}
	return proof.GetSeqno() == seqno && proof.GetRoot().EqualVT(root), nil
}
