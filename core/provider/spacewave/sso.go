package provider_spacewave

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"path"

	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	provider_spacewave_handoff "github.com/s4wave/spacewave/core/provider/spacewave/handoff"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/keypem"
	"github.com/s4wave/spacewave/net/peer"
)

// SSOCodeExchange calls POST /api/auth/sso/code/exchange to authenticate via
// OAuth.
// Standalone version for use by SpacewaveProviderResource (pre-session).
func SSOCodeExchange(
	ctx context.Context,
	httpCli *http.Client,
	endpoint string,
	provider string,
	code string,
	redirectUri string,
) (*api.SSOCodeExchangeResponse, error) {
	// Post the code exchange.
	body, err := (&api.SSOCodeExchangeRequest{
		Provider:    provider,
		Code:        code,
		RedirectUri: redirectUri,
	}).MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal sso callback request")
	}
	respBody, err := postUnsigned(ctx, httpCli, endpoint, "/api/auth/sso/code/exchange", body, nil)
	if err != nil {
		return nil, err
	}

	// Decode the sign-in result.
	var result api.SSOCodeExchangeResponse
	if err := result.UnmarshalVT(respBody); err != nil {
		return nil, errors.Wrap(err, "unmarshal sso code exchange response")
	}
	return &result, nil
}

// SSONonceExchange calls POST /api/auth/sso/result/exchange to exchange a
// browser auth-session nonce for the stored OAuth sign-in result.
// Per the OQ-8 result/exchange split, this endpoint serves the sign-in flow
// only. The desktop SSO-link flow uses /api/auth/sso/link/result/exchange.
func SSONonceExchange(
	ctx context.Context,
	httpCli *http.Client,
	endpoint string,
	nonce string,
	verifier []byte,
	devicePrivateKey []byte,
) (*api.SSOCodeExchangeResponse, error) {
	// Post the nonce exchange.
	body, err := (&api.AuthSessionResultExchangeRequest{Nonce: nonce, Verifier: verifier}).MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal sso nonce exchange request")
	}
	respBody, err := postUnsigned(ctx, httpCli, endpoint, "/api/auth/sso/result/exchange", body, nil)
	if err != nil {
		return nil, err
	}

	// Decode the result and decrypt a linked entity key for this device.
	var result api.SSOCodeExchangeResponse
	if err := result.UnmarshalVT(respBody); err != nil {
		return nil, errors.Wrap(err, "unmarshal sso nonce exchange response")
	}
	if result.GetLinked() && result.GetEncryptedBlob() != "" {
		if !result.GetDeviceEncrypted() {
			return nil, errors.New("sso result entity key is not device encrypted")
		}
		plaintext, err := provider_spacewave_handoff.DecryptDeviceEncrypted(devicePrivateKey, result.GetEncryptedBlob())
		if err != nil {
			return nil, errors.Wrap(err, "decrypt browser sso result")
		}
		result.EncryptedBlob = base64.StdEncoding.EncodeToString(plaintext)
		result.DeviceEncrypted = false
	}
	return &result, nil
}

// StartDesktopSSOLink calls POST /api/auth/sso/link/start with the session
// key. The cloud returns a WebSocket ticket and the final provider authorize
// URL for the native desktop SessionDetails OAuth link flow.
func (c *SessionClient) StartDesktopSSOLink(
	ctx context.Context,
	provider string,
) (*api.DesktopSSOLinkStartResponse, error) {
	req := &api.DesktopSSOLinkStartRequest{Provider: provider}
	body, err := req.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal desktop sso link start request")
	}
	respBody, err := c.doPostBinary(ctx, "/api/auth/sso/link/start", body, nil, SeedReasonMutation)
	if err != nil {
		return nil, err
	}
	var resp api.DesktopSSOLinkStartResponse
	if err := resp.UnmarshalVT(respBody); err != nil {
		return nil, errors.Wrap(err, "parse desktop sso link start response")
	}
	return &resp, nil
}

// SSOCodeExchange calls POST /api/auth/sso/code/exchange to authenticate via
// OAuth.
// This endpoint is unauthenticated (no entity key signing).
func (c *EntityClient) SSOCodeExchange(
	ctx context.Context,
	provider string,
	code string,
	redirectUri string,
) (*api.SSOCodeExchangeResponse, error) {
	// Exchange the code without an entity signature.
	return SSOCodeExchange(ctx, c.httpCli, c.baseURL, provider, code, redirectUri)
}

// LinkSSO calls POST /api/account/:accountId/sso/link with multi-sig auth and
// returns the per-action result.
func (c *EntityClient) LinkSSO(
	ctx context.Context,
	accountID string,
	action *api.SSOLinkAction,
	entityKeys []crypto.PrivKey,
	entityPeerIDs []string,
) (*api.SsoLinkResult, error) {
	payload, err := action.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal sso link action")
	}
	resp, err := c.postMultiSig(
		ctx,
		accountID,
		path.Join("/api/account", accountID, "sso", "link"),
		api.MultiSigActionKind_MULTI_SIG_ACTION_KIND_SSO_LINK,
		payload,
		entityKeys,
		entityPeerIDs,
	)
	if err != nil {
		return nil, err
	}
	result := resp.GetSsoLink()
	if result == nil {
		return nil, errors.New("multi-sig response missing sso link result")
	}
	return result, nil
}

// LinkSSOProvider generates a keypair, optionally wraps the private key with
// PIN via age, and calls LinkSSO to register the SSO-custodied key with the
// cloud.
func (a *ProviderAccount) LinkSSOProvider(
	ctx context.Context,
	provider string,
	oauthCode string,
	redirectUri string,
	pin []byte,
	entityKeys []crypto.PrivKey,
	entityPeerIDs []string,
) error {
	// Generate a random Ed25519 keypair for the custodied key.
	privKey, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return errors.Wrap(err, "generate custodied keypair")
	}
	peerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return errors.Wrap(err, "derive peer ID")
	}

	// Serialize the private key to PEM.
	pemData, err := keypem.MarshalPrivKeyPem(privKey)
	if err != nil {
		return errors.Wrap(err, "marshal PEM")
	}

	// Build CustodiedKeyParams.
	params := &api.CustodiedKeyParams{}

	// Optional Layer 1: wrap with age scrypt if PIN provided.
	encrypted := pemData
	if len(pin) > 0 {
		encrypted, err = WrapPemWithPin(pemData, string(pin))
		if err != nil {
			return err
		}
		params.PinWrapped = true
	}

	// Serialize auth params.
	authParams, err := params.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal auth params")
	}

	// Build and send the SSOLinkAction.
	action := &api.SSOLinkAction{
		Provider:         provider,
		Code:             oauthCode,
		RedirectUri:      redirectUri,
		EncryptedPrivkey: encrypted,
		PeerId:           peerID.String(),
		AuthParams:       authParams,
	}

	// Submit the signed link action.
	_, err = a.entityCli.LinkSSO(
		ctx,
		a.accountID,
		action,
		entityKeys,
		entityPeerIDs,
	)
	return err
}

// ConfirmDesktopSSO calls POST /api/auth/sso/confirm for native desktop
// signup.
func ConfirmDesktopSSO(
	ctx context.Context,
	httpCli *http.Client,
	endpoint string,
	req *api.ConfirmSSORequest,
) (*api.ConfirmSSOResponse, error) {
	// Post the confirmation.
	body, err := req.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal desktop sso confirm request")
	}
	respBody, err := postUnsigned(ctx, httpCli, endpoint, "/api/auth/sso/confirm", body, nil)
	if err != nil {
		return nil, err
	}

	// Decode the confirmed account.
	var result api.ConfirmSSOResponse
	if err := result.UnmarshalVT(respBody); err != nil {
		return nil, errors.Wrap(err, "unmarshal desktop sso confirm response")
	}
	return &result, nil
}
