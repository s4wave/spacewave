package provider_spacewave

import (
	"testing"

	provider "github.com/s4wave/spacewave/core/provider"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

func TestUnauthenticatedAccountStatusUsesDeletedState(t *testing.T) {
	status := unauthenticatedAccountStatus(&api.AccountStateResponse{
		LifecycleState: api.AccountLifecycleState_ACCOUNT_LIFECYCLE_STATE_DELETED_PENDING_PURGE,
	})
	if status != provider.ProviderAccountStatus_ProviderAccountStatus_DELETED {
		t.Fatalf("expected deleted status for deleted lifecycle, got %v", status)
	}
}

func TestUnauthenticatedAccountStatusUsesReauthState(t *testing.T) {
	status := unauthenticatedAccountStatus(&api.AccountStateResponse{
		LifecycleState: api.AccountLifecycleState_ACCOUNT_LIFECYCLE_STATE_ACTIVE,
	})
	if status != provider.ProviderAccountStatus_ProviderAccountStatus_UNAUTHENTICATED {
		t.Fatalf("expected unauthenticated status for active lifecycle, got %v", status)
	}
}

func TestLoadedAccountStatusUsesDeletedState(t *testing.T) {
	status := loadedAccountStatus(&api.AccountStateResponse{
		LifecycleState: api.AccountLifecycleState_ACCOUNT_LIFECYCLE_STATE_DELETED_PENDING_PURGE,
	})
	if status != provider.ProviderAccountStatus_ProviderAccountStatus_DELETED {
		t.Fatalf("expected deleted status for deleted lifecycle, got %v", status)
	}
}

func TestLoadedAccountStatusUsesReadyState(t *testing.T) {
	status := loadedAccountStatus(&api.AccountStateResponse{
		LifecycleState: api.AccountLifecycleState_ACCOUNT_LIFECYCLE_STATE_ACTIVE,
	})
	if status != provider.ProviderAccountStatus_ProviderAccountStatus_READY {
		t.Fatalf("expected ready status for active lifecycle, got %v", status)
	}
}

func TestCloudMutationAllowedReadOnlyState(t *testing.T) {
	allowed := cloudMutationAllowed(&api.AccountStateResponse{
		SubscriptionStatus: s4wave_provider_spacewave.BillingStatus_BillingStatus_CANCELED,
		LifecycleState:     api.AccountLifecycleState_ACCOUNT_LIFECYCLE_STATE_CANCELED_GRACE_READONLY,
	})
	if allowed {
		t.Fatal("expected read-only lifecycle to block cloud mutations")
	}
}

func TestCloudSelfEnrollmentAllowedReadOnlyExportState(t *testing.T) {
	allowed := cloudSelfEnrollmentAllowed(&api.AccountStateResponse{
		SubscriptionStatus: s4wave_provider_spacewave.BillingStatus_BillingStatus_CANCELED,
		LifecycleState:     api.AccountLifecycleState_ACCOUNT_LIFECYCLE_STATE_CANCELED_GRACE_READONLY,
	})
	if !allowed {
		t.Fatal("expected cancellation grace read-only lifecycle to allow self-enrollment")
	}
}

func TestCloudSelfEnrollmentBlocksPendingDeleteState(t *testing.T) {
	allowed := cloudSelfEnrollmentAllowed(&api.AccountStateResponse{
		SubscriptionStatus: s4wave_provider_spacewave.BillingStatus_BillingStatus_CANCELED,
		LifecycleState:     api.AccountLifecycleState_ACCOUNT_LIFECYCLE_STATE_PENDING_DELETE_READONLY,
	})
	if allowed {
		t.Fatal("expected pending delete lifecycle to block self-enrollment")
	}
}

func TestCanMutateCloudObjectsDormantStatus(t *testing.T) {
	acc := &ProviderAccount{}
	acc.state.info = &api.AccountStateResponse{
		SubscriptionStatus: s4wave_provider_spacewave.BillingStatus_BillingStatus_ACTIVE,
		LifecycleState:     api.AccountLifecycleState_ACCOUNT_LIFECYCLE_STATE_ACTIVE,
	}
	acc.state.status = provider.ProviderAccountStatus_ProviderAccountStatus_DORMANT

	if acc.canMutateCloudObjects() {
		t.Fatal("expected dormant account status to block cloud mutations")
	}
}

func TestCanMutateCloudObjectsReadyWriteAllowedState(t *testing.T) {
	acc := &ProviderAccount{}
	acc.state.info = &api.AccountStateResponse{
		SubscriptionStatus: s4wave_provider_spacewave.BillingStatus_BillingStatus_TRIALING,
		LifecycleState:     api.AccountLifecycleState_ACCOUNT_LIFECYCLE_STATE_ACTIVE,
	}
	acc.state.status = provider.ProviderAccountStatus_ProviderAccountStatus_READY

	if !acc.canMutateCloudObjects() {
		t.Fatal("expected ready active account to allow cloud mutations")
	}
}
