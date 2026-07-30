package provider_local

import (
	"bytes"
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_invite "github.com/s4wave/spacewave/core/sobject/invite"
	sobject_sync "github.com/s4wave/spacewave/core/sobject/sync"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/db/block"
	dex_solicit "github.com/s4wave/spacewave/db/dex/solicit"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// p2pSyncState holds P2P sync startup or running state and DEX stores.
type p2pSyncState struct {
	// bcast guards every lifecycle and resource field below.
	bcast broadcast.Broadcast

	ctx              context.Context
	sessionTransport *transport.SessionTransport
	cancel           context.CancelFunc
	owners           int
	startComplete    bool
	started          bool
	startupExited    bool
	stopping         bool
	cleanupRunning   bool
	cleanupDone      bool
	restartPending   bool
	startErr         error
	lowerSource      *p2pSyncState
	lowerSourceHeld  bool
	workers          int
	refs             []directive.Reference
	relFns           []func()
	stores           map[string]block.StoreOps
	soIDs            map[string]struct{}
}

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

func (a *ProviderAccount) watchP2PSyncOwner(ctx context.Context, state *p2pSyncState) {
	if ctx.Done() == nil {
		return
	}
	go func() {
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
			select {
			case <-ctx.Done():
				a.releaseP2PSyncState(state)
				return
			case <-waitCh:
			}
		}
	}()
}

func (a *ProviderAccount) releaseP2PSyncState(state *p2pSyncState) {
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

	if retire {
		a.retireP2PSyncState(state)
	}
}

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

func (s *p2pSyncState) addStore(bucketID string, store block.StoreOps) {
	s.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		if s.stores == nil {
			s.stores = make(map[string]block.StoreOps)
		}
		s.stores[bucketID] = store
		bcast()
	})
}

func (s *p2pSyncState) hasStore(bucketID string) bool {
	var ok bool
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		_, ok = s.stores[bucketID]
	})
	return ok
}

func (s *p2pSyncState) hasSO(soID string) bool {
	var ok bool
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		_, ok = s.soIDs[soID]
	})
	return ok
}

func (s *p2pSyncState) addSO(soID string) {
	s.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		if s.soIDs == nil {
			s.soIDs = make(map[string]struct{})
		}
		s.soIDs[soID] = struct{}{}
		bcast()
	})
}

func (s *p2pSyncState) addRef(ref directive.Reference) {
	s.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		s.refs = append(s.refs, ref)
		bcast()
	})
}

func (s *p2pSyncState) addRelease(rel func()) {
	s.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		s.relFns = append(s.relFns, rel)
		bcast()
	})
}

func (s *p2pSyncState) addWorker() {
	s.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		s.workers++
		bcast()
	})
}

func (s *p2pSyncState) workerDone() {
	s.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		if s.workers == 0 {
			return
		}
		s.workers--
		bcast()
	})
}

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
// childBus is the session transport's child bus where solicit
// controllers run. The session transport must be running before
// calling this method.
func (a *ProviderAccount) StartP2PSync(ctx context.Context, sessionTransport *transport.SessionTransport) error {
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
				if previous.startComplete || previous.stopping || previous.ctx.Err() != nil {
					return
				}
				if ctx.Err() != nil {
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
		if !a.retainP2PSyncStateLocked(ctx, state) {
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
		if waitState != nil {
			a.watchP2PSyncOwner(ctx, waitState)
		} else {
			a.watchP2PSyncOwner(ctx, state)
		}
	}
	if state == nil {
		if waitState == nil {
			return ctx.Err()
		}
		return a.awaitP2PSyncStart(ctx, waitState)
	}

	// Startup belongs to every owner of the state, not to the caller that
	// happened to create it. Running it here would tie it to that caller's
	// goroutine, so a caller whose context is canceled while later owners keep
	// the state alive could not return until startup finished on its own.
	go a.runP2PSyncStart(state, previous, previousRetained, sessionTransport, childBus)
	return a.awaitP2PSyncStart(ctx, state)
}

// awaitP2PSyncStart waits for the shared startup to finish or for the caller's
// own context to end, whichever comes first. Giving up releases this caller's
// ownership without stopping the run; it ends only when the last owner leaves.
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
	)
	for {
		err = a.startP2PSyncControllers(
			state,
			sessionTransport,
			childBus,
			&inviteStarted,
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
		a.releaseP2PSyncLowerSource(state)
		state.markStartupExited()
		if previous != nil {
			a.retireP2PSyncState(previous)
		}
		return
	}

	state.markStartupExited()
	a.restoreP2PSyncAfterFailedStart(state, previous, previousRetained)
	a.retireP2PSyncState(state)
}

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
		if restorePrevious {
			a.p2pSync = previous
		} else {
			a.p2pSync = nil
			retirePrevious = previous != nil
		}
		bcast()
	})
	if retirePrevious {
		a.retireP2PSyncState(previous)
	}
}

func (a *ProviderAccount) startP2PSyncControllers(
	state *p2pSyncState,
	sessionTransport *transport.SessionTransport,
	childBus bus.Bus,
	inviteStarted *bool,
) error {
	syncCtx := state.ctx
	soList := a.soListCtr.GetValue()
	for _, entry := range soList.GetSharedObjects() {
		ref := entry.GetRef()
		provRef := ref.GetProviderResourceRef()
		soID := provRef.GetId()
		blockStoreID := ref.GetBlockStoreId()

		providerID := provRef.GetProviderId()
		providerAccountID := provRef.GetProviderAccountId()
		bucketID := BlockStoreBucketID(providerID, providerAccountID, blockStoreID)
		if !state.hasStore(bucketID) {
			if err := a.startDEXSolicit(syncCtx, childBus, bucketID, state); err != nil {
				if syncCtx.Err() != nil {
					return syncCtx.Err()
				}
				a.le.WithError(err).WithField("bucket-id", bucketID).Warn("failed to start dex solicit")
			}
		}

		if !state.hasSO(soID) {
			if err := a.startSOSync(syncCtx, childBus, ref, soID, state); err != nil {
				if syncCtx.Err() != nil {
					return syncCtx.Err()
				}
				a.le.WithError(err).WithField("so-id", soID).Warn("failed to start so sync")
			} else {
				state.addSO(soID)
			}
		}
	}

	if !*inviteStarted {
		if err := a.startInviteServer(syncCtx, childBus, sessionTransport, state); err != nil {
			if syncCtx.Err() != nil {
				return syncCtx.Err()
			}
			a.le.WithError(err).Warn("failed to start invite server")
		} else {
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

// StopP2PSync stops all P2P sync controllers, waits for goroutines
// to finish, and releases references.
func (a *ProviderAccount) StopP2PSync() {
	a.retireP2PSyncState(nil)
}

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
func (a *ProviderAccount) startSOSync(ctx context.Context, childBus bus.Bus, ref *sobject.SharedObjectRef, soID string, state *p2pSyncState) error {
	// Mount the SO to ensure the tracker is initialized with the ref.
	// This is necessary when StartP2PSync is called from auto-start
	// (before any UI-driven mount).
	so, relSO, err := a.MountSharedObject(ctx, ref, nil)
	if err != nil {
		return err
	}

	localSO := so.(*SharedObject)
	soSync := sobject_sync.NewSOSync(a.le, childBus, soID, localSO.soHost)
	state.addWorker()
	go func() {
		defer state.workerDone()
		defer relSO()
		if err := soSync.Execute(ctx); err != nil && ctx.Err() == nil {
			a.le.WithError(err).WithField("so-id", soID).Warn("so sync exited with error")
		}
	}()

	return nil
}

// startInviteServer registers the SO invite SRPC server on the child bus.
// The server handles incoming alpha/so-invite streams from invitees.
func (a *ProviderAccount) startInviteServer(ctx context.Context, childBus bus.Bus, st *transport.SessionTransport, state *p2pSyncState) error {
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

	enrollFn := func(ctx context.Context, result *sobject_invite.InviteLookupResult, inviteePeerID peer.ID, inviteePubKey crypto.PubKey) (*sobject.SOGrant, error) {
		ownerPeerIDStr, err := peer.IDFromPrivateKey(result.OwnerPrivKey)
		if err != nil {
			return nil, err
		}
		return sobject.AddSOParticipant(
			ctx,
			result.Host,
			result.SharedObjectID,
			result.OwnerPrivKey,
			ownerPeerIDStr.String(),
			inviteePeerID.String(),
			inviteePubKey,
			result.Invite.GetRole(),
			"",
		)
	}

	ctrl, err := sobject_invite.NewInviteController(
		a.le,
		childBus,
		lookupFn,
		enrollFn,
		[]string{localPeerID},
	)
	if err != nil {
		return err
	}

	relCtrl, err := childBus.AddController(ctx, ctrl, nil)
	if err != nil {
		return err
	}
	state.addRelease(relCtrl)
	return nil
}

// startDEXSolicit loads a DEX solicit controller on the child bus for
// the given block store bucket.
func (a *ProviderAccount) startDEXSolicit(ctx context.Context, childBus bus.Bus, bucketID string, state *p2pSyncState) error {
	ctrl, _, dexRef, err := loader.WaitExecControllerRunningTyped[*dex_solicit.Controller](
		ctx,
		childBus,
		resolver.NewLoadControllerWithConfig(&dex_solicit.Config{
			BucketId: bucketID,
		}),
		nil,
	)
	if err != nil {
		return err
	}
	state.addRef(dexRef)
	state.addStore(bucketID, dex_solicit.NewStore(ctrl))
	return nil
}
