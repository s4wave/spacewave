package provider_local

import (
	"bytes"
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/routine"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_invite "github.com/s4wave/spacewave/core/sobject/invite"
	sobject_sync "github.com/s4wave/spacewave/core/sobject/sync"
	"github.com/s4wave/spacewave/core/space"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/db/block"
	dex_solicit "github.com/s4wave/spacewave/db/dex/solicit"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
)

// p2pSyncState holds P2P sync startup or running state and DEX stores.
type p2pSyncState struct {
	// bcast guards every lifecycle and resource field below.
	bcast broadcast.Broadcast

	// ctx carries cancellation across startup and running workers.
	ctx context.Context
	// sessionTransport owns the child bus used by this sync generation.
	sessionTransport *transport.SessionTransport
	// cancel ends ctx when the state enters retirement.
	cancel context.CancelFunc
	// owners counts callers and successor states retaining this generation.
	owners int
	// startComplete reports that startup published its final result.
	startComplete bool
	// started reports that startup completed successfully.
	started bool
	// startupExited reports that startup exited or became a registered worker.
	startupExited bool
	// stopping reports that cancellation and retirement have begun.
	stopping bool
	// cleanupRunning elects the goroutine responsible for cleanup.
	cleanupRunning bool
	// cleanupDone reports that all registered resources were released.
	cleanupDone bool
	// restartPending requests another startup pass before publishing success.
	restartPending bool
	// startErr records the final startup error.
	startErr error
	// lowerSource provides stores from the generation being replaced.
	lowerSource *p2pSyncState
	// lowerSourceHeld reports that this state owns a reference to lowerSource.
	lowerSourceHeld bool
	// workers counts registered background workers that cleanup must await.
	workers int
	// refs holds controllerbus references until cleanup.
	refs []directive.Reference
	// peerRefs indexes retained peer directives whose references also appear in refs.
	peerRefs map[string]directive.Reference
	// relFns holds non-directive resource releases until cleanup.
	relFns []func()
	// stores indexes DEX stores by bucket ID for this generation.
	stores map[string]block.StoreOps
	// exchanges owns the traffic counters for each mounted DEX bucket.
	exchanges map[string]*dex_solicit.Controller
	// soSync indexes restartable shared-object sync routines.
	soSync map[string]*accountObjectSync
	// copyProgress records each Space's latest local durability check.
	copyProgress map[string]*AccountReplicaCopyState
	// replicaSync follows the canonical account catalog and Session membership.
	replicaSync *routine.RoutineContainer
}

// retainP2PSyncStateLocked adds an owner while a.p2pSyncBcast is locked.
// It returns false when either the owner or state lifetime has ended.
func (a *ProviderAccount) retainP2PSyncStateLocked(ctx context.Context, state *p2pSyncState) bool {
	if ctx.Err() != nil {
		return false
	}

	retained := false
	state.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		if state.stopping || state.ctx.Err() != nil || ctx.Err() != nil {
			return
		}
		state.owners++
		retained = true
		bcast()
	})
	return retained
}

// retainP2PSyncLowerSourceLocked retains a state as the store source for its
// successor while a.p2pSyncBcast is locked.
func (a *ProviderAccount) retainP2PSyncLowerSourceLocked(state *p2pSyncState) bool {
	retained := false
	state.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		if state.stopping || state.ctx.Err() != nil {
			return
		}
		state.owners++
		retained = true
		bcast()
	})
	return retained
}

// watchP2PSyncOwner releases one state owner when ctx ends. A context without a
// cancellation channel leaves its owner retained until explicit retirement.
func (a *ProviderAccount) watchP2PSyncOwner(ctx context.Context, state *p2pSyncState) {
	// Ignore contexts that cannot signal cancellation.
	if ctx.Done() == nil {
		return
	}

	// Watch ownership until the state stops or the caller cancels.
	go func() {
		// Reconcile state ownership on each lifecycle transition.
		for {
			var waitCh <-chan struct{}
			var stopped bool
			state.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
				stopped = state.stopping
				if !stopped {
					waitCh = getWaitCh()
				}
			})
			if stopped {
				return
			}

			// Release the state when the caller exits.
			select {
			case <-ctx.Done():
				a.releaseP2PSyncState(state)
				return
			case <-waitCh:
			}
		}
	}()
}

// releaseP2PSyncState drops one owner and retires the state after the last one.
func (a *ProviderAccount) releaseP2PSyncState(state *p2pSyncState) {
	// Decrement ownership and mark the state for retirement.
	retire := false
	state.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		if state.owners == 0 {
			return
		}
		state.owners--
		if state.owners != 0 {
			bcast()
			return
		}
		state.cancel()
		state.stopping = true
		if !state.startComplete {
			state.startErr = context.Canceled
			state.startComplete = true
		}
		bcast()
		retire = true
	})

	// Retire the state after the final owner releases it.
	if retire {
		a.retireP2PSyncState(state)
	}
}

// releaseP2PSyncLowerSource releases the predecessor retained by state.
func (a *ProviderAccount) releaseP2PSyncLowerSource(state *p2pSyncState) {
	var lower *p2pSyncState
	state.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		if !state.lowerSourceHeld {
			return
		}
		lower = state.lowerSource
		state.lowerSource = nil
		state.lowerSourceHeld = false
		bcast()
	})
	if lower != nil {
		a.releaseP2PSyncState(lower)
	}
}

// addStore registers a DEX store for lookup by bucket ID.
func (s *p2pSyncState) addStore(bucketID string, store block.StoreOps) {
	s.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		if s.stores == nil {
			s.stores = make(map[string]block.StoreOps)
		}
		s.stores[bucketID] = store
		bcast()
	})
}

// hasStore reports whether this generation has registered bucketID.
func (s *p2pSyncState) hasStore(bucketID string) bool {
	var ok bool
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		_, ok = s.stores[bucketID]
	})
	return ok
}

// hasSO reports whether this generation started sync for soID.
func (s *p2pSyncState) hasSO(soID string) bool {
	var ok bool
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		_, ok = s.soSync[soID]
	})
	return ok
}

// addSO registers the sync routine for soID while the generation is active.
func (s *p2pSyncState) addSO(soID string, syncRoutine *accountObjectSync) bool {
	var added bool
	s.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		if s.stopping || s.ctx.Err() != nil {
			return
		}
		if s.soSync == nil {
			s.soSync = make(map[string]*accountObjectSync)
		}
		if _, exists := s.soSync[soID]; exists {
			return
		}
		s.soSync[soID] = syncRoutine
		added = true
		bcast()
	})
	return added
}

// addRef retains a controllerbus reference until state cleanup.
func (s *p2pSyncState) addRef(ref directive.Reference) {
	s.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		s.refs = append(s.refs, ref)
		bcast()
	})
}

// addRelease retains a cleanup function until state cleanup.
func (s *p2pSyncState) addRelease(rel func()) {
	s.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		s.relFns = append(s.relFns, rel)
		bcast()
	})
}

// addWorker registers a background worker that cleanup must await.
func (s *p2pSyncState) addWorker() {
	s.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		s.workers++
		bcast()
	})
}

// workerDone releases one background worker registration.
func (s *p2pSyncState) workerDone() {
	s.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		if s.workers == 0 {
			return
		}
		s.workers--
		bcast()
	})
}

// getStore returns this generation's store or its startup predecessor's store.
func (s *p2pSyncState) getStore(bucketID string) block.StoreOps {
	var store block.StoreOps
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		store = s.stores[bucketID]
		if store == nil && !s.started && s.lowerSource != nil {
			store = s.lowerSource.getStore(bucketID)
		}
	})
	return store
}

// StartP2PSync starts SO sync and DEX block exchange for all mounted
// shared objects. Called when a P2P-linked device connects.
//
// The method waits for sessionTransport readiness and starts sync controllers
// on its child bus.
func (a *ProviderAccount) StartP2PSync(ctx context.Context, sessionTransport *transport.SessionTransport) error {
	return a.startP2PSync(ctx, ctx, sessionTransport)
}

// StartPersistentP2PSync starts P2P sync for the provider-account lifetime
// while bounding startup by ctx.
func (a *ProviderAccount) StartPersistentP2PSync(ctx context.Context, sessionTransport *transport.SessionTransport) error {
	if a.lifecycleCtx == nil {
		return errors.New("provider account lifecycle is unavailable")
	}
	return a.startP2PSync(ctx, a.lifecycleCtx, sessionTransport)
}

// startP2PSync selects or replaces the current sync generation. ctx bounds the
// readiness and startup wait; ownerCtx retains a new or in-progress generation.
func (a *ProviderAccount) startP2PSync(ctx, ownerCtx context.Context, sessionTransport *transport.SessionTransport) error {
	if err := sessionTransport.AwaitReady(ctx); err != nil {
		return err
	}
	childBus := sessionTransport.GetChildBus()
	if childBus == nil {
		return errors.New("session transport child bus is not ready")
	}

	var (
		previous         *p2pSyncState
		previousRetained bool
		waitState        *p2pSyncState
		state            *p2pSyncState
		watch            bool
	)
	a.p2pSyncBcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		previous = a.p2pSync
		if previous != nil && previous.sessionTransport == sessionTransport {
			previous.bcast.HoldLock(func(stateBcast func(), _ func() <-chan struct{}) {
				if previous.stopping || previous.ctx.Err() != nil || ownerCtx.Err() != nil {
					return
				}
				if previous.started && previous.startErr == nil {
					// The running pass watches the SO list. A later session mount must
					// reuse it instead of dropping every active link and DEX stream.
					waitState = previous
					return
				}
				if previous.startComplete {
					return
				}
				previous.owners++
				previous.restartPending = true
				stateBcast()
				waitState = previous
				watch = true
			})
			if waitState != nil {
				return
			}
		}

		syncCtx, syncCancel := context.WithCancel(context.WithoutCancel(ctx))
		state = &p2pSyncState{
			ctx:              syncCtx,
			sessionTransport: sessionTransport,
			cancel:           syncCancel,
		}
		if !a.retainP2PSyncStateLocked(ownerCtx, state) {
			syncCancel()
			state = nil
			return
		}
		if previous != nil && a.retainP2PSyncLowerSourceLocked(previous) {
			state.lowerSource = previous
			state.lowerSourceHeld = true
			previousRetained = true
		}
		a.p2pSync = state
		bcast()
		watch = true
	})
	if watch {
		watchState := state
		if waitState != nil {
			watchState = waitState
		}
		a.watchP2PSyncOwner(ownerCtx, watchState)
	}
	if state == nil {
		if waitState == nil {
			return ctx.Err()
		}
		return a.awaitP2PSyncStart(ctx, waitState)
	}

	// Startup belongs to every holder of the state, not to the caller that
	// happened to create it. Running it here would tie it to that caller's
	// goroutine, so a caller whose context is canceled while later holders keep
	// the state alive could not return until startup finished on its own.
	go a.runP2PSyncStart(state, previous, previousRetained, sessionTransport, childBus)
	return a.awaitP2PSyncStart(ctx, state)
}

// markP2PPendingEnrollPeer records a device peer recorded by SpaceLink
// approval whose invite has not been consumed yet. Auto-start skips these
// peers so the owner never dials a device that has not connected once in
// this process; the Device dials the owner through the one-use invite.
func (a *ProviderAccount) markP2PPendingEnrollPeer(remotePeerID peer.ID) {
	a.p2pSyncBcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		if a.p2pPendingEnrollPeers == nil {
			a.p2pPendingEnrollPeers = make(map[string]struct{})
		}
		a.p2pPendingEnrollPeers[remotePeerID.String()] = struct{}{}
		bcast()
	})
}

// clearP2PPendingEnrollPeer removes a device peer from the pending-enrollment
// set after the device joined through its invite.
func (a *ProviderAccount) clearP2PPendingEnrollPeer(remotePeerID peer.ID) {
	a.p2pSyncBcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		delete(a.p2pPendingEnrollPeers, remotePeerID.String())
		bcast()
	})
}

// RetainP2PPeer keeps an EstablishLinkWithPeer directive across P2P sync
// state restarts. Device enrollment calls it with the persisted invite owner
// so daemon restart reconnects without repeating the one-use invite.
func (a *ProviderAccount) RetainP2PPeer(ctx context.Context, remotePeerID peer.ID) error {
	if remotePeerID == "" {
		return errors.New("remote P2P peer ID is required")
	}
	var state *p2pSyncState
	a.p2pSyncBcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		if a.p2pPeerIDs == nil {
			a.p2pPeerIDs = make(map[string]peer.ID)
		}
		a.p2pPeerIDs[remotePeerID.String()] = remotePeerID
		state = a.p2pSync
		bcast()
	})
	if state == nil {
		return errors.New("P2P sync is not running")
	}
	return a.retainP2PPeerOnState(ctx, state, remotePeerID)
}

// retainP2PPeerOnState retains one peer-link directive for the state lifetime.
func (a *ProviderAccount) retainP2PPeerOnState(
	ctx context.Context,
	state *p2pSyncState,
	remotePeerID peer.ID,
) error {
	if remotePeerID == state.sessionTransport.GetPeerID() {
		return nil
	}
	remoteKey := remotePeerID.String()
	var alreadyRetained bool
	state.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		_, alreadyRetained = state.peerRefs[remoteKey]
	})
	if alreadyRetained {
		return nil
	}
	childBus := state.sessionTransport.GetChildBus()
	if childBus == nil {
		return errors.New("session transport child bus is not ready")
	}
	// Keep an attached value reference as well as the directive reference.
	// A directive with no value handler leaves its MountedLink unreferenced, so
	// controllerbus disposes the link after EstablishLinkWithPeer's grace period.
	handler := directive.NewTypedCallbackHandler[link.MountedLink](
		func(directive.TypedAttachedValue[link.MountedLink]) {},
		nil,
		nil,
		nil,
	)
	_, ref, err := childBus.AddDirective(
		link.NewEstablishLinkWithPeer(state.sessionTransport.GetPeerID(), remotePeerID),
		handler,
	)
	if err != nil {
		return err
	}

	retained := false
	state.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		if state.stopping || state.ctx.Err() != nil {
			return
		}
		if state.peerRefs == nil {
			state.peerRefs = make(map[string]directive.Reference)
		}
		if _, exists := state.peerRefs[remoteKey]; exists {
			return
		}
		state.peerRefs[remoteKey] = ref
		state.refs = append(state.refs, ref)
		retained = true
		bcast()
	})
	if !retained {
		ref.Release()
	}
	return nil
}

// retainConfiguredP2PPeers retains every account peer on state.
func (a *ProviderAccount) retainConfiguredP2PPeers(state *p2pSyncState) error {
	var peers []peer.ID
	a.p2pSyncBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		peers = make([]peer.ID, 0, len(a.p2pPeerIDs))
		for _, remotePeerID := range a.p2pPeerIDs {
			peers = append(peers, remotePeerID)
		}
	})
	for _, remotePeerID := range peers {
		if err := a.retainP2PPeerOnState(state.ctx, state, remotePeerID); err != nil {
			return err
		}
	}
	return nil
}

// awaitP2PSyncStart waits for the shared startup to finish or for the caller's
// own context to end, whichever comes first. Giving up releases this caller's
// reference without stopping the run; it ends only when the last holder leaves.
func (a *ProviderAccount) awaitP2PSyncStart(ctx context.Context, state *p2pSyncState) error {
	for {
		var waitCh <-chan struct{}
		var complete bool
		state.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			complete = state.startComplete && (state.startErr != nil || !state.lowerSourceHeld)
			if !complete {
				waitCh = getWaitCh()
			}
		})
		if complete {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-waitCh:
		}
	}

	var running bool
	var startErr error
	a.p2pSyncBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		running = a.p2pSync == state
		state.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			startErr = state.startErr
		})
	})
	if startErr != nil {
		return startErr
	}
	if running {
		return nil
	}
	if err := state.ctx.Err(); err != nil {
		return err
	}
	return errors.New("P2P sync stopped during startup")
}

// finishStart records a stable startup result or consumes a pending restart.
func (s *p2pSyncState) finishStart(err error) bool {
	restart := false
	s.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		if err == nil && s.stopping {
			err = context.Canceled
		}
		if err == nil {
			err = s.ctx.Err()
		}
		if err == nil && s.restartPending && !s.stopping && !s.startComplete {
			s.restartPending = false
			restart = true
			bcast()
			return
		}
		if s.startComplete {
			return
		}
		s.startErr = err
		s.started = err == nil
		s.startComplete = true
		bcast()
	})
	return restart
}

// markStartupExited allows cleanup to proceed once registered workers finish.
func (s *p2pSyncState) markStartupExited() {
	s.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		s.startupExited = true
		bcast()
	})
}

// runP2PSyncStart performs startup and publishes each stable lifecycle outcome
// through the state broadcast.
func (a *ProviderAccount) runP2PSyncStart(
	state *p2pSyncState,
	previous *p2pSyncState,
	previousRetained bool,
	sessionTransport *transport.SessionTransport,
	childBus bus.Bus,
) {
	var (
		err           error
		inviteStarted bool
		soList        *sobject.SharedObjectList
	)
	for {
		soList = a.soListCtr.GetValue()
		err = a.startP2PSyncControllers(
			state,
			sessionTransport,
			childBus,
			&inviteStarted,
			soList,
		)
		if state.finishStart(err) {
			continue
		}
		state.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			err = state.startErr
		})
		a.p2pSyncBcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
			bcast()
		})
		break
	}
	if err == nil {
		// Keep list reconciliation in this owned worker after startup completes.
		state.addWorker()
		defer state.workerDone()
		a.releaseP2PSyncLowerSource(state)
		state.markStartupExited()
		if previous != nil {
			a.retireP2PSyncState(previous)
		}
		for {
			soList, err = a.soListCtr.WaitValueChange(state.ctx, soList, nil)
			if err != nil {
				return
			}
			if err := a.startP2PSyncControllers(state, sessionTransport, childBus, &inviteStarted, soList); err != nil {
				if state.ctx.Err() == nil {
					a.le.WithError(err).Warn("failed to synchronize added shared objects")
				}
				return
			}
		}
	}

	state.markStartupExited()
	a.restoreP2PSyncAfterFailedStart(state, previous, previousRetained)
	a.retireP2PSyncState(state)
}

// restoreP2PSyncAfterFailedStart reconciles the account state and predecessor
// cleanup after state fails to start.
func (a *ProviderAccount) restoreP2PSyncAfterFailedStart(
	state *p2pSyncState,
	previous *p2pSyncState,
	previousRetained bool,
) {
	retirePrevious := false
	a.p2pSyncBcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		if a.p2pSync != state {
			return
		}
		restorePrevious := false
		if previousRetained {
			previous.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
				restorePrevious = !previous.stopping && previous.ctx.Err() == nil
			})
		}
		a.p2pSync = nil
		retirePrevious = previous != nil
		if restorePrevious {
			a.p2pSync = previous
		}
		bcast()
	})
	if retirePrevious {
		a.retireP2PSyncState(previous)
	}
}

// startP2PSyncControllers reconciles peer, DEX, shared-object, and invite
// controllers with soList for one sync generation.
func (a *ProviderAccount) startP2PSyncControllers(
	state *p2pSyncState,
	sessionTransport *transport.SessionTransport,
	childBus bus.Bus,
	inviteStarted *bool,
	soList *sobject.SharedObjectList,
) error {
	syncCtx := state.ctx
	state.removeMissingObjects(soList)
	if err := a.retainConfiguredP2PPeers(state); err != nil {
		return errors.Wrap(err, "retain configured P2P peers")
	}
	for _, entry := range soList.GetSharedObjects() {
		ref := entry.GetRef()
		provRef := ref.GetProviderResourceRef()
		soID := provRef.GetId()
		blockStoreID := ref.GetBlockStoreId()

		providerID := provRef.GetProviderId()
		providerAccountID := provRef.GetProviderAccountId()
		bucketID := BlockStoreBucketID(providerID, providerAccountID, blockStoreID)
		if !state.hasStore(bucketID) {
			if err := a.startDEXSolicit(syncCtx, childBus, bucketID, soID, state); err != nil {
				if syncCtx.Err() != nil {
					return syncCtx.Err()
				}
				a.le.WithError(err).WithField("bucket-id", bucketID).Warn("failed to start dex solicit")
			}
		}

		if state.hasSO(soID) {
			continue
		}
		if err := a.startSOSync(syncCtx, childBus, ref, entry.GetMeta().GetBodyType(), soID, state); err != nil {
			if syncCtx.Err() != nil {
				return syncCtx.Err()
			}
			a.le.WithError(err).WithField("so-id", soID).Warn("failed to start so sync")
			continue
		}
	}

	if !*inviteStarted {
		if err := a.startInviteServer(syncCtx, childBus, sessionTransport, state); err != nil {
			if syncCtx.Err() != nil {
				return syncCtx.Err()
			}
			a.le.WithError(err).Warn("failed to start invite server")
		} else {
			if err := a.startAccountReplicaSync(state); err != nil {
				return err
			}
			*inviteStarted = true
		}
	}
	return syncCtx.Err()
}

// GetP2PSyncSnapshotWithWait returns whether P2P sync is running and a channel
// that closes when its lifecycle changes.
func (a *ProviderAccount) GetP2PSyncSnapshotWithWait() (bool, <-chan struct{}) {
	var (
		running bool
		ch      <-chan struct{}
	)
	a.p2pSyncBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		ch = getWaitCh()
		if a.p2pSync == nil {
			return
		}
		a.p2pSync.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			running = a.p2pSync.started && !a.p2pSync.stopping
		})
	})
	return running, ch
}

// IsP2PSyncRunning returns whether P2P sync is currently active.
// Safe to call from any goroutine.
func (a *ProviderAccount) IsP2PSyncRunning() bool {
	running, _ := a.GetP2PSyncSnapshotWithWait()
	return running
}

// RetrySharedObjectSync replaces the running SO sync directive for soID while
// preserving the active transport generation and every other controller. It
// reports whether an existing routine was restarted.
func (a *ProviderAccount) RetrySharedObjectSync(soID string) bool {
	if soID == "" {
		return false
	}

	// Hold both lifecycle locks through the restart decision so retirement or
	// generation replacement cannot redirect the request to a stale routine.
	var restarted bool
	a.p2pSyncBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		state := a.p2pSync
		if state == nil {
			return
		}
		state.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			if !state.started || state.stopping || state.ctx.Err() != nil {
				return
			}
			syncRoutine := state.soSync[soID]
			if syncRoutine != nil {
				restarted = syncRoutine.sync.RestartRoutine()
			}
		})
	})
	return restarted
}

// StopP2PSync stops all P2P sync controllers, waits for goroutines
// to finish, and releases references.
func (a *ProviderAccount) StopP2PSync() {
	a.retireP2PSyncState(nil)
}

// getP2PStore returns the active lifecycle's store for bucketID, including its
// retained predecessor while replacement startup is incomplete.
func (a *ProviderAccount) getP2PStore(bucketID string) block.StoreOps {
	var state *p2pSyncState
	a.p2pSyncBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		state = a.p2pSync
	})
	if state == nil {
		return nil
	}
	return state.getStore(bucketID)
}

// retireP2PSyncState removes one state from the account lifecycle and releases
// its resources. A nil state selects the current state atomically with removal.
func (a *ProviderAccount) retireP2PSyncState(state *p2pSyncState) {
	a.p2pSyncBcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		if state == nil {
			state = a.p2pSync
		}
		if state == nil {
			return
		}
		if a.p2pSync == state {
			a.p2pSync = nil
			bcast()
		}
	})
	a.stopP2PSyncState(state)
}

// stopP2PSyncState cancels state, waits for startup and registered workers, and
// releases each resource exactly once. Concurrent callers wait for cleanup.
func (a *ProviderAccount) stopP2PSyncState(state *p2pSyncState) {
	if state == nil {
		return
	}

	for {
		var (
			waitCh <-chan struct{}
			done   bool
			owner  bool
		)
		state.bcast.HoldLock(func(bcast func(), getWaitCh func() <-chan struct{}) {
			if !state.stopping {
				state.stopping = true
				state.cancel()
				if !state.startComplete {
					state.startErr = context.Canceled
					state.startComplete = true
				}
				bcast()
			}
			if state.cleanupDone {
				done = true
				return
			}
			if !state.cleanupRunning {
				state.cleanupRunning = true
				owner = true
				bcast()
				return
			}
			waitCh = getWaitCh()
		})
		if done {
			return
		}
		if owner {
			break
		}
		<-waitCh
	}

	for {
		var (
			waitCh <-chan struct{}
			ready  bool
		)
		state.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			ready = state.startupExited && state.workers == 0
			if !ready {
				waitCh = getWaitCh()
			}
		})
		if ready {
			break
		}
		<-waitCh
	}

	// Stop and join every SO sync before releasing the mounts and controller
	// references on which those routines depend.
	var syncRoutines []*routine.RoutineContainer
	state.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		syncRoutines = make([]*routine.RoutineContainer, 0, len(state.soSync))
		for _, syncRoutine := range state.soSync {
			syncRoutines = append(syncRoutines, syncRoutine.sync)
			if syncRoutine.copy != nil {
				syncRoutines = append(syncRoutines, syncRoutine.copy)
			}
			if syncRoutine.body != nil {
				syncRoutines = append(syncRoutines, syncRoutine.body)
			}
			if syncRoutine.hosting != nil {
				syncRoutines = append(syncRoutines, syncRoutine.hosting)
			}
		}
		if state.replicaSync != nil {
			syncRoutines = append(syncRoutines, state.replicaSync)
		}
	})
	for _, syncRoutine := range syncRoutines {
		exitedCh, _ := syncRoutine.SetRoutine(nil)
		if exitedCh != nil {
			<-exitedCh
		}
	}

	var (
		refs   []directive.Reference
		relFns []func()
	)
	state.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		refs = state.refs
		relFns = state.relFns
		state.refs = nil
		state.relFns = nil
	})
	for _, ref := range refs {
		ref.Release()
	}
	for _, rel := range relFns {
		rel()
	}
	a.releaseP2PSyncLowerSource(state)

	state.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		state.cleanupDone = true
		state.cleanupRunning = false
		bcast()
	})
}

// startSOSync mounts the shared object and starts an SOSync instance for it.
func (a *ProviderAccount) startSOSync(
	ctx context.Context,
	childBus bus.Bus,
	ref *sobject.SharedObjectRef,
	bodyType string,
	soID string,
	state *p2pSyncState,
) (retErr error) {
	// Cancel every routine of this object when startup fails.
	ctx, cancel := context.WithCancel(ctx)
	defer func() {
		if retErr != nil {
			cancel()
		}
	}()

	// Mount the SO to ensure the tracker is initialized with the ref.
	// This is necessary when StartP2PSync is called from auto-start
	// (before any UI-driven mount).
	so, relSO, err := a.MountSharedObject(ctx, ref, nil)
	if err != nil {
		return err
	}
	localSO := so.(*SharedObject)

	// Keep the body processor running on validators while the Space is shared.
	// Writers submit operations through SO sync; without a validator body
	// admission, their operations remain queued whenever no UI has the Space
	// open on the primary host. An ownership transfer can promote this account
	// later, so the processor waits for the role instead of deciding once.
	var bodyRoutine *routine.RoutineContainer
	if bodyType == space.SpaceBodyType && ref.GetProviderResourceRef() != nil {
		bodyRoutine = routine.NewRoutineContainerWithLogger(a.le.WithField("routine", "account-space-body"), routine.WithRetry(providerBackoff))
		bodyRoutine.SetRoutine(func(ctx context.Context) error {
			// Mount the body only once this account can validate.
			if err := waitSOValidator(ctx, localSO); err != nil {
				return err
			}
			_, bodyRef, err := sobject.ExMountSharedObjectBodyWithSource[space.SpaceSharedObjectBody](
				ctx,
				a.t.p.b,
				ref,
				space.SpaceBodyType,
				localSO,
				false,
				nil,
			)
			if err != nil {
				return err
			}
			defer bodyRef.Release()

			// Hold the mounted body until the object stops syncing.
			<-ctx.Done()
			return ctx.Err()
		})
	}

	// Validate inbound snapshots against the local storage identity that holds
	// the Space grant. The session transport peer routes the stream but is not
	// necessarily a participant in a local Space.
	validateSnapshotAccess := func(ctx context.Context, state *sobject.SOState) error {
		snapshot := sobject.NewSOStateParticipantHandle(
			a.le,
			a.t.p.sfs,
			soID,
			state,
			localSO.localPriv,
			localSO.localPid,
		)
		_, err := snapshot.GetRootInner(ctx)
		return err
	}

	// Report peer admission and recovery to the object's health.
	soSync := sobject_sync.NewSOSync(
		a.le,
		childBus,
		soID,
		localSO.GetPeerID(),
		localSO.localPriv,
		localSO.soHost,
		func(remoteID peer.ID, accepted bool) {
			localSO.tkr.healthCtr.SwapValue(func(health *sobject.SharedObjectHealth) *sobject.SharedObjectHealth {
				return health.WithSyncPeerAdmission(remoteID.String(), accepted)
			})
		},
		validateSnapshotAccess,
	)
	soSync.SetPeerRecoveryObserver(func(remoteID peer.ID, required bool) {
		localSO.tkr.healthCtr.SwapValue(func(health *sobject.SharedObjectHealth) *sobject.SharedObjectHealth {
			return health.WithSyncPeerRecovery(remoteID.String(), required)
		})
	})

	// Retain the mount for this generation while allowing an explicit invite to
	// replace only the solicitation routine. RoutineContainer serializes the
	// replacement behind the prior execution's exit.
	syncRoutine := routine.NewRoutineContainer()
	syncRoutine.SetRoutine(func(runCtx context.Context) error {
		err := soSync.Execute(runCtx)
		if err != nil && runCtx.Err() == nil {
			a.le.WithError(err).WithField("so-id", soID).Warn("so sync exited with error")
		}
		return err
	})

	// Move hosting with each ownership transfer.
	hostingRoutine := routine.NewRoutineContainerWithLogger(a.le.WithField("routine", "account-so-hosting"), routine.WithRetry(providerBackoff))
	hostingRoutine.SetRoutine(func(ctx context.Context) error { return a.watchSOHosting(ctx, soID, localSO) })
	objectSync := &accountObjectSync{sync: syncRoutine, body: bodyRoutine, hosting: hostingRoutine, cancel: cancel, release: relSO}

	// Copy a Space to the account replica alongside its sync.
	if bodyType == space.SpaceBodyType {
		objectSync.copy = routine.NewRoutineContainerWithLogger(a.le.WithField("routine", "account-copy"), routine.WithRetry(providerBackoff))
		objectSync.copy.SetRoutine(func(ctx context.Context) error { return a.runAccountReplicaCopy(ctx, so, state) })
	}

	// Register the object once; a concurrent start keeps the first registration.
	if !state.addSO(soID, objectSync) {
		relSO()
		if err := ctx.Err(); err != nil {
			return err
		}
		return errors.Errorf("shared object sync already started: %s", soID)
	}
	state.addRelease(relSO)
	state.addRelease(cancel)

	// Start every routine registered for the object.
	syncRoutine.SetContext(ctx, false)
	if objectSync.copy != nil {
		objectSync.copy.SetContext(ctx, false)
	}
	if objectSync.body != nil {
		objectSync.body.SetContext(ctx, false)
	}
	objectSync.hosting.SetContext(ctx, false)
	return nil
}

// startInviteServer registers the SO invite SRPC server on the child bus.
// The server handles incoming alpha/so-invite streams from invitees.
func (a *ProviderAccount) startInviteServer(ctx context.Context, childBus bus.Bus, st *transport.SessionTransport, state *p2pSyncState) error {
	// The invite server answers on the session transport peer.
	localPeerID := st.GetPeerID().String()

	// Build lookup function: scan all mounted SOs for matching token_hash.
	lookupFn := func(ctx context.Context, tokenHash []byte) (*sobject_invite.InviteLookupResult, error) {
		soList := a.soListCtr.GetValue()
		for _, entry := range soList.GetSharedObjects() {
			ref := entry.GetRef()
			soID := ref.GetProviderResourceRef().GetId()

			so, relSO, err := a.MountSharedObject(ctx, ref, nil)
			if err != nil {
				continue
			}

			localSO, ok := so.(*SharedObject)
			if !ok {
				relSO()
				continue
			}

			soState, err := localSO.soHost.GetHostState(ctx)
			if err != nil {
				relSO()
				continue
			}

			for _, inv := range soState.GetInvites() {
				if bytes.Equal(inv.GetTokenHash(), tokenHash) {
					// Get the owner's private key for signing config changes.
					volPeer, err := a.vol.GetPeer(ctx, true)
					if err != nil {
						relSO()
						return nil, err
					}
					volPriv, err := volPeer.GetPrivKey(ctx)
					if err != nil {
						relSO()
						return nil, err
					}
					relSO()

					return &sobject_invite.InviteLookupResult{
						Host:           localSO.soHost,
						InviteMutator:  localSO,
						Invite:         inv,
						SharedObjectID: soID,
						OwnerPrivKey:   volPriv,
					}, nil
				}
			}
			relSO()
		}
		return nil, nil
	}

	// Build enroll function: add the invitee as a participant.
	enrollFn := func(ctx context.Context, result *sobject_invite.InviteLookupResult, inviteePeerID peer.ID, inviteePubKey crypto.PubKey) (*sobject.SOGrant, error) {
		// Add the invitee under the owner key. Local participants have no entity.
		ownerPeerIDStr, err := peer.IDFromPrivateKey(result.OwnerPrivKey)
		if err != nil {
			return nil, err
		}
		grant, err := sobject.AddSOParticipant(
			ctx,
			result.Host,
			result.SharedObjectID,
			result.OwnerPrivKey,
			ownerPeerIDStr.String(),
			inviteePeerID.String(),
			inviteePubKey,
			result.Invite.GetRole(),
			"",
			"",
		)
		if err != nil {
			return nil, err
		}

		// Return the existing root grant when the invitee was already added.
		if grant == nil {
			state, err := result.Host.GetHostState(ctx)
			if err != nil {
				return nil, err
			}
			for _, existing := range state.GetRootGrants() {
				if existing.GetPeerId() == inviteePeerID.String() {
					grant = existing.CloneVT()
					break
				}
			}
			if grant == nil {
				return nil, errors.New("participant exists without a root grant")
			}
		}

		// Keep a targeted invitee connected once enrolled.
		if result.Invite.GetTargetPeerId() == inviteePeerID.String() {
			a.clearP2PPendingEnrollPeer(inviteePeerID)
			if err := a.RetainP2PPeer(ctx, inviteePeerID); err != nil {
				return nil, errors.Wrap(err, "retain enrolled peer")
			}
		}
		return grant, nil
	}

	// Build queue function: hold a redemption for the owner's approval.
	queueFn := func(ctx context.Context, result *sobject_invite.InviteLookupResult, joinResp *sobject.SOJoinResponse) error {
		// Mount the invite's shared object.
		so, relSO, err := a.mountListedSharedObject(ctx, result.SharedObjectID)
		if err != nil {
			return err
		}
		if so == nil {
			return errors.New("shared object is not held")
		}
		defer relSO()

		// Persist the request on the shared object.
		return so.QueueJoinRequest(ctx, joinResp)
	}

	// Build withdraw function: drop a requester's held redemption.
	withdrawFn := func(ctx context.Context, sharedObjectID string, peerID peer.ID) error {
		// Mount the shared object; one this account does not hold has no requests.
		so, relSO, err := a.mountListedSharedObject(ctx, sharedObjectID)
		if err != nil || so == nil {
			return err
		}
		defer relSO()

		// Remove the request, if one is still pending.
		return so.WithdrawJoinRequest(ctx, peerID.String())
	}

	// Build held config function: read the config of a Space this account holds.
	heldConfigFn := func(ctx context.Context, sharedObjectID string) (*sobject.SharedObjectConfig, error) {
		// Mount the shared object, or report nothing when it is not held.
		so, relSO, err := a.mountListedSharedObject(ctx, sharedObjectID)
		if err != nil || so == nil {
			return nil, err
		}
		defer relSO()

		// Read its current config from the host state.
		soState, err := so.soHost.GetHostState(ctx)
		if err != nil {
			return nil, err
		}
		return soState.GetConfig(), nil
	}

	// Construct the invite controller with the handlers.
	ctrl, err := sobject_invite.NewInviteController(
		a.le,
		childBus,
		sobject_invite.Handlers{
			Lookup:     lookupFn,
			Enroll:     enrollFn,
			Leave:      a.acceptSharedObjectLeave,
			Queue:      queueFn,
			Withdraw:   withdrawFn,
			HeldConfig: heldConfigFn,
		},
		[]string{localPeerID},
	)
	if err != nil {
		return err
	}

	// Run the controller on the child bus until sync stops.
	relCtrl, err := childBus.AddController(ctx, ctrl, nil)
	if err != nil {
		return err
	}
	state.addRelease(relCtrl)
	return nil
}

// mountListedSharedObject mounts a shared object in the account's list by ID.
// Returns nil when the account does not hold the shared object.
func (a *ProviderAccount) mountListedSharedObject(ctx context.Context, sharedObjectID string) (*SharedObject, func(), error) {
	for _, entry := range a.soListCtr.GetValue().GetSharedObjects() {
		ref := entry.GetRef()
		if ref.GetProviderResourceRef().GetId() != sharedObjectID {
			continue
		}
		so, relSO, err := a.MountSharedObject(ctx, ref, nil)
		if err != nil {
			return nil, nil, err
		}
		localSO, ok := so.(*SharedObject)
		if !ok {
			relSO()
			return nil, nil, errors.New("shared object is not local")
		}
		return localSO, relSO, nil
	}
	return nil, nil, nil
}

// startDEXSolicit loads a DEX solicit controller on the child bus for
// the given block store bucket.
func (a *ProviderAccount) startDEXSolicit(ctx context.Context, childBus bus.Bus, bucketID, protocolContext string, state *p2pSyncState) error {
	ctrl, _, dexRef, err := loader.WaitExecControllerRunningTyped[*dex_solicit.Controller](
		ctx,
		childBus,
		resolver.NewLoadControllerWithConfig(&dex_solicit.Config{
			BucketId: bucketID,
			// Allow a participant to read writer blocks through its shared immediate
			// peer without requiring that peer to prefetch them.
			MaxForwardHops:  1,
			ProtocolContext: []byte(protocolContext),
		}),
		nil,
	)
	if err != nil {
		return err
	}
	state.addRef(dexRef)
	state.addStore(bucketID, dex_solicit.NewStore(ctrl))
	state.bcast.HoldLock(func(changed func(), _ func() <-chan struct{}) {
		if state.exchanges == nil {
			state.exchanges = make(map[string]*dex_solicit.Controller)
		}
		state.exchanges[bucketID] = ctrl
		changed()
	})
	return nil
}
