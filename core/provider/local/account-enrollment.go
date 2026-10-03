package provider_local

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/core/pairing"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_invite "github.com/s4wave/spacewave/core/sobject/invite"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/net/peer"
)

// buildPairingIdentity proves the Session and volume keys independently.
func (a *ProviderAccount) buildPairingIdentity(ctx context.Context, offer *pairing.AccountOffer, sess session.Session, sourcePeer, receivingPeer peer.ID) (*pairing.Identity, error) {
	// Resolve the volume's storage peer and its key.
	storagePeer, err := a.vol.GetPeer(ctx, true)
	if err != nil {
		return nil, err
	}
	storageKey, err := storagePeer.GetPrivKey(ctx)
	if err != nil {
		return nil, err
	}
	return pairing.BuildIdentity(offer, sess.GetSessionRef(), sess.GetPrivKey(), storageKey, sourcePeer, receivingPeer)
}

// enrollPairingObject grants the approved receiving identities and exports the
// resulting checkpoint. Its caller must have completed bilateral approval.
func (a *ProviderAccount) enrollPairingObject(ctx context.Context, entry *sobject.SharedObjectListEntry, identity *pairing.Identity) (*pairing.SharedObject, error) {
	// Initial enrollment requires signed possession of both independent keys.
	for _, proof := range []*sobject.SOJoinResponse{identity.GetSessionProof(), identity.GetStorageProof()} {
		if _, _, err := sobject_invite.ValidateJoinResponse(proof); err != nil {
			return nil, err
		}
	}
	return a.enrollAccountMemberObject(ctx, entry, &account_settings.AccountSession{
		PeerId: identity.GetSessionProof().GetResponderPeerId(), StoragePeerId: identity.GetStorageProof().GetResponderPeerId(),
	})
}

// installPairingObject accepts a checkpoint only for the offered account. The
// authenticated enrollment exchange supplies its trust; the catalog does not.
func (a *ProviderAccount) installPairingObject(ctx context.Context, offer *pairing.AccountOffer, object *pairing.SharedObject, sourcePeer peer.ID) error {
	// Validate the object reference and its account binding.
	entry := object.GetEntry()
	ref := entry.GetRef()
	if err := ref.Validate(); err != nil {
		return err
	}
	providerRef := ref.GetProviderResourceRef()
	if offer.GetAccountId() != a.GetAccountID() || providerRef.GetProviderAccountId() != a.GetAccountID() || providerRef.GetProviderId() != a.GetProviderID() {
		return errors.New("pairing object belongs to another account")
	}

	// Require a complete checkpoint before mounting.
	if object.GetState() == nil || entry.GetMeta().GetBodyType() == "" {
		return errors.New("pairing object checkpoint is incomplete")
	}

	// Mount the enrolled object with its config lineage.
	return a.mountEnrolledSO(ctx, providerRef.GetId(), entry.GetMeta(), entry.GetSource(), object.GetState(), object.GetConfigLineage(), sourcePeer)
}

// bindPairingSettings replaces only the empty settings object created while
// opening a fresh replica. Existing account data prevents an identity change.
func (a *ProviderAccount) bindPairingSettings(ctx context.Context, offer *pairing.AccountOffer) error {
	// Serialize the persisted binding and local list update with account mutations.
	release, err := a.mtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	current, err := a.GetAccountSettingsRef(ctx)
	if err != nil {
		return err
	}
	if current.GetProviderResourceRef().GetId() == offer.GetSettingsId() {
		return nil
	}
	list := a.soListCtr.GetValue()
	if len(list.GetSharedObjects()) != 1 || list.GetSharedObjects()[0].GetRef().GetProviderResourceRef().GetId() != current.GetProviderResourceRef().GetId() {
		return errors.New("existing account has a different settings identity; merge or repair is required")
	}

	// A fresh account's default object has no user operations or state to preserve.
	so, releaseSO, err := a.MountSharedObject(ctx, current, nil)
	if err != nil {
		return err
	}
	defer releaseSO()
	snapshot, err := so.GetSharedObjectState(ctx)
	if err != nil {
		return err
	}
	folded, err := sobject.Fold(ctx, snapshot, account_settings.ProcessAccountSettingsOps)
	if err != nil {
		return err
	}
	if len(folded.StateData) != 0 {
		return errors.New("existing account settings contain data; merge or repair is required")
	}

	// Keep the old object durable until the new binding is committed.
	ref := sobject.NewSharedObjectRef(a.GetProviderID(), a.GetAccountID(), offer.GetSettingsId(), SobjectBlockStoreID(offer.GetSettingsId()))

	// Replace the old object with the offered settings in the list.
	next := list.CloneVT()
	next.SharedObjects = slices.DeleteFunc(next.SharedObjects, func(entry *sobject.SharedObjectListEntry) bool {
		return entry.GetRef().GetProviderResourceRef().GetId() == current.GetProviderResourceRef().GetId()
	})

	// Open the object store and marshal the new binding and list.
	store, releaseStore, err := a.buildSoObjectStore(ctx)
	if err != nil {
		return err
	}
	defer releaseStore()
	binding, err := ref.MarshalVT()
	if err != nil {
		return err
	}
	listData, err := next.MarshalVT()
	if err != nil {
		return err
	}

	// Commit the new binding and list, then publish them in memory.
	if err := kvtx.RunTransaction(ctx, true, func(ctx context.Context) (kvtx.Tx, error) {
		return store.NewTransaction(ctx, true)
	}, func(ctx context.Context, tx kvtx.Tx) error {
		if err := tx.Set(ctx, SobjectBindingKey(accountSettingsBindingPurpose), binding); err != nil {
			return err
		}
		return tx.Set(ctx, SobjectObjectStoreListKey(), listData)
	}); err != nil {
		return err
	}

	// Publish the new list in memory.
	a.soListCtr.SetValue(next)
	return nil
}
