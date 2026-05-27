package provider_spacewave

import (
	"bytes"
	"context"
	"net/http"
	"net/url"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/provider/spacewave/passkeyconfirm"
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

// postPasskeyConfirm posts a PasskeyConfirmRequest and returns the response body.
func postPasskeyConfirm(
	ctx context.Context,
	httpCli *http.Client,
	endpoint string,
	req *passkeyconfirm.Request,
) ([]byte, error) {
	body, err := passkeyconfirm.MarshalRequest(req)
	if err != nil {
		return nil, errors.Wrap(err, "marshal passkey confirm request")
	}
	reqURL, err := url.JoinPath(endpoint, passkeyconfirm.Path)
	if err != nil {
		return nil, errors.Wrap(err, "build URL")
	}
	httpReq, err := http.NewRequestWithContext(
		ctx,
		passkeyconfirm.Method,
		reqURL,
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, errors.Wrap(err, "create request")
	}
	httpReq.Header.Set("Content-Type", passkeyconfirm.ContentType)
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
	confirmReq := &passkeyconfirm.Request{
		CredentialJSON:   req.CredentialJSON,
		Username:         req.Username,
		WrappedEntityKey: req.WrappedEntityKey,
		EntityPeerID:     req.EntityPeerID,
		SessionPeerID:    req.SessionPeerID,
		PinWrapped:       req.PinWrapped,
		PrfCapable:       req.PrfCapable,
		PrfSalt:          req.PrfSalt,
		AuthParams:       req.AuthParams,
	}
	if _, err := postPasskeyConfirm(ctx, httpCli, endpoint, confirmReq); err != nil {
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
	confirmReq := &passkeyconfirm.Request{
		CredentialJSON:   req.CredentialJSON,
		Username:         req.Username,
		WrappedEntityKey: req.WrappedEntityKey,
		EntityPeerID:     req.EntityPeerID,
		SessionPeerID:    req.SessionPeerID,
		PinWrapped:       req.PinWrapped,
		PrfCapable:       req.PrfCapable,
		PrfSalt:          req.PrfSalt,
		AuthParams:       req.AuthParams,
	}
	respBody, err := postPasskeyConfirm(ctx, httpCli, endpoint, confirmReq)
	if err != nil {
		return nil, err
	}
	parsed, err := passkeyconfirm.ParseDesktopResponse(respBody)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal desktop passkey confirm response")
	}
	return &ConfirmDesktopPasskeyResponse{
		AccountID:     parsed.AccountID,
		SessionPeerID: parsed.SessionPeerID,
	}, nil
}
