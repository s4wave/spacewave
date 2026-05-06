package provider_spacewave

import (
	provider "github.com/s4wave/spacewave/core/provider"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

// OnboardingStatusProjectionContext carries session-scoped Onboarding Status
// state that is not owned by ProviderAccount.
type OnboardingStatusProjectionContext struct {
	HasLinkedLocal               bool
	LinkedLocalSessionIndex      uint32
	LinkedLocalHasContent        bool
	HasLinkedCloud               bool
	LinkedCloudSessionIndex      uint32
	ManagedBaCount               uint32
	ManagedActiveBaCount         uint32
	ManagedNoSubscriptionBaCount uint32
	BillingSummaryLoaded         bool
}

// BuildOnboardingStatusProjection builds the session Onboarding Status
// projection from cached Provider Account state plus explicit session context.
func (a *ProviderAccount) BuildOnboardingStatusProjection(
	ctx OnboardingStatusProjectionContext,
) *s4wave_provider_spacewave.WatchOnboardingStatusResponse {
	var accountStatus provider.ProviderAccountStatus
	var subStatus s4wave_provider_spacewave.BillingStatus
	var cancelAt int64
	var deleteAt int64
	var lifecycleUpdatedAt int64
	var deletedAt int64
	var emailVerified bool
	var lifecycleState s4wave_provider_spacewave.AccountLifecycleState
	var selfEnrollmentSummary *SelfEnrollmentSummary
	var selfEnrollmentAutoRejoinRunning bool
	a.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		accountStatus = a.state.status
		if a.state.selfEnrollmentSummary != nil {
			selfEnrollmentSummary = a.state.selfEnrollmentSummary.clone()
		}
		selfEnrollmentAutoRejoinRunning = a.state.selfRejoinSweepRunning
		state := a.state.info
		if state != nil {
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

	billingStatus := subStatus
	hasSubscription := billingStatus == s4wave_provider_spacewave.BillingStatus_BillingStatus_ACTIVE ||
		billingStatus == s4wave_provider_spacewave.BillingStatus_BillingStatus_TRIALING

	resp := &s4wave_provider_spacewave.WatchOnboardingStatusResponse{
		HasSubscription:              hasSubscription,
		SubscriptionStatus:           billingStatus,
		HasLinkedLocal:               ctx.HasLinkedLocal,
		LinkedLocalSessionIndex:      ctx.LinkedLocalSessionIndex,
		LinkedLocalHasContent:        ctx.LinkedLocalHasContent,
		HasLinkedCloud:               ctx.HasLinkedCloud,
		LinkedCloudSessionIndex:      ctx.LinkedCloudSessionIndex,
		CheckoutInProgress:           a.checkoutWatcher.HasTicket(),
		CancelAt:                     cancelAt,
		DeleteAt:                     deleteAt,
		LifecycleUpdatedAt:           lifecycleUpdatedAt,
		DeletedAt:                    deletedAt,
		EmailVerified:                emailVerified,
		LifecycleState:               lifecycleState,
		AccountStatus:                accountStatus,
		ManagedBaCount:               ctx.ManagedBaCount,
		ManagedActiveBaCount:         ctx.ManagedActiveBaCount,
		ManagedNoSubscriptionBaCount: ctx.ManagedNoSubscriptionBaCount,
		BillingSummaryLoaded:         ctx.BillingSummaryLoaded,
	}
	if selfEnrollmentSummary != nil {
		resp.SessionSelfEnrollmentGenerationKey = selfEnrollmentSummary.GetGenerationKey()
		resp.SessionSelfEnrollmentCount = selfEnrollmentSummary.GetCount()
	}
	resp.SelfEnrollmentGateState = selfEnrollmentGateState(
		selfEnrollmentSummary,
		selfEnrollmentAutoRejoinRunning,
	)
	return resp
}

func selfEnrollmentGateState(
	summary *SelfEnrollmentSummary,
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
