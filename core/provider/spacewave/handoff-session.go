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
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/keypem"
	"github.com/s4wave/spacewave/net/peer"
)

// MountHandoffSession mounts a session returned by browser auth handoff.
func (p *Provider) MountHandoffSession(
	ctx context.Context,
	accountID string,
	entityID string,
	sessionPriv crypto.PrivKey,
	sessionCtrl session.SessionController,
) (*session.SessionListEntry, error) {
	if sessionPriv == nil {
		return nil, errors.New("session private key is required")
	}

	sessions, err := sessionCtrl.ListSessions(ctx)
	if err == nil {
		for _, entry := range sessions {
			ref := entry.GetSessionRef().GetProviderResourceRef()
			if ref.GetProviderAccountId() == accountID {
				return entry, nil
			}
		}
	}

	provAccValue, relProvAcc, err := p.AccessProviderAccount(ctx, accountID, nil)
	if err != nil {
		return nil, errors.Wrap(err, "access provider account")
	}
	defer relProvAcc()

	provAcc := provAccValue.(*ProviderAccount)
	sessProv, err := session.GetSessionProviderAccountFeature(ctx, provAcc)
	if err != nil {
		return nil, errors.Wrap(err, "get session provider")
	}

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

	sess, relSess, err := sessProv.MountSession(ctx, sessRef, nil)
	if err != nil {
		return nil, errors.Wrap(err, "mount session")
	}
	defer relSess()
	_ = sess

	meta := &session.SessionMetadata{
		DisplayName:         entityID,
		ProviderDisplayName: "Cloud",
		ProviderAccountId:   accountID,
		ProviderId:          "spacewave",
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
	ctx, ctxCancel := context.WithCancel(ctx)
	defer ctxCancel()

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

	privPEM, err := keypem.MarshalPrivKeyPem(sessionPriv)
	if err != nil {
		return errors.Wrap(err, "marshal session private key")
	}
	defer scrub.Scrub(privPEM)

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

	sessionPeerID, err := peer.IDFromPrivateKey(sessionPriv)
	if err != nil {
		return errors.Wrap(err, "derive session peer id")
	}

	regKey := []byte(sessionID + "/registered")
	otx, err := objStoreHandle.GetObjectStore().NewTransaction(ctx, true)
	if err != nil {
		return errors.Wrap(err, "create registration transaction")
	}
	defer otx.Discard()
	if err := otx.Set(ctx, regKey, []byte(sessionPeerID.String())); err != nil {
		return errors.Wrap(err, "write registration marker")
	}
	if err := otx.Commit(ctx); err != nil {
		return errors.Wrap(err, "commit registration marker")
	}

	return nil
}
