package resource_provider

import (
	"context"

	"github.com/pkg/errors"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

// ConfirmDesktopSSO completes native desktop SSO account creation.
func (s *SpacewaveProviderResource) ConfirmDesktopSSO(
	ctx context.Context,
	req *s4wave_provider_spacewave.ConfirmDesktopSSORequest,
) (*s4wave_provider_spacewave.ConfirmDesktopSSOResponse, error) {
	if req.GetNonce() == "" {
		return nil, errors.New("nonce is required")
	}
	if req.GetUsername() == "" {
		return nil, errors.New("username is required")
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

	resp, err := provider_spacewave.ConfirmDesktopSSO(
		ctx,
		s.provider.GetHTTPClient(),
		s.provider.GetEndpoint(),
		&api.ConfirmSSORequest{
			Nonce:            req.GetNonce(),
			Username:         req.GetUsername(),
			WrappedEntityKey: req.GetWrappedEntityKey(),
			EntityPeerId:     req.GetEntityPeerId(),
			SessionPeerId:    req.GetSessionPeerId(),
			PinWrapped:       req.GetPinWrapped(),
		},
	)
	if err != nil {
		return nil, errors.Wrap(err, "confirm desktop sso")
	}
	return &s4wave_provider_spacewave.ConfirmDesktopSSOResponse{
		AccountId:     resp.GetAccountId(),
		SessionPeerId: resp.GetSessionPeerId(),
	}, nil
}
