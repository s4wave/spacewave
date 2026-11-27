package resource_session

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/provider"
	resource_account "github.com/s4wave/spacewave/core/resource/account"
	"github.com/s4wave/spacewave/core/session"
	session_handoff "github.com/s4wave/spacewave/core/session/handoff"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

// ResetSession resets a PIN-locked session.
func (r *SpacewaveSessionResource) ResetSession(
	ctx context.Context,
	req *s4wave_provider_spacewave.ResetSessionRequest,
) (*s4wave_provider_spacewave.ResetSessionResponse, error) {
	// Require a credential before resetting a locked Session.
	cred := req.GetCredential()
	if cred == nil {
		return nil, errors.New("credential is required")
	}

	// Open the Session controller to resolve the selected Session.
	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, r.b, "", false, nil)
	if err != nil {
		return nil, err
	}
	defer sessionCtrlRef.Release()

	// Load the Session selected for credential-authorized reset.
	sessInfo, err := sessionCtrl.GetSessionByIdx(ctx, req.GetSessionIdx())
	if err != nil {
		return nil, err
	}
	if sessInfo == nil {
		return nil, session.ErrSessionNotFound
	}

	// Resolve the provider account reference for the selected Session.
	ref := sessInfo.GetSessionRef()
	provRef := ref.GetProviderResourceRef()

	// Open the provider account whose credential authorizes the reset.
	provAcc, provAccRef, err := provider.ExAccessProviderAccount(
		ctx, r.b,
		provRef.GetProviderId(),
		provRef.GetProviderAccountId(),
		false, nil,
	)
	if err != nil {
		return nil, err
	}
	defer provAccRef.Release()

	// Create the provider account resource used to verify the credential.
	accResource := resource_account.NewAccountResource(provAcc)
	if accResource == nil {
		return nil, errors.New("account resource not available for this provider")
	}
	defer accResource.Release()
	if _, _, err := accResource.ResolveEntityKey(ctx, cred); err != nil {
		return nil, errors.Wrap(err, "verify credential")
	}

	// Resolve the provider's Session operations before resetting its PIN.
	sessFeature, err := session.GetSessionProviderAccountFeature(ctx, provAcc)
	if err != nil {
		return nil, err
	}

	// Reset the selected Session after credential verification.
	if err := sessFeature.ResetPINSession(ctx, ref, nil); err != nil {
		return nil, err
	}

	return &s4wave_provider_spacewave.ResetSessionResponse{}, nil
}

// EnrollForHandoff registers the receiving client's independent Session key.
func (r *SpacewaveSessionResource) EnrollForHandoff(
	ctx context.Context,
	req *s4wave_provider_spacewave.EnrollForHandoffRequest,
) (*s4wave_provider_spacewave.EnrollForHandoffResponse, error) {
	// Validate the request.
	devicePubRaw := req.GetDevicePublicKey()
	if len(devicePubRaw) == 0 {
		return nil, errors.New("device_public_key is required")
	}
	nonce := req.GetSessionNonce()
	if nonce == "" {
		return nil, errors.New("session_nonce is required")
	}

	// Mount this Session to sign the registration.
	sessRef := r.getSessionRef()
	sess, relSess, err := r.swAcc.MountSession(ctx, sessRef, nil)
	if err != nil {
		return nil, errors.Wrap(err, "mount session")
	}
	defer relSess()

	// A locked Session cannot sign.
	sessionPrivKey := sess.GetPrivKey()
	if sessionPrivKey == nil {
		return nil, errors.New("session is locked")
	}

	// Derive the receiving client's peer ID from its public key.
	devicePubKey, err := crypto.UnmarshalEd25519PublicKey(devicePubRaw)
	if err != nil {
		return nil, errors.Wrap(err, "parse device public key")
	}
	receivingPeer, err := peer.IDFromPublicKey(devicePubKey)
	if err != nil {
		return nil, err
	}

	// Register the receiving Session on the account.
	registration, err := r.swAcc.LinkSession(ctx, sessionPrivKey, receivingPeer, req.GetDeviceName())
	if err != nil {
		return nil, errors.Wrap(err, "register receiving Session")
	}

	// Build HandoffCompletion and relay to AuthSessionDO.
	completion := &session_handoff.HandoffCompletion{
		SessionPeerId: registration.GetPeerId(),
		AccountId:     registration.GetAccountId(),
		EntityId:      registration.GetEntityId(),
	}
	if err := r.swAcc.RelayHandoffCompletion(ctx, nonce, completion); err != nil {
		return nil, err
	}

	return &s4wave_provider_spacewave.EnrollForHandoffResponse{
		SessionPeerId: registration.GetPeerId(),
		AccountId:     registration.GetAccountId(),
		EntityId:      registration.GetEntityId(),
	}, nil
}
