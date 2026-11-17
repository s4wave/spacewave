package provider_spacewave

import (
	"context"
	"net/http"

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
	// Post the recovery request with its Turnstile token.
	body, err := (&api.RequestRecoveryEmailRequest{Email: email}).MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal recover request")
	}
	var header http.Header
	if turnstileToken != "" {
		header = http.Header{"X-Turnstile-Token": {turnstileToken}}
	}
	_, err = postUnsigned(ctx, httpCli, endpoint, "/api/account/recover/request", body, header)
	return err
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
	// Post the recovery token.
	body, err := (&api.RecoverVerifyRequest{Token: token}).MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal recover verify request")
	}
	respBody, err := postUnsigned(ctx, httpCli, endpoint, "/api/account/recover/verify", body, nil)
	if err != nil {
		return nil, err
	}

	// Decode the verified recovery.
	var result api.RecoverVerifyResponse
	if err := result.UnmarshalVT(respBody); err != nil {
		return nil, errors.Wrap(err, "unmarshal recover verify response")
	}
	return &result, nil
}

// RecoverExecute completes account recovery with the cloud.
// Unauthenticated (no signing).
func RecoverExecute(ctx context.Context, httpCli *http.Client, endpoint string, req *api.RecoverExecuteRequest) error {
	// Post the recovery and check its acknowledgment.
	body, err := req.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal recover execute request")
	}
	respBody, err := postUnsigned(ctx, httpCli, endpoint, "/api/account/recover/execute", body, nil)
	if err != nil {
		return err
	}
	var result api.RecoverExecuteResponse
	return errors.Wrap(result.UnmarshalVT(respBody), "unmarshal recover execute response")
}
