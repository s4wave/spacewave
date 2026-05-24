package provider_spacewave

import (
	"context"

	provider "github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/provider/spacewave/onboardingstatus"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

// ManagedBillingSummary carries managed billing account counts for Onboarding Status.
type ManagedBillingSummary = onboardingstatus.ManagedBillingSummary

// OnboardingStatusProjectionContext carries session-scoped inputs for
// Onboarding Status, the Spacewave cloud session route-status projection served
// by WatchOnboardingStatus. These fields are not owned by ProviderAccount.
type OnboardingStatusProjectionContext = onboardingstatus.ProjectionContext

// BuildOnboardingStatusProjection builds Onboarding Status: the session
// route-status projection from cached Provider Account state plus explicit
// session context. The wire message keeps its historical onboarding name.
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
	var selfEnrollmentSummary onboardingstatus.SelfEnrollmentSummary
	var selfEnrollmentAutoRejoinRunning bool
	a.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		accountStatus = a.state.status
		if a.state.selfEnrollmentSummary != nil {
			summary := a.state.selfEnrollmentSummary.clone()
			selfEnrollmentSummary = onboardingstatus.SelfEnrollmentSummary{
				Present:       true,
				GenerationKey: summary.GetGenerationKey(),
				Count:         summary.GetCount(),
				Loaded:        summary.GetLoaded(),
			}
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
	return onboardingstatus.BuildProjection(onboardingstatus.Snapshot{
		AccountStatus:              accountStatus,
		SubscriptionStatus:         subStatus,
		CancelAt:                   cancelAt,
		DeleteAt:                   deleteAt,
		LifecycleUpdatedAt:         lifecycleUpdatedAt,
		DeletedAt:                  deletedAt,
		EmailVerified:              emailVerified,
		LifecycleState:             lifecycleState,
		SelfEnrollmentSummary:      selfEnrollmentSummary,
		SelfEnrollmentRejoinActive: selfEnrollmentAutoRejoinRunning,
		CheckoutInProgress:         a.checkoutWatcher.HasTicket(),
		ManagedBillingSummary:      managedSummary,
	}, projCtx), err
}

// ShouldLoadManagedBillingSummary returns whether the Onboarding Status
// route-status projection should query the managed billing-account summary.
func ShouldLoadManagedBillingSummary(accountStatus provider.ProviderAccountStatus) bool {
	return onboardingstatus.ShouldLoadManagedBillingSummary(accountStatus)
}

// BuildManagedBillingSummary builds managed billing counts for Onboarding
// Status routing from the Provider Account-owned managed billing account cache.
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
	return onboardingstatus.BuildManagedBillingSummary(managedBAs), nil
}
