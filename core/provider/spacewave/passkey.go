package provider_spacewave

import (
	"bytes"
	"context"
	"net/http"
	"net/url"

	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

// StartDesktopPasskeyRegister calls POST /api/auth/passkey/register/start with
// the session key. The cloud returns the auth-session nonce, WebSocket ticket,
// and account-hosted browser URL for the native desktop add-passkey flow.
func (c *SessionClient) StartDesktopPasskeyRegister(
	ctx context.Context,
) (*api.DesktopPasskeyStartResponse, error) {
	req := &api.DesktopPasskeyStartRequest{}
	body, err := req.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal desktop passkey register start request")
	}
	data, err := c.doPostBinary(
		ctx,
		"/api/auth/passkey/register/start",
		body,
		nil,
		SeedReasonMutation,
	)
	if err != nil {
		return nil, errors.Wrap(err, "desktop passkey register start")
	}
	resp := &api.DesktopPasskeyStartResponse{}
	if err := resp.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal desktop passkey register start response")
	}
	return resp, nil
}

// StartDesktopPasskeyReauth calls POST /api/auth/passkey/reauth/start with the
// session key and target entity peer ID. The cloud returns the auth-session
// nonce, WebSocket ticket, and account-hosted browser URL for the native
// desktop passkey reauth flow.
func (c *SessionClient) StartDesktopPasskeyReauth(
	ctx context.Context,
	peerID string,
) (*api.DesktopPasskeyStartResponse, error) {
	req := &api.StartDesktopPasskeyReauthRequest{
		PeerId: peerID,
	}
	body, err := req.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal desktop passkey reauth start request")
	}
	data, err := c.doPostBinary(
		ctx,
		"/api/auth/passkey/reauth/start",
		body,
		nil,
		SeedReasonMutation,
	)
	if err != nil {
		return nil, errors.Wrap(err, "desktop passkey reauth start")
	}
	resp := &api.DesktopPasskeyStartResponse{}
	if err := resp.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal desktop passkey reauth start response")
	}
	return resp, nil
}

// PasskeyRegisterOptions fetches WebAuthn registration options from the cloud.
// Session-authenticated.
func (c *SessionClient) PasskeyRegisterOptions(ctx context.Context) (string, error) {
	body, err := (&api.RegisterPasskeyOptionsRequest{}).MarshalVT()
	if err != nil {
		return "", errors.Wrap(err, "marshal passkey register options request")
	}
	data, err := c.doPostBinary(
		ctx,
		"/api/auth/passkey/register/options",
		body,
		nil,
		SeedReasonMutation,
	)
	if err != nil {
		return "", errors.Wrap(err, "passkey register options")
	}
	resp := &api.PasskeyOptionsResponse{}
	if err := resp.UnmarshalVT(data); err != nil {
		return "", errors.Wrap(err, "unmarshal passkey register options")
	}
	return resp.GetOptions(), nil
}

// PasskeyRegisterVerify verifies a WebAuthn registration credential with the cloud.
// Session-authenticated.
func (c *SessionClient) PasskeyRegisterVerify(
	ctx context.Context,
	credentialJSON string,
	prfCapable bool,
	encryptedPrivkey string,
	peerID string,
	authParams string,
	prfSalt string,
) (string, error) {
	req := &api.PasskeyRegisterVerifyRequest{
		CredentialJson:   string(credentialJSON),
		PrfCapable:       prfCapable,
		EncryptedPrivkey: encryptedPrivkey,
		PeerId:           peerID,
		AuthParams:       authParams,
		PrfSalt:          prfSalt,
	}
	body, err := req.MarshalVT()
	if err != nil {
		return "", errors.Wrap(err, "marshal passkey register verify request")
	}
	data, err := c.doPostBinary(
		ctx,
		"/api/auth/passkey/register/verify",
		body,
		nil,
		SeedReasonMutation,
	)
	if err != nil {
		return "", errors.Wrap(err, "passkey register verify")
	}
	resp := &api.PasskeyRegisterVerifyResponse{}
	if err := resp.UnmarshalVT(data); err != nil {
		return "", errors.Wrap(err, "unmarshal passkey register verify response")
	}
	return resp.GetCredentialId(), nil
}

func doPasskeyPostJSON(
	ctx context.Context,
	httpCli *http.Client,
	endpoint string,
	path string,
	body []byte,
) ([]byte, error) {
	return doPasskeyPost(ctx, httpCli, endpoint, path, body, "application/json")
}

func doPasskeyPostBinary(
	ctx context.Context,
	httpCli *http.Client,
	endpoint string,
	path string,
	body []byte,
) ([]byte, error) {
	return doPasskeyPost(
		ctx,
		httpCli,
		endpoint,
		path,
		body,
		"application/octet-stream",
	)
}

func doPasskeyPost(
	ctx context.Context,
	httpCli *http.Client,
	endpoint string,
	path string,
	body []byte,
	contentType string,
) ([]byte, error) {
	reqURL, err := url.JoinPath(endpoint, path)
	if err != nil {
		return nil, errors.Wrap(err, "build URL")
	}
	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		reqURL,
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, errors.Wrap(err, "create request")
	}
	httpReq.Header.Set("Content-Type", contentType)
	resp, err := httpCli.Do(httpReq)
	if err != nil {
		return nil, errors.Wrap(err, "request")
	}
	defer resp.Body.Close()
	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, parseCloudResponseError(resp, respBody)
	}
	return respBody, nil
}

// PasskeyCheckUsername acknowledges the opaque first passkey step.
func PasskeyCheckUsername(
	ctx context.Context,
	httpCli *http.Client,
	endpoint string,
	username string,
) (bool, error) {
	req := &api.PasskeyCheckUsernameRequest{Username: username}
	body, err := req.MarshalVT()
	if err != nil {
		return false, errors.Wrap(err, "marshal passkey check username request")
	}
	data, err := doPasskeyPostBinary(
		ctx,
		httpCli,
		endpoint,
		"/api/auth/passkey/check-username",
		body,
	)
	if err != nil {
		return false, errors.Wrap(err, "passkey check username")
	}
	resp := &api.PasskeyCheckUsernameResponse{}
	if err := resp.UnmarshalVT(data); err != nil {
		return false, errors.Wrap(err, "unmarshal passkey check username response")
	}
	return resp.GetOk(), nil
}

// PasskeyRegisterChallenge fetches WebAuthn registration options for new-account signup.
func PasskeyRegisterChallenge(
	ctx context.Context,
	httpCli *http.Client,
	endpoint string,
	username string,
) (string, error) {
	req := &api.PasskeyRegisterChallengeRequest{Username: username}
	body, err := req.MarshalVT()
	if err != nil {
		return "", errors.Wrap(err, "marshal passkey register challenge request")
	}
	data, err := doPasskeyPostBinary(
		ctx,
		httpCli,
		endpoint,
		"/api/auth/passkey/register/challenge",
		body,
	)
	if err != nil {
		return "", errors.Wrap(err, "passkey register challenge")
	}
	resp := &api.PasskeyOptionsResponse{}
	if err := resp.UnmarshalVT(data); err != nil {
		return "", errors.Wrap(err, "unmarshal passkey register challenge")
	}
	return resp.GetOptions(), nil
}

// PasskeyAuthOptions fetches WebAuthn authentication options from the cloud.
// Unauthenticated (no signing).
func PasskeyAuthOptions(
	ctx context.Context,
	httpCli *http.Client,
	endpoint string,
	username string,
) (string, error) {
	req := &api.PasskeyAuthOptionsRequest{Username: username}
	body, err := req.MarshalVT()
	if err != nil {
		return "", errors.Wrap(err, "marshal passkey auth options request")
	}
	respBody, err := doPasskeyPostBinary(
		ctx,
		httpCli,
		endpoint,
		"/api/auth/passkey/auth/options",
		body,
	)
	if err != nil {
		return "", errors.Wrap(err, "passkey auth options")
	}
	result := &api.PasskeyOptionsResponse{}
	if err := result.UnmarshalVT(respBody); err != nil {
		return "", errors.Wrap(err, "unmarshal passkey auth options")
	}
	return result.GetOptions(), nil
}

// PasskeyAuthVerify verifies a WebAuthn authentication credential with the cloud.
// Unauthenticated (no signing).
func PasskeyAuthVerify(ctx context.Context, httpCli *http.Client, endpoint string, credentialJSON string) (*api.PasskeyAuthVerifyResponse, error) {
	req := &api.PasskeyAuthVerifyRequest{
		CredentialJson: string(credentialJSON),
	}
	body, err := req.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal passkey auth verify request")
	}
	respBody, err := doPasskeyPostBinary(
		ctx,
		httpCli,
		endpoint,
		"/api/auth/passkey/auth/verify",
		body,
	)
	if err != nil {
		return nil, errors.Wrap(err, "passkey auth verify")
	}
	var result api.PasskeyAuthVerifyResponse
	if err := result.UnmarshalVT(respBody); err != nil {
		return nil, errors.Wrap(err, "unmarshal passkey auth verify response")
	}
	return &result, nil
}

// RelayDesktopPasskey relays a browser ceremony result back to native alpha.
func RelayDesktopPasskey(
	ctx context.Context,
	httpCli *http.Client,
	endpoint string,
	req *api.DesktopPasskeyRelayResult,
) error {
	body, err := req.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal desktop passkey relay request")
	}
	if _, err := doPasskeyPostBinary(
		ctx,
		httpCli,
		endpoint,
		"/api/auth/passkey/desktop/relay",
		body,
	); err != nil {
		return errors.Wrap(err, "desktop passkey relay")
	}
	return nil
}
