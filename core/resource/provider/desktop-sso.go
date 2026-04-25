//go:build !js

package resource_provider

import (
	"context"

	"github.com/pkg/errors"
	provider_spacewave_handoff "github.com/s4wave/spacewave/core/provider/spacewave/handoff"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

// StartDesktopSSO starts the native desktop SSO flow on native builds.
func (s *SpacewaveProviderResource) StartDesktopSSO(
	ctx context.Context,
	req *s4wave_provider_spacewave.StartDesktopSSORequest,
) (*s4wave_provider_spacewave.StartDesktopSSOResponse, error) {
	provider := req.GetSsoProvider()
	if provider == "" {
		return nil, errors.New("sso_provider is required")
	}

	result, pemPrivateKey, nonce, err := provider_spacewave_handoff.StartSSOHandoff(
		ctx,
		s.provider.GetHTTPClient(),
		s.provider.GetEndpoint(),
		provider,
	)
	if err != nil {
		return nil, errors.Wrap(err, "start desktop sso")
	}
	if result == nil {
		return nil, errors.New("desktop sso returned no result")
	}

	if result.Linked {
		if len(pemPrivateKey) == 0 {
			return nil, errors.New("desktop sso did not return a linked entity key")
		}
		return &s4wave_provider_spacewave.StartDesktopSSOResponse{
			Result: &s4wave_provider_spacewave.StartDesktopSSOResponse_Linked{
				Linked: &s4wave_provider_spacewave.DesktopSSOLinkedResult{
					Nonce:         nonce,
					AccountId:     result.AccountID,
					EntityId:      result.EntityID,
					PemPrivateKey: pemPrivateKey,
					PinWrapped:    result.PinWrapped,
					Username:      result.Username,
				},
			},
		}, nil
	}

	return &s4wave_provider_spacewave.StartDesktopSSOResponse{
		Result: &s4wave_provider_spacewave.StartDesktopSSOResponse_NewAccount{
			NewAccount: &s4wave_provider_spacewave.DesktopSSONewAccountResult{
				Nonce:       nonce,
				SsoProvider: result.Provider,
				Email:       result.Email,
			},
		},
	}, nil
}
