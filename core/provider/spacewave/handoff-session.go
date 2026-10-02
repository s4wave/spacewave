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

	// Access the provider account and read its local Sessions' registered peers.
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
	accountSessions, err := provAcc.readAccountSessions(ctx, entries)
	if err != nil {
		return nil, err
	}

	// Return the Session already registered with the handoff key.
	for _, sess := range accountSessions {
		if sess.peerID == peerID.String() {
			return sess.entry, nil
		}
	}

	// A repeat sign-in joins the account's existing Session instead of adding one.
	if len(accountSessions) != 0 {
		return provAcc.joinAccountSession(ctx, client, accountSessions, sessionPriv)
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

// accountSession is a local Session of the account and the peer ID in its
// registration marker. The peer ID is empty when the Session never registered.
type accountSession struct {
	entry  *session.SessionListEntry
	peerID string
}

// readAccountSessions reads the registration markers of the account's Sessions
// without unlocking them. Reusing the same key preserves its lock policy.
func (a *ProviderAccount) readAccountSessions(ctx context.Context, entries []*session.SessionListEntry) ([]accountSession, error) {
	// Open the account object store holding the registration markers.
	handle, _, ref, err := volume.ExBuildObjectStoreAPI(ctx, a.p.b, false, SessionObjectStoreID(a.accountID), a.vol.GetID(), nil)
	if err != nil {
		return nil, err
	}
	defer ref.Release()

	// Collect each of the account's entries with its marker.
	store := handle.GetObjectStore()
	var sessions []accountSession
	err = kvtx.RunTransaction(ctx, false, func(ctx context.Context) (kvtx.Tx, error) {
		return store.NewTransaction(ctx, false)
	}, func(ctx context.Context, tx kvtx.Tx) error {
		for _, entry := range entries {
			ref := entry.GetSessionRef().GetProviderResourceRef()
			if ref.GetProviderAccountId() != a.accountID || ref.GetProviderId() != a.GetProviderID() {
				continue
			}
			data, _, err := tx.Get(ctx, []byte(ref.GetId()+"/registered"))
			if err != nil {
				return err
			}
			sessions = append(sessions, accountSession{entry: entry, peerID: string(data)})
		}
		return nil
	})
	return sessions, err
}

// joinAccountSession returns one of the account's local Sessions for a handoff
// key that none of them holds. A Session whose key the cloud still lists keeps
// that key, and the redundant handoff key is revoked. Otherwise the first
// Session takes the handoff key and its tracker restarts with it.
func (a *ProviderAccount) joinAccountSession(
	ctx context.Context,
	client *SessionClient,
	sessions []accountSession,
	sessionPriv crypto.PrivKey,
) (*session.SessionListEntry, error) {
	// List the keys the cloud still accepts for the account.
	enrolled, err := client.ListSessions(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "list enrolled Sessions")
	}
	enrolledPeers := make(map[string]struct{}, len(enrolled))
	for _, info := range enrolled {
		enrolledPeers[info.GetPeerId()] = struct{}{}
	}

	// Keep an enrolled local Session and withdraw the handoff key.
	for _, sess := range sessions {
		if _, ok := enrolledPeers[sess.peerID]; ok && sess.peerID != "" {
			if err := client.SelfRevoke(ctx); err != nil {
				return nil, errors.Wrap(err, "revoke redundant handoff key")
			}
			return sess.entry, nil
		}
	}

	// Rekey the first Session and restart its tracker with the handoff key.
	target := sessions[0].entry
	if err := a.p.seedHandoffSession(ctx, a, target.GetSessionRef(), sessionPriv); err != nil {
		return nil, err
	}
	a.sessions.RemoveKey(target.GetSessionRef().GetProviderResourceRef().GetId())
	return target, nil
}
