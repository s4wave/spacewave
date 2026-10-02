//go:build !js

package resource_session

import (
	"github.com/pkg/errors"
	provider_spacewave_handoff "github.com/s4wave/spacewave/core/provider/spacewave/handoff"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// StartDesktopPasskeyReauth runs the native-owned desktop passkey reauth flow
// for one specific entity keypair. The handler calls the authenticated cloud
// start endpoint, sends the ceremony URL, opens the system browser to it,
// waits for the browser-authenticated result on the auth-session WebSocket,
// and sends the unwrap artifacts for the existing unlock path.
func (r *SpacewaveSessionResource) StartDesktopPasskeyReauth(
	req *s4wave_provider_spacewave.StartDesktopPasskeyReauthRequest,
	strm s4wave_session.SRPCSpacewaveSessionResourceService_StartDesktopPasskeyReauthStream,
) error {
	// Start the ceremony for the requested keypair.
	ctx := strm.Context()
	peerID := req.GetPeerId()
	if peerID == "" {
		return errors.New("peer_id is required")
	}
	startResp, err := r.swAcc.GetSessionClient().StartDesktopPasskeyReauth(ctx, peerID)
	if err != nil {
		return errors.Wrap(err, "start desktop passkey reauth")
	}

	// Send the ceremony URL so the caller can show it if the browser does not open.
	err = strm.Send(&s4wave_provider_spacewave.StartDesktopPasskeyReauthResponse{
		OpenUrl: startResp.GetOpenUrl(),
	})
	if err != nil {
		return err
	}

	// Open the browser and wait for its result.
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
		return errors.Wrap(err, "wait for desktop passkey reauth result")
	}
	if result == nil {
		return errors.New("desktop passkey reauth returned no result")
	}

	// Send the unwrap artifacts.
	return strm.Send(&s4wave_provider_spacewave.StartDesktopPasskeyReauthResponse{
		EncryptedBlob: result.GetEncryptedBlob(),
		PrfCapable:    result.GetPrfCapable(),
		PrfSalt:       result.GetPrfSalt(),
		AuthParams:    result.GetAuthParams(),
		PinWrapped:    result.GetPinWrapped(),
		PrfOutput:     result.GetPrfOutput(),
	})
}
