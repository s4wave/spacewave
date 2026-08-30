package provider_spacewave

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

// p2pSyncState holds running P2P sync and read stores for one mounted
// Session.
type p2pSyncState struct {
	sessionID string
	cancel    context.CancelFunc
	// bcast guards every resource field and the running worker count below.
	bcast  broadcast.Broadcast
	refs   []directive.Reference
	relFns []func()
	stores map[string]block.StoreOps
	// workers counts running sync goroutines; stop waits for it to drain.
	workers int
}

func (s *p2pSyncState) addStore(bucketID string, store block.StoreOps) {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if s.stores == nil {
			s.stores = make(map[string]block.StoreOps)
		}
		s.stores[bucketID] = store
		broadcast()
	})
}

func (s *p2pSyncState) getStore(bucketID string) block.StoreOps {
	var store block.StoreOps
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		store = s.stores[bucketID]
	})
	return store
}

func (s *p2pSyncState) addRef(ref directive.Reference) {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		s.refs = append(s.refs, ref)
		broadcast()
	})
}

func (s *p2pSyncState) addRelease(release func()) {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		s.relFns = append(s.relFns, release)
		broadcast()
	})
}

// startWorker registers a running sync goroutine before it starts so a
// concurrent stop observes it in the worker count rather than missing it.
func (s *p2pSyncState) startWorker() {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		s.workers++
		broadcast()
	})
}

// workerExited releases one running sync goroutine.
func (s *p2pSyncState) workerExited() {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		s.workers--
		broadcast()
	})
}

// awaitWorkersIdle blocks until every running sync goroutine has exited.
// Callers cancel the state context first; workers exit in response to it.
func (s *p2pSyncState) awaitWorkersIdle() {
	for {
		var waitCh <-chan struct{}
		idle := false
		s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			if s.workers != 0 {
				waitCh = getWaitCh()
				return
			}
			idle = true
		})
		if idle {
			return
		}
		<-waitCh
	}
}

func (s *p2pSyncState) cleanup() {
	var refs []directive.Reference
	var relFns []func()
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		refs, relFns = s.refs, s.relFns
		s.refs, s.relFns = nil, nil
		broadcast()
	})
	for _, ref := range refs {
		ref.Release()
	}
	for _, release := range relFns {
		release()
	}
}

// StartP2PSync starts SO sync, DEX block exchange, and the direct invite
// server for the legacy default session transport.
func (a *ProviderAccount) StartP2PSync(
	ctx context.Context,
	sessionTransport *transport.SessionTransport,
) error {
	return a.startP2PSyncForSession(ctx, "", sessionTransport)
}

func (a *ProviderAccount) startP2PSyncForSession(
	ctx context.Context,
	sessionID string,
	sessionTransport *transport.SessionTransport,
) error {
	childBus := sessionTransport.GetChildBus()
	if childBus == nil {
		return nil
	}

	a.stopP2PSyncForSession(sessionID)

	syncCtx, syncCancel := context.WithCancel(ctx)
	state := &p2pSyncState{sessionID: sessionID, cancel: syncCancel}
	a.p2pSyncMtx.Lock()
	if a.p2pSyncs == nil {
		a.p2pSyncs = make(map[string]*p2pSyncState)
	}
	a.p2pSyncs[sessionID] = state
	a.p2pSyncMtx.Unlock()

	mountCtrl := &sessionSharedObjectMountController{account: a, sessionID: sessionID}
	releaseMountCtrl, err := childBus.AddController(syncCtx, mountCtrl, nil)
	if err != nil {
		a.stopP2PSyncForSession(sessionID)
		return errors.Wrap(err, "register session shared object mount")
	}
	state.addRelease(releaseMountCtrl)

	soList := a.soListCtr.GetValue()
	if soList != nil {
		for _, entry := range soList.GetSharedObjects() {
			ref := entry.GetRef()
			if ref == nil {
				continue
			}
			provRef := ref.GetProviderResourceRef()
			if provRef == nil {
				continue
			}
			soID := provRef.GetId()
			blockStoreID := ref.GetBlockStoreId()

			bucketID := BlockStoreBucketID(a.accountID, blockStoreID)
			if err := a.startDEXSolicit(syncCtx, childBus, bucketID, soID, state); err != nil {
				a.le.WithError(err).WithField("bucket-id", bucketID).Warn("failed to start dex solicit")
			}

			if err := a.startSOSync(syncCtx, childBus, sessionTransport.GetPeerID(), ref, soID, state); err != nil {
				a.le.WithError(err).WithField("so-id", soID).Warn("failed to start so sync")
			}
		}
	}

	if err := a.startInviteServer(syncCtx, childBus, sessionTransport, state); err != nil {
		a.le.WithError(err).Warn("failed to start invite server")
	}

	return nil
}

// StopP2PSync stops the legacy default session's P2P controllers.
func (a *ProviderAccount) StopP2PSync() {
	a.stopP2PSyncForSession("")
}

func (a *ProviderAccount) stopP2PSyncForSession(sessionID string) {
	a.p2pSyncMtx.Lock()
	state := a.p2pSyncs[sessionID]
	if state != nil {
		delete(a.p2pSyncs, sessionID)
	}
	a.p2pSyncMtx.Unlock()
	if state == nil {
		return
	}
	state.cancel()
	state.awaitWorkersIdle()
	state.cleanup()
}

// startSOSync mounts the shared object and starts an SOSync instance for it.
func (a *ProviderAccount) startSOSync(
	ctx context.Context,
	childBus bus.Bus,
	localPeerID peer.ID,
	ref *sobject.SharedObjectRef,
	soID string,
	state *p2pSyncState,
) error {
	so, relSO, err := sobject.ExMountSharedObject(ctx, childBus, ref, false, nil)
	if err != nil {
		return err
	}

	swSO, ok := so.(*sessionSharedObject)
	if !ok {
		relSO.Release()
		return errors.New("unexpected session shared object type")
	}

	validateSnapshotAccess := func(ctx context.Context, state *sobject.SOState) error {
		snapshot := sobject.NewSOStateParticipantHandle(
			a.le,
			a.p.sfs,
			soID,
			state,
			swSO.privKey,
			swSO.localPid,
		)
		_, err := snapshot.GetTransformer(ctx)
		return err
	}
	soSync := sobject_sync.NewSOSync(
		a.le,
		childBus,
		soID,
		localPeerID,
		swSO.GetSOHost(),
		validateSnapshotAccess,
	)
	state.startWorker()
	go func() {
		defer state.workerExited()
		defer relSO.Release()
		if err := soSync.Execute(ctx); err != nil && ctx.Err() == nil {
			a.le.WithError(err).WithField("so-id", soID).Warn("so sync exited with error")
		}
	}()
	return nil
}

func (a *ProviderAccount) getSessionDEXStore(sessionID, bucketID string) block.StoreOps {
	a.p2pSyncMtx.Lock()
	state := a.p2pSyncs[sessionID]
	a.p2pSyncMtx.Unlock()
	if state == nil {
		return nil
	}
	return state.getStore(bucketID)
}

// startInviteServer registers the SO invite SRPC server on the child bus.
func (a *ProviderAccount) startInviteServer(
	ctx context.Context,
	childBus bus.Bus,
	st *transport.SessionTransport,
	state *p2pSyncState,
) error {
	localPeerID := st.GetPeerID().String()

	lookupFn := func(ctx context.Context, tokenHash []byte) (*sobject_invite.InviteLookupResult, error) {
		soList := a.soListCtr.GetValue()
		if soList == nil {
			return nil, nil
		}
		for _, entry := range soList.GetSharedObjects() {
			ref := entry.GetRef()
			if ref == nil {
				continue
			}
			soID := ref.GetProviderResourceRef().GetId()

			swSO, relSO, err := a.mountSpaceSO(ctx, soID)
			if err != nil {
				continue
			}

			soState, err := swSO.GetSOHost().GetHostState(ctx)
			if err != nil {
				relSO()
				continue
			}

			for _, inv := range soState.GetInvites() {
				if bytes.Equal(inv.GetTokenHash(), tokenHash) {
					result := &sobject_invite.InviteLookupResult{
						Host:           swSO.GetSOHost(),
						InviteMutator:  swSO,
						Invite:         inv,
						SharedObjectID: soID,
						OwnerPrivKey:   swSO.privKey,
					}
					relSO()
					return result, nil
				}
			}
			relSO()
		}
		return nil, nil
	}

	enrollFn := func(
		ctx context.Context,
		result *sobject_invite.InviteLookupResult,
		inviteePeerID peer.ID,
		inviteePubKey crypto.PubKey,
	) (*sobject.SOGrant, error) {
		swSO, relSO, err := a.mountSpaceSO(ctx, result.SharedObjectID)
		if err != nil {
			return nil, err
		}
		defer relSO()

		return swSO.AddParticipant(
			ctx,
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
func (a *ProviderAccount) startDEXSolicit(
	ctx context.Context,
	childBus bus.Bus,
	bucketID string,
	protocolContext string,
	state *p2pSyncState,
) error {
	ctrl, _, dexRef, err := loader.WaitExecControllerRunningTyped[*dex_solicit.Controller](
		ctx,
		childBus,
		resolver.NewLoadControllerWithConfig(&dex_solicit.Config{
			BucketId:        bucketID,
			ProtocolContext: []byte(protocolContext),
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
