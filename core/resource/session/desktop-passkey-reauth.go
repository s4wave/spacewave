//go:build !js

package resource_session

import (
	"context"

	"github.com/pkg/errors"
	provider_spacewave_handoff "github.com/s4wave/spacewave/core/provider/spacewave/handoff"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

// StartDesktopPasskeyReauth runs the native-owned desktop passkey reauth flow
// for one specific entity keypair. The handler calls the authenticated cloud
// start endpoint, opens the system browser to the account-hosted ceremony,
// waits for the browser-authenticated result on the auth-session WebSocket,
// and returns the unwrap artifacts for the existing unlock path.
func (r *SpacewaveSessionResource) StartDesktopPasskeyReauth(
	ctx context.Context,
	req *s4wave_provider_spacewave.StartDesktopPasskeyReauthRequest,
) (*s4wave_provider_spacewave.StartDesktopPasskeyReauthResponse, error) {
	peerID := req.GetPeerId()
	if peerID == "" {
		return nil, errors.New("peer_id is required")
	}

	cli := r.swAcc.GetSessionClient()
	startResp, err := cli.StartDesktopPasskeyReauth(ctx, peerID)
	if err != nil {
		return nil, errors.Wrap(err, "start desktop passkey reauth")
	}

	p := r.swAcc.GetProvider()
	result, err := provider_spacewave_handoff.WaitForDesktopPasskeyReauth(
		ctx,
		p.GetHTTPClient(),
		p.GetEndpoint(),
		p.GetAccountEndpoint(),
		startResp.GetNonce(),
		startResp.GetWsTicket(),
		startResp.GetOpenUrl(),
	)
	if err != nil {
		return nil, errors.Wrap(err, "wait for desktop passkey reauth result")
	}
	if result == nil {
		return nil, errors.New("desktop passkey reauth returned no result")
	}

	return &s4wave_provider_spacewave.StartDesktopPasskeyReauthResponse{
		EncryptedBlob: result.GetEncryptedBlob(),
		PrfCapable:    result.GetPrfCapable(),
		PrfSalt:       result.GetPrfSalt(),
		AuthParams:    result.GetAuthParams(),
		PinWrapped:    result.GetPinWrapped(),
		PrfOutput:     result.GetPrfOutput(),
	}, nil
}
