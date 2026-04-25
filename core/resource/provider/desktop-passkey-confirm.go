//go:build !js

package resource_provider

import (
	"context"

	"github.com/pkg/errors"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

// ConfirmDesktopPasskey completes native desktop passkey account creation.
func (s *SpacewaveProviderResource) ConfirmDesktopPasskey(
	ctx context.Context,
	req *s4wave_provider_spacewave.ConfirmDesktopPasskeyRequest,
) (*s4wave_provider_spacewave.ConfirmDesktopPasskeyResponse, error) {
	if req.GetNonce() == "" {
		return nil, errors.New("nonce is required")
	}
	if req.GetUsername() == "" {
		return nil, errors.New("username is required")
	}
	if req.GetCredentialJson() == "" {
		return nil, errors.New("credential_json is required")
	}
	if req.GetWrappedEntityKey() == "" {
		return nil, errors.New("wrapped_entity_key is required")
	}
	if req.GetEntityPeerId() == "" {
		return nil, errors.New("entity_peer_id is required")
	}
	if req.GetSessionPeerId() == "" {
		return nil, errors.New("session_peer_id is required")
	}
	if req.GetPrfCapable() && req.GetPrfSalt() == "" {
		return nil, errors.New("prf_salt is required for prf-capable passkeys")
	}
	if req.GetPrfCapable() && req.GetAuthParams() == "" {
		return nil, errors.New("auth_params is required for prf-capable passkeys")
	}

	resp, err := provider_spacewave.ConfirmDesktopPasskey(
		ctx,
		s.provider.GetHTTPClient(),
		s.provider.GetEndpoint(),
		&provider_spacewave.ConfirmDesktopPasskeyRequest{
			Nonce:            req.GetNonce(),
			Username:         req.GetUsername(),
			CredentialJSON:   req.GetCredentialJson(),
			WrappedEntityKey: req.GetWrappedEntityKey(),
			EntityPeerID:     req.GetEntityPeerId(),
			SessionPeerID:    req.GetSessionPeerId(),
			PinWrapped:       req.GetPinWrapped(),
			PrfCapable:       req.GetPrfCapable(),
			PrfSalt:          req.GetPrfSalt(),
			AuthParams:       req.GetAuthParams(),
		},
	)
	if err != nil {
		return nil, errors.Wrap(err, "confirm desktop passkey")
	}
	return &s4wave_provider_spacewave.ConfirmDesktopPasskeyResponse{
		AccountId:     resp.AccountID,
		SessionPeerId: resp.SessionPeerID,
	}, nil
}
