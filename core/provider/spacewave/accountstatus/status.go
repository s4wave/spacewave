package accountstatus

import (
	provider "github.com/s4wave/spacewave/core/provider"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

// IsDeleted returns true when the cloud lifecycle is logically deleted.
func IsDeleted(state *api.AccountStateResponse) bool {
	if state == nil {
		return false
	}
	return state.GetLifecycleState() == api.AccountLifecycleState_ACCOUNT_LIFECYCLE_STATE_DELETED_PENDING_PURGE ||
		state.GetLifecycleState() == api.AccountLifecycleState_ACCOUNT_LIFECYCLE_STATE_DELETED
}

// IsReadOnly returns true when the cloud lifecycle permits read-only/export
// access but blocks owner-side mutations and mailbox handling.
func IsReadOnly(state *api.AccountStateResponse) bool {
	if state == nil {
		return false
	}
	switch state.GetLifecycleState() {
	case api.AccountLifecycleState_ACCOUNT_LIFECYCLE_STATE_CANCELED_GRACE_READONLY,
		api.AccountLifecycleState_ACCOUNT_LIFECYCLE_STATE_PENDING_DELETE_READONLY,
		api.AccountLifecycleState_ACCOUNT_LIFECYCLE_STATE_LAPSED_READONLY,
		api.AccountLifecycleState_ACCOUNT_LIFECYCLE_STATE_DISPUTED_HARD_SUSPEND:
		return true
	}
	return false
}

// CloudMutationAllowed returns true when cached billing and lifecycle state
// permit owner-side cloud mutations and owner-only reads. Unknown cached state
// does not block; only known non-write-allowed snapshots short-circuit.
func CloudMutationAllowed(state *api.AccountStateResponse) bool {
	if state == nil {
		return true
	}
	if IsDeleted(state) || IsReadOnly(state) {
		return false
	}
	status := state.GetSubscriptionStatus()
	if status == s4wave_provider_spacewave.BillingStatus_BillingStatus_UNKNOWN {
		return true
	}
	return status.IsWriteAllowed()
}

// CloudSelfEnrollmentAllowed returns true when the account may repair local
// same-entity session enrollment for read-only export.
func CloudSelfEnrollmentAllowed(state *api.AccountStateResponse) bool {
	if state == nil {
		return true
	}
	switch state.GetLifecycleState() {
	case api.AccountLifecycleState_ACCOUNT_LIFECYCLE_STATE_DELETED_PENDING_PURGE,
		api.AccountLifecycleState_ACCOUNT_LIFECYCLE_STATE_DELETED,
		api.AccountLifecycleState_ACCOUNT_LIFECYCLE_STATE_PENDING_DELETE_READONLY,
		api.AccountLifecycleState_ACCOUNT_LIFECYCLE_STATE_DISPUTED_HARD_SUSPEND:
		return false
	}
	status := state.GetSubscriptionStatus()
	if status == s4wave_provider_spacewave.BillingStatus_BillingStatus_UNKNOWN {
		return true
	}
	switch status {
	case s4wave_provider_spacewave.BillingStatus_BillingStatus_ACTIVE,
		s4wave_provider_spacewave.BillingStatus_BillingStatus_TRIALING,
		s4wave_provider_spacewave.BillingStatus_BillingStatus_PAST_DUE,
		s4wave_provider_spacewave.BillingStatus_BillingStatus_PAST_DUE_READONLY,
		s4wave_provider_spacewave.BillingStatus_BillingStatus_CANCELED,
		s4wave_provider_spacewave.BillingStatus_BillingStatus_LAPSED:
		return true
	default:
		return false
	}
}

// ProviderStatusAllowsCloudMutation returns true when the current provider
// account status permits owner-side cloud mutations.
func ProviderStatusAllowsCloudMutation(
	status provider.ProviderAccountStatus,
) bool {
	switch status {
	case provider.ProviderAccountStatus_ProviderAccountStatus_DORMANT,
		provider.ProviderAccountStatus_ProviderAccountStatus_DELETED,
		provider.ProviderAccountStatus_ProviderAccountStatus_FAILED,
		provider.ProviderAccountStatus_ProviderAccountStatus_UNAUTHENTICATED:
		return false
	default:
		return true
	}
}

// CanMutateCloudObjects returns true when cached account status, billing, and
// lifecycle state all permit owner-side cloud mutations.
func CanMutateCloudObjects(
	status provider.ProviderAccountStatus,
	state *api.AccountStateResponse,
) bool {
	return ProviderStatusAllowsCloudMutation(status) && CloudMutationAllowed(state)
}

// CanSelfEnrollCloudObjects returns true when cached account status, billing,
// and lifecycle state permit same-entity cloud self-enrollment for export.
func CanSelfEnrollCloudObjects(
	status provider.ProviderAccountStatus,
	state *api.AccountStateResponse,
) bool {
	return ProviderStatusAllowsCloudMutation(status) &&
		CloudSelfEnrollmentAllowed(state)
}

// Unauthenticated derives the local terminal status for a stale cloud session
// from the cached lifecycle state.
func Unauthenticated(
	state *api.AccountStateResponse,
) provider.ProviderAccountStatus {
	if IsDeleted(state) {
		return provider.ProviderAccountStatus_ProviderAccountStatus_DELETED
	}
	return provider.ProviderAccountStatus_ProviderAccountStatus_UNAUTHENTICATED
}

// Loaded derives the steady-state account status for a loaded cloud account
// snapshot.
func Loaded(
	state *api.AccountStateResponse,
) provider.ProviderAccountStatus {
	if IsDeleted(state) {
		return provider.ProviderAccountStatus_ProviderAccountStatus_DELETED
	}
	return provider.ProviderAccountStatus_ProviderAccountStatus_READY
}
