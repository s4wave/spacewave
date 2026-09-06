package provider_spacewave

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/pkg/errors"
	provider "github.com/s4wave/spacewave/core/provider"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// RegisterDeviceSession authorizes an independently held Device key with an
// account entity key and mounts the registered DEVICE Session. The caller
// persists both keys before calling; retries reuse the Device's local resource
// identity. Registration survives a failed local mount so retry can finish it.
// This operation does not grant Space membership or create a World Device.
func (p *Provider) RegisterDeviceSession(
	ctx context.Context,
	entityPriv crypto.PrivKey,
	devicePriv crypto.PrivKey,
	label string,
	sessionCtrl session.SessionController,
) (*session.SessionListEntry, error) {
	// Require independently held account and Device identities.
	if entityPriv == nil || devicePriv == nil {
		return nil, errors.New("account and device private keys are required")
	}
	if sessionCtrl == nil {
		return nil, errors.New("session controller is required")
	}
	entityPeer, err := peer.IDFromPrivateKey(entityPriv)
	if err != nil {
		return nil, errors.Wrap(err, "derive account peer")
	}
	devicePeer, err := peer.IDFromPrivateKey(devicePriv)
	if err != nil {
		return nil, errors.Wrap(err, "derive device peer")
	}
	if entityPeer == devicePeer {
		return nil, errors.New("device key must differ from account key")
	}
	if label == "" {
		label = "Spacewave Device"
	}

	// The signed registration derives the account from its current credential.
	client := NewEntityClientDirect(p.GetHTTPClient(), p.GetEndpoint(), p.GetSigningEnvPrefix(), entityPriv, entityPeer)
	registered, err := client.RegisterSessionWithRequest(ctx, &api.RegisterSessionRequest{
		SessionPeerId: devicePeer.String(),
		Type:          session.SessionType_SESSION_TYPE_DEVICE,
		Label:         label,
	}, "")
	if err != nil {
		return nil, errors.Wrap(err, "register device session")
	}
	if registered.GetAccountId() == "" {
		return nil, errors.New("device registration returned no account")
	}

	// Match the stable local identity used by SpaceLink Device enrollment.
	digest := sha256.Sum256([]byte(devicePeer))
	sessionID := "device-" + hex.EncodeToString(digest[:16])
	return p.MountLinkedDeviceSession(ctx, registered.GetAccountId(), sessionID, label, devicePriv, sessionCtrl)
}

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
	// Validate the approved registration before touching local session storage.
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

	// Retain the account while seeding and mounting its Device Session.
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

	// Establish the mounted Session before publishing its local list entry.
	_, relSess, err := sessProv.MountSession(ctx, sessRef, nil)
	if err != nil {
		return nil, errors.Wrap(err, "mount session")
	}
	defer relSess()

	// Keep Device identity in the Session metadata used by local consumers.
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
