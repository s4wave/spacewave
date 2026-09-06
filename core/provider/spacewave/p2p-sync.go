package provider_spacewave

import (
	"bytes"
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/routine"
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

// p2pSyncSpaceKey identifies one Space and its current backing block store.
type p2pSyncSpaceKey struct {
	// id is the shared object and protocol context.
	id string
	// blockStoreID selects the provider's backing store.
	blockStoreID string
}

// p2pSyncState retains one mounted Session's direct synchronization resources.
// The list watcher owns desired membership; active records only in-flight
// routines so shutdown can drain removed keys as well as current keys.
type p2pSyncState struct {
	// cancel stops the Session's synchronization context.
	cancel context.CancelFunc
	// watcher reconciles Space membership for the synchronization lifetime.
	watcher *routine.RoutineContainer

	// bcast guards teardown, live routines, releases, and available read stores.
	bcast broadcast.Broadcast
	// closing prevents late routines and controller acquisitions from surviving shutdown.
	closing bool
	// active tracks in-flight routines, including removed keys; nil identifies the list watcher.
	active map[*p2pSyncSpaceKey]struct{}
	// relFns retains the Session mount adapter and invitation server.
	relFns []func()
	// stores contains only currently running block exchange adapters.
	stores map[string]block.StoreOps
}

// beginRoutine admits a synchronization routine unless teardown has already begun.
func (s *p2pSyncState) beginRoutine(key *p2pSyncSpaceKey) bool {
	var admitted bool
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if s.closing {
			return
		}
		if s.active == nil {
			s.active = make(map[*p2pSyncSpaceKey]struct{})
		}
		s.active[key] = struct{}{}
		admitted = true
		broadcast()
	})
	return admitted
}

// endRoutine publishes completion only after the routine releases its controllers.
func (s *p2pSyncState) endRoutine(key *p2pSyncSpaceKey) {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		delete(s.active, key)
		broadcast()
	})
}

// addStore exposes a ready direct read adapter for the running Space routine.
func (s *p2pSyncState) addStore(bucketID string, store block.StoreOps) {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if s.stores == nil {
			s.stores = make(map[string]block.StoreOps)
		}
		s.stores[bucketID] = store
		broadcast()
	})
}

// removeStore withdraws this generation without removing a newer adapter.
func (s *p2pSyncState) removeStore(bucketID string, store block.StoreOps) {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if s.stores[bucketID] == store {
			delete(s.stores, bucketID)
			broadcast()
		}
	})
}

// getStore returns the current direct adapter, or nil while it is unavailable.
func (s *p2pSyncState) getStore(bucketID string) block.StoreOps {
	var store block.StoreOps
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		store = s.stores[bucketID]
	})
	return store
}

// addRelease retains a controller or releases it immediately during shutdown.
func (s *p2pSyncState) addRelease(release func()) {
	var closing bool
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		closing = s.closing
		if !closing {
			s.relFns = append(s.relFns, release)
		}
	})
	if closing {
		release()
	}
}

// stop cancels desired membership and drains every admitted Space routine.
func (s *p2pSyncState) stop() {
	// Fence late routine admission before canceling the list watcher.
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		s.closing = true
		broadcast()
	})
	s.cancel()
	s.watcher.ClearContext()

	// Removed keys can still be releasing resources after keyed cancellation.
	for {
		var waitCh <-chan struct{}
		s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			if len(s.active) != 0 {
				waitCh = getWaitCh()
			}
		})
		if waitCh == nil {
			break
		}
		<-waitCh
	}

	// The transport remains mounted until its dependent controllers are gone.
	var releases []func()
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		releases, s.relFns = s.relFns, nil
	})
	for _, release := range releases {
		release()
	}
}

// StartP2PSync starts direct synchronization on the default Session transport.
func (a *ProviderAccount) StartP2PSync(ctx context.Context, st *transport.SessionTransport) error {
	return a.startP2PSyncForSession(ctx, "", st)
}

// startP2PSyncForSession retains invitation handling and reconciles Spaces as
// they load, are created, or leave the account while this Session stays mounted.
func (a *ProviderAccount) startP2PSyncForSession(ctx context.Context, sessionID string, st *transport.SessionTransport) error {
	// Retire any preceding composition before publishing its replacement.
	childBus := st.GetChildBus()
	if childBus == nil {
		return nil
	}
	a.stopP2PSyncForSession(sessionID)
	syncCtx, cancel := context.WithCancel(ctx)
	state := &p2pSyncState{cancel: cancel, watcher: newNamedRoutineContainer(a.le, "p2p-space-list")}
	objects := keyed.NewKeyed(func(key p2pSyncSpaceKey) (keyed.Routine, struct{}) {
		return func(ctx context.Context) error {
			if !state.beginRoutine(&key) {
				return nil
			}
			defer state.endRoutine(&key)
			return a.runP2PSpace(ctx, childBus, st, state, key)
		}, struct{}{}
	}, keyed.WithExitLogger[p2pSyncSpaceKey, struct{}](a.le), keyed.WithRetry[p2pSyncSpaceKey, struct{}](providerBackoff))
	state.watcher.SetRoutine(func(ctx context.Context) error {
		// The watcher joins the same drain fence as the Space routines it starts.
		if !state.beginRoutine(nil) {
			return nil
		}
		defer state.endRoutine(nil)

		// Keyed owns each Space's cancellation and retry independently.
		objects.SetContext(ctx, true)
		defer objects.ClearContext()
		current := a.soListCtr.GetValue()
		for {
			var keys []p2pSyncSpaceKey
			for _, entry := range current.GetSharedObjects() {
				ref := entry.GetRef()
				if ref.GetProviderResourceRef().GetId() != "" {
					keys = append(keys, p2pSyncSpaceKey{ref.GetProviderResourceRef().GetId(), ref.GetBlockStoreId()})
				}
			}
			objects.SyncKeys(keys, false)

			// Wait against the same snapshot used for reconciliation.
			next, err := a.soListCtr.WaitValueChange(ctx, current, nil)
			if err != nil {
				return err
			}
			current = next
		}
	})
	a.p2pSyncMtx.Lock()
	if a.p2pSyncs == nil {
		a.p2pSyncs = make(map[string]*p2pSyncState)
	}
	a.p2pSyncs[sessionID] = state
	a.p2pSyncMtx.Unlock()

	// Session-scoped mounts and invitation RPCs precede per-Space synchronization.
	release, err := childBus.AddController(syncCtx, &sessionSharedObjectMountController{account: a, sessionID: sessionID}, nil)
	if err != nil {
		a.stopP2PSyncForSession(sessionID)
		return errors.Wrap(err, "register session shared object mount")
	}
	state.addRelease(release)
	if err := a.startInviteServer(syncCtx, childBus, st, state); err != nil {
		a.stopP2PSyncForSession(sessionID)
		return errors.Wrap(err, "start invite server")
	}
	state.watcher.SetContext(syncCtx, true)
	return nil
}

// StopP2PSync drains the default Session's direct synchronization resources.
func (a *ProviderAccount) StopP2PSync() {
	a.stopP2PSyncForSession("")
}

// stopP2PSyncForSession withdraws direct reads before draining their controllers.
func (a *ProviderAccount) stopP2PSyncForSession(sessionID string) {
	a.p2pSyncMtx.Lock()
	state := a.p2pSyncs[sessionID]
	delete(a.p2pSyncs, sessionID)
	a.p2pSyncMtx.Unlock()
	if state != nil {
		state.stop()
	}
}

// runP2PSpace retains block exchange and signed state synchronization for one Space.
func (a *ProviderAccount) runP2PSpace(ctx context.Context, childBus bus.Bus, st *transport.SessionTransport, state *p2pSyncState, key p2pSyncSpaceKey) error {
	// Publish the direct read adapter only while its controller remains attached.
	bucketID := BlockStoreBucketID(a.accountID, key.blockStoreID)
	ctrl, _, dexRef, err := loader.WaitExecControllerRunningTyped[*dex_solicit.Controller](ctx, childBus, resolver.NewLoadControllerWithConfig(&dex_solicit.Config{
		BucketId: bucketID, ProtocolContext: []byte(key.id),
	}), nil)
	if err != nil {
		return err
	}
	defer dexRef.Release()
	store := dex_solicit.NewStore(ctrl)
	state.addStore(bucketID, store)
	defer state.removeStore(bucketID, store)

	// The Session facade supplies its own identity and participant validation.
	ref := sobject.NewSharedObjectRef(a.p.GetProviderInfo().GetProviderId(), a.accountID, key.id, key.blockStoreID)
	so, relSO, err := sobject.ExMountSharedObject(ctx, childBus, ref, false, nil)
	if err != nil {
		return err
	}
	defer relSO.Release()
	swSO, ok := so.(*sessionSharedObject)
	if !ok {
		return errors.New("unexpected session shared object type")
	}
	validateSnapshotAccess := func(ctx context.Context, state *sobject.SOState) error {
		snapshot := sobject.NewSOStateParticipantHandle(a.le, a.p.sfs, key.id, state, swSO.privKey, swSO.localPid)
		_, err := snapshot.GetTransformer(ctx)
		return err
	}
	return sobject_sync.NewSOSync(a.le, childBus, key.id, st.GetPeerID(), swSO.GetSOHost(), validateSnapshotAccess).Execute(ctx)
}

// getSessionDEXStore reads the live adapter without retaining a stopped composition.
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
	// Invitation lookup follows the current account inventory.
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

			// Read invitation state while the Space is retained.
			swSO, relSO, err := a.mountSpaceSO(ctx, soID)
			if err != nil {
				continue
			}
			soState, err := swSO.GetSOHost().GetHostState(ctx)
			if err != nil {
				relSO()
				continue
			}

			// Return only the invite matching the signed capability hash.
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

	// Enrollment reacquires the Space for the duration of participant mutation.
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

	// Retain invitation RPC handling until every dependent Space routine drains.
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

	// The Session composition releases this registration during shutdown.
	relCtrl, err := childBus.AddController(ctx, ctrl, nil)
	if err != nil {
		return err
	}
	state.addRelease(relCtrl)
	return nil
}
