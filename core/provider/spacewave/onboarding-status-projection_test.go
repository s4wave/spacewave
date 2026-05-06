package provider_spacewave

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	provider "github.com/s4wave/spacewave/core/provider"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	"github.com/sirupsen/logrus"
)

func TestBuildOnboardingStatusProjectionReadyCloudRouteStatus(t *testing.T) {
	acc := newProjectionEmptyManagedBillingTestAccount(t)
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

	got, err := acc.BuildOnboardingStatusProjection(context.Background(), OnboardingStatusProjectionContext{
		HasLinkedLocal:          true,
		LinkedLocalSessionIndex: 3,
		LinkedLocalHasContent:   true,
		HasLinkedCloud:          true,
		LinkedCloudSessionIndex: 4,
	})
	if err != nil {
		t.Fatalf("build projection: %v", err)
	}

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

func TestBuildOnboardingStatusProjectionTerminalRouteStatusWithoutAccountSnapshot(t *testing.T) {
	acc := newProjectionTestAccount(t)
	acc.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		acc.state.status = provider.ProviderAccountStatus_ProviderAccountStatus_DELETED
	})

	got, err := acc.BuildOnboardingStatusProjection(context.Background(), OnboardingStatusProjectionContext{})
	if err != nil {
		t.Fatalf("build projection: %v", err)
	}

	expected := &s4wave_provider_spacewave.WatchOnboardingStatusResponse{
		AccountStatus:           provider.ProviderAccountStatus_ProviderAccountStatus_DELETED,
		SelfEnrollmentGateState: s4wave_provider_spacewave.SelfEnrollmentGateState_SELF_ENROLLMENT_GATE_STATE_CHECKING,
	}
	if !got.EqualVT(expected) {
		t.Fatalf("unexpected projection:\n got: %+v\nwant: %+v", got, expected)
	}
}

func TestBuildOnboardingStatusProjectionAutoRejoinRouteGate(t *testing.T) {
	acc := newProjectionEmptyManagedBillingTestAccount(t)
	acc.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		acc.state.status = provider.ProviderAccountStatus_ProviderAccountStatus_READY
		acc.state.selfRejoinSweepRunning = true
		acc.state.selfEnrollmentSummary = &SelfEnrollmentSummary{
			loaded: true,
		}
	})

	got, err := acc.BuildOnboardingStatusProjection(context.Background(), OnboardingStatusProjectionContext{})
	if err != nil {
		t.Fatalf("build projection: %v", err)
	}
	if got.GetSelfEnrollmentGateState() != s4wave_provider_spacewave.SelfEnrollmentGateState_SELF_ENROLLMENT_GATE_STATE_AUTO_CONNECTING {
		t.Fatalf("expected auto-connecting gate, got %v", got.GetSelfEnrollmentGateState())
	}
}

func TestBuildOnboardingStatusProjectionLoadsRouteBillingSummaryWhenReady(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/billing/accounts" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		calls++
		body, err := (&s4wave_provider_spacewave.ListManagedBillingAccountsResponse{
			Accounts: []*s4wave_provider_spacewave.ManagedBillingAccount{
				{Id: "ba-1", SubscriptionStatus: s4wave_provider_spacewave.BillingStatus_BillingStatus_ACTIVE},
				{Id: "ba-2", SubscriptionStatus: s4wave_provider_spacewave.BillingStatus_BillingStatus_TRIALING},
				{Id: "ba-3", SubscriptionStatus: s4wave_provider_spacewave.BillingStatus_BillingStatus_NONE},
			},
		}).MarshalVT()
		if err != nil {
			t.Fatalf("marshal response: %v", err)
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	acc := newProjectionManagedBillingTestAccount(t, srv.URL)
	acc.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		acc.state.info = &api.AccountStateResponse{
			SubscriptionStatus: s4wave_provider_spacewave.BillingStatus_BillingStatus_ACTIVE,
		}
		acc.state.status = provider.ProviderAccountStatus_ProviderAccountStatus_READY
	})

	got, err := acc.BuildOnboardingStatusProjection(context.Background(), OnboardingStatusProjectionContext{})
	if err != nil {
		t.Fatalf("build projection: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected one managed billing fetch, got %d", calls)
	}
	if got.GetManagedBaCount() != 3 ||
		got.GetManagedActiveBaCount() != 2 ||
		got.GetManagedNoSubscriptionBaCount() != 1 ||
		!got.GetBillingSummaryLoaded() {
		t.Fatalf("unexpected managed billing summary: %+v", got)
	}
}

func TestBuildManagedBillingSummarySkipsNonReady(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("managed billing summary should not fetch for non-ready account")
	}))
	defer srv.Close()

	acc := newProjectionManagedBillingTestAccount(t, srv.URL)
	got, err := acc.BuildManagedBillingSummary(
		context.Background(),
		provider.ProviderAccountStatus_ProviderAccountStatus_DORMANT,
	)
	if err != nil {
		t.Fatalf("build managed billing summary: %v", err)
	}
	if got != (ManagedBillingSummary{}) {
		t.Fatalf("expected empty summary, got %+v", got)
	}
}

func TestBuildManagedBillingSummaryReturnsUnloadedOnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	acc := newProjectionManagedBillingTestAccount(t, srv.URL)
	got, err := acc.BuildManagedBillingSummary(
		context.Background(),
		provider.ProviderAccountStatus_ProviderAccountStatus_READY,
	)
	if err == nil {
		t.Fatal("expected managed billing summary error")
	}
	if got != (ManagedBillingSummary{}) {
		t.Fatalf("expected empty summary on error, got %+v", got)
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

func newProjectionManagedBillingTestAccount(t *testing.T, endpoint string) *ProviderAccount {
	t.Helper()
	acc := NewTestProviderAccount(t, endpoint)
	acc.checkoutWatcher = newCheckoutWatcher(
		logrus.New().WithField("test", t.Name()),
		nil,
		nil,
	)
	return acc
}

func newProjectionEmptyManagedBillingTestAccount(t *testing.T) *ProviderAccount {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/billing/accounts" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		body, err := (&s4wave_provider_spacewave.ListManagedBillingAccountsResponse{}).MarshalVT()
		if err != nil {
			t.Fatalf("marshal response: %v", err)
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return newProjectionManagedBillingTestAccount(t, srv.URL)
}
