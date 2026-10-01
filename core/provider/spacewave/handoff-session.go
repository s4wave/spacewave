package provider_spacewave

import (
	"context"
	"time"

	"github.com/aperturerobotics/util/scrub"
	"github.com/aperturerobotics/util/ulid"
	"github.com/pkg/errors"
	provider "github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/session"
	session_lock "github.com/s4wave/spacewave/core/session/lock"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/keypem"
	"github.com/s4wave/spacewave/net/peer"
)

// MountHandoffSession mounts a session returned by browser auth handoff.
func (p *Provider) MountHandoffSession(
	ctx context.Context,
	accountID string,
	sessionPriv crypto.PrivKey,
	sessionCtrl session.SessionController,
) (*session.SessionListEntry, error) {
	// Verify the handoff key identifies an enrolled Session in this account.
	if sessionPriv == nil {
		return nil, errors.New("session private key is required")
	}
	peerID, err := peer.IDFromPrivateKey(sessionPriv)
	if err != nil {
		return nil, err
	}
	client := NewSessionClient(p.httpCli, p.endpoint, p.GetSigningEnvPrefix(), sessionPriv, peerID.String())
	info, err := client.GetAccountInfo(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "verify enrolled Session")
	}
	if info.GetAccountId() != accountID {
		return nil, errors.New("enrolled Session belongs to another account")
	}

	// Access the provider account and skip mounting an already-registered Session.
	provAccValue, relProvAcc, err := p.AccessProviderAccount(ctx, accountID, nil)
	if err != nil {
		return nil, errors.Wrap(err, "access provider account")
	}
	defer relProvAcc()
	provAcc := provAccValue.(*ProviderAccount)
	entries, err := sessionCtrl.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	if existing, err := provAcc.findRegisteredSession(ctx, entries, peerID); err != nil || existing != nil {
		return existing, err
	}

	// Seed the handoff Session and mount it under the account.
	sessProv, err := session.GetSessionProviderAccountFeature(ctx, provAcc)
	if err != nil {
		return nil, errors.Wrap(err, "get session provider")
	}

	// Build a fresh Session ref for the handoff identity.
	sessRef := &session.SessionRef{
		ProviderResourceRef: &provider.ProviderResourceRef{
			Id:                ulid.NewULID(),
			ProviderAccountId: accountID,
			ProviderId:        p.info.GetProviderId(),
		},
	}
	if err := p.seedHandoffSession(ctx, provAcc, sessRef, sessionPriv); err != nil {
		return nil, err
	}

	// Mount the Session with the session provider.
	_, relSess, err := sessProv.MountSession(ctx, sessRef, nil)
	if err != nil {
		return nil, errors.Wrap(err, "mount session")
	}
	defer relSess()

	// Register the Session with the session controller.
	meta := &session.SessionMetadata{
		DisplayName:         info.GetEntityId(),
		ProviderDisplayName: "Cloud",
		ProviderAccountId:   accountID,
		ProviderId:          p.info.GetProviderId(),
		CreatedAt:           time.Now().UnixMilli(),
	}
	listEntry, err := sessionCtrl.RegisterSession(ctx, sessRef, meta)
	if err != nil {
		return nil, errors.Wrap(err, "register session")
	}

	return listEntry, nil
}

func (p *Provider) seedHandoffSession(
	ctx context.Context,
	acc *ProviderAccount,
	sessRef *session.SessionRef,
	sessionPriv crypto.PrivKey,
) error {
	// Open the account's Session object store handle for seeding.
	ctx, ctxCancel := context.WithCancel(ctx)
	defer ctxCancel()

	// Build the object store handle for the Session's ID.
	sessionID := sessRef.GetProviderResourceRef().GetId()
	objStoreHandle, _, diRef, err := volume.ExBuildObjectStoreAPI(
		ctx,
		p.b,
		false,
		SessionObjectStoreID(acc.accountID),
		acc.vol.GetID(),
		ctxCancel,
	)
	if err != nil {
		return errors.Wrap(err, "mount session object store")
	}
	defer diRef.Release()

	// Derive the storage key from the volume's private key.
	volPeer, err := acc.vol.GetPeer(ctx, true)
	if err != nil {
		return errors.Wrap(err, "get volume peer")
	}
	volPrivKey, err := volPeer.GetPrivKey(ctx)
	if err != nil {
		return errors.Wrap(err, "get volume priv key")
	}
	storageKey, err := session_lock.DeriveStorageKey(volPrivKey)
	if err != nil {
		return errors.Wrap(err, "derive storage key")
	}

	// Marshal the Session key and write its encrypted auto-unlock record.
	privPEM, err := keypem.MarshalPrivKeyPem(sessionPriv)
	if err != nil {
		return errors.Wrap(err, "marshal session private key")
	}
	defer scrub.Scrub(privPEM)

	// Encrypt the key with the storage key and write the auto-unlock record.
	encPriv, err := session_lock.EncryptAutoUnlock(storageKey, privPEM)
	if err != nil {
		return errors.Wrap(err, "encrypt session private key")
	}
	if err := session_lock.WriteAutoUnlock(
		ctx,
		objStoreHandle.GetObjectStore(),
		sessionID,
		encPriv,
	); err != nil {
		return errors.Wrap(err, "write auto-unlock key")
	}

	// Record the Session's registered peer ID in the object store.
	sessionPeerID, err := peer.IDFromPrivateKey(sessionPriv)
	if err != nil {
		return errors.Wrap(err, "derive session peer id")
	}

	// Write the registration marker in a committed transaction.
	regKey := []byte(sessionID + "/registered")
	err = kvtx.RunTransaction(ctx, true,
		func(ctx context.Context) (kvtx.Tx, error) {
			return objStoreHandle.GetObjectStore().NewTransaction(ctx, true)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			if err := tx.Set(ctx, regKey, []byte(sessionPeerID.String())); err != nil {
				return errors.Wrap(err, "write registration marker")
			}
			return nil
		},
	)
	if err != nil {
		return errors.Wrap(err, "registration transaction")
	}

	return nil
}

// findRegisteredSession reads persisted public registration markers without
// unlocking existing Sessions. Reusing the same key preserves its lock policy.
func (a *ProviderAccount) findRegisteredSession(ctx context.Context, entries []*session.SessionListEntry, peerID peer.ID) (*session.SessionListEntry, error) {
	// Open the account object store and scan registration markers.
	handle, _, ref, err := volume.ExBuildObjectStoreAPI(ctx, a.p.b, false, SessionObjectStoreID(a.accountID), a.vol.GetID(), nil)
	if err != nil {
		return nil, err
	}
	defer ref.Release()

	// Return the first entry whose marker matches the peer ID.
	store := handle.GetObjectStore()
	var existing *session.SessionListEntry
	err = kvtx.RunTransaction(ctx, false, func(ctx context.Context) (kvtx.Tx, error) {
		return store.NewTransaction(ctx, false)
	}, func(ctx context.Context, tx kvtx.Tx) error {
		for _, entry := range entries {
			ref := entry.GetSessionRef().GetProviderResourceRef()
			if ref.GetProviderAccountId() != a.accountID || ref.GetProviderId() != a.GetProviderID() {
				continue
			}
			data, found, err := tx.Get(ctx, []byte(ref.GetId()+"/registered"))
			if err != nil {
				return err
			}
			if found && string(data) == peerID.String() {
				existing = entry
				return nil
			}
		}
		return nil
	})
	return existing, err
}
