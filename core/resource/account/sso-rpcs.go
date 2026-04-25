package resource_account

import (
	"context"

	"github.com/pkg/errors"
	bifrost_crypto "github.com/s4wave/spacewave/net/crypto"
	s4wave_account "github.com/s4wave/spacewave/sdk/account"
)

// SSOCodeExchange exchanges an OAuth authorization code for account info.
func (r *AccountResource) SSOCodeExchange(
	ctx context.Context,
	req *s4wave_account.SSOCodeExchangeRequest,
) (*s4wave_account.SSOCodeExchangeResponse, error) {
	provider := req.GetProvider()
	code := req.GetCode()
	redirectUri := req.GetRedirectUri()
	if provider == "" || code == "" {
		return nil, errors.New("provider and code are required")
	}

	result, err := r.account.GetEntityClient().SSOCodeExchange(ctx, provider, code, redirectUri)
	if err != nil {
		return nil, errors.Wrap(err, "sso callback")
	}

	return &s4wave_account.SSOCodeExchangeResponse{
		Linked:        result.GetLinked(),
		AccountId:     result.GetAccountId(),
		EntityId:      result.GetEntityId(),
		EncryptedBlob: result.GetEncryptedBlob(),
		PinWrapped:    result.GetPinWrapped(),
		AuthParams:    result.GetAuthParams(),
		SsoProvider:   result.GetProvider(),
		Email:         result.GetEmail(),
	}, nil
}

// LinkSSO generates a custodied keypair, wraps with optional PIN, and
// registers it with the cloud via multi-sig.
func (r *AccountResource) LinkSSO(
	ctx context.Context,
	req *s4wave_account.LinkSSORequest,
) (*s4wave_account.LinkSSOResponse, error) {
	provider := req.GetProvider()
	code := req.GetCode()
	redirectUri := req.GetRedirectUri()
	if provider == "" || code == "" {
		return nil, errors.New("provider and code are required")
	}

	cred := req.GetCredential()

	// If credential provided, resolve entity key and use LinkSSOProvider.
	if cred != nil {
		entityPriv, entityPeerID, err := r.ResolveEntityKey(ctx, cred)
		if err != nil {
			return nil, err
		}
		err = r.account.LinkSSOProvider(
			ctx,
			provider,
			code,
			redirectUri,
			req.GetPin(),
			[]bifrost_crypto.PrivKey{entityPriv},
			[]string{entityPeerID.String()},
		)
		if err != nil {
			return nil, errors.Wrap(err, "link sso")
		}
		r.account.BumpLocalEpoch()
		return &s4wave_account.LinkSSOResponse{}, nil
	}

	// Fall back to tracker-signed path.
	trackerKeys, trackerPeerIDs := r.account.GetEntityKeypairTracker().
		GetUnlockedKeysAndPeerIDs()
	if len(trackerKeys) == 0 {
		return nil, errors.New("no credentials provided and no keypairs unlocked")
	}

	err := r.account.LinkSSOProvider(
		ctx,
		provider,
		code,
		redirectUri,
		req.GetPin(),
		trackerKeys,
		trackerPeerIDs,
	)
	if err != nil {
		return nil, errors.Wrap(err, "link sso")
	}
	r.account.BumpLocalEpoch()
	return &s4wave_account.LinkSSOResponse{}, nil
}
