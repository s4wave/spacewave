package provider_spacewave

import (
	"bytes"
	"context"
	"net/http"
	"net/url"

	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

// RequestRecoveryEmail requests an account recovery email from the cloud.
// Unauthenticated (no signing).
func RequestRecoveryEmail(
	ctx context.Context,
	httpCli *http.Client,
	endpoint string,
	email string,
	turnstileToken string,
) error {
	body, err := (&api.RequestRecoveryEmailRequest{Email: email}).MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal recover request")
	}
	reqURL, err := url.JoinPath(endpoint, "/api/account/recover/request")
	if err != nil {
		return errors.Wrap(err, "build URL")
	}
	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		reqURL,
		bytes.NewReader(body),
	)
	if err != nil {
		return errors.Wrap(err, "create request")
	}
	httpReq.Header.Set("Content-Type", "application/octet-stream")
	if turnstileToken != "" {
		httpReq.Header.Set("X-Turnstile-Token", turnstileToken)
	}
	resp, err := httpCli.Do(httpReq)
	if err != nil {
		return errors.Wrap(err, "recover request")
	}
	defer resp.Body.Close()
	respBody, err := readResponseBody(resp)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return parseCloudResponseError(resp, respBody)
	}
	return nil
}

// RequestEmailVerificationResult is the parsed response from a verify-request call.
type RequestEmailVerificationResult struct {
	// RetryAfter is the number of seconds to wait before resending (0 if sent).
	RetryAfter uint32
}

// RequestEmailVerification requests a verification email be sent to the given
// address. Session-authenticated.
func (c *SessionClient) RequestEmailVerification(ctx context.Context, email string) (*RequestEmailVerificationResult, error) {
	body, err := (&api.RequestEmailVerificationRequest{Email: email}).MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal verify-request request")
	}
	data, err := c.doPostBinary(ctx, "/api/account/email/verify-request", body, nil, SeedReasonMutation)
	if err != nil {
		return nil, errors.Wrap(err, "request email verification")
	}
	var resp api.RequestEmailVerificationResponse
	if err := resp.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal verify-request response")
	}
	return &RequestEmailVerificationResult{
		RetryAfter: resp.GetRetryAfter(),
	}, nil
}

// RecoverVerify verifies a recovery token with the cloud.
// Unauthenticated (no signing).
func RecoverVerify(ctx context.Context, httpCli *http.Client, endpoint string, token string) (*api.RecoverVerifyResponse, error) {
	req := &api.RecoverVerifyRequest{Token: token}
	body, err := req.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal recover verify request")
	}
	reqURL, err := url.JoinPath(endpoint, "/api/account/recover/verify")
	if err != nil {
		return nil, errors.Wrap(err, "build URL")
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(body))
	if err != nil {
		return nil, errors.Wrap(err, "create request")
	}
	httpReq.Header.Set("Content-Type", "application/octet-stream")
	resp, err := httpCli.Do(httpReq)
	if err != nil {
		return nil, errors.Wrap(err, "recover verify request")
	}
	defer resp.Body.Close()
	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, parseCloudResponseError(resp, respBody)
	}
	var result api.RecoverVerifyResponse
	if err := result.UnmarshalVT(respBody); err != nil {
		return nil, errors.Wrap(err, "unmarshal recover verify response")
	}
	return &result, nil
}

// RecoverExecute completes account recovery with the cloud.
// Unauthenticated (no signing).
func RecoverExecute(ctx context.Context, httpCli *http.Client, endpoint string, req *api.RecoverExecuteRequest) error {
	body, err := req.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal recover execute request")
	}
	reqURL, err := url.JoinPath(endpoint, "/api/account/recover/execute")
	if err != nil {
		return errors.Wrap(err, "build URL")
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(body))
	if err != nil {
		return errors.Wrap(err, "create request")
	}
	httpReq.Header.Set("Content-Type", "application/octet-stream")
	resp, err := httpCli.Do(httpReq)
	if err != nil {
		return errors.Wrap(err, "recover execute request")
	}
	defer resp.Body.Close()
	respBody, err := readResponseBody(resp)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return parseCloudResponseError(resp, respBody)
	}
	var result api.RecoverExecuteResponse
	if err := result.UnmarshalVT(respBody); err != nil {
		return errors.Wrap(err, "unmarshal recover execute response")
	}
	return nil
}
