package provider_spacewave

import (
	"context"

	provider "github.com/s4wave/spacewave/core/provider"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

// ManagedBillingSummary carries route-level managed billing account counts.
type ManagedBillingSummary struct {
	ManagedBaCount               uint32
	ManagedActiveBaCount         uint32
	ManagedNoSubscriptionBaCount uint32
	BillingSummaryLoaded         bool
}

// OnboardingStatusProjectionContext carries session-scoped Onboarding Status
// state that is not owned by ProviderAccount.
type OnboardingStatusProjectionContext struct {
	HasLinkedLocal          bool
	LinkedLocalSessionIndex uint32
	LinkedLocalHasContent   bool
	HasLinkedCloud          bool
	LinkedCloudSessionIndex uint32
}

// BuildOnboardingStatusProjection builds the session Onboarding Status
// projection from cached Provider Account state plus explicit session context.
func (a *ProviderAccount) BuildOnboardingStatusProjection(
	ctx context.Context,
	projCtx OnboardingStatusProjectionContext,
) (*s4wave_provider_spacewave.WatchOnboardingStatusResponse, error) {
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

	managedSummary, err := a.BuildManagedBillingSummary(ctx, accountStatus)
	if err != nil {
		managedSummary = ManagedBillingSummary{}
	}

	billingStatus := subStatus
	hasSubscription := billingStatus == s4wave_provider_spacewave.BillingStatus_BillingStatus_ACTIVE ||
		billingStatus == s4wave_provider_spacewave.BillingStatus_BillingStatus_TRIALING

	resp := &s4wave_provider_spacewave.WatchOnboardingStatusResponse{
		HasSubscription:              hasSubscription,
		SubscriptionStatus:           billingStatus,
		HasLinkedLocal:               projCtx.HasLinkedLocal,
		LinkedLocalSessionIndex:      projCtx.LinkedLocalSessionIndex,
		LinkedLocalHasContent:        projCtx.LinkedLocalHasContent,
		HasLinkedCloud:               projCtx.HasLinkedCloud,
		LinkedCloudSessionIndex:      projCtx.LinkedCloudSessionIndex,
		CheckoutInProgress:           a.checkoutWatcher.HasTicket(),
		CancelAt:                     cancelAt,
		DeleteAt:                     deleteAt,
		LifecycleUpdatedAt:           lifecycleUpdatedAt,
		DeletedAt:                    deletedAt,
		EmailVerified:                emailVerified,
		LifecycleState:               lifecycleState,
		AccountStatus:                accountStatus,
		ManagedBaCount:               managedSummary.ManagedBaCount,
		ManagedActiveBaCount:         managedSummary.ManagedActiveBaCount,
		ManagedNoSubscriptionBaCount: managedSummary.ManagedNoSubscriptionBaCount,
		BillingSummaryLoaded:         managedSummary.BillingSummaryLoaded,
	}
	if selfEnrollmentSummary != nil {
		resp.SessionSelfEnrollmentGenerationKey = selfEnrollmentSummary.GetGenerationKey()
		resp.SessionSelfEnrollmentCount = selfEnrollmentSummary.GetCount()
	}
	resp.SelfEnrollmentGateState = selfEnrollmentGateState(
		selfEnrollmentSummary,
		selfEnrollmentAutoRejoinRunning,
	)
	return resp, err
}

// ShouldLoadManagedBillingSummary returns whether route-status projection should
// query the managed billing-account summary.
func ShouldLoadManagedBillingSummary(accountStatus provider.ProviderAccountStatus) bool {
	return accountStatus == provider.ProviderAccountStatus_ProviderAccountStatus_READY
}

// BuildManagedBillingSummary builds managed billing route counts from the
// Provider Account-owned managed billing account cache.
func (a *ProviderAccount) BuildManagedBillingSummary(
	ctx context.Context,
	accountStatus provider.ProviderAccountStatus,
) (ManagedBillingSummary, error) {
	if !ShouldLoadManagedBillingSummary(accountStatus) {
		return ManagedBillingSummary{}, nil
	}
	managedBAs, err := a.GetManagedBAsSnapshot(ctx)
	if err != nil {
		return ManagedBillingSummary{}, err
	}
	summary := ManagedBillingSummary{BillingSummaryLoaded: true}
	for _, ba := range managedBAs {
		summary.ManagedBaCount++
		switch ba.GetSubscriptionStatus() {
		case s4wave_provider_spacewave.BillingStatus_BillingStatus_ACTIVE,
			s4wave_provider_spacewave.BillingStatus_BillingStatus_TRIALING:
			summary.ManagedActiveBaCount++
		case s4wave_provider_spacewave.BillingStatus_BillingStatus_NONE:
			summary.ManagedNoSubscriptionBaCount++
		}
	}
	return summary, nil
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
