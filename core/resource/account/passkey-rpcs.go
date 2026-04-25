package resource_account

import (
	"context"

	"github.com/pkg/errors"
	s4wave_account "github.com/s4wave/spacewave/sdk/account"
)

// StartDesktopPasskeyRegister starts the desktop add-passkey flow.
func (r *AccountResource) StartDesktopPasskeyRegister(
	ctx context.Context,
	req *s4wave_account.StartDesktopPasskeyRegisterRequest,
) (*s4wave_account.StartDesktopPasskeyRegisterResponse, error) {
	cli := r.account.GetSessionClient()
	startResp, err := cli.StartDesktopPasskeyRegister(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "start desktop passkey register")
	}
	return &s4wave_account.StartDesktopPasskeyRegisterResponse{
		Nonce:    startResp.GetNonce(),
		WsTicket: startResp.GetWsTicket(),
		OpenUrl:  startResp.GetOpenUrl(),
	}, nil
}

// PasskeyRegisterOptions fetches WebAuthn registration options from the cloud.
func (r *AccountResource) PasskeyRegisterOptions(
	ctx context.Context,
	req *s4wave_account.PasskeyRegisterOptionsRequest,
) (*s4wave_account.PasskeyRegisterOptionsResponse, error) {
	cli := r.account.GetSessionClient()
	optionsJSON, err := cli.PasskeyRegisterOptions(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "passkey register options")
	}
	return &s4wave_account.PasskeyRegisterOptionsResponse{
		OptionsJson: optionsJSON,
	}, nil
}

// PasskeyRegisterVerify verifies a WebAuthn registration credential and
// registers the passkey with the cloud.
func (r *AccountResource) PasskeyRegisterVerify(
	ctx context.Context,
	req *s4wave_account.PasskeyRegisterVerifyRequest,
) (*s4wave_account.PasskeyRegisterVerifyResponse, error) {
	cli := r.account.GetSessionClient()
	credID, err := cli.PasskeyRegisterVerify(
		ctx,
		req.GetCredentialJson(),
		req.GetPrfCapable(),
		req.GetEncryptedPrivkey(),
		req.GetPeerId(),
		req.GetAuthParams(),
		req.GetPrfSalt(),
	)
	if err != nil {
		return nil, errors.Wrap(err, "passkey register verify")
	}
	r.account.BumpLocalEpoch()
	return &s4wave_account.PasskeyRegisterVerifyResponse{
		CredentialId: credID,
	}, nil
}
