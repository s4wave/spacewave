package provider_spacewave

import (
	"context"
	"slices"

	"github.com/aperturerobotics/util/scrub"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/provider"
	provider_migration "github.com/s4wave/spacewave/core/provider/migration"
	"github.com/s4wave/spacewave/core/session"
	session_lock "github.com/s4wave/spacewave/core/session/lock"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/object"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/net/keypem"
)

// SessionCredentialStore retains this mounted Session's own encrypted credential.
func (s *Session) SessionCredentialStore() object.ObjectStore { return s.objStore }

// RetireAccountTransport joins the old Session's cloud and direct connections.
func (s *Session) RetireAccountTransport(context.Context) error {
	s.tkr.a.StopSessionTransportComposition(s.tkr.id)
	return nil
}

// AttachMigratedSession verifies provider authorization, preserves the moving
// client's key and lock policy, and starts its new account attachment.
func (a *ProviderAccount) AttachMigratedSession(ctx context.Context, source session.Session, transition *provider.AccountTransition) (*session.SessionRef, error) {
	// Validate the account transition.
	if err := transition.Validate(); err != nil {
		return nil, err
	}

	// Verify the transition authorizes this destination account and Session.
	if transition.GetDestination().GetProviderId() != a.GetProviderID() || transition.GetDestination().GetProviderAccountId() != a.accountID || transition.GetDestinationEndpoint() != a.p.endpoint || !slices.Contains(transition.GetSessionPeerIds(), source.GetPeerId().String()) {
		return nil, errors.New("account transition does not authorize this Session and destination")
	}

	// Require the source Session to carry its protected credential.
	credential, ok := source.(provider_migration.CredentialSource)
	if !ok {
		return nil, errors.New("source provider cannot move this Session's protected credential")
	}

	// Read the cloud account info with the source Session's key.
	client, err := a.migrationClient(source.GetPrivKey())
	if err != nil {
		return nil, err
	}
	info, err := client.GetAccountInfo(ctx)
	if err != nil {
		return nil, err
	}

	// Verify the cloud has accepted the transition.
	if info.GetAccountId() != a.accountID || !info.GetTransition().EqualVT(transition) {
		return nil, errors.New("cloud provider has not accepted this Session's account transition")
	}

	// Derive the local storage key for the credential copy.
	storage, err := a.vol.GetPeer(ctx, true)
	if err != nil {
		return nil, err
	}
	key, err := storage.GetPrivKey(ctx)
	if err != nil {
		return nil, err
	}
	storageKey, err := session_lock.DeriveStorageKey(key)
	if err != nil {
		return nil, err
	}

	// Rewrite the Session ref to point at this account.
	ref := source.GetSessionRef().CloneVT()
	ref.ProviderResourceRef.ProviderId = a.GetProviderID()
	ref.ProviderResourceRef.ProviderAccountId = a.accountID

	// Open this account's object store handle.
	handle, _, releaseStore, err := volume.ExBuildObjectStoreAPI(ctx, a.p.b, false, SessionObjectStoreID(a.accountID), a.vol.GetID(), nil)
	if err != nil {
		return nil, err
	}
	defer releaseStore.Release()
	id := ref.GetProviderResourceRef().GetId()

	// Copy the Session credential into the account object store.
	if err := session_lock.CopyCredential(ctx, credential.SessionCredentialStore(), handle.GetObjectStore(), source.GetSessionRef().GetProviderResourceRef().GetId(), id, source.GetPrivKey(), storageKey); err != nil {
		return nil, err
	}

	// Record the registered peer ID for the Session.
	if err := kvtx.RunTransaction(ctx, true, func(ctx context.Context) (kvtx.Tx, error) {
		return handle.GetObjectStore().NewTransaction(ctx, true)
	}, func(ctx context.Context, tx kvtx.Tx) error {
		return tx.Set(ctx, []byte(id+"/registered"), []byte(source.GetPeerId().String()))
	}); err != nil {
		return nil, err
	}

	// Retire the old account transport.
	if err := credential.RetireAccountTransport(ctx); err != nil {
		return nil, err
	}

	// Marshal and scrub the Session's private key for the unlock result.
	plain, err := keypem.MarshalPrivKeyPem(source.GetPrivKey())
	if err != nil {
		return nil, err
	}
	defer scrub.Scrub(plain)

	// Resolve the pending unlock for the mounted Session key.
	reference, tracker, _ := a.sessions.AddKeyRef(id)
	defer reference.Release()
	tracker.unlockProm.SetResult(slices.Clone(plain), nil)

	// Mount the migrated Session under this account.
	_, releaseSession, err := a.MountSession(ctx, ref, nil)
	if err != nil {
		return nil, err
	}
	defer releaseSession()
	return ref, nil
}
