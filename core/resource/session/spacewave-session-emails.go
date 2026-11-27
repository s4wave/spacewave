package resource_session

import (
	"context"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// WatchEmails streams the account's email list, emitting on changes.
func (r *SpacewaveSessionResource) WatchEmails(
	req *s4wave_provider_spacewave.WatchEmailsRequest,
	strm s4wave_session.SRPCSpacewaveSessionResourceService_WatchEmailsStream,
) error {
	ctx := strm.Context()
	accountBcast := r.swAcc.GetAccountBroadcast()
	var prev *s4wave_provider_spacewave.WatchEmailsResponse
	for {
		var ch <-chan struct{}
		var cached []*api.AccountEmailInfo
		var valid bool
		accountBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			ch = getWaitCh()
			cached, valid = r.swAcc.EmailsSnapshot()
		})

		if valid {
			emails := make([]*s4wave_provider_spacewave.EmailInfo, len(cached))
			for i, e := range cached {
				emails[i] = &s4wave_provider_spacewave.EmailInfo{
					Email:    e.GetEmail(),
					Verified: e.GetVerified(),
					Source:   e.GetSource(),
					Primary:  e.GetPrimary(),
				}
			}
			resp := &s4wave_provider_spacewave.WatchEmailsResponse{Emails: emails}
			if prev == nil || !resp.EqualVT(prev) {
				if err := strm.Send(resp); err != nil {
					return err
				}
				prev = resp
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ch:
		}
	}
}

// SendVerificationEmail sends a verification email to the given address.
func (r *SpacewaveSessionResource) SendVerificationEmail(
	ctx context.Context,
	req *s4wave_provider_spacewave.SendVerificationEmailRequest,
) (*s4wave_provider_spacewave.SendVerificationEmailResponse, error) {
	// Resolve the current signer before the cloud request.
	cli, err := r.swAcc.GetSessionClient()
	if err != nil {
		return nil, err
	}
	result, err := cli.RequestEmailVerification(ctx, req.GetEmail())
	if err != nil {
		return nil, err
	}
	return &s4wave_provider_spacewave.SendVerificationEmailResponse{
		Sent:       true,
		RetryAfter: result.RetryAfter,
	}, nil
}

// VerifyEmailCode verifies a 6-digit code for in-app email verification.
func (r *SpacewaveSessionResource) VerifyEmailCode(
	ctx context.Context,
	req *s4wave_provider_spacewave.VerifyEmailCodeRequest,
) (*s4wave_provider_spacewave.VerifyEmailCodeResponse, error) {
	// Resolve the current signer before the cloud request.
	cli, err := r.swAcc.GetSessionClient()
	if err != nil {
		return nil, err
	}
	if err := cli.VerifyEmailCode(ctx, req.GetEmail(), req.GetCode()); err != nil {
		return nil, err
	}

	// Bump local epoch to trigger account state re-fetch (picks up emailVerified).
	r.swAcc.BumpLocalEpoch()
	return &s4wave_provider_spacewave.VerifyEmailCodeResponse{Verified: true}, nil
}

// AddEmail adds an email address and sends verification.
func (r *SpacewaveSessionResource) AddEmail(
	ctx context.Context,
	req *s4wave_provider_spacewave.AddEmailRequest,
) (*s4wave_provider_spacewave.AddEmailResponse, error) {
	// Add the email address and invalidate cached account state.
	cli, err := r.swAcc.GetSessionClient()
	if err != nil {
		return nil, err
	}
	result, err := cli.AddEmail(ctx, req.GetEmail())
	if err != nil {
		return nil, err
	}
	r.swAcc.BumpLocalEpoch()
	return &s4wave_provider_spacewave.AddEmailResponse{
		Sent:       true,
		RetryAfter: result.RetryAfter,
	}, nil
}

// RemoveEmail removes an email address from the account.
func (r *SpacewaveSessionResource) RemoveEmail(
	ctx context.Context,
	req *s4wave_provider_spacewave.RemoveEmailRequest,
) (*s4wave_provider_spacewave.RemoveEmailResponse, error) {
	// Resolve the current signer before the cloud request.
	cli, err := r.swAcc.GetSessionClient()
	if err != nil {
		return nil, err
	}
	if err := cli.RemoveEmail(ctx, req.GetEmail()); err != nil {
		return nil, err
	}
	r.swAcc.BumpLocalEpoch()
	return &s4wave_provider_spacewave.RemoveEmailResponse{}, nil
}

// SetPrimaryEmail promotes a verified email to primary.
func (r *SpacewaveSessionResource) SetPrimaryEmail(
	ctx context.Context,
	req *s4wave_provider_spacewave.SetPrimaryEmailRequest,
) (*s4wave_provider_spacewave.SetPrimaryEmailResponse, error) {
	// Promote the verified address and refresh the account's email state.
	cli, err := r.swAcc.GetSessionClient()
	if err != nil {
		return nil, err
	}
	result, err := cli.SetPrimaryEmail(ctx, req.GetEmail())
	if err != nil {
		return nil, err
	}
	r.swAcc.SetCachedPrimaryEmail(result.Primary)
	r.swAcc.BumpLocalEpoch()
	return &s4wave_provider_spacewave.SetPrimaryEmailResponse{
		Primary: result.Primary,
	}, nil
}
