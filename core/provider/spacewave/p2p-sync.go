package provider_spacewave

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

// StartP2PSync starts SO sync, DEX block exchange, and the direct invite
// server for the mounted cloud session transport.
func (a *ProviderAccount) StartP2PSync(
	ctx context.Context,
	sessionTransport *transport.SessionTransport,
) error {
	childBus := sessionTransport.GetChildBus()
	if childBus == nil {
		return nil
	}

	a.StopP2PSync()

	syncCtx, syncCancel := context.WithCancel(ctx)
	state := &p2pSyncState{cancel: syncCancel}

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

			if err := a.startSOSync(syncCtx, childBus, ref, soID, state); err != nil {
				a.le.WithError(err).WithField("so-id", soID).Warn("failed to start so sync")
				continue
			}

			bucketID := BlockStoreBucketID(a.accountID, blockStoreID)
			if err := a.startDEXSolicit(syncCtx, childBus, bucketID, state); err != nil {
				a.le.WithError(err).WithField("bucket-id", bucketID).Warn("failed to start dex solicit")
				continue
			}
		}
	}

	if err := a.startInviteServer(syncCtx, childBus, sessionTransport, state); err != nil {
		a.le.WithError(err).Warn("failed to start invite server")
	}

	a.p2pSync = state
	return nil
}

// StopP2PSync stops all P2P sync controllers, waits for goroutines
// to finish, and releases references.
func (a *ProviderAccount) StopP2PSync() {
	if a.p2pSync == nil {
		return
	}
	a.p2pSync.cancel()
	a.p2pSync.wg.Wait()
	for _, ref := range a.p2pSync.refs {
		ref.Release()
	}
	for _, rel := range a.p2pSync.relFns {
		rel()
	}
	a.p2pSync = nil
}

// startSOSync mounts the shared object and starts an SOSync instance for it.
func (a *ProviderAccount) startSOSync(
	ctx context.Context,
	childBus bus.Bus,
	ref *sobject.SharedObjectRef,
	soID string,
	state *p2pSyncState,
) error {
	so, relSO, err := a.MountSharedObject(ctx, ref, nil)
	if err != nil {
		return err
	}

	swSO, ok := so.(*SharedObject)
	if !ok {
		relSO()
		return errors.New("unexpected shared object type")
	}

	soSync := sobject_sync.NewSOSync(a.le, childBus, soID, swSO.GetSOHost())
	state.wg.Go(func() {
		defer relSO()
		if err := soSync.Execute(ctx); err != nil && ctx.Err() == nil {
			a.le.WithError(err).WithField("so-id", soID).Warn("so sync exited with error")
		}
	})
	return nil
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
	state.relFns = append(state.relFns, relCtrl)
	return nil
}

// startDEXSolicit loads a DEX solicit controller on the child bus for
// the given block store bucket.
func (a *ProviderAccount) startDEXSolicit(
	ctx context.Context,
	childBus bus.Bus,
	bucketID string,
	state *p2pSyncState,
) error {
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
