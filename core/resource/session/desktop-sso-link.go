//go:build !js

package resource_session

import (
	"context"

	"github.com/pkg/errors"
	provider_spacewave_handoff "github.com/s4wave/spacewave/core/provider/spacewave/handoff"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

// StartDesktopSSOLink runs the native-owned desktop SessionDetails SSO-link
// flow. The handler calls the authenticated cloud start endpoint for the
// waiting auth-session relay material, opens the system browser to the
// provider authorize URL, waits for the OAuth result on the auth-session
// WebSocket, and returns the { provider, code } pair so the UI can complete
// account linking through the existing Account.LinkSSO mutation.
func (r *SpacewaveSessionResource) StartDesktopSSOLink(
	ctx context.Context,
	req *s4wave_provider_spacewave.StartDesktopSSOLinkRequest,
) (*s4wave_provider_spacewave.StartDesktopSSOLinkResponse, error) {
	provider := req.GetSsoProvider()
	if provider != "google" && provider != "github" {
		return nil, errors.Errorf("unsupported sso provider %q", provider)
	}

	cli := r.swAcc.GetSessionClient()
	startResp, err := cli.StartDesktopSSOLink(ctx, provider)
	if err != nil {
		return nil, errors.Wrap(err, "start desktop sso link")
	}

	p := r.swAcc.GetProvider()
	result, err := provider_spacewave_handoff.WaitForDesktopSSOLink(
		ctx,
		p.GetHTTPClient(),
		p.GetEndpoint(),
		provider,
		startResp.GetWsTicket(),
		startResp.GetOpenUrl(),
	)
	if err != nil {
		return nil, errors.Wrap(err, "wait for desktop sso-link result")
	}

	return &s4wave_provider_spacewave.StartDesktopSSOLinkResponse{
		SsoProvider: result.GetProvider(),
		Code:        result.GetCode(),
	}, nil
}
