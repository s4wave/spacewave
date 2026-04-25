package resource_session

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/http"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/util/scrub"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	alpha_nethttp "github.com/s4wave/spacewave/core/nethttp"
	"github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	resource_account "github.com/s4wave/spacewave/core/resource/account"
	"github.com/s4wave/spacewave/core/session"
	session_handoff "github.com/s4wave/spacewave/core/session/handoff"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/keypem"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_org "github.com/s4wave/spacewave/sdk/org"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	"github.com/sirupsen/logrus"
)

// SpacewaveSessionResource implements SpacewaveSessionResourceService.
// It wraps a session-scoped spacewave ProviderAccount, resolving it directly
// from the session without scanning.
type SpacewaveSessionResource struct {
	parent  *SessionResource
	le      *logrus.Entry
	b       bus.Bus
	session session.Session
	swAcc   *provider_spacewave.ProviderAccount
}

// NewSpacewaveSessionResource creates a new SpacewaveSessionResource.
func NewSpacewaveSessionResource(
	sr *SessionResource,
	le *logrus.Entry,
	b bus.Bus,
	sess session.Session,
	swAcc *provider_spacewave.ProviderAccount,
) *SpacewaveSessionResource {
	return &SpacewaveSessionResource{
		parent:  sr,
		le:      le,
		b:       b,
		session: sess,
		swAcc:   swAcc,
	}
}

// getSessionRef returns the session reference.
func (r *SpacewaveSessionResource) getSessionRef() *session.SessionRef {
	return r.session.GetSessionRef()
}

// getSessionID returns the session ID from the provider resource ref.
func (r *SpacewaveSessionResource) getSessionID() string {
	return r.getSessionRef().GetProviderResourceRef().GetId()
}

// WatchOnboardingStatus streams onboarding state changes.
func (r *SpacewaveSessionResource) WatchOnboardingStatus(
	req *s4wave_provider_spacewave.WatchOnboardingStatusRequest,
	strm s4wave_session.SRPCSpacewaveSessionResourceService_WatchOnboardingStatusStream,
) error {
	ctx := strm.Context()
	sessionID := r.getSessionID()
	sessRef := r.getSessionRef()
	accountBcast := r.swAcc.GetAccountBroadcast()

	var prev *s4wave_provider_spacewave.WatchOnboardingStatusResponse
	for {
		var ch <-chan struct{}
		var accountStatus provider.ProviderAccountStatus
		var subStatus s4wave_provider_spacewave.BillingStatus
		var cancelAt int64
		var deleteAt int64
		var lifecycleUpdatedAt int64
		var deletedAt int64
		var emailVerified bool
		var stateLoaded bool
		var lifecycleState s4wave_provider_spacewave.AccountLifecycleState
		var selfEnrollmentSummary *provider_spacewave.SelfEnrollmentSummary
		var selfEnrollmentAutoRejoinRunning bool
		accountBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			ch = getWaitCh()
			accountStatus = r.swAcc.GetAccountStatus()
			selfEnrollmentSummary = r.swAcc.GetSelfEnrollmentSummary()
			selfEnrollmentAutoRejoinRunning = r.swAcc.GetSelfRejoinSweepRunning()
			state := r.swAcc.AccountStateSnapshot()
			if state != nil {
				stateLoaded = true
				subStatus = state.GetSubscriptionStatus()
				cancelAt = state.GetCancelAt()
				deleteAt = state.GetDeleteAt()
				lifecycleUpdatedAt = state.GetLifecycleUpdatedAt()
				deletedAt = state.GetDeletedAt()
				emailVerified = state.GetEmailVerified()
				lifecycleState = s4wave_provider_spacewave.AccountLifecycleState(
					state.GetLifecycleState(),
				)
			}
		})

		// Hold the first emission until the account snapshot has been
		// populated from the cloud, unless the account is in a terminal
		// non-ready state. Prevents a "not subscribed" flash for subscribed
		// users while the account fetcher is still loading.
		if prev == nil && !shouldEmitOnboardingStatus(stateLoaded, accountStatus) {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ch:
			}
			continue
		}

		billingStatus := subStatus
		hasSubscription := billingStatus == s4wave_provider_spacewave.BillingStatus_BillingStatus_ACTIVE ||
			billingStatus == s4wave_provider_spacewave.BillingStatus_BillingStatus_TRIALING

		managedBAsLoaded := false
		var managedBAs []*s4wave_provider_spacewave.ManagedBillingAccount
		if shouldLoadManagedBillingSummary(accountStatus) {
			var err error
			managedBAs, err = r.swAcc.GetManagedBAsSnapshot(ctx)
			if err != nil {
				r.le.WithError(err).Warn("failed to fetch managed billing account list")
				managedBAs = nil
			} else {
				managedBAsLoaded = true
			}
		}
		var managedTotal, managedActive, managedNoSubscription uint32
		for _, ba := range managedBAs {
			managedTotal++
			switch ba.GetSubscriptionStatus() {
			case s4wave_provider_spacewave.BillingStatus_BillingStatus_ACTIVE,
				s4wave_provider_spacewave.BillingStatus_BillingStatus_TRIALING:
				managedActive++
			case s4wave_provider_spacewave.BillingStatus_BillingStatus_NONE:
				managedNoSubscription++
			}
		}

		resp := &s4wave_provider_spacewave.WatchOnboardingStatusResponse{
			HasSubscription:              hasSubscription,
			SubscriptionStatus:           billingStatus,
			CheckoutInProgress:           r.swAcc.GetCheckoutWatcher().HasTicket(),
			CancelAt:                     cancelAt,
			DeleteAt:                     deleteAt,
			LifecycleUpdatedAt:           lifecycleUpdatedAt,
			DeletedAt:                    deletedAt,
			EmailVerified:                emailVerified,
			LifecycleState:               lifecycleState,
			AccountStatus:                accountStatus,
			ManagedBaCount:               managedTotal,
			ManagedActiveBaCount:         managedActive,
			ManagedNoSubscriptionBaCount: managedNoSubscription,
			BillingSummaryLoaded:         managedBAsLoaded,
		}
		if selfEnrollmentSummary != nil {
			resp.SessionSelfEnrollmentGenerationKey = selfEnrollmentSummary.GetGenerationKey()
			resp.SessionSelfEnrollmentCount = selfEnrollmentSummary.GetCount()
		}
		resp.SelfEnrollmentGateState = selfEnrollmentGateState(
			selfEnrollmentSummary,
			selfEnrollmentAutoRejoinRunning,
		)

		found, localIdx, _ := r.swAcc.GetLinkedLocalSession(ctx, sessionID)
		if found {
			resp.HasLinkedLocal = true
			resp.LinkedLocalSessionIndex = localIdx
			resp.LinkedLocalHasContent = r.checkLocalHasContent(ctx, localIdx)
		}

		// Populate the cloud session index for local sessions that need
		// to redirect to the migration wizard on the cloud session.
		// Skip for cloud sessions: they would find themselves.
		if sessRef.GetProviderResourceRef().GetProviderId() != "spacewave" {
			cloudIdx := r.findCloudSessionIndex(ctx, r.swAcc.GetAccountID())
			if cloudIdx != 0 {
				resp.HasLinkedCloud = true
				resp.LinkedCloudSessionIndex = cloudIdx
			}
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

// shouldEmitOnboardingStatus returns whether a WatchOnboardingStatus response
// should be sent given the current account state. The first emission holds
// until the cloud account snapshot has been populated by the fetcher. Terminal
// non-ready statuses are meaningful to clients even with a nil snapshot.
func shouldEmitOnboardingStatus(stateLoaded bool, accountStatus provider.ProviderAccountStatus) bool {
	if stateLoaded {
		return true
	}
	switch accountStatus {
	case provider.ProviderAccountStatus_ProviderAccountStatus_UNAUTHENTICATED,
		provider.ProviderAccountStatus_ProviderAccountStatus_DELETED,
		provider.ProviderAccountStatus_ProviderAccountStatus_DORMANT,
		provider.ProviderAccountStatus_ProviderAccountStatus_FAILED:
		return true
	}
	return false
}

// shouldLoadManagedBillingSummary returns whether ambient onboarding watches
// should query the managed billing-account summary.
func shouldLoadManagedBillingSummary(accountStatus provider.ProviderAccountStatus) bool {
	return accountStatus == provider.ProviderAccountStatus_ProviderAccountStatus_READY
}

func selfEnrollmentGateState(
	summary *provider_spacewave.SelfEnrollmentSummary,
	autoRejoinRunning bool,
) s4wave_provider_spacewave.SelfEnrollmentGateState {
	if autoRejoinRunning {
		return s4wave_provider_spacewave.SelfEnrollmentGateState_SELF_ENROLLMENT_GATE_STATE_AUTO_CONNECTING
	}
	if summary == nil || !summary.GetLoaded() {
		return s4wave_provider_spacewave.SelfEnrollmentGateState_SELF_ENROLLMENT_GATE_STATE_CHECKING
	}
	if summary.GetCount() != 0 {
		return s4wave_provider_spacewave.SelfEnrollmentGateState_SELF_ENROLLMENT_GATE_STATE_ACTION_REQUIRED
	}
	return s4wave_provider_spacewave.SelfEnrollmentGateState_SELF_ENROLLMENT_GATE_STATE_READY
}

// checkLocalHasContent returns true if the local session at the given index has SharedObjects.
func (r *SpacewaveSessionResource) checkLocalHasContent(ctx context.Context, localIdx uint32) bool {
	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, r.b, "", false, nil)
	if err != nil {
		return false
	}
	defer sessionCtrlRef.Release()

	entry, err := sessionCtrl.GetSessionByIdx(ctx, localIdx)
	if err != nil {
		return false
	}
	provRef := entry.GetSessionRef().GetProviderResourceRef()
	providerID := provRef.GetProviderId()
	accountID := provRef.GetProviderAccountId()

	provAcc, provAccRef, accErr := provider.ExAccessProviderAccount(ctx, r.b, providerID, accountID, false, nil)
	if accErr != nil {
		return false
	}
	defer provAccRef.Release()
	localAcc, ok := provAcc.(*provider_local.ProviderAccount)
	if !ok {
		return false
	}
	volID := localAcc.GetVolume().GetID()
	soObjStoreID := provider_local.SobjectObjectStoreID(providerID, accountID)
	soHandle, _, soDiRef, soErr := volume.ExBuildObjectStoreAPI(ctx, r.b, false, soObjStoreID, volID, nil)
	if soErr != nil {
		return false
	}
	defer soDiRef.Release()

	soStore := soHandle.GetObjectStore()
	soTx, txErr := soStore.NewTransaction(ctx, false)
	if txErr != nil {
		return false
	}
	defer soTx.Discard()

	data, found, gErr := soTx.Get(ctx, provider_local.SobjectObjectStoreListKey())
	if gErr != nil || !found {
		return false
	}
	list := &sobject.SharedObjectList{}
	if err := list.UnmarshalVT(data); err != nil {
		return false
	}
	return len(list.GetSharedObjects()) > 0
}

// findCloudSessionIndex finds the session index for a spacewave (cloud) session
// with the given account ID. Returns 0 if not found.
func (r *SpacewaveSessionResource) findCloudSessionIndex(ctx context.Context, accountID string) uint32 {
	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, r.b, "", false, nil)
	if err != nil {
		return 0
	}
	defer sessionCtrlRef.Release()

	sessions, err := sessionCtrl.ListSessions(ctx)
	if err != nil {
		return 0
	}
	for _, entry := range sessions {
		ref := entry.GetSessionRef().GetProviderResourceRef()
		if ref.GetProviderAccountId() == accountID && ref.GetProviderId() == "spacewave" {
			return entry.GetSessionIndex()
		}
	}
	return 0
}

// CreateLinkedLocalSession creates a local provider session with cloud identity metadata.
func (r *SpacewaveSessionResource) CreateLinkedLocalSession(
	ctx context.Context,
	req *s4wave_provider_spacewave.CreateLinkedLocalSessionRequest,
) (*s4wave_provider_spacewave.CreateLinkedLocalSessionResponse, error) {
	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, r.b, "", false, nil)
	if err != nil {
		return nil, err
	}
	defer sessionCtrlRef.Release()

	sessionID := r.getSessionID()

	// Idempotency: check if a linked local session already exists.
	found, localIdx, err := r.swAcc.GetLinkedLocalSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if found {
		entry, err := sessionCtrl.GetSessionByIdx(ctx, localIdx)
		if err != nil {
			return nil, err
		}
		return &s4wave_provider_spacewave.CreateLinkedLocalSessionResponse{SessionListEntry: entry}, nil
	}

	info, err := r.swAcc.GetAccountState(ctx)
	if err != nil {
		return nil, err
	}

	localProv, localProvRef, err := provider.ExLookupProvider(ctx, r.b, provider_local.ProviderID, false, nil)
	if err != nil {
		return nil, err
	}
	defer localProvRef.Release()

	prov := localProv.(*provider_local.Provider)
	localSessRef, err := prov.CreateLocalAccountAndSession(ctx, info.AccountId)
	if err != nil {
		return nil, err
	}

	meta := &session.SessionMetadata{
		DisplayName:         info.EntityId,
		ProviderDisplayName: "Local",
		ProviderAccountId:   localSessRef.GetProviderResourceRef().GetProviderAccountId(),
		CloudAccountId:      info.AccountId,
		CloudEntityId:       info.EntityId,
		ProviderId:          "local",
		CreatedAt:           time.Now().UnixMilli(),
	}
	listEntry, err := sessionCtrl.RegisterSession(ctx, localSessRef, meta)
	if err != nil {
		return nil, err
	}

	// Write linked-local cross-reference on the spacewave sessionTracker ObjectStore.
	if err := r.swAcc.SetLinkedLocalSession(ctx, sessionID, listEntry.GetSessionIndex()); err != nil {
		r.le.WithError(err).Warn("failed to write linked-local key")
	}

	return &s4wave_provider_spacewave.CreateLinkedLocalSessionResponse{SessionListEntry: listEntry}, nil
}

// GetLinkedLocalSession returns the session index of the linked local session.
func (r *SpacewaveSessionResource) GetLinkedLocalSession(
	ctx context.Context,
	req *s4wave_provider_spacewave.GetLinkedLocalSessionRequest,
) (*s4wave_provider_spacewave.GetLinkedLocalSessionResponse, error) {
	sessionID := r.getSessionID()
	found, localIdx, err := r.swAcc.GetLinkedLocalSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	return &s4wave_provider_spacewave.GetLinkedLocalSessionResponse{
		Found:        found,
		SessionIndex: localIdx,
	}, nil
}

// UnlinkLocalSession removes the linked-local session cross-reference.
func (r *SpacewaveSessionResource) UnlinkLocalSession(
	ctx context.Context,
	req *s4wave_provider_spacewave.UnlinkLocalSessionRequest,
) (*s4wave_provider_spacewave.UnlinkLocalSessionResponse, error) {
	sessionID := r.getSessionID()

	if err := r.swAcc.DeleteLinkedLocalSession(ctx, sessionID); err != nil {
		return nil, err
	}

	r.swAcc.GetAccountBroadcast().HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		broadcast()
	})

	return &s4wave_provider_spacewave.UnlinkLocalSessionResponse{}, nil
}

// CreateCheckoutSession creates or resumes a Stripe Checkout Session.
func (r *SpacewaveSessionResource) CreateCheckoutSession(
	ctx context.Context,
	req *s4wave_provider_spacewave.CreateCheckoutSessionRequest,
) (*s4wave_provider_spacewave.CreateCheckoutSessionResponse, error) {
	if req.GetSuccessUrl() == "" || req.GetCancelUrl() == "" {
		return nil, errors.New("success_url and cancel_url required")
	}

	cli := r.swAcc.GetSessionClient()
	resp, err := cli.CreateCheckoutSession(
		ctx,
		req.GetSuccessUrl(),
		req.GetCancelUrl(),
		req.GetBillingInterval(),
		req.GetBillingAccountId(),
	)
	if err != nil {
		return nil, err
	}

	// Store the ticket on the ProviderAccount so WatchCheckoutStatus can use it.
	if resp.GetStatus() == "pending" && resp.GetWsTicket() != "" {
		r.swAcc.GetCheckoutWatcher().SetTicket(resp.GetWsTicket())
	}

	return &s4wave_provider_spacewave.CreateCheckoutSessionResponse{
		CheckoutUrl: resp.GetCheckoutUrl(),
		WsTicket:    resp.GetWsTicket(),
		Status:      mapCheckoutStatus(resp.GetStatus()),
	}, nil
}

// CancelCheckoutSession cancels pending checkout attempts.
func (r *SpacewaveSessionResource) CancelCheckoutSession(
	ctx context.Context,
	req *s4wave_provider_spacewave.CancelCheckoutSessionRequest,
) (*s4wave_provider_spacewave.CancelCheckoutSessionResponse, error) {
	cli := r.swAcc.GetSessionClient()
	resp, err := cli.CancelCheckoutSession(ctx)
	if err != nil {
		return nil, err
	}

	return &s4wave_provider_spacewave.CancelCheckoutSessionResponse{
		Status: mapCheckoutStatus(resp.GetStatus()),
	}, nil
}

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

// WatchCheckoutStatus streams checkout status changes via the checkout WS.
func (r *SpacewaveSessionResource) WatchCheckoutStatus(
	req *s4wave_provider_spacewave.WatchCheckoutStatusRequest,
	strm s4wave_session.SRPCSpacewaveSessionResourceService_WatchCheckoutStatusStream,
) error {
	ctx := strm.Context()

	watcher := r.swAcc.GetCheckoutWatcher()
	ref := watcher.AddRef()
	defer ref.Release()

	var prev s4wave_provider_spacewave.CheckoutStatus
	for {
		ch, status := watcher.WaitStatus()

		mapped := mapCheckoutStatus(status)
		if mapped != prev {
			if err := strm.Send(&s4wave_provider_spacewave.WatchCheckoutStatusResponse{
				Status: mapped,
			}); err != nil {
				return err
			}
			prev = mapped
		}

		// Terminal statuses end the stream.
		if status == "completed" || status == "expired" {
			return nil
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
	baID, err := r.resolveBillingAccountID(ctx, req.GetBillingAccountId())
	if err != nil {
		return nil, err
	}
	if baID == "" {
		return nil, errors.New("no billing account found")
	}

	cli := r.swAcc.GetSessionClient()
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
	baID, err := r.resolveBillingAccountID(ctx, req.GetBillingAccountId())
	if err != nil {
		return nil, err
	}
	if baID == "" {
		return nil, errors.New("no billing account found")
	}

	cli := r.swAcc.GetSessionClient()
	cloudResp, err := cli.ReactivateSubscription(ctx, baID)
	if err != nil {
		return nil, err
	}

	resp := &s4wave_provider_spacewave.ReactivateSubscriptionResponse{}
	if cloudResp.GetStatus() == "needs_checkout" {
		resp.NeedsCheckout = true
	}
	r.swAcc.InvalidateBillingSnapshot(baID)
	return resp, nil
}

// SwitchBillingInterval switches between monthly and annual billing.
func (r *SpacewaveSessionResource) SwitchBillingInterval(
	ctx context.Context,
	req *s4wave_provider_spacewave.SwitchBillingIntervalRequest,
) (*s4wave_provider_spacewave.SwitchBillingIntervalResponse, error) {
	baID, err := r.resolveBillingAccountID(ctx, req.GetBillingAccountId())
	if err != nil {
		return nil, err
	}
	if baID == "" {
		return nil, errors.New("no billing account found")
	}

	if req.GetBillingInterval() == s4wave_provider_spacewave.BillingInterval_BillingInterval_UNKNOWN {
		return nil, errors.New("billing_interval is required")
	}

	cli := r.swAcc.GetSessionClient()
	if _, err := cli.SwitchBillingInterval(ctx, baID, req.GetBillingInterval()); err != nil {
		return nil, err
	}
	r.swAcc.InvalidateBillingSnapshot(baID)
	return &s4wave_provider_spacewave.SwitchBillingIntervalResponse{}, nil
}

// CreateBillingPortal creates a Stripe billing portal session URL.
func (r *SpacewaveSessionResource) CreateBillingPortal(
	ctx context.Context,
	req *s4wave_provider_spacewave.CreateBillingPortalRequest,
) (*s4wave_provider_spacewave.CreateBillingPortalResponse, error) {
	baID, err := r.resolveBillingAccountID(ctx, req.GetBillingAccountId())
	if err != nil {
		return nil, err
	}
	if baID == "" {
		return nil, errors.New("no billing account found")
	}

	cli := r.swAcc.GetSessionClient()
	url, err := cli.CreateBillingPortal(ctx, baID)
	if err != nil {
		return nil, err
	}
	return &s4wave_provider_spacewave.CreateBillingPortalResponse{Url: url}, nil
}

// RequestDeleteNowEmail sends a delete-now confirmation email with a code and link.
func (r *SpacewaveSessionResource) RequestDeleteNowEmail(
	ctx context.Context,
	_ *s4wave_provider_spacewave.RequestDeleteNowEmailRequest,
) (*s4wave_provider_spacewave.RequestDeleteNowEmailResponse, error) {
	cli := r.swAcc.GetSessionClient()
	result, err := cli.RequestDeleteNowEmail(ctx)
	if err != nil {
		return nil, err
	}
	return &s4wave_provider_spacewave.RequestDeleteNowEmailResponse{
		Sent:       true,
		RetryAfter: result.RetryAfter,
		Email:      result.Email,
	}, nil
}

// ConfirmDeleteNowCode finalizes delete-now using the 6-digit code from email.
func (r *SpacewaveSessionResource) ConfirmDeleteNowCode(
	ctx context.Context,
	req *s4wave_provider_spacewave.ConfirmDeleteNowCodeRequest,
) (*s4wave_provider_spacewave.ConfirmDeleteNowCodeResponse, error) {
	cli := r.swAcc.GetSessionClient()
	result, err := cli.ConfirmDeleteNowCode(ctx, req.GetCode())
	if err != nil {
		return nil, err
	}
	r.swAcc.BumpLocalEpoch()
	return &s4wave_provider_spacewave.ConfirmDeleteNowCodeResponse{
		DeleteAt:         result.DeleteAt,
		InvoiceTotal:     result.InvoiceTotal,
		InvoiceAmountDue: result.InvoiceAmountDue,
		InvoiceCurrency:  result.InvoiceCurrency,
		InvoiceStatus:    result.InvoiceStatus,
		ChargeAttempted:  result.ChargeAttempted,
		RefundAmount:     result.RefundAmount,
		RefundCurrency:   result.RefundCurrency,
	}, nil
}

// UndoDeleteNow cancels a pending delete-now countdown.
func (r *SpacewaveSessionResource) UndoDeleteNow(
	ctx context.Context,
	_ *s4wave_provider_spacewave.UndoDeleteNowRequest,
) (*s4wave_provider_spacewave.UndoDeleteNowResponse, error) {
	cli := r.swAcc.GetSessionClient()
	if err := cli.UndoDeleteNow(ctx); err != nil {
		return nil, err
	}
	r.swAcc.BumpLocalEpoch()
	return &s4wave_provider_spacewave.UndoDeleteNowResponse{}, nil
}

// loadBillingWatchState loads the billing state snapshot and a wait channel for changes.
func (r *SpacewaveSessionResource) loadBillingWatchState(
	ctx context.Context,
	requestedBillingID string,
) (*s4wave_provider_spacewave.WatchBillingStateResponse, <-chan struct{}, error) {
	baID, err := r.resolveBillingAccountID(ctx, requestedBillingID)
	if err != nil {
		return nil, nil, err
	}

	var ch <-chan struct{}
	r.swAcc.GetAccountBroadcast().HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		ch = getWaitCh()
	})
	if baID == "" {
		return &s4wave_provider_spacewave.WatchBillingStateResponse{
			BillingAccount: &s4wave_provider_spacewave.BillingAccountInfo{
				Status: s4wave_provider_spacewave.BillingStatus_BillingStatus_NONE,
			},
			Usage: buildEmptyBillingUsageInfo(),
		}, ch, nil
	}

	state, usage, err := r.swAcc.GetBillingSnapshot(ctx, baID)
	if err != nil {
		return nil, nil, err
	}
	return &s4wave_provider_spacewave.WatchBillingStateResponse{
		BillingAccount: buildBillingAccountInfo(baID, state),
		Usage:          buildBillingUsageInfo(usage),
	}, ch, nil
}

// resolveBillingAccountID returns the requested billing account ID or falls back to the account default.
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

// buildEmptyBillingUsageInfo returns a BillingUsageInfo with only baseline values.
func buildEmptyBillingUsageInfo() *s4wave_provider_spacewave.BillingUsageInfo {
	return &s4wave_provider_spacewave.BillingUsageInfo{
		StorageBaselineBytes: storageBaselineBytes,
		WriteOpsBaseline:     writeOpsBaseline,
		ReadOpsBaseline:      readOpsBaseline,
	}
}

// buildBillingUsageInfo converts a cloud billing usage response to a proto BillingUsageInfo.
func buildBillingUsageInfo(
	usage *api.BillingUsageResponse,
) *s4wave_provider_spacewave.BillingUsageInfo {
	if usage == nil {
		return buildEmptyBillingUsageInfo()
	}
	return &s4wave_provider_spacewave.BillingUsageInfo{
		StorageBytes:                             usage.GetStorageBytes(),
		StorageBaselineBytes:                     storageBaselineBytes,
		WriteOps:                                 usage.GetWriteOps(),
		WriteOpsBaseline:                         writeOpsBaseline,
		ReadOps:                                  usage.GetReadOps(),
		ReadOpsBaseline:                          readOpsBaseline,
		StorageOverageBytes:                      usage.GetStorageOverageBytes(),
		StorageOverageMonthlyCostEstimateUsd:     usage.GetStorageOverageMonthlyCostEstimateUsd(),
		StorageOverageMonthToDateGbMonths:        usage.GetStorageOverageMonthToDateGbMonths(),
		StorageOverageMonthToDateCostEstimateUsd: usage.GetStorageOverageMonthToDateCostEstimateUsd(),
		StorageOverageDeletedGbMonths:            usage.GetStorageOverageDeletedGbMonths(),
		StorageOverageDeletedCostEstimateUsd:     usage.GetStorageOverageDeletedCostEstimateUsd(),
		UsageMeteredThroughAt:                    usage.GetUsageMeteredThroughAt(),
	}
}

// WatchOrganizations streams the user's org list, emitting on membership changes.
func (r *SpacewaveSessionResource) WatchOrganizations(
	req *s4wave_provider_spacewave.WatchOrganizationsRequest,
	strm s4wave_session.SRPCSpacewaveSessionResourceService_WatchOrganizationsStream,
) error {
	ctx := strm.Context()
	return r.swAcc.WatchOrgList(ctx, func(list []*api.OrgResponse) {
		orgs := make([]*s4wave_provider_spacewave.OrganizationInfo, len(list))
		for i, o := range list {
			orgs[i] = &s4wave_provider_spacewave.OrganizationInfo{
				Id:          o.GetId(),
				DisplayName: o.GetDisplayName(),
				Role:        o.GetRole(),
				SpaceIds:    o.GetSpaceIds(),
			}
		}
		_ = strm.Send(&s4wave_provider_spacewave.WatchOrganizationsResponse{Organizations: orgs})
	})
}

// CreateOrganization creates a new organization.
func (r *SpacewaveSessionResource) CreateOrganization(
	ctx context.Context,
	req *s4wave_provider_spacewave.CreateOrganizationRequest,
) (*s4wave_provider_spacewave.CreateOrganizationResponse, error) {
	cli := r.swAcc.GetSessionClient()
	data, err := cli.CreateOrganization(ctx, req.GetDisplayName())
	if err != nil {
		return nil, err
	}
	var info api.OrgResponse
	if err := info.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal org")
	}

	// Create org SharedObject using the cloud org ID as the SO ID.
	orgID := info.GetId()
	r.refreshOrganizationCaches(ctx, orgID, true)
	r.initOrgSharedObject(ctx, orgID, req.GetDisplayName())

	return &s4wave_provider_spacewave.CreateOrganizationResponse{
		Organization: &s4wave_provider_spacewave.OrganizationInfo{
			Id:          orgID,
			DisplayName: info.GetDisplayName(),
			Role:        info.GetRole(),
		},
	}, nil
}

// initOrgSharedObject creates a local org SharedObject and submits InitOrganizationOp.
// Failures are logged as warnings since the cloud org already exists.
func (r *SpacewaveSessionResource) initOrgSharedObject(ctx context.Context, orgID string, displayName string) {
	le := r.le.WithField("org-id", orgID)

	ref, err := r.swAcc.CreateSharedObject(ctx, orgID, s4wave_org.NewOrgSharedObjectMeta(displayName), sobject.OwnerTypeOrganization, orgID)
	if err != nil {
		le.WithError(err).Warn("failed to create org SO")
		return
	}

	so, relSO, err := r.swAcc.MountSharedObject(ctx, ref, nil)
	if err != nil {
		le.WithError(err).Warn("failed to mount org SO")
		return
	}
	defer relSO()

	initOp := &s4wave_org.InitOrganizationOp{
		OrgObjectKey:     s4wave_org.OrgObjectKey,
		DisplayName:      displayName,
		CreatorAccountId: r.swAcc.GetAccountID(),
		Timestamp:        timestamppb.Now(),
	}
	opData, err := s4wave_org.MarshalInitOrgSOOp(initOp)
	if err != nil {
		le.WithError(err).Warn("failed to marshal init org op")
		return
	}
	if _, err := so.QueueOperation(ctx, opData); err != nil {
		le.WithError(err).Warn("failed to queue init org op")
	}
}

// queueOrgUpdateOp mounts the org SO and queues an UpdateOrgOp.
// Failures are logged as warnings since the cloud mutation already succeeded.
func (r *SpacewaveSessionResource) queueOrgUpdateOp(ctx context.Context, orgID string, op *s4wave_org.UpdateOrgOp) {
	le := r.le.WithField("org-id", orgID)
	if !r.swAcc.HasCachedOwnerOrganization(orgID) {
		le.Debug("skipping local org update op: org SO is not owner-authoritative")
		return
	}
	if !r.swAcc.HasCachedSharedObject(orgID) {
		le.Debug("skipping local org update op: org SO not cached")
		return
	}
	ref := sobject.NewSharedObjectRef(
		r.swAcc.GetProviderID(),
		r.swAcc.GetAccountID(),
		orgID,
		provider_spacewave.SobjectBlockStoreID(orgID),
	)
	so, relSO, err := r.swAcc.MountSharedObject(ctx, ref, nil)
	if err != nil {
		le.WithError(err).Debug("failed to mount org SO for update op")
		return
	}
	defer relSO()

	opData, err := s4wave_org.MarshalUpdateOrgSOOp(op)
	if err != nil {
		le.WithError(err).Warn("failed to marshal update org op")
		return
	}
	if _, err := so.QueueOperation(ctx, opData); err != nil {
		le.WithError(err).Debug("failed to queue update org op")
	}
}

// refreshOrganizationCaches invalidates cached org detail and optionally
// refreshes the org list cache after a successful cloud mutation.
func (r *SpacewaveSessionResource) refreshOrganizationCaches(
	ctx context.Context,
	orgID string,
	refreshList bool,
) {
	r.swAcc.InvalidateOrganizationState(orgID)
	if !refreshList {
		return
	}
	if err := r.swAcc.RefreshOrganizationList(ctx); err != nil {
		r.le.WithError(err).Warn("failed to refresh organization list")
	}
}

// WatchOrganizationState streams one organization's combined mutable state.
func (r *SpacewaveSessionResource) WatchOrganizationState(
	req *s4wave_provider_spacewave.WatchOrganizationStateRequest,
	strm s4wave_session.SRPCSpacewaveSessionResourceService_WatchOrganizationStateStream,
) error {
	orgID := req.GetOrgId()
	if orgID == "" {
		return errors.New("org_id is required")
	}

	ctx := strm.Context()
	orgBcast := r.swAcc.GetOrgBroadcast()
	var prev *s4wave_provider_spacewave.WatchOrganizationStateResponse
	for {
		resp, err := r.loadOrganizationState(ctx, orgID)
		if err != nil {
			return err
		}
		if prev == nil || !resp.EqualVT(prev) {
			if err := strm.Send(resp); err != nil {
				return err
			}
			prev = resp
		}

		var ch <-chan struct{}
		orgBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			ch = getWaitCh()
		})
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ch:
		}
	}
}

// loadOrganizationState loads a combined organization snapshot.
func (r *SpacewaveSessionResource) loadOrganizationState(
	ctx context.Context,
	orgID string,
) (*s4wave_provider_spacewave.WatchOrganizationStateResponse, error) {
	info, inviteResp, roleID, err := r.swAcc.GetOrganizationSnapshot(ctx, orgID)
	if err != nil {
		return nil, err
	}
	members := make([]*s4wave_provider_spacewave.OrgMemberInfo, len(info.GetMembers()))
	for i, m := range info.GetMembers() {
		members[i] = &s4wave_provider_spacewave.OrgMemberInfo{
			Id:        m.GetId(),
			SubjectId: m.GetSubjectId(),
			RoleId:    m.GetRoleId(),
			CreatedAt: m.GetCreatedAt(),
			EntityId:  m.GetEntityId(),
		}
	}
	spaces := make([]*s4wave_provider_spacewave.OrgSpaceInfo, len(info.GetSpaces()))
	for i, s := range info.GetSpaces() {
		spaces[i] = &s4wave_provider_spacewave.OrgSpaceInfo{
			Id:          s.GetId(),
			DisplayName: s.GetDisplayName(),
			ObjectType:  s.GetObjectType(),
		}
	}

	var invites []*s4wave_provider_spacewave.OrgInviteInfo
	invites = make([]*s4wave_provider_spacewave.OrgInviteInfo, len(inviteResp.GetInvites()))
	for i, inv := range inviteResp.GetInvites() {
		invites[i] = &s4wave_provider_spacewave.OrgInviteInfo{
			Id:        inv.GetId(),
			Type:      inv.GetType(),
			Token:     inv.GetToken(),
			Uses:      inv.GetUses(),
			MaxUses:   inv.GetMaxUses(),
			ExpiresAt: inv.GetExpiresAt(),
		}
	}

	var rootState *s4wave_provider_spacewave.OrganizationRootStateInfo
	rootStateSOID := info.GetRootStateSoId()
	if rootStateSOID != "" {
		health, err := r.parent.loadSharedObjectHealthSnapshot(ctx, rootStateSOID)
		if err != nil {
			return nil, err
		}
		rootState = buildOrganizationRootStateInfo(rootStateSOID, health, roleID)
	}

	return &s4wave_provider_spacewave.WatchOrganizationStateResponse{
		Organization: &s4wave_provider_spacewave.OrganizationInfo{
			Id:               info.GetId(),
			DisplayName:      info.GetDisplayName(),
			Role:             roleID,
			BillingAccountId: info.GetBillingAccountId(),
		},
		Members:   members,
		Spaces:    spaces,
		Invites:   invites,
		RootState: rootState,
	}, nil
}

func organizationRootMutationDisabledReason(roleID string) string {
	if roleID == "org:owner" || roleID == "owner" {
		return ""
	}
	return "Only organization owners can repair or reinitialize this shared object."
}

func buildOrganizationRootStateInfo(
	sharedObjectID string,
	health *sobject.SharedObjectHealth,
	roleID string,
) *s4wave_provider_spacewave.OrganizationRootStateInfo {
	if sharedObjectID == "" {
		return nil
	}
	canMutate := roleID == "org:owner" || roleID == "owner"
	return &s4wave_provider_spacewave.OrganizationRootStateInfo{
		SharedObjectId: sharedObjectID,
		Health:         health,
		MutationPermission: &s4wave_provider_spacewave.SharedObjectMutationPermission{
			CanRepair:       canMutate,
			CanReinitialize: canMutate,
			DisabledReason:  organizationRootMutationDisabledReason(roleID),
		},
	}
}

// DeleteOrganization deletes an organization.
func (r *SpacewaveSessionResource) DeleteOrganization(
	ctx context.Context,
	req *s4wave_provider_spacewave.DeleteOrganizationRequest,
) (*s4wave_provider_spacewave.DeleteOrganizationResponse, error) {
	orgID := req.GetOrgId()
	if orgID == "" {
		return nil, errors.New("org_id is required")
	}

	cli := r.swAcc.GetSessionClient()
	if _, err := cli.DeleteOrganization(ctx, orgID); err != nil {
		return nil, err
	}
	r.refreshOrganizationCaches(ctx, orgID, true)
	return &s4wave_provider_spacewave.DeleteOrganizationResponse{}, nil
}

// CreateOrgInvite creates an invite for an organization.
func (r *SpacewaveSessionResource) CreateOrgInvite(
	ctx context.Context,
	req *s4wave_provider_spacewave.CreateOrgInviteRequest,
) (*s4wave_provider_spacewave.CreateOrgInviteResponse, error) {
	cli := r.swAcc.GetSessionClient()
	data, err := cli.CreateOrgInvite(ctx, req.GetOrgId(), req.GetType(), req.GetMaxUses(), req.GetExpiresAt(), req.GetEmail())
	if err != nil {
		return nil, err
	}
	var inv api.OrgInviteResponse
	if err := inv.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal invite")
	}
	// Mirror to local org SO.
	var inviteType s4wave_org.OrgInviteType
	switch req.GetType() {
	case "code":
		inviteType = s4wave_org.OrgInviteType_ORG_INVITE_TYPE_CODE
	case "email":
		inviteType = s4wave_org.OrgInviteType_ORG_INVITE_TYPE_EMAIL
	default:
		inviteType = s4wave_org.OrgInviteType_ORG_INVITE_TYPE_LINK
	}
	createInviteOp := &s4wave_org.CreateOrgInviteOp{
		Type:      inviteType,
		MaxUses:   uint32(req.GetMaxUses()),
		Config:    req.GetEmail(),
		Timestamp: timestamppb.Now(),
	}
	if req.GetExpiresAt() > 0 {
		createInviteOp.ExpiresAt = timestamppb.New(time.UnixMilli(req.GetExpiresAt()))
	}
	r.refreshOrganizationCaches(ctx, req.GetOrgId(), false)
	r.queueOrgUpdateOp(ctx, req.GetOrgId(), &s4wave_org.UpdateOrgOp{
		OrgObjectKey: s4wave_org.OrgObjectKey,
		Body: &s4wave_org.UpdateOrgOp_CreateInvite{
			CreateInvite: createInviteOp,
		},
	})

	resp := &s4wave_provider_spacewave.CreateOrgInviteResponse{
		Invite: &s4wave_provider_spacewave.OrgInviteInfo{
			Id:        inv.GetId(),
			Type:      inv.GetType(),
			Token:     inv.GetToken(),
			Uses:      inv.GetUses(),
			MaxUses:   inv.GetMaxUses(),
			ExpiresAt: inv.GetExpiresAt(),
		},
	}
	return resp, nil
}

// JoinOrganization joins an organization via invite token.
func (r *SpacewaveSessionResource) JoinOrganization(
	ctx context.Context,
	req *s4wave_provider_spacewave.JoinOrganizationRequest,
) (*s4wave_provider_spacewave.JoinOrganizationResponse, error) {
	cli := r.swAcc.GetSessionClient()
	data, err := cli.JoinOrganization(ctx, req.GetToken())
	if err != nil {
		return nil, err
	}
	var info api.OrgResponse
	if err := info.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal org")
	}

	// Refresh SO list so the org SO appears immediately for the joiner.
	if err := r.swAcc.RefreshSharedObjectList(ctx); err != nil {
		r.le.WithError(err).Warn("failed to refresh SO list after join")
	}
	r.refreshOrganizationCaches(ctx, info.GetId(), true)

	// Mirror join to local org SO.
	r.queueOrgUpdateOp(ctx, info.GetId(), &s4wave_org.UpdateOrgOp{
		OrgObjectKey: s4wave_org.OrgObjectKey,
		Body: &s4wave_org.UpdateOrgOp_JoinViaInvite{
			JoinViaInvite: &s4wave_org.JoinOrgViaInviteOp{
				Token:     req.GetToken(),
				AccountId: r.swAcc.GetAccountID(),
				Timestamp: timestamppb.Now(),
			},
		},
	})

	return &s4wave_provider_spacewave.JoinOrganizationResponse{
		Organization: &s4wave_provider_spacewave.OrganizationInfo{
			Id:          info.GetId(),
			DisplayName: info.GetDisplayName(),
			Role:        info.GetRole(),
		},
	}, nil
}

// RevokeOrgInvite revokes an invite by ID.
func (r *SpacewaveSessionResource) RevokeOrgInvite(
	ctx context.Context,
	req *s4wave_provider_spacewave.RevokeOrgInviteRequest,
) (*s4wave_provider_spacewave.RevokeOrgInviteResponse, error) {
	orgID := req.GetOrgId()
	if orgID == "" {
		return nil, errors.New("org_id is required")
	}
	inviteID := req.GetInviteId()
	if inviteID == "" {
		return nil, errors.New("invite_id is required")
	}

	cli := r.swAcc.GetSessionClient()
	if _, err := cli.RevokeOrgInvite(ctx, orgID, inviteID); err != nil {
		return nil, err
	}
	r.refreshOrganizationCaches(ctx, orgID, false)

	r.queueOrgUpdateOp(ctx, orgID, &s4wave_org.UpdateOrgOp{
		OrgObjectKey: s4wave_org.OrgObjectKey,
		Body: &s4wave_org.UpdateOrgOp_RevokeInvite{
			RevokeInvite: &s4wave_org.RevokeOrgInviteOp{
				InviteId: inviteID,
			},
		},
	})

	return &s4wave_provider_spacewave.RevokeOrgInviteResponse{}, nil
}

// LeaveOrganization leaves an organization.
func (r *SpacewaveSessionResource) LeaveOrganization(
	ctx context.Context,
	req *s4wave_provider_spacewave.LeaveOrganizationRequest,
) (*s4wave_provider_spacewave.LeaveOrganizationResponse, error) {
	orgID := req.GetOrgId()
	if orgID == "" {
		return nil, errors.New("org_id is required")
	}

	cli := r.swAcc.GetSessionClient()
	if _, err := cli.LeaveOrganization(ctx, orgID); err != nil {
		return nil, err
	}
	r.refreshOrganizationCaches(ctx, orgID, true)

	r.queueOrgUpdateOp(ctx, orgID, &s4wave_org.UpdateOrgOp{
		OrgObjectKey: s4wave_org.OrgObjectKey,
		Body: &s4wave_org.UpdateOrgOp_RemoveMember{
			RemoveMember: &s4wave_org.RemoveOrgMember{
				AccountId: r.swAcc.GetAccountID(),
			},
		},
	})

	return &s4wave_provider_spacewave.LeaveOrganizationResponse{}, nil
}

// RemoveOrgMember removes a member from an organization.
func (r *SpacewaveSessionResource) RemoveOrgMember(
	ctx context.Context,
	req *s4wave_provider_spacewave.RemoveOrgMemberRequest,
) (*s4wave_provider_spacewave.RemoveOrgMemberResponse, error) {
	orgID := req.GetOrgId()
	if orgID == "" {
		return nil, errors.New("org_id is required")
	}
	memberID := req.GetMemberId()
	if memberID == "" {
		return nil, errors.New("member_id is required")
	}

	cli := r.swAcc.GetSessionClient()
	if _, err := cli.RemoveOrgMember(ctx, orgID, memberID); err != nil {
		return nil, err
	}
	r.refreshOrganizationCaches(ctx, orgID, true)

	r.queueOrgUpdateOp(ctx, orgID, &s4wave_org.UpdateOrgOp{
		OrgObjectKey: s4wave_org.OrgObjectKey,
		Body: &s4wave_org.UpdateOrgOp_RemoveMember{
			RemoveMember: &s4wave_org.RemoveOrgMember{
				AccountId: memberID,
			},
		},
	})

	return &s4wave_provider_spacewave.RemoveOrgMemberResponse{}, nil
}

// UpdateOrganization updates an organization's display name.
func (r *SpacewaveSessionResource) UpdateOrganization(
	ctx context.Context,
	req *s4wave_provider_spacewave.UpdateOrganizationRequest,
) (*s4wave_provider_spacewave.UpdateOrganizationResponse, error) {
	orgID := req.GetOrgId()
	if orgID == "" {
		return nil, errors.New("org_id is required")
	}
	displayName := req.GetDisplayName()
	if displayName == "" {
		return nil, errors.New("display_name is required")
	}

	cli := r.swAcc.GetSessionClient()
	if _, err := cli.UpdateOrganization(ctx, orgID, displayName); err != nil {
		return nil, err
	}
	r.refreshOrganizationCaches(ctx, orgID, true)

	r.queueOrgUpdateOp(ctx, orgID, &s4wave_org.UpdateOrgOp{
		OrgObjectKey: s4wave_org.OrgObjectKey,
		Body: &s4wave_org.UpdateOrgOp_UpdateDisplayName{
			UpdateDisplayName: &s4wave_org.UpdateOrgDisplayName{
				DisplayName: displayName,
			},
		},
	})

	return &s4wave_provider_spacewave.UpdateOrganizationResponse{}, nil
}

// TransferResource transfers a resource to a typed principal owner.
func (r *SpacewaveSessionResource) TransferResource(
	ctx context.Context,
	req *s4wave_provider_spacewave.TransferResourceRequest,
) (*s4wave_provider_spacewave.TransferResourceResponse, error) {
	cli := r.swAcc.GetSessionClient()
	_, err := cli.TransferResource(
		ctx,
		req.GetResourceId(),
		req.GetNewOwnerType(),
		req.GetNewOwnerId(),
	)
	if err != nil {
		return nil, err
	}
	return &s4wave_provider_spacewave.TransferResourceResponse{}, nil
}

// RepairSharedObject retries owner-side repair for a broken shared object.
func (r *SpacewaveSessionResource) RepairSharedObject(
	ctx context.Context,
	req *s4wave_provider_spacewave.RepairSharedObjectRequest,
) (*s4wave_provider_spacewave.RepairSharedObjectResponse, error) {
	sharedObjectID := req.GetSharedObjectId()
	if sharedObjectID == "" {
		return nil, errors.New("shared object id is required")
	}
	if err := r.swAcc.RepairSharedObject(ctx, sharedObjectID); err != nil {
		return nil, err
	}
	return &s4wave_provider_spacewave.RepairSharedObjectResponse{}, nil
}

// ReinitializeSharedObject destructively rewrites a broken shared object in place.
func (r *SpacewaveSessionResource) ReinitializeSharedObject(
	ctx context.Context,
	req *s4wave_provider_spacewave.ReinitializeSharedObjectRequest,
) (*s4wave_provider_spacewave.ReinitializeSharedObjectResponse, error) {
	sharedObjectID := req.GetSharedObjectId()
	if sharedObjectID == "" {
		return nil, errors.New("shared object id is required")
	}
	if err := r.swAcc.ReinitializeSharedObject(ctx, sharedObjectID); err != nil {
		return nil, err
	}
	r.refreshOrganizationCaches(ctx, sharedObjectID, true)
	return &s4wave_provider_spacewave.ReinitializeSharedObjectResponse{}, nil
}

// MountSharedObjectSelfEnrollment mounts the self-enrollment resource.
func (r *SpacewaveSessionResource) MountSharedObjectSelfEnrollment(
	ctx context.Context,
	req *s4wave_session.MountSharedObjectSelfEnrollmentRequest,
) (*s4wave_session.MountSharedObjectSelfEnrollmentResponse, error) {
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}
	res := NewSharedObjectSelfEnrollmentResource(r.swAcc)
	id, err := resourceCtx.AddResource(res.GetMux(), func() {})
	if err != nil {
		return nil, err
	}
	return &s4wave_session.MountSharedObjectSelfEnrollmentResponse{ResourceId: id}, nil
}

// CreateBillingAccount creates a new unassigned billing account managed by the caller.
func (r *SpacewaveSessionResource) CreateBillingAccount(
	ctx context.Context,
	req *s4wave_provider_spacewave.CreateBillingAccountRequest,
) (*s4wave_provider_spacewave.CreateBillingAccountResponse, error) {
	cli := r.swAcc.GetSessionClient()
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
	cli := r.swAcc.GetSessionClient()
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
	cli := r.swAcc.GetSessionClient()
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
	cli := r.swAcc.GetSessionClient()
	_, err := cli.AssignBillingAccount(
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
	cli := r.swAcc.GetSessionClient()
	_, err := cli.DetachBillingAccount(
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

// resolveMemberPeersAndMountSO resolves an account's session peers and mounts
// the space's shared object. Caller must defer releaseFn.
func (r *SpacewaveSessionResource) resolveMemberPeersAndMountSO(
	ctx context.Context,
	spaceID, accountID string,
) (*provider_spacewave.SharedObject, func(), []*api.EnrollMemberPeer, error) {
	cli := r.swAcc.GetSessionClient()
	enrollResp, err := cli.EnrollMember(ctx, spaceID, accountID, true)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "resolve member peers")
	}

	peers := enrollResp.GetPeers()
	if len(peers) == 0 {
		return nil, func() {}, nil, nil
	}

	swSO, rel, err := r.mountSpaceSO(ctx, spaceID)
	if err != nil {
		return nil, nil, nil, err
	}

	return swSO, rel, peers, nil
}

// resolveMemberParticipantPeersAndMountSO resolves an account's existing SO
// participant peers and mounts the space's shared object. Caller must defer
// releaseFn.
func (r *SpacewaveSessionResource) resolveMemberParticipantPeersAndMountSO(
	ctx context.Context,
	spaceID, accountID string,
) (*provider_spacewave.SharedObject, func(), []string, error) {
	cli := r.swAcc.GetSessionClient()
	resolveResp, err := cli.ResolveMemberParticipants(ctx, spaceID, accountID)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "resolve member participant peers")
	}

	peerIDs := resolveResp.GetPeerIds()
	if len(peerIDs) == 0 {
		return nil, func() {}, nil, nil
	}

	swSO, rel, err := r.mountSpaceSO(ctx, spaceID)
	if err != nil {
		return nil, nil, nil, err
	}

	return swSO, rel, peerIDs, nil
}

// EnrollSpaceMember enrolls an org member into a space by adding them as a participant.
func (r *SpacewaveSessionResource) EnrollSpaceMember(
	ctx context.Context,
	req *s4wave_provider_spacewave.EnrollSpaceMemberRequest,
) (*s4wave_provider_spacewave.EnrollSpaceMemberResponse, error) {
	spaceID := req.GetSpaceId()
	if spaceID == "" {
		return nil, errors.New("space_id is required")
	}
	accountID := req.GetAccountId()
	if accountID == "" {
		return nil, errors.New("account_id is required")
	}

	swSO, relSO, peers, err := r.resolveMemberPeersAndMountSO(ctx, spaceID, accountID)
	if err != nil {
		return nil, err
	}
	defer relSO()
	if len(peers) == 0 {
		return &s4wave_provider_spacewave.EnrollSpaceMemberResponse{}, nil
	}

	role := req.GetRole()
	state, err := swSO.GetSOHost().GetHostState(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "get current SO state")
	}
	existingRoles := make(map[string]sobject.SOParticipantRole)
	for _, participant := range state.GetConfig().GetParticipants() {
		peerID := participant.GetPeerId()
		if peerID == "" {
			continue
		}
		existingRoles[peerID] = participant.GetRole()
	}
	results := make([]*s4wave_provider_spacewave.EnrollSpaceMemberResult, 0, len(peers))
	for _, p := range peers {
		peerID := p.GetPeerId()
		result := &s4wave_provider_spacewave.EnrollSpaceMemberResult{PeerId: peerID}

		targetPub, err := session.ExtractPublicKeyFromPeerID(peerID)
		if err != nil {
			result.Error = errors.Wrap(err, "extract pubkey").Error()
			results = append(results, result)
			continue
		}

		participantRole := role
		if existingRole, ok := existingRoles[peerID]; ok {
			if existingRole > participantRole {
				participantRole = existingRole
			}
		}
		grant, err := swSO.AddParticipant(ctx, peerID, targetPub, participantRole, accountID)
		if err != nil {
			result.Error = err.Error()
			results = append(results, result)
			continue
		}

		if grant == nil {
			result.AlreadyParticipant = true
		} else {
			result.Enrolled = true
		}
		results = append(results, result)
	}

	return &s4wave_provider_spacewave.EnrollSpaceMemberResponse{Results: results}, nil
}

// RemoveSpaceMember removes an org member from a space by removing them as a participant.
func (r *SpacewaveSessionResource) RemoveSpaceMember(
	ctx context.Context,
	req *s4wave_provider_spacewave.RemoveSpaceMemberRequest,
) (*s4wave_provider_spacewave.RemoveSpaceMemberResponse, error) {
	spaceID := req.GetSpaceId()
	if spaceID == "" {
		return nil, errors.New("space_id is required")
	}
	accountID := req.GetAccountId()
	if accountID == "" {
		return nil, errors.New("account_id is required")
	}

	swSO, relSO, peerIDs, err := r.resolveMemberParticipantPeersAndMountSO(ctx, spaceID, accountID)
	if err != nil {
		return nil, err
	}
	defer relSO()
	if len(peerIDs) == 0 {
		return nil, errors.New("no participant peers found for account")
	}

	results := make([]*s4wave_provider_spacewave.RemoveSpaceMemberResult, 0, len(peerIDs))
	for _, peerID := range peerIDs {
		result := &s4wave_provider_spacewave.RemoveSpaceMemberResult{PeerId: peerID}

		revInfo := &sobject.SORevocationInfo{
			Reason: sobject.SORevocationReason_SO_REVOCATION_REASON_OWNER_REMOVED,
		}
		removed, err := swSO.RemoveParticipantWithRevocation(ctx, peerID, revInfo)
		if err != nil {
			result.Error = err.Error()
			results = append(results, result)
			continue
		}

		if removed {
			result.Removed = true
		} else {
			result.NotParticipant = true
		}
		results = append(results, result)
	}

	return &s4wave_provider_spacewave.RemoveSpaceMemberResponse{Results: results}, nil
}

// mountSpaceSO mounts a space shared object by ID and returns the typed SO.
// Caller must defer releaseFn.
func (r *SpacewaveSessionResource) mountSpaceSO(
	ctx context.Context,
	spaceID string,
) (*provider_spacewave.SharedObject, func(), error) {
	ref := sobject.NewSharedObjectRef(
		r.swAcc.GetProviderID(),
		r.swAcc.GetAccountID(),
		spaceID,
		provider_spacewave.SobjectBlockStoreID(spaceID),
	)
	so, rel, err := r.swAcc.MountSharedObject(ctx, ref, nil)
	if err != nil {
		return nil, nil, errors.Wrap(err, "mount shared object")
	}
	swSO, ok := so.(*provider_spacewave.SharedObject)
	if !ok {
		rel()
		return nil, nil, errors.New("unexpected shared object type")
	}
	return swSO, rel, nil
}

// ResetSession resets a PIN-locked session.
func (r *SpacewaveSessionResource) ResetSession(
	ctx context.Context,
	req *s4wave_provider_spacewave.ResetSessionRequest,
) (*s4wave_provider_spacewave.ResetSessionResponse, error) {
	cred := req.GetCredential()
	if cred == nil {
		return nil, errors.New("credential is required")
	}

	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, r.b, "", false, nil)
	if err != nil {
		return nil, err
	}
	defer sessionCtrlRef.Release()

	sessInfo, err := sessionCtrl.GetSessionByIdx(ctx, req.GetSessionIdx())
	if err != nil {
		return nil, err
	}
	if sessInfo == nil {
		return nil, session.ErrSessionNotFound
	}

	ref := sessInfo.GetSessionRef()
	provRef := ref.GetProviderResourceRef()

	provAcc, provAccRef, err := provider.ExAccessProviderAccount(
		ctx, r.b,
		provRef.GetProviderId(),
		provRef.GetProviderAccountId(),
		false, nil,
	)
	if err != nil {
		return nil, err
	}
	defer provAccRef.Release()

	accResource := resource_account.NewAccountResource(provAcc)
	if accResource == nil {
		return nil, errors.New("account resource not available for this provider")
	}
	defer accResource.Release()
	if _, _, err := accResource.ResolveEntityKey(ctx, cred); err != nil {
		return nil, errors.Wrap(err, "verify credential")
	}

	sessFeature, err := session.GetSessionProviderAccountFeature(ctx, provAcc)
	if err != nil {
		return nil, err
	}

	if err := sessFeature.ResetPINSession(ctx, ref, nil); err != nil {
		return nil, err
	}

	return &s4wave_provider_spacewave.ResetSessionResponse{}, nil
}

// EncryptForHandoff encrypts the active session privkey to a device pubkey.
func (r *SpacewaveSessionResource) EncryptForHandoff(
	ctx context.Context,
	req *s4wave_provider_spacewave.EncryptForHandoffRequest,
) (*s4wave_provider_spacewave.EncryptForHandoffResponse, error) {
	devicePubRaw := req.GetDevicePublicKey()
	if len(devicePubRaw) == 0 {
		return nil, errors.New("device_public_key is required")
	}
	nonce := req.GetSessionNonce()
	if nonce == "" {
		return nil, errors.New("session_nonce is required")
	}

	sessRef := r.getSessionRef()
	sess, relSess, err := r.swAcc.MountSession(ctx, sessRef, nil)
	if err != nil {
		return nil, errors.Wrap(err, "mount session")
	}
	defer relSess()

	sessionPrivKey := sess.GetPrivKey()
	if sessionPrivKey == nil {
		return nil, errors.New("session is locked")
	}

	privPEM, err := keypem.MarshalPrivKeyPem(sessionPrivKey)
	if err != nil {
		return nil, errors.Wrap(err, "marshal session privkey")
	}
	defer scrub.Scrub(privPEM)

	devicePubKey, err := crypto.UnmarshalEd25519PublicKey(devicePubRaw)
	if err != nil {
		return nil, errors.Wrap(err, "parse device public key")
	}

	encrypted, err := peer.EncryptToPubKey(devicePubKey, session_handoff.EncryptContext, privPEM)
	if err != nil {
		return nil, errors.Wrap(err, "encrypt session key")
	}

	info, err := r.swAcc.GetAccountState(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "get account info")
	}

	// Build HandoffCompletion and relay to AuthSessionDO.
	completion := &session_handoff.HandoffCompletion{
		EncryptedSessionKey: encrypted,
		AccountId:           info.AccountId,
		EntityId:            info.EntityId,
	}
	completionData, err := completion.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal handoff completion")
	}

	endpoint := r.swAcc.GetProvider().GetEndpoint()
	completeURL := endpoint + "/api/auth/session/" + nonce + "/complete"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, completeURL, bytes.NewReader(completionData))
	if err != nil {
		return nil, errors.Wrap(err, "build completion request")
	}
	httpReq.Header.Set("Content-Type", "application/octet-stream")
	httpResp, err := r.swAcc.GetProvider().GetHTTPClient().Do(httpReq)
	if err != nil {
		return nil, errors.Wrap(err, "relay handoff completion")
	}
	defer alpha_nethttp.DrainAndCloseResponseBody(httpResp)
	if httpResp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("handoff completion relay failed: %d", httpResp.StatusCode)
	}

	return &s4wave_provider_spacewave.EncryptForHandoffResponse{
		EncryptedSessionKey: encrypted,
		AccountId:           info.AccountId,
		EntityId:            info.EntityId,
	}, nil
}

// mapCheckoutStatus maps a checkout status string to the CheckoutStatus enum.
func mapCheckoutStatus(s string) s4wave_provider_spacewave.CheckoutStatus {
	switch s {
	case "pending":
		return s4wave_provider_spacewave.CheckoutStatus_CheckoutStatus_PENDING
	case "completed":
		return s4wave_provider_spacewave.CheckoutStatus_CheckoutStatus_COMPLETED
	case "expired":
		return s4wave_provider_spacewave.CheckoutStatus_CheckoutStatus_EXPIRED
	case "canceled":
		return s4wave_provider_spacewave.CheckoutStatus_CheckoutStatus_CANCELED
	default:
		return s4wave_provider_spacewave.CheckoutStatus_CheckoutStatus_UNKNOWN
	}
}

// storageBaselineBytes is the included storage baseline (100 GB).
const storageBaselineBytes = 100 * 1024 * 1024 * 1024

// writeOpsBaseline is the included write ops baseline per period (1M).
const writeOpsBaseline = 1000000

// readOpsBaseline is the included read ops baseline per period (10M).
const readOpsBaseline = 10000000

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
	cli := r.swAcc.GetSessionClient()
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
	cli := r.swAcc.GetSessionClient()
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
	cli := r.swAcc.GetSessionClient()
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
	cli := r.swAcc.GetSessionClient()
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
	cli := r.swAcc.GetSessionClient()
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

// LookupInviteCode resolves a short invite code to the full SOInviteMessage.
func (r *SpacewaveSessionResource) LookupInviteCode(
	ctx context.Context,
	req *s4wave_provider_spacewave.LookupInviteCodeRequest,
) (*s4wave_provider_spacewave.LookupInviteCodeResponse, error) {
	code := req.GetCode()
	if code == "" {
		return nil, errors.New("code is required")
	}

	cli := r.swAcc.GetSessionClient()
	lookupResp, err := cli.LookupInviteCode(ctx, code)
	if err != nil {
		return nil, err
	}

	// Decode the base64 invite message to SOInviteMessage proto.
	inviteMsgBytes, err := base64.StdEncoding.DecodeString(lookupResp.GetInviteMessage())
	if err != nil {
		return nil, errors.Wrap(err, "decode invite message")
	}
	inviteMsg := &sobject.SOInviteMessage{}
	if err := inviteMsg.UnmarshalVT(inviteMsgBytes); err != nil {
		return nil, errors.Wrap(err, "unmarshal invite message")
	}

	return &s4wave_provider_spacewave.LookupInviteCodeResponse{
		InviteId:      lookupResp.GetInviteId(),
		InviteMessage: inviteMsg,
	}, nil
}

// ProcessMailboxEntry accepts or rejects a mailbox entry.
func (r *SpacewaveSessionResource) ProcessMailboxEntry(
	ctx context.Context,
	req *s4wave_provider_spacewave.ProcessMailboxEntryRequest,
) (*s4wave_provider_spacewave.ProcessMailboxEntryResponse, error) {
	spaceID := req.GetSpaceId()
	if spaceID == "" {
		return nil, errors.New("space_id is required")
	}
	entryID := req.GetEntryId()
	if entryID == 0 {
		return nil, errors.New("entry_id is required")
	}

	if err := r.swAcc.ProcessMailboxEntry(ctx, spaceID, entryID, req.GetAccept()); err != nil {
		return nil, err
	}

	return &s4wave_provider_spacewave.ProcessMailboxEntryResponse{}, nil
}

// PreviewSpaceLink verifies a SpaceLink ticket for trusted UI display.
func (r *SpacewaveSessionResource) PreviewSpaceLink(
	ctx context.Context,
	req *s4wave_provider_spacewave.PreviewSpaceLinkRequest,
) (*s4wave_provider_spacewave.PreviewSpaceLinkResponse, error) {
	if _, err := unmarshalSpaceLinkAuthTicket(req.GetTicket()); err != nil {
		return nil, err
	}
	return nil, errors.New("spacelink preview is not implemented")
}

// ApproveSpaceLink approves a SpaceLink ticket for a target Space.
func (r *SpacewaveSessionResource) ApproveSpaceLink(
	ctx context.Context,
	req *s4wave_provider_spacewave.ApproveSpaceLinkRequest,
) (*s4wave_provider_spacewave.ApproveSpaceLinkResponse, error) {
	if _, err := unmarshalSpaceLinkAuthTicket(req.GetTicket()); err != nil {
		return nil, err
	}
	return nil, errors.New("spacelink approval is not implemented")
}

func unmarshalSpaceLinkAuthTicket(data []byte) (*s4wave_provider_spacewave.SpaceLinkAuthTicket, error) {
	if len(data) == 0 {
		return nil, errors.New("ticket is required")
	}
	ticket := &s4wave_provider_spacewave.SpaceLinkAuthTicket{}
	if err := ticket.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal spacelink auth ticket")
	}
	if len(ticket.GetPayload()) == 0 {
		return nil, errors.New("ticket payload is required")
	}
	if len(ticket.GetAgentSignature()) == 0 {
		return nil, errors.New("ticket agent signature is required")
	}
	return ticket, nil
}

// _ is a type assertion
var _ s4wave_session.SRPCSpacewaveSessionResourceServiceServer = ((*SpacewaveSessionResource)(nil))
