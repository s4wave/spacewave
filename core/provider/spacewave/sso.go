package provider_spacewave

import (
	"bytes"
	"context"
	"crypto/rand"
	"net/http"
	"net/url"
	"path"

	"filippo.io/age"
	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
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
	req := &api.SSOCodeExchangeRequest{
		Provider:    provider,
		Code:        code,
		RedirectUri: redirectUri,
	}
	body, err := req.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal sso callback request")
	}

	reqURL, err := url.JoinPath(endpoint, "/api/auth/sso/code/exchange")
	if err != nil {
		return nil, errors.Wrap(err, "build URL")
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(body))
	if err != nil {
		return nil, errors.Wrap(err, "create request")
	}
	httpReq.Header.Set("Content-Type", "application/octet-stream")
	httpReq.Header.Set("Accept", "application/octet-stream")

	resp, err := httpCli.Do(httpReq)
	if err != nil {
		return nil, errors.Wrap(err, "sso callback request")
	}
	defer resp.Body.Close()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, parseCloudResponseError(resp, respBody)
	}

	var result api.SSOCodeExchangeResponse
	if err := result.UnmarshalVT(respBody); err != nil {
		return nil, errors.Wrap(err, "unmarshal sso code exchange response")
	}
	return &result, nil
}

// SSONonceExchange calls POST /api/auth/sso/result/exchange to exchange a
// browser auth-session nonce for the stored OAuth sign-in result.
// Per the OQ-8 result/exchange split, this endpoint serves the sign-in flow
// only; the desktop SSO-link flow uses /api/auth/sso/link/result/exchange.
func SSONonceExchange(
	ctx context.Context,
	httpCli *http.Client,
	endpoint string,
	nonce string,
) (*api.SSOCodeExchangeResponse, error) {
	req := &api.AuthSessionResultExchangeRequest{Nonce: nonce}
	body, err := req.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal sso nonce exchange request")
	}

	reqURL, err := url.JoinPath(endpoint, "/api/auth/sso/result/exchange")
	if err != nil {
		return nil, errors.Wrap(err, "build URL")
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(body))
	if err != nil {
		return nil, errors.Wrap(err, "create request")
	}
	httpReq.Header.Set("Content-Type", "application/octet-stream")
	httpReq.Header.Set("Accept", "application/octet-stream")

	resp, err := httpCli.Do(httpReq)
	if err != nil {
		return nil, errors.Wrap(err, "sso nonce exchange request")
	}
	defer resp.Body.Close()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, parseCloudResponseError(resp, respBody)
	}

	var result api.SSOCodeExchangeResponse
	if err := result.UnmarshalVT(respBody); err != nil {
		return nil, errors.Wrap(err, "unmarshal sso nonce exchange response")
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
	req := &api.SSOCodeExchangeRequest{
		Provider:    provider,
		Code:        code,
		RedirectUri: redirectUri,
	}
	body, err := req.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal sso callback request")
	}

	reqURL, err := url.JoinPath(c.baseURL, "/api/auth/sso/code/exchange")
	if err != nil {
		return nil, errors.Wrap(err, "build URL")
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(body))
	if err != nil {
		return nil, errors.Wrap(err, "create request")
	}
	httpReq.Header.Set("Content-Type", "application/octet-stream")
	httpReq.Header.Set("Accept", "application/octet-stream")

	resp, err := c.httpCli.Do(httpReq)
	if err != nil {
		return nil, errors.Wrap(err, "sso callback request")
	}
	defer resp.Body.Close()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, parseCloudResponseError(resp, respBody)
	}

	var result api.SSOCodeExchangeResponse
	if err := result.UnmarshalVT(respBody); err != nil {
		return nil, errors.Wrap(err, "unmarshal sso code exchange response")
	}
	return &result, nil
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

// LinkSSOProvider generates a keypair, optionally wraps with PIN via age,
// and calls LinkSSO to register the SSO-custodied key with the cloud.
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

	// Serialize private key to PEM.
	pemData, err := keypem.MarshalPrivKeyPem(privKey)
	if err != nil {
		return errors.Wrap(err, "marshal PEM")
	}

	// Build CustodiedKeyParams.
	params := &api.CustodiedKeyParams{}

	// Optional Layer 1: wrap with age scrypt if PIN provided.
	encrypted := pemData
	if len(pin) > 0 {
		r, err := age.NewScryptRecipient(string(pin))
		if err != nil {
			return errors.Wrap(err, "create scrypt recipient")
		}
		r.SetWorkFactor(18)

		var buf bytes.Buffer
		w, err := age.Encrypt(&buf, r)
		if err != nil {
			return errors.Wrap(err, "create age encryptor")
		}
		if _, err := w.Write(pemData); err != nil {
			return errors.Wrap(err, "write to age encryptor")
		}
		if err := w.Close(); err != nil {
			return errors.Wrap(err, "close age encryptor")
		}
		encrypted = buf.Bytes()
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
	body, err := req.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal desktop sso confirm request")
	}

	reqURL, err := url.JoinPath(endpoint, "/api/auth/sso/confirm")
	if err != nil {
		return nil, errors.Wrap(err, "build URL")
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(body))
	if err != nil {
		return nil, errors.Wrap(err, "create request")
	}
	httpReq.Header.Set("Content-Type", "application/octet-stream")
	httpReq.Header.Set("Accept", "application/octet-stream")

	resp, err := httpCli.Do(httpReq)
	if err != nil {
		return nil, errors.Wrap(err, "desktop sso confirm request")
	}
	defer resp.Body.Close()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, parseCloudResponseError(resp, respBody)
	}

	var result api.ConfirmSSOResponse
	if err := result.UnmarshalVT(respBody); err != nil {
		return nil, errors.Wrap(err, "unmarshal desktop sso confirm response")
	}
	return &result, nil
}
