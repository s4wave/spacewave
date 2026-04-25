//go:build !js

package resource_account

import (
	"context"

	"github.com/pkg/errors"
	provider_spacewave_handoff "github.com/s4wave/spacewave/core/provider/spacewave/handoff"
	s4wave_account "github.com/s4wave/spacewave/sdk/account"
)

// StartDesktopPasskeyRegisterHandoff runs the native desktop add-passkey
// browser handoff and returns the browser-collected register artifacts.
func (r *AccountResource) StartDesktopPasskeyRegisterHandoff(
	ctx context.Context,
	req *s4wave_account.StartDesktopPasskeyRegisterHandoffRequest,
) (*s4wave_account.StartDesktopPasskeyRegisterHandoffResponse, error) {
	_ = req
	cli := r.account.GetSessionClient()
	startResp, err := cli.StartDesktopPasskeyRegister(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "start desktop passkey register")
	}

	p := r.account.GetProvider()
	result, err := provider_spacewave_handoff.WaitForDesktopPasskeyRegister(
		ctx,
		p.GetHTTPClient(),
		p.GetEndpoint(),
		p.GetAccountEndpoint(),
		startResp.GetNonce(),
		startResp.GetWsTicket(),
		startResp.GetOpenUrl(),
	)
	if err != nil {
		return nil, errors.Wrap(err, "wait for desktop passkey register result")
	}
	if result == nil {
		return nil, errors.New("desktop passkey register returned no result")
	}

	return &s4wave_account.StartDesktopPasskeyRegisterHandoffResponse{
		Username:       result.GetUsername(),
		CredentialJson: result.GetCredentialJson(),
		PrfCapable:     result.GetPrfCapable(),
		PrfSalt:        result.GetPrfSalt(),
		PrfOutput:      result.GetPrfOutput(),
	}, nil
}
