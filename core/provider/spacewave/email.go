package provider_spacewave

import (
	"context"

	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

// ListEmails returns the account's email addresses.
func (c *SessionClient) ListEmails(ctx context.Context) (*api.ListAccountEmailsResponse, error) {
	data, err := c.doGetBinary(ctx, "/api/account/emails", SeedReasonColdSeed)
	if err != nil {
		return nil, errors.Wrap(err, "list emails")
	}
	var resp api.ListAccountEmailsResponse
	if err := resp.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal list emails response")
	}
	return &resp, nil
}

// VerifyEmailCode verifies a 6-digit code for in-app email verification.
func (c *SessionClient) VerifyEmailCode(ctx context.Context, email, code string) error {
	body, err := (&api.EmailVerifyCodeRequest{Email: email, Code: code}).MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal verify-code request")
	}
	data, err := c.doPostBinary(ctx, "/api/account/email/verify-code", body, nil, SeedReasonMutation)
	if err != nil {
		return errors.Wrap(err, "verify email code")
	}
	var resp api.EmailVerifyCodeResponse
	if err := resp.UnmarshalVT(data); err != nil {
		return errors.Wrap(err, "unmarshal verify-code response")
	}
	return nil
}

// AddEmailResult is the parsed response from an add-email call.
type AddEmailResult struct {
	// RetryAfter is the number of seconds to wait before resending (0 if sent).
	RetryAfter uint32
}

// AddEmail adds an email address to the account and sends verification.
func (c *SessionClient) AddEmail(ctx context.Context, email string) (*AddEmailResult, error) {
	body, err := (&api.AddEmailRequest{Email: email}).MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal add email request")
	}
	data, err := c.doPostBinary(ctx, "/api/account/emails/add", body, nil, SeedReasonMutation)
	if err != nil {
		return nil, errors.Wrap(err, "add email")
	}
	var resp api.AddEmailResponse
	if err := resp.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal add email response")
	}
	return &AddEmailResult{
		RetryAfter: resp.GetRetryAfter(),
	}, nil
}

// RemoveEmail removes an email address from the account.
func (c *SessionClient) RemoveEmail(ctx context.Context, email string) error {
	body, err := (&api.RemoveEmailRequest{Email: email}).MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal remove email request")
	}
	data, err := c.doPostBinary(ctx, "/api/account/emails/remove", body, nil, SeedReasonMutation)
	if err != nil {
		return errors.Wrap(err, "remove email")
	}
	var resp api.RemoveEmailResponse
	if err := resp.UnmarshalVT(data); err != nil {
		return errors.Wrap(err, "unmarshal remove email response")
	}
	return nil
}

// SetPrimaryEmailResult is the parsed response from a set-primary-email call.
type SetPrimaryEmailResult struct {
	// Primary is the email that is now primary after the call.
	Primary string
}

// SetPrimaryEmail promotes an existing verified email to primary.
func (c *SessionClient) SetPrimaryEmail(ctx context.Context, email string) (*SetPrimaryEmailResult, error) {
	body, err := (&api.SetPrimaryEmailRequest{Email: email}).MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal set primary email request")
	}
	data, err := c.doPostBinary(ctx, "/api/account/emails/primary", body, nil, SeedReasonMutation)
	if err != nil {
		return nil, errors.Wrap(err, "set primary email")
	}
	var resp api.SetPrimaryEmailResponse
	if err := resp.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal set primary email response")
	}
	return &SetPrimaryEmailResult{
		Primary: resp.GetPrimary(),
	}, nil
}
