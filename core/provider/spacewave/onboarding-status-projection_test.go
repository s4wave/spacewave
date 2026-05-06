package provider_spacewave

import (
	"testing"

	provider "github.com/s4wave/spacewave/core/provider"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	"github.com/sirupsen/logrus"
)

func TestBuildOnboardingStatusProjectionReadySnapshot(t *testing.T) {
	acc := newProjectionTestAccount(t)
	acc.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		acc.state.info = &api.AccountStateResponse{
			SubscriptionStatus: s4wave_provider_spacewave.BillingStatus_BillingStatus_TRIALING,
			CancelAt:           10,
			DeleteAt:           20,
			LifecycleUpdatedAt: 30,
			DeletedAt:          40,
			EmailVerified:      true,
			LifecycleState:     api.AccountLifecycleState_ACCOUNT_LIFECYCLE_STATE_ACTIVE,
		}
		acc.state.status = provider.ProviderAccountStatus_ProviderAccountStatus_READY
		acc.state.selfEnrollmentSummary = &SelfEnrollmentSummary{
			generationKey: "gen-1",
			count:         2,
			loaded:        true,
		}
	})
	acc.checkoutWatcher.SetTicket("checkout-ticket")

	got := acc.BuildOnboardingStatusProjection(OnboardingStatusProjectionContext{
		HasLinkedLocal:               true,
		LinkedLocalSessionIndex:      3,
		LinkedLocalHasContent:        true,
		HasLinkedCloud:               true,
		LinkedCloudSessionIndex:      4,
		ManagedBaCount:               5,
		ManagedActiveBaCount:         1,
		ManagedNoSubscriptionBaCount: 2,
		BillingSummaryLoaded:         true,
	})

	expected := &s4wave_provider_spacewave.WatchOnboardingStatusResponse{
		HasSubscription:                    true,
		SubscriptionStatus:                 s4wave_provider_spacewave.BillingStatus_BillingStatus_TRIALING,
		HasLinkedLocal:                     true,
		LinkedLocalSessionIndex:            3,
		LinkedLocalHasContent:              true,
		HasLinkedCloud:                     true,
		LinkedCloudSessionIndex:            4,
		CheckoutInProgress:                 true,
		CancelAt:                           10,
		DeleteAt:                           20,
		LifecycleUpdatedAt:                 30,
		DeletedAt:                          40,
		EmailVerified:                      true,
		LifecycleState:                     s4wave_provider_spacewave.AccountLifecycleState_AccountLifecycleState_ACTIVE,
		AccountStatus:                      provider.ProviderAccountStatus_ProviderAccountStatus_READY,
		ManagedBaCount:                     5,
		ManagedActiveBaCount:               1,
		ManagedNoSubscriptionBaCount:       2,
		BillingSummaryLoaded:               true,
		SelfEnrollmentGateState:            s4wave_provider_spacewave.SelfEnrollmentGateState_SELF_ENROLLMENT_GATE_STATE_ACTION_REQUIRED,
		SessionSelfEnrollmentGenerationKey: "gen-1",
		SessionSelfEnrollmentCount:         2,
	}
	if !got.EqualVT(expected) {
		t.Fatalf("unexpected projection:\n got: %+v\nwant: %+v", got, expected)
	}
}

func TestBuildOnboardingStatusProjectionTerminalWithoutAccountState(t *testing.T) {
	acc := newProjectionTestAccount(t)
	acc.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		acc.state.status = provider.ProviderAccountStatus_ProviderAccountStatus_DELETED
	})

	got := acc.BuildOnboardingStatusProjection(OnboardingStatusProjectionContext{})

	expected := &s4wave_provider_spacewave.WatchOnboardingStatusResponse{
		AccountStatus:           provider.ProviderAccountStatus_ProviderAccountStatus_DELETED,
		SelfEnrollmentGateState: s4wave_provider_spacewave.SelfEnrollmentGateState_SELF_ENROLLMENT_GATE_STATE_CHECKING,
	}
	if !got.EqualVT(expected) {
		t.Fatalf("unexpected projection:\n got: %+v\nwant: %+v", got, expected)
	}
}

func TestBuildOnboardingStatusProjectionAutoRejoinGate(t *testing.T) {
	acc := newProjectionTestAccount(t)
	acc.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		acc.state.status = provider.ProviderAccountStatus_ProviderAccountStatus_READY
		acc.state.selfRejoinSweepRunning = true
		acc.state.selfEnrollmentSummary = &SelfEnrollmentSummary{
			loaded: true,
		}
	})

	got := acc.BuildOnboardingStatusProjection(OnboardingStatusProjectionContext{})
	if got.GetSelfEnrollmentGateState() != s4wave_provider_spacewave.SelfEnrollmentGateState_SELF_ENROLLMENT_GATE_STATE_AUTO_CONNECTING {
		t.Fatalf("expected auto-connecting gate, got %v", got.GetSelfEnrollmentGateState())
	}
}

func newProjectionTestAccount(t *testing.T) *ProviderAccount {
	t.Helper()
	return &ProviderAccount{
		checkoutWatcher: newCheckoutWatcher(
			logrus.New().WithField("test", t.Name()),
			nil,
			nil,
		),
	}
}
