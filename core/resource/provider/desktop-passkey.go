//go:build !js

package resource_provider

import (
	"context"

	"github.com/pkg/errors"
	provider_spacewave_handoff "github.com/s4wave/spacewave/core/provider/spacewave/handoff"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

// StartDesktopPasskey starts the native desktop passkey flow on native builds.
func (s *SpacewaveProviderResource) StartDesktopPasskey(
	ctx context.Context,
	req *s4wave_provider_spacewave.StartDesktopPasskeyRequest,
) (*s4wave_provider_spacewave.StartDesktopPasskeyResponse, error) {
	_ = req
	result, err := provider_spacewave_handoff.StartPasskeyHandoff(
		ctx,
		s.provider.GetHTTPClient(),
		s.provider.GetEndpoint(),
		s.provider.GetAccountEndpoint(),
	)
	if err != nil {
		return nil, errors.Wrap(err, "start desktop passkey")
	}
	if result == nil {
		return nil, errors.New("desktop passkey returned no result")
	}

	if linked := result.GetLinked(); linked != nil {
		return &s4wave_provider_spacewave.StartDesktopPasskeyResponse{
			Result: &s4wave_provider_spacewave.StartDesktopPasskeyResponse_Linked{
				Linked: &s4wave_provider_spacewave.DesktopPasskeyLinkedResult{
					Nonce:         result.GetNonce(),
					EncryptedBlob: linked.GetEncryptedBlob(),
					PrfCapable:    linked.GetPrfCapable(),
					PrfSalt:       linked.GetPrfSalt(),
					AuthParams:    linked.GetAuthParams(),
					PinWrapped:    linked.GetPinWrapped(),
					PrfOutput:     linked.GetPrfOutput(),
				},
			},
		}, nil
	}
	if newAccount := result.GetNewAccount(); newAccount != nil {
		return &s4wave_provider_spacewave.StartDesktopPasskeyResponse{
			Result: &s4wave_provider_spacewave.StartDesktopPasskeyResponse_NewAccount{
				NewAccount: &s4wave_provider_spacewave.DesktopPasskeyNewAccountResult{
					Nonce:          result.GetNonce(),
					Username:       newAccount.GetUsername(),
					CredentialJson: newAccount.GetCredentialJson(),
					PrfCapable:     newAccount.GetPrfCapable(),
					PrfSalt:        newAccount.GetPrfSalt(),
					PrfOutput:      newAccount.GetPrfOutput(),
				},
			},
		}, nil
	}
	return nil, errors.New("desktop passkey returned no linked or new-account result")
}
