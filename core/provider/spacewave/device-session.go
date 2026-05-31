package provider_spacewave

import (
	"context"
	"time"

	"github.com/pkg/errors"
	provider "github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// MountLinkedDeviceSession mounts a SpaceLink-approved DEVICE session without
// re-registering it as a USER session.
func (p *Provider) MountLinkedDeviceSession(
	ctx context.Context,
	accountID string,
	sessionID string,
	label string,
	sessionPriv crypto.PrivKey,
	sessionCtrl session.SessionController,
) (*session.SessionListEntry, error) {
	if accountID == "" {
		return nil, errors.New("account id is required")
	}
	if err := provider.ValidateResourceID(sessionID); err != nil {
		return nil, errors.Wrap(err, "session id")
	}
	if sessionPriv == nil {
		return nil, errors.New("session private key is required")
	}
	if label == "" {
		label = "Spacewave Device"
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
			Id:                sessionID,
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

	sessionPeerID, err := peer.IDFromPrivateKey(sessionPriv)
	if err != nil {
		return nil, errors.Wrap(err, "derive session peer id")
	}
	meta := &session.SessionMetadata{
		DisplayName:         label,
		ProviderDisplayName: "Cloud",
		ProviderAccountId:   accountID,
		ProviderId:          "spacewave",
		CreatedAt:           time.Now().UnixMilli(),
		CloudAccountId:      accountID,
		CloudEntityId:       sessionPeerID.String(),
	}
	listEntry, err := sessionCtrl.RegisterSession(ctx, sessRef, meta)
	if err != nil {
		return nil, errors.Wrap(err, "register session")
	}

	return listEntry, nil
}
