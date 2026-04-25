package provider_spacewave

import (
	"bytes"
	"context"
	"net/http"
	"net/url"

	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

// ConfirmPasskeySignupRequest is the browser-owned passkey signup payload.
type ConfirmPasskeySignupRequest struct {
	CredentialJSON   string
	Username         string
	WrappedEntityKey string
	EntityPeerID     string
	SessionPeerID    string
	PinWrapped       bool
	PrfCapable       bool
	PrfSalt          string
	AuthParams       string
}

// ConfirmDesktopPasskeyRequest is the native desktop passkey confirm payload.
type ConfirmDesktopPasskeyRequest struct {
	Nonce            string
	Username         string
	CredentialJSON   string
	WrappedEntityKey string
	EntityPeerID     string
	SessionPeerID    string
	PinWrapped       bool
	PrfCapable       bool
	PrfSalt          string
	AuthParams       string
}

// buildPasskeyConfirmRequest builds a PasskeyConfirmRequest proto from the
// per-flow Go request struct fields.
func buildPasskeyConfirmRequest(
	credentialJSON, username, wrappedEntityKey, entityPeerID, sessionPeerID string,
	pinWrapped, prfCapable bool,
	prfSalt, authParams string,
) *api.PasskeyConfirmRequest {
	return &api.PasskeyConfirmRequest{
		CredentialJson:   credentialJSON,
		Username:         username,
		WrappedEntityKey: wrappedEntityKey,
		EntityPeerId:     entityPeerID,
		SessionPeerId:    sessionPeerID,
		PinWrapped:       pinWrapped,
		PrfCapable:       prfCapable,
		PrfSalt:          prfSalt,
		AuthParams:       authParams,
	}
}

// postPasskeyConfirm posts a PasskeyConfirmRequest and returns the response body.
func postPasskeyConfirm(
	ctx context.Context,
	httpCli *http.Client,
	endpoint string,
	req *api.PasskeyConfirmRequest,
) ([]byte, error) {
	body, err := req.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal passkey confirm request")
	}
	reqURL, err := url.JoinPath(endpoint, "/api/auth/passkey/confirm")
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
	httpReq.Header.Set("Content-Type", "application/octet-stream")
	resp, err := httpCli.Do(httpReq)
	if err != nil {
		return nil, errors.Wrap(err, "passkey confirm request")
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

// ConfirmPasskeySignup posts the web passkey signup confirm request.
func ConfirmPasskeySignup(
	ctx context.Context,
	httpCli *http.Client,
	endpoint string,
	req *ConfirmPasskeySignupRequest,
) error {
	protoReq := buildPasskeyConfirmRequest(
		req.CredentialJSON,
		req.Username,
		req.WrappedEntityKey,
		req.EntityPeerID,
		req.SessionPeerID,
		req.PinWrapped,
		req.PrfCapable,
		req.PrfSalt,
		req.AuthParams,
	)
	if _, err := postPasskeyConfirm(ctx, httpCli, endpoint, protoReq); err != nil {
		return err
	}
	return nil
}

// ConfirmDesktopPasskey posts the native desktop passkey confirm request.
func ConfirmDesktopPasskey(
	ctx context.Context,
	httpCli *http.Client,
	endpoint string,
	req *ConfirmDesktopPasskeyRequest,
) (*ConfirmDesktopPasskeyResponse, error) {
	protoReq := buildPasskeyConfirmRequest(
		req.CredentialJSON,
		req.Username,
		req.WrappedEntityKey,
		req.EntityPeerID,
		req.SessionPeerID,
		req.PinWrapped,
		req.PrfCapable,
		req.PrfSalt,
		req.AuthParams,
	)
	respBody, err := postPasskeyConfirm(ctx, httpCli, endpoint, protoReq)
	if err != nil {
		return nil, err
	}
	respProto := &api.PasskeyConfirmResponse{}
	if err := respProto.UnmarshalVT(respBody); err != nil {
		return nil, errors.Wrap(err, "unmarshal desktop passkey confirm response")
	}
	return &ConfirmDesktopPasskeyResponse{
		AccountID:     respProto.GetAccountId(),
		SessionPeerID: respProto.GetSessionPeerId(),
	}, nil
}
