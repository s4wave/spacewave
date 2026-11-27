package resource_session

import (
	"context"

	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/resource/session/sessioninfo"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// WatchSubscriptionStatus streams billing account state changes.
func (r *SpacewaveSessionResource) WatchSubscriptionStatus(
	req *s4wave_provider_spacewave.WatchSubscriptionStatusRequest,
	strm s4wave_session.SRPCSpacewaveSessionResourceService_WatchSubscriptionStatusStream,
) error {
	ctx := strm.Context()
	var prev *s4wave_provider_spacewave.WatchSubscriptionStatusResponse
	for {
		state, ch, err := r.loadBillingWatchState(ctx, "")
		if err != nil {
			return err
		}

		resp := &s4wave_provider_spacewave.WatchSubscriptionStatusResponse{
			BillingAccount: state.GetBillingAccount(),
		}
		if prev == nil || !resp.EqualVT(prev) {
			if err := strm.Send(resp); err != nil {
				return err
			}
			prev = resp
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ch:
		}
	}
}

// WatchBillingState streams combined billing account state and usage.
func (r *SpacewaveSessionResource) WatchBillingState(
	req *s4wave_provider_spacewave.WatchBillingStateRequest,
	strm s4wave_session.SRPCSpacewaveSessionResourceService_WatchBillingStateStream,
) error {
	ctx := strm.Context()
	requestedBillingID := req.GetBillingAccountId()

	var prev *s4wave_provider_spacewave.WatchBillingStateResponse
	for {
		resp, ch, err := r.loadBillingWatchState(ctx, requestedBillingID)
		if err != nil {
			return err
		}
		if prev == nil || !resp.EqualVT(prev) {
			if err := strm.Send(resp); err != nil {
				return err
			}
			prev = resp
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ch:
		}
	}
}

// RefreshBillingState invalidates the cached billing snapshot so watches reload.
func (r *SpacewaveSessionResource) RefreshBillingState(
	ctx context.Context,
	req *s4wave_provider_spacewave.RefreshBillingStateRequest,
) (*s4wave_provider_spacewave.RefreshBillingStateResponse, error) {
	baID, err := r.resolveBillingAccountID(ctx, req.GetBillingAccountId())
	if err != nil {
		return nil, err
	}
	if baID == "" {
		r.swAcc.BumpLocalEpoch()
	} else {
		r.swAcc.InvalidateBillingSnapshot(baID)
	}
	return &s4wave_provider_spacewave.RefreshBillingStateResponse{}, nil
}

// CancelSubscription cancels the active subscription.
func (r *SpacewaveSessionResource) CancelSubscription(
	ctx context.Context,
	req *s4wave_provider_spacewave.CancelSubscriptionRequest,
) (*s4wave_provider_spacewave.CancelSubscriptionResponse, error) {
	// Resolve the billing account selected by this request.
	baID, err := r.resolveBillingAccountID(ctx, req.GetBillingAccountId())
	if err != nil {
		return nil, err
	}
	if baID == "" {
		return nil, errors.New("no billing account found")
	}

	// Cancel the Cloud subscription and invalidate its cached snapshot.
	cli, err := r.swAcc.GetSessionClient()
	if err != nil {
		return nil, err
	}
	if _, err := cli.CancelSubscription(ctx, baID); err != nil {
		return nil, err
	}
	r.swAcc.InvalidateBillingSnapshot(baID)
	return &s4wave_provider_spacewave.CancelSubscriptionResponse{}, nil
}

// ReactivateSubscription reactivates a canceled subscription.
func (r *SpacewaveSessionResource) ReactivateSubscription(
	ctx context.Context,
	req *s4wave_provider_spacewave.ReactivateSubscriptionRequest,
) (*s4wave_provider_spacewave.ReactivateSubscriptionResponse, error) {
	// Resolve the billing account selected by this request.
	baID, err := r.resolveBillingAccountID(ctx, req.GetBillingAccountId())
	if err != nil {
		return nil, err
	}
	if baID == "" {
		return nil, errors.New("no billing account found")
	}

	// Reactivate the Cloud subscription and interpret its next step.
	cli, err := r.swAcc.GetSessionClient()
	if err != nil {
		return nil, err
	}
	cloudResp, err := cli.ReactivateSubscription(ctx, baID)
	if err != nil {
		return nil, err
	}

	// Build the response and invalidate the billing snapshot after reactivation.
	resp := &s4wave_provider_spacewave.ReactivateSubscriptionResponse{}
	if cloudResp.GetStatus() == "needs_checkout" {
		resp.NeedsCheckout = true
	}
	r.swAcc.InvalidateBillingSnapshot(baID)
	return resp, nil
}

// SetBillingSpendingLimit updates the selected payer's recurring operation budget.
func (r *SpacewaveSessionResource) SetBillingSpendingLimit(
	ctx context.Context,
	req *s4wave_provider_spacewave.SetBillingSpendingLimitRequest,
) (*s4wave_provider_spacewave.SetBillingSpendingLimitResponse, error) {
	// Resolve the payer's account before changing its recurring budget.
	baID, err := r.resolveBillingAccountID(ctx, req.GetBillingAccountId())
	if err != nil {
		return nil, err
	}
	if baID == "" {
		return nil, errors.New("no billing account found")
	}

	// Resolve the current signer before the cloud request.
	cli, err := r.swAcc.GetSessionClient()
	if err != nil {
		return nil, err
	}

	// Update the spending limit and invalidate the billing snapshot.
	if err := cli.SetBillingSpendingLimit(ctx, baID, req.GetConsent()); err != nil {
		return nil, err
	}
	r.swAcc.InvalidateBillingSnapshot(baID)
	return &s4wave_provider_spacewave.SetBillingSpendingLimitResponse{}, nil
}

// CreateBillingPortal creates a Stripe billing portal session URL.
func (r *SpacewaveSessionResource) CreateBillingPortal(
	ctx context.Context,
	req *s4wave_provider_spacewave.CreateBillingPortalRequest,
) (*s4wave_provider_spacewave.CreateBillingPortalResponse, error) {
	// Resolve the billing account before creating its portal session.
	baID, err := r.resolveBillingAccountID(ctx, req.GetBillingAccountId())
	if err != nil {
		return nil, err
	}
	if baID == "" {
		return nil, errors.New("no billing account found")
	}

	// Create a portal URL for the selected billing account.
	cli, err := r.swAcc.GetSessionClient()
	if err != nil {
		return nil, err
	}
	url, err := cli.CreateBillingPortal(ctx, baID)
	if err != nil {
		return nil, err
	}
	return &s4wave_provider_spacewave.CreateBillingPortalResponse{Url: url}, nil
}

// loadBillingWatchState loads the billing state snapshot and a wait channel for changes.
func (r *SpacewaveSessionResource) loadBillingWatchState(
	ctx context.Context,
	requestedBillingID string,
) (*s4wave_provider_spacewave.WatchBillingStateResponse, <-chan struct{}, error) {
	// Resolve the requested or default billing account for this snapshot.
	baID, err := r.resolveBillingAccountID(ctx, requestedBillingID)
	if err != nil {
		return nil, nil, err
	}

	// Capture the account broadcast revision before loading billing state.
	var ch <-chan struct{}
	r.swAcc.GetAccountBroadcast().HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		ch = getWaitCh()
	})
	if baID == "" {
		return &s4wave_provider_spacewave.WatchBillingStateResponse{
			BillingAccount: &s4wave_provider_spacewave.BillingAccountInfo{
				Status: s4wave_provider_spacewave.BillingStatus_BillingStatus_NONE,
			},
			Usage: sessioninfo.BuildEmptyBillingUsageInfo(),
		}, ch, nil
	}

	// Load the cached billing account and usage snapshot.
	state, usage, err := r.swAcc.GetBillingSnapshot(ctx, baID)
	if err != nil {
		return nil, nil, err
	}
	return &s4wave_provider_spacewave.WatchBillingStateResponse{
		BillingAccount: buildBillingAccountInfo(baID, state),
		Usage:          sessioninfo.BuildBillingUsageInfo(usage),
	}, ch, nil
}

// resolveBillingAccountID returns the requested billing account ID or falls
// back to the account's default billing account ID.
func (r *SpacewaveSessionResource) resolveBillingAccountID(
	ctx context.Context,
	requestedBillingID string,
) (string, error) {
	if requestedBillingID != "" {
		return requestedBillingID, nil
	}
	info, err := r.swAcc.GetAccountState(ctx)
	if err != nil {
		return "", err
	}
	return info.GetBillingAccountId(), nil
}

// buildBillingAccountInfo converts a cloud billing state response to a proto BillingAccountInfo.
func buildBillingAccountInfo(
	baID string,
	state *api.BillingStateResponse,
) *s4wave_provider_spacewave.BillingAccountInfo {
	if state == nil {
		return &s4wave_provider_spacewave.BillingAccountInfo{
			Id:     baID,
			Status: s4wave_provider_spacewave.BillingStatus_BillingStatus_NONE,
		}
	}
	return &s4wave_provider_spacewave.BillingAccountInfo{
		Id:               baID,
		Status:           state.GetStatus(),
		BillingInterval:  state.GetBillingInterval(),
		PastDueSince:     state.GetPastDueSince(),
		CancelAt:         state.GetCancelAt(),
		CurrentPeriodEnd: state.GetCurrentPeriodEnd(),
		LifecycleState: s4wave_provider_spacewave.AccountLifecycleState(
			state.GetLifecycleState(),
		),
		DeleteAt:           state.GetDeleteAt(),
		LifecycleUpdatedAt: state.GetLifecycleUpdatedAt(),
		DeletedAt:          state.GetDeletedAt(),
		DisplayName:        state.GetDisplayName(),
	}
}

// CreateBillingAccount creates a new unassigned billing account managed by the caller.
func (r *SpacewaveSessionResource) CreateBillingAccount(
	ctx context.Context,
	req *s4wave_provider_spacewave.CreateBillingAccountRequest,
) (*s4wave_provider_spacewave.CreateBillingAccountResponse, error) {
	// Create the caller-managed billing account and invalidate its list cache.
	cli, err := r.swAcc.GetSessionClient()
	if err != nil {
		return nil, err
	}
	baID, err := cli.CreateBillingAccount(ctx, req.GetDisplayName())
	if err != nil {
		return nil, err
	}
	r.swAcc.InvalidateManagedBAsList()
	return &s4wave_provider_spacewave.CreateBillingAccountResponse{
		BillingAccountId: baID,
	}, nil
}

// RenameBillingAccount updates the display name on a BA the caller manages.
func (r *SpacewaveSessionResource) RenameBillingAccount(
	ctx context.Context,
	req *s4wave_provider_spacewave.RenameBillingAccountRequest,
) (*s4wave_provider_spacewave.RenameBillingAccountResponse, error) {
	// Rename the managed billing account and invalidate its cached records.
	cli, err := r.swAcc.GetSessionClient()
	if err != nil {
		return nil, err
	}
	if err := cli.RenameBillingAccount(ctx, req.GetBillingAccountId(), req.GetDisplayName()); err != nil {
		return nil, err
	}
	r.swAcc.InvalidateBillingSnapshot(req.GetBillingAccountId())
	r.swAcc.InvalidateManagedBAsList()
	return &s4wave_provider_spacewave.RenameBillingAccountResponse{}, nil
}

// DeleteBillingAccount permanently removes a managed billing account.
func (r *SpacewaveSessionResource) DeleteBillingAccount(
	ctx context.Context,
	req *s4wave_provider_spacewave.DeleteBillingAccountRequest,
) (*s4wave_provider_spacewave.DeleteBillingAccountResponse, error) {
	// Delete the managed billing account and invalidate its cached records.
	cli, err := r.swAcc.GetSessionClient()
	if err != nil {
		return nil, err
	}
	if err := cli.DeleteBillingAccount(ctx, req.GetBillingAccountId()); err != nil {
		return nil, err
	}
	r.swAcc.InvalidateBillingSnapshot(req.GetBillingAccountId())
	r.swAcc.InvalidateManagedBAsList()
	return &s4wave_provider_spacewave.DeleteBillingAccountResponse{}, nil
}

// ListManagedBillingAccounts lists billing accounts the caller manages.
func (r *SpacewaveSessionResource) ListManagedBillingAccounts(
	ctx context.Context,
	_ *s4wave_provider_spacewave.ListManagedBillingAccountsRequest,
) (*s4wave_provider_spacewave.ListManagedBillingAccountsResponse, error) {
	accounts, err := r.swAcc.GetManagedBAsSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	return &s4wave_provider_spacewave.ListManagedBillingAccountsResponse{
		Accounts: accounts,
	}, nil
}

// AssignBillingAccount binds a billing account to a principal.
func (r *SpacewaveSessionResource) AssignBillingAccount(
	ctx context.Context,
	req *s4wave_provider_spacewave.AssignBillingAccountRequest,
) (*s4wave_provider_spacewave.AssignBillingAccountResponse, error) {
	// Assign the billing account to its requested owner.
	cli, err := r.swAcc.GetSessionClient()
	if err != nil {
		return nil, err
	}
	_, err = cli.AssignBillingAccount(
		ctx,
		req.GetBillingAccountId(),
		req.GetTargetOwnerType(),
		req.GetTargetOwnerId(),
	)
	if err != nil {
		return nil, err
	}
	r.swAcc.InvalidateManagedBAsList()
	if req.GetTargetOwnerType() == "account" {
		r.swAcc.BumpLocalEpoch()
	}
	if req.GetTargetOwnerType() == "organization" {
		r.refreshOrganizationCaches(ctx, req.GetTargetOwnerId(), false)
	}
	return &s4wave_provider_spacewave.AssignBillingAccountResponse{}, nil
}

// DetachBillingAccount clears a principal's billing account assignment.
func (r *SpacewaveSessionResource) DetachBillingAccount(
	ctx context.Context,
	req *s4wave_provider_spacewave.DetachBillingAccountRequest,
) (*s4wave_provider_spacewave.DetachBillingAccountResponse, error) {
	// Detach the billing account from its current owner.
	cli, err := r.swAcc.GetSessionClient()
	if err != nil {
		return nil, err
	}
	_, err = cli.DetachBillingAccount(
		ctx,
		req.GetTargetOwnerType(),
		req.GetTargetOwnerId(),
	)
	if err != nil {
		return nil, err
	}
	r.swAcc.InvalidateManagedBAsList()
	if req.GetTargetOwnerType() == "account" {
		r.swAcc.BumpLocalEpoch()
	}
	if req.GetTargetOwnerType() == "organization" {
		r.refreshOrganizationCaches(ctx, req.GetTargetOwnerId(), false)
	}
	return &s4wave_provider_spacewave.DetachBillingAccountResponse{}, nil
}
