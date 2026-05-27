package onboardingstatus

import (
	provider "github.com/s4wave/spacewave/core/provider"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

// ManagedBillingSummary carries managed billing account counts for the
// Onboarding Status route-status projection.
type ManagedBillingSummary struct {
	ManagedBaCount               uint32
	ManagedActiveBaCount         uint32
	ManagedNoSubscriptionBaCount uint32
	BillingSummaryLoaded         bool
}

// ProjectionContext carries session-scoped inputs for Onboarding Status, the
// Spacewave cloud session route-status projection served by WatchOnboardingStatus.
type ProjectionContext struct {
	HasLinkedLocal          bool
	LinkedLocalSessionIndex uint32
	LinkedLocalHasContent   bool
	HasLinkedCloud          bool
	LinkedCloudSessionIndex uint32
}

// SelfEnrollmentSummary carries the self-enrollment gate facts used by the
// route-status projection.
type SelfEnrollmentSummary struct {
	Present       bool
	GenerationKey string
	Count         uint32
	Loaded        bool
}

// Snapshot carries Provider Account state needed by the route-status projection.
type Snapshot struct {
	AccountStatus              provider.ProviderAccountStatus
	SubscriptionStatus         s4wave_provider_spacewave.BillingStatus
	CancelAt                   int64
	DeleteAt                   int64
	LifecycleUpdatedAt         int64
	DeletedAt                  int64
	EmailVerified              bool
	LifecycleState             s4wave_provider_spacewave.AccountLifecycleState
	SelfEnrollmentSummary      SelfEnrollmentSummary
	SelfEnrollmentRejoinActive bool
	CheckoutInProgress         bool
	ManagedBillingSummary      ManagedBillingSummary
}

// BuildProjection builds the Onboarding Status route-status response from a
// Provider Account snapshot plus explicit session context.
func BuildProjection(
	snapshot Snapshot,
	ctx ProjectionContext,
) *s4wave_provider_spacewave.WatchOnboardingStatusResponse {
	billingStatus := snapshot.SubscriptionStatus
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
		CheckoutInProgress:           snapshot.CheckoutInProgress,
		CancelAt:                     snapshot.CancelAt,
		DeleteAt:                     snapshot.DeleteAt,
		LifecycleUpdatedAt:           snapshot.LifecycleUpdatedAt,
		DeletedAt:                    snapshot.DeletedAt,
		EmailVerified:                snapshot.EmailVerified,
		LifecycleState:               snapshot.LifecycleState,
		AccountStatus:                snapshot.AccountStatus,
		ManagedBaCount:               snapshot.ManagedBillingSummary.ManagedBaCount,
		ManagedActiveBaCount:         snapshot.ManagedBillingSummary.ManagedActiveBaCount,
		ManagedNoSubscriptionBaCount: snapshot.ManagedBillingSummary.ManagedNoSubscriptionBaCount,
		BillingSummaryLoaded:         snapshot.ManagedBillingSummary.BillingSummaryLoaded,
		SelfEnrollmentGateState: SelfEnrollmentGateState(
			snapshot.SelfEnrollmentSummary,
			snapshot.SelfEnrollmentRejoinActive,
		),
	}
	if snapshot.SelfEnrollmentSummary.Present {
		resp.SessionSelfEnrollmentGenerationKey = snapshot.SelfEnrollmentSummary.GenerationKey
		resp.SessionSelfEnrollmentCount = snapshot.SelfEnrollmentSummary.Count
	}
	return resp
}

// ShouldLoadManagedBillingSummary returns whether the Onboarding Status
// route-status projection should query the managed billing-account summary.
func ShouldLoadManagedBillingSummary(status provider.ProviderAccountStatus) bool {
	return status == provider.ProviderAccountStatus_ProviderAccountStatus_READY
}

// BuildManagedBillingSummary builds managed billing counts for Onboarding Status.
func BuildManagedBillingSummary(
	accounts []*s4wave_provider_spacewave.ManagedBillingAccount,
) ManagedBillingSummary {
	summary := ManagedBillingSummary{BillingSummaryLoaded: true}
	for _, ba := range accounts {
		summary.ManagedBaCount++
		switch ba.GetSubscriptionStatus() {
		case s4wave_provider_spacewave.BillingStatus_BillingStatus_ACTIVE,
			s4wave_provider_spacewave.BillingStatus_BillingStatus_TRIALING:
			summary.ManagedActiveBaCount++
		case s4wave_provider_spacewave.BillingStatus_BillingStatus_NONE:
			summary.ManagedNoSubscriptionBaCount++
		}
	}
	return summary
}

// SelfEnrollmentGateState derives the route gate state from self-enrollment facts.
func SelfEnrollmentGateState(
	summary SelfEnrollmentSummary,
	autoRejoinRunning bool,
) s4wave_provider_spacewave.SelfEnrollmentGateState {
	if autoRejoinRunning {
		return s4wave_provider_spacewave.SelfEnrollmentGateState_SELF_ENROLLMENT_GATE_STATE_AUTO_CONNECTING
	}
	if !summary.Present || !summary.Loaded {
		return s4wave_provider_spacewave.SelfEnrollmentGateState_SELF_ENROLLMENT_GATE_STATE_CHECKING
	}
	if summary.Count != 0 {
		return s4wave_provider_spacewave.SelfEnrollmentGateState_SELF_ENROLLMENT_GATE_STATE_ACTION_REQUIRED
	}
	return s4wave_provider_spacewave.SelfEnrollmentGateState_SELF_ENROLLMENT_GATE_STATE_READY
}
