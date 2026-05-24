package onboardingstatus

import (
	"testing"

	provider "github.com/s4wave/spacewave/core/provider"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

func TestBuildProjectionReadyCloudRouteStatus(t *testing.T) {
	got := BuildProjection(Snapshot{
		AccountStatus:      provider.ProviderAccountStatus_ProviderAccountStatus_READY,
		SubscriptionStatus: s4wave_provider_spacewave.BillingStatus_BillingStatus_TRIALING,
		CancelAt:           10,
		DeleteAt:           20,
		LifecycleUpdatedAt: 30,
		DeletedAt:          40,
		EmailVerified:      true,
		LifecycleState:     s4wave_provider_spacewave.AccountLifecycleState_AccountLifecycleState_ACTIVE,
		SelfEnrollmentSummary: SelfEnrollmentSummary{
			Present:       true,
			GenerationKey: "gen-1",
			Count:         2,
			Loaded:        true,
		},
		CheckoutInProgress: true,
		ManagedBillingSummary: ManagedBillingSummary{
			BillingSummaryLoaded: true,
		},
	}, ProjectionContext{
		HasLinkedLocal:          true,
		LinkedLocalSessionIndex: 3,
		LinkedLocalHasContent:   true,
		HasLinkedCloud:          true,
		LinkedCloudSessionIndex: 4,
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
		BillingSummaryLoaded:               true,
		SelfEnrollmentGateState:            s4wave_provider_spacewave.SelfEnrollmentGateState_SELF_ENROLLMENT_GATE_STATE_ACTION_REQUIRED,
		SessionSelfEnrollmentGenerationKey: "gen-1",
		SessionSelfEnrollmentCount:         2,
	}
	if !got.EqualVT(expected) {
		t.Fatalf("unexpected projection:\n got: %+v\nwant: %+v", got, expected)
	}
}

func TestBuildProjectionTerminalRouteStatusWithoutAccountSnapshot(t *testing.T) {
	got := BuildProjection(Snapshot{
		AccountStatus: provider.ProviderAccountStatus_ProviderAccountStatus_DELETED,
	}, ProjectionContext{})

	expected := &s4wave_provider_spacewave.WatchOnboardingStatusResponse{
		AccountStatus:           provider.ProviderAccountStatus_ProviderAccountStatus_DELETED,
		SelfEnrollmentGateState: s4wave_provider_spacewave.SelfEnrollmentGateState_SELF_ENROLLMENT_GATE_STATE_CHECKING,
	}
	if !got.EqualVT(expected) {
		t.Fatalf("unexpected projection:\n got: %+v\nwant: %+v", got, expected)
	}
}

func TestBuildProjectionAutoRejoinRouteGate(t *testing.T) {
	got := BuildProjection(Snapshot{
		AccountStatus:              provider.ProviderAccountStatus_ProviderAccountStatus_READY,
		SelfEnrollmentRejoinActive: true,
		SelfEnrollmentSummary: SelfEnrollmentSummary{
			Present: true,
			Loaded:  true,
		},
	}, ProjectionContext{})
	if got.GetSelfEnrollmentGateState() != s4wave_provider_spacewave.SelfEnrollmentGateState_SELF_ENROLLMENT_GATE_STATE_AUTO_CONNECTING {
		t.Fatalf("expected auto-connecting gate, got %v", got.GetSelfEnrollmentGateState())
	}
}

func TestBuildManagedBillingSummaryCountsReadyAccounts(t *testing.T) {
	got := BuildManagedBillingSummary([]*s4wave_provider_spacewave.ManagedBillingAccount{
		{Id: "ba-1", SubscriptionStatus: s4wave_provider_spacewave.BillingStatus_BillingStatus_ACTIVE},
		{Id: "ba-2", SubscriptionStatus: s4wave_provider_spacewave.BillingStatus_BillingStatus_TRIALING},
		{Id: "ba-3", SubscriptionStatus: s4wave_provider_spacewave.BillingStatus_BillingStatus_NONE},
	})
	if got.ManagedBaCount != 3 ||
		got.ManagedActiveBaCount != 2 ||
		got.ManagedNoSubscriptionBaCount != 1 ||
		!got.BillingSummaryLoaded {
		t.Fatalf("unexpected managed billing summary: %+v", got)
	}
}

func TestShouldLoadManagedBillingSummaryForReadyRouteStatus(t *testing.T) {
	if !ShouldLoadManagedBillingSummary(provider.ProviderAccountStatus_ProviderAccountStatus_READY) {
		t.Fatal("expected READY Onboarding Status to load billing summary")
	}
}

func TestShouldLoadManagedBillingSummarySkipsUnauthenticatedRouteStatus(t *testing.T) {
	if ShouldLoadManagedBillingSummary(provider.ProviderAccountStatus_ProviderAccountStatus_UNAUTHENTICATED) {
		t.Fatal("expected UNAUTHENTICATED Onboarding Status to skip billing summary")
	}
}

func TestShouldLoadManagedBillingSummarySkipsDormantRouteStatus(t *testing.T) {
	if ShouldLoadManagedBillingSummary(provider.ProviderAccountStatus_ProviderAccountStatus_DORMANT) {
		t.Fatal("expected DORMANT Onboarding Status to skip billing summary")
	}
}
