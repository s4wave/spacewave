package provider_local

import (
	"context"
	"slices"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/routine"
	"github.com/pkg/errors"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/core/pairing"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/peer"
	stream_srpc "github.com/s4wave/spacewave/net/stream/srpc"
)

// publishAccountCatalogEntry commits catalog changes through canonical settings.
// The automatically created settings object is published by account enrollment.
func (a *ProviderAccount) publishAccountCatalogEntry(ctx context.Context, entry *sobject.SharedObjectListEntry, deleted bool) error {
	// The settings object never lists itself.
	ref, err := a.GetAccountSettingsRef(ctx)
	if err != nil {
		return err
	}
	if ref.EqualVT(entry.GetRef()) {
		if deleted {
			return errors.New("cannot delete canonical account settings")
		}
		return nil
	}

	// Commit the entry through the mounted settings object.
	so, release, err := a.MountSharedObject(ctx, ref, nil)
	if err != nil {
		return err
	}
	defer release()
	return commitAccountSettingsOp(ctx, so, &account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_UpsertCatalogEntry{
			UpsertCatalogEntry: &account_settings.AccountCatalogEntry{Entry: entry.CloneVT(), Deleted: deleted},
		},
	})
}

// runAccountReplicaSync reacts to settings and local inventory changes. Network
// failures use the owning routine's backoff; unchanged accounts produce no traffic.
func (a *ProviderAccount) runAccountReplicaSync(ctx context.Context, state *p2pSyncState) error {
	// Deliver account transitions as the settings change.
	delivery := routine.NewStateRoutineContainerWithLoggerVT[*account_settings.AccountSettings](a.le.WithField("routine", "account-transition-delivery"), routine.WithRetry(providerBackoff))
	delivery.SetStateRoutine(func(ctx context.Context, settings *account_settings.AccountSettings) error {
		return a.deliverAccountTransitions(ctx, state, settings)
	})
	delivery.SetContext(ctx, false)
	defer func() {
		if exited, _, _ := delivery.SetStateRoutine(nil); exited != nil {
			<-exited
		}
	}()

	// Watch the settings object.
	ref, err := a.GetAccountSettingsRef(ctx)
	if err != nil {
		return err
	}
	so, release, err := a.MountSharedObject(ctx, ref, nil)
	if err != nil {
		return err
	}
	defer release()
	states, releaseStates, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		return err
	}
	defer releaseStates()

	// Reconcile the replica on each settings change.
	var snapshot sobject.SharedObjectStateSnapshot
	for {
		// Catalog mutations wake this same owner through the settings state.
		snapshot, err = states.WaitValueChange(ctx, snapshot, nil)
		if err != nil {
			return err
		}
		if snapshot != nil {
			settings, err := account_settings.ReadSnapshot(ctx, snapshot)
			if err != nil {
				return err
			}
			if err := a.reconcileAccountReplica(ctx, state, ref.GetProviderResourceRef().GetId(), settings, a.soListCtr.GetValue()); err != nil {
				return err
			}
			delivery.SetState(settings)
		}
	}
}

// reconcileAccountReplica follows approved membership and discovers new objects
// through their native owner service. Inventory metadata never supplies trust.
func (a *ProviderAccount) reconcileAccountReplica(ctx context.Context, state *p2pSyncState, settingsID string, settings *account_settings.AccountSettings, list *sobject.SharedObjectList) error {
	// Only an active Session of the account follows its membership.
	localPeer := state.sessionTransport.GetPeerID().String()
	self := settings.FindAccountSession(localPeer)
	if self == nil || self.GetRevoked() {
		return nil
	}

	// A moved account's offline clients already have approved Session keys.
	// Retain their endpoints so the signed recovery object can deliver the
	// redirect before they know the destination account's current peer mesh.
	for _, migration := range settings.GetAcceptedMigrations() {
		for _, id := range migration.GetSessionPeerIds() {
			if id == localPeer || settings.FindAccountSession(id).GetRevoked() {
				continue
			}
			remote, _, err := peer.ParsePeerIDWithPubKey(id)
			if err != nil {
				return err
			}
			if err := a.retainP2PPeerOnState(ctx, state, remote); err != nil {
				return err
			}
		}
	}

	// Every active Session retains every other Session, producing the account mesh.
	var members []*account_settings.AccountSession
	for _, member := range settings.GetSessions() {
		if member.GetPeerId() == localPeer {
			continue
		}
		remote, _, err := peer.ParsePeerIDWithPubKey(member.GetPeerId())
		if err != nil {
			return err
		}
		if member.GetRevoked() {
			a.releaseAccountReplicaPeer(remote)
			if err := a.revokeAccountReplicaAccess(ctx, settings, member); err != nil {
				return err
			}
			continue
		}
		if err := a.retainP2PPeerOnState(ctx, state, remote); err != nil {
			return err
		}
		members = append(members, member)
	}

	// Apply the catalog to the local list, then fetch the objects it lacks.
	missing, err := a.reconcileAccountCatalog(ctx, settingsID, settings, list)
	if err != nil {
		return err
	}
	installed := false
	for _, entry := range missing {
		if err := a.fetchAccountReplicaObject(ctx, state, settingsID, entry, members); err != nil {
			return err
		}
		installed = true
	}
	if installed {
		for _, member := range settings.GetSessions() {
			if member.GetRevoked() {
				if err := a.revokeAccountReplicaAccess(ctx, settings, member); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// reconcileAccountCatalog republishes local objects the catalog lacks,
// removes local objects whose tombstone names their generation, applies
// catalog metadata, and returns the live catalog entries not held locally.
//
// The reconcile removes a local object only when a tombstone names its
// generation; empty equals empty for objects made before generations. A local
// object of another generation than the catalog's is kept untouched, deleted
// or live: a delete is not carried to it, and the catalog's object is not
// installed over it. The generation is checked again under the account lock,
// so a create that lands after list was read is never removed.
func (a *ProviderAccount) reconcileAccountCatalog(ctx context.Context, settingsID string, settings *account_settings.AccountSettings, list *sobject.SharedObjectList) ([]*sobject.SharedObjectListEntry, error) {
	// Recover locally created objects whose catalog publication was
	// interrupted: an object the catalog lacks, or one created under the id
	// of an earlier object whose tombstone names another generation.
	local := make(map[string]*sobject.SharedObjectListEntry, len(list.GetSharedObjects()))
	for _, entry := range list.GetSharedObjects() {
		id := entry.GetRef().GetProviderResourceRef().GetId()
		local[id] = entry
		catalog := settings.FindCatalogEntry(id)
		unpublished := catalog == nil || (catalog.GetDeleted() && catalog.GetEntry().GetGeneration() != entry.GetGeneration())
		if unpublished && id != settingsID {
			if err := a.publishAccountCatalogEntry(ctx, entry, false); err != nil {
				return nil, err
			}
		}
	}

	// Reconcile deletions and presentation changes, collecting new objects.
	var missing []*sobject.SharedObjectListEntry
	for _, catalog := range settings.GetCatalog() {
		entry := catalog.GetEntry()
		id := entry.GetRef().GetProviderResourceRef().GetId()
		if entry.GetRef().GetProviderResourceRef().GetProviderAccountId() != a.GetAccountID() || entry.GetRef().GetProviderResourceRef().GetProviderId() != a.GetProviderID() {
			return nil, errors.New("catalog object belongs to another account")
		}
		current := local[id]
		if current != nil && id != settingsID && current.GetGeneration() != entry.GetGeneration() {
			a.le.WithField("sobject-id", id).Warn("keeping local shared object of another generation than the account catalog's")
			continue
		}
		if catalog.GetDeleted() {
			if current != nil && id != settingsID {
				if err := a.deleteCatalogTombstoned(ctx, id, entry.GetGeneration()); err != nil {
					return nil, err
				}
			}
			continue
		}
		if current != nil {
			if !current.GetMeta().EqualVT(entry.GetMeta()) {
				if err := a.updateSharedObjectMeta(ctx, id, entry.GetMeta(), false); err != nil {
					return nil, err
				}
			}
			continue
		}
		missing = append(missing, entry)
	}
	return missing, nil
}

// deleteCatalogTombstoned removes the local object id if it is still of the
// tombstone's generation once the account lock is held.
func (a *ProviderAccount) deleteCatalogTombstoned(ctx context.Context, id, generation string) error {
	// Recheck the generation under the lock a create holds.
	release, err := a.mtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	listed := a.listedSharedObjectEntry(id)
	if listed == nil || listed.GetGeneration() != generation {
		return nil
	}

	// Remove the object the tombstone names.
	err = a.deleteSharedObjectLocked(ctx, id)
	if err != nil && !errors.Is(err, sobject.ErrSharedObjectNotFound) {
		return err
	}
	return nil
}

// fetchAccountReplicaObject tries each approved account peer with a bounded
// request, allowing a third replica to serve while the original client is offline.
func (a *ProviderAccount) fetchAccountReplicaObject(ctx context.Context, state *p2pSyncState, settingsID string, entry *sobject.SharedObjectListEntry, members []*account_settings.AccountSession) error {
	// Use the transport's live links before waiting on an offline original.
	// Requests remain sequential because issuing owner grants can mutate a
	// SharedObject's configuration lineage on the serving replica.
	peers := make([]peer.ID, 0, len(members))
	for _, member := range members {
		remote, _, err := peer.ParsePeerIDWithPubKey(member.GetPeerId())
		if err != nil {
			return err
		}
		peers = append(peers, remote)
	}
	linked, _ := state.sessionTransport.GetLinkedPeerIDsSnapshotWithWait(peers)
	online := make(map[string]bool, len(linked))
	for id := range linked {
		online[id.String()] = true
	}
	slices.SortStableFunc(members, func(a, b *account_settings.AccountSession) int {
		if online[a.GetPeerId()] == online[b.GetPeerId()] {
			return 0
		}
		if online[a.GetPeerId()] {
			return -1
		}
		return 1
	})

	// Request each approved replica through the generation bus.
	var lastErr error
	for _, member := range members {
		remote, _, err := peer.ParsePeerIDWithPubKey(member.GetPeerId())
		if err != nil {
			return err
		}
		requestCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		open := stream_srpc.NewOpenStreamFunc(state.childBus, accountReplicaProtocol, state.sessionTransport.GetPeerID(), remote, 0)
		object, err := NewSRPCAccountReplicaServiceClient(srpc.NewClient(open)).FetchObject(requestCtx, &AccountReplicaObjectRequest{
			SettingsId: settingsID, ObjectId: entry.GetRef().GetProviderResourceRef().GetId(),
		})
		cancel()
		if err != nil {
			lastErr = err
			continue
		}
		if !object.GetEntry().GetRef().EqualVT(entry.GetRef()) {
			return errors.New("account peer returned a different object")
		}
		return a.installPairingObject(ctx, &pairing.AccountOffer{AccountId: a.GetAccountID(), SettingsId: settingsID, StoragePeerId: member.GetStoragePeerId()}, object)
	}
	if lastErr == nil {
		lastErr = errors.New("no other active account Session can provide the object")
	}
	return errors.Wrap(lastErr, "fetch account object "+entry.GetRef().GetProviderResourceRef().GetId())
}

// releaseAccountReplicaPeer removes reconnect demand after membership revocation.
func (a *ProviderAccount) releaseAccountReplicaPeer(remote peer.ID) {
	// Drop the reconnect demand for the peer.
	var state *p2pSyncState
	a.p2pSyncBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		delete(a.p2pPeerIDs, remote.String())
		state = a.p2pSync
	})
	if state == nil {
		return
	}

	// Release the running link to the peer.
	var release func()
	state.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if ref := state.peerRefs[remote.String()]; ref != nil {
			release = ref.Release
			delete(state.peerRefs, remote.String())
		}
	})
	if release != nil {
		release()
	}
}
