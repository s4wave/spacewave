package provider_local

import (
	"bytes"
	"context"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
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
	ctx context.Context
	// sessionTransport is the transport this state's controllers run on. A
	// start for a different transport cannot join this one: the startup pass
	// loads controllers onto the child bus of the transport it began with, so
	// coalescing across transports would leave the newer transport without
	// them while its caller was told sync had started.
	sessionTransport *transport.SessionTransport
	cancel           context.CancelFunc
	startDone        chan struct{}
	startOnce        sync.Once
	runDone          chan struct{}
	stopDone         chan struct{}
	stopOnce         sync.Once
	wg               sync.WaitGroup
	mtx              sync.Mutex
	owners           int
	restartPending   bool
	// startErr is the failure that ended startup, published to every waiter
	// under the account's sync lock before startDone closes.
	startErr error
	refs     []directive.Reference
	relFns   []func()
	stores   map[string]block.StoreOps
	soIDs    map[string]struct{}
}

func (s *p2pSyncState) completeStart() {
	s.startOnce.Do(func() {
		close(s.startDone)
	})
}

// retainP2PSyncState is called with the account sync lock held before it takes
// state.mtx. The release path drops state.mtx before reacquiring the account
// lock to retire the state, so the locks are never held in reverse order.
func (a *ProviderAccount) retainP2PSyncState(ctx context.Context, state *p2pSyncState) bool {
	if ctx.Err() != nil {
		return false
	}

	state.mtx.Lock()
	if state.ctx.Err() != nil {
		state.mtx.Unlock()
		return false
	}
	state.owners++
	state.mtx.Unlock()

	if ctx.Done() == nil {
		return true
	}
	go func() {
		select {
		case <-ctx.Done():
			a.releaseP2PSyncState(state)
		case <-state.stopDone:
		}
	}()
	return true
}

func (a *ProviderAccount) releaseP2PSyncState(state *p2pSyncState) {
	retire := false
	state.mtx.Lock()
	if state.owners > 0 {
		state.owners--
		if state.owners == 0 {
			state.cancel()
			retire = true
		}
	}
	state.mtx.Unlock()

	if retire {
		a.retireP2PSyncState(state)
	}
}

func (s *p2pSyncState) addStore(bucketID string, store block.StoreOps) {
	s.mtx.Lock()
	if s.stores == nil {
		s.stores = make(map[string]block.StoreOps)
	}
	s.stores[bucketID] = store
	s.mtx.Unlock()
}

func (s *p2pSyncState) hasStore(bucketID string) bool {
	s.mtx.Lock()
	_, ok := s.stores[bucketID]
	s.mtx.Unlock()
	return ok
}

func (s *p2pSyncState) hasSO(soID string) bool {
	s.mtx.Lock()
	_, ok := s.soIDs[soID]
	s.mtx.Unlock()
	return ok
}

func (s *p2pSyncState) addSO(soID string) {
	s.mtx.Lock()
	if s.soIDs == nil {
		s.soIDs = make(map[string]struct{})
	}
	s.soIDs[soID] = struct{}{}
	s.mtx.Unlock()
}

func (s *p2pSyncState) getStore(bucketID string) block.StoreOps {
	s.mtx.Lock()
	store := s.stores[bucketID]
	s.mtx.Unlock()
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
		previous  *p2pSyncState
		waitState *p2pSyncState
		state     *p2pSyncState
	)
	a.p2pSyncBcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		previous = a.p2pSync
		if previous != nil && previous.sessionTransport == sessionTransport {
			select {
			case <-previous.startDone:
			default:
				if previous.ctx.Err() == nil && a.retainP2PSyncState(ctx, previous) {
					previous.restartPending = true
					waitState = previous
					return
				}
			}
		}

		syncCtx, syncCancel := context.WithCancel(context.WithoutCancel(ctx))
		state = &p2pSyncState{
			ctx:              syncCtx,
			sessionTransport: sessionTransport,
			cancel:           syncCancel,
			startDone:        make(chan struct{}),
			runDone:          make(chan struct{}),
			stopDone:         make(chan struct{}),
		}
		if !a.retainP2PSyncState(ctx, state) {
			syncCancel()
			state = nil
			return
		}
		a.p2pSync = state
		bcast()
	})
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
	go a.runP2PSyncStart(state, previous, sessionTransport, childBus)
	return a.awaitP2PSyncStart(ctx, state)
}

// awaitP2PSyncStart waits for the shared startup to finish or for the caller's
// own context to end, whichever comes first. Giving up releases this caller's
// ownership without stopping the run; it ends only when the last owner leaves.
func (a *ProviderAccount) awaitP2PSyncStart(ctx context.Context, state *p2pSyncState) error {
	select {
	case <-state.startDone:
	case <-ctx.Done():
		return ctx.Err()
	}

	var running bool
	var startErr error
	a.p2pSyncBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		running = a.p2pSync == state
		startErr = state.startErr
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

// runP2PSyncStart performs the startup pass and publishes its outcome to every
// caller waiting on the state.
func (a *ProviderAccount) runP2PSyncStart(
	state *p2pSyncState,
	previous *p2pSyncState,
	sessionTransport *transport.SessionTransport,
	childBus bus.Bus,
) {
	err := a.startP2PSyncControllers(state, previous, sessionTransport, childBus)

	a.p2pSyncBcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		state.startErr = err
		state.completeStart()
		bcast()
	})
	close(state.runDone)

	if err != nil {
		a.retireP2PSyncState(state)
	}
}

func (a *ProviderAccount) startP2PSyncControllers(
	state *p2pSyncState,
	previous *p2pSyncState,
	sessionTransport *transport.SessionTransport,
	childBus bus.Bus,
) error {
	if previous != nil {
		// The replacement has its own transport, so its startup can proceed
		// while the prior state's lifecycle owner waits for detached startup.
		go a.retireP2PSyncState(previous)
	}

	syncCtx := state.ctx
	inviteStarted := false
	for {
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

		// Start the SO invite server so invitees can join via alpha/so-invite.
		if !inviteStarted {
			if err := a.startInviteServer(syncCtx, childBus, sessionTransport, state); err != nil {
				if syncCtx.Err() != nil {
					return syncCtx.Err()
				}
				a.le.WithError(err).Warn("failed to start invite server")
			} else {
				inviteStarted = true
			}
		}

		restart := false
		a.p2pSyncBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			if a.p2pSync == state && state.restartPending {
				state.restartPending = false
				restart = true
			}
		})
		if !restart {
			return syncCtx.Err()
		}
	}
}

// GetP2PSyncSnapshotWithWait returns whether P2P sync is running and a channel
// that closes when its lifecycle changes.
func (a *ProviderAccount) GetP2PSyncSnapshotWithWait() (bool, <-chan struct{}) {
	var running bool
	var ch <-chan struct{}
	a.p2pSyncBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		ch = getWaitCh()
		if a.p2pSync == nil {
			return
		}
		select {
		case <-a.p2pSync.startDone:
			running = true
		default:
		}
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
//
// Callers never hold state.mtx here: the account lock may nest state.mtx while
// retaining an owner, so retirement acquires only the account lock. Resource
// cleanup waits outside both locks.
func (a *ProviderAccount) retireP2PSyncState(state *p2pSyncState) {
	a.p2pSyncBcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		if state == nil {
			state = a.p2pSync
		}
		if state != nil && a.p2pSync == state {
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
	state.stopOnce.Do(func() {
		close(state.stopDone)
		state.cancel()
		state.completeStart()
		<-state.runDone
		state.wg.Wait()
		for _, ref := range state.refs {
			ref.Release()
		}
		for _, rel := range state.relFns {
			rel()
		}
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
	state.wg.Go(func() {
		defer relSO()
		if err := soSync.Execute(ctx); err != nil && ctx.Err() == nil {
			a.le.WithError(err).WithField("so-id", soID).Warn("so sync exited with error")
		}
	})

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
	state.relFns = append(state.relFns, relCtrl)
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
	state.refs = append(state.refs, dexRef)
	state.addStore(bucketID, dex_solicit.NewStore(ctrl))
	return nil
}
