package provider_local

import (
	"bytes"
	"context"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_invite "github.com/s4wave/spacewave/core/sobject/invite"
	sobject_sync "github.com/s4wave/spacewave/core/sobject/sync"
	"github.com/s4wave/spacewave/core/transport"
	dex_solicit "github.com/s4wave/spacewave/db/dex/solicit"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// p2pSyncState holds running P2P sync state for a provider account.
type p2pSyncState struct {
	cancel context.CancelFunc
	wg     sync.WaitGroup
	refs   []directive.Reference
	relFns []func()
}

// StartP2PSync starts SO sync and DEX block exchange for all mounted
// shared objects. Called when a P2P-linked device connects.
//
// childBus is the session transport's child bus where solicit
// controllers run. The session transport must be running before
// calling this method.
func (a *ProviderAccount) StartP2PSync(ctx context.Context, sessionTransport *transport.SessionTransport) error {
	childBus := sessionTransport.GetChildBus()
	if childBus == nil {
		return nil
	}

	a.p2pSyncMtx.Lock()
	defer a.p2pSyncMtx.Unlock()

	a.stopP2PSyncLocked()

	syncCtx, syncCancel := context.WithCancel(ctx)
	state := &p2pSyncState{cancel: syncCancel}

	soList := a.soListCtr.GetValue()
	for _, entry := range soList.GetSharedObjects() {
		ref := entry.GetRef()
		provRef := ref.GetProviderResourceRef()
		soID := provRef.GetId()
		blockStoreID := ref.GetBlockStoreId()

		if err := a.startSOSync(syncCtx, childBus, ref, soID, state); err != nil {
			if ctx.Err() != nil {
				a.stopP2PSyncState(state)
				return ctx.Err()
			}
			a.le.WithError(err).WithField("so-id", soID).Warn("failed to start so sync")
			continue
		}

		providerID := provRef.GetProviderId()
		providerAccountID := provRef.GetProviderAccountId()
		bucketID := BlockStoreBucketID(providerID, providerAccountID, blockStoreID)
		if err := a.startDEXSolicit(syncCtx, childBus, bucketID, state); err != nil {
			if ctx.Err() != nil {
				a.stopP2PSyncState(state)
				return ctx.Err()
			}
			a.le.WithError(err).WithField("bucket-id", bucketID).Warn("failed to start dex solicit")
			continue
		}
	}

	// Start the SO invite server so invitees can join via alpha/so-invite.
	if err := a.startInviteServer(syncCtx, childBus, sessionTransport, state); err != nil {
		a.le.WithError(err).Warn("failed to start invite server")
	}

	a.p2pSync = state
	return nil
}

// IsP2PSyncRunning returns whether P2P sync is currently active.
// Safe to call from any goroutine.
func (a *ProviderAccount) IsP2PSyncRunning() bool {
	a.p2pSyncMtx.Lock()
	running := a.p2pSync != nil
	a.p2pSyncMtx.Unlock()
	return running
}

// StopP2PSync stops all P2P sync controllers, waits for goroutines
// to finish, and releases references.
func (a *ProviderAccount) StopP2PSync() {
	a.p2pSyncMtx.Lock()
	defer a.p2pSyncMtx.Unlock()

	a.stopP2PSyncLocked()
}

func (a *ProviderAccount) stopP2PSyncLocked() {
	state := a.p2pSync
	if state == nil {
		return
	}
	a.p2pSync = nil
	a.stopP2PSyncState(state)
}

func (a *ProviderAccount) stopP2PSyncState(state *p2pSyncState) {
	if state == nil {
		return
	}
	state.cancel()
	state.wg.Wait()
	for _, ref := range state.refs {
		ref.Release()
	}
	for _, rel := range state.relFns {
		rel()
	}
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
	_, _, dexRef, err := loader.WaitExecControllerRunning(
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
	return nil
}
