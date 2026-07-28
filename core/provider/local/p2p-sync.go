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
	ctx            context.Context
	cancel         context.CancelFunc
	startDone      chan struct{}
	startOnce      sync.Once
	stopDone       chan struct{}
	stopOnce       sync.Once
	wg             sync.WaitGroup
	mtx            sync.Mutex
	owners         int
	restartPending bool
	refs           []directive.Reference
	relFns         []func()
	stores         map[string]block.StoreOps
	soIDs          map[string]struct{}
}

func (s *p2pSyncState) completeStart() {
	s.startOnce.Do(func() {
		close(s.startDone)
	})
}

func (s *p2pSyncState) retain(ctx context.Context) bool {
	if ctx.Err() != nil {
		return false
	}

	s.mtx.Lock()
	if s.ctx.Err() != nil {
		s.mtx.Unlock()
		return false
	}
	s.owners++
	s.mtx.Unlock()

	if ctx.Done() == nil {
		return true
	}
	go func() {
		select {
		case <-ctx.Done():
			s.release()
		case <-s.stopDone:
		}
	}()
	return true
}

func (s *p2pSyncState) release() {
	s.mtx.Lock()
	if s.owners > 0 {
		s.owners--
		if s.owners == 0 {
			s.cancel()
		}
	}
	s.mtx.Unlock()
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
func (a *ProviderAccount) StartP2PSync(ctx context.Context, sessionTransport *transport.SessionTransport) (rerr error) {
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
		syncCtx   context.Context
		state     *p2pSyncState
	)
	a.p2pSyncBcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		previous = a.p2pSync
		if previous != nil {
			select {
			case <-previous.startDone:
			default:
				if previous.ctx.Err() == nil && previous.retain(ctx) {
					previous.restartPending = true
					waitState = previous
					return
				}
			}
		}

		var syncCancel context.CancelFunc
		syncCtx, syncCancel = context.WithCancel(context.WithoutCancel(ctx))
		state = &p2pSyncState{
			ctx:       syncCtx,
			cancel:    syncCancel,
			startDone: make(chan struct{}),
			stopDone:  make(chan struct{}),
		}
		if !state.retain(ctx) {
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
		select {
		case <-waitState.startDone:
		case <-ctx.Done():
			return ctx.Err()
		}
		running := false
		a.p2pSyncBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			running = a.p2pSync == waitState
		})
		if running {
			return nil
		}
		if err := waitState.ctx.Err(); err != nil {
			return err
		}
		return errors.New("P2P sync stopped during startup")
	}
	defer func() {
		if rerr == nil {
			return
		}

		cleanup := false
		a.p2pSyncBcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
			cleanup = a.p2pSync == state
			if cleanup {
				a.p2pSync = nil
			}
			state.completeStart()
			bcast()
		})
		if cleanup {
			a.stopP2PSyncState(state)
		}
	}()

	a.stopP2PSyncState(previous)

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
		a.p2pSyncBcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
			if a.p2pSync == state && state.restartPending {
				state.restartPending = false
				restart = true
				return
			}
			state.completeStart()
			bcast()
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
	var state *p2pSyncState
	a.p2pSyncBcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		state = a.p2pSync
		a.p2pSync = nil
		bcast()
	})

	a.stopP2PSyncState(state)
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

func (a *ProviderAccount) stopP2PSyncState(state *p2pSyncState) {
	if state == nil {
		return
	}
	state.stopOnce.Do(func() {
		close(state.stopDone)
		state.cancel()
		state.completeStart()
		<-state.startDone
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
