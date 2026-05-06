import { ProviderAccountStatus } from '@s4wave/core/provider/provider.pb.js'
import type { WatchOnboardingStatusResponse } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'

// isAccountStatusLoaded returns true once the account_status field in
// Onboarding Status has advanced past the pre-fetch placeholder states. NONE
// or PENDING indicate the server has not yet loaded the cloud account snapshot
// and the route-status projection should not drive routing decisions
// (specifically: subscription_status is still its zero value and cannot be
// trusted).
export function isAccountStatusLoaded(
  status: ProviderAccountStatus | undefined,
): boolean {
  switch (status) {
    case ProviderAccountStatus.ProviderAccountStatus_READY:
    case ProviderAccountStatus.ProviderAccountStatus_DORMANT:
    case ProviderAccountStatus.ProviderAccountStatus_UNAUTHENTICATED:
    case ProviderAccountStatus.ProviderAccountStatus_DELETED:
    case ProviderAccountStatus.ProviderAccountStatus_FAILED:
      return true
    default:
      return false
  }
}

// isOnboardingReady returns true when the Onboarding Status route-status
// projection is populated, account_status is out of the pre-fetch placeholder
// states, AND the managed billing account summary has been loaded. Use this as
// the gate before routing decisions that read subscription_status or the
// managed_ba_count fields, so loading windows never drive a redirect.
export function isOnboardingReady(
  onboarding: WatchOnboardingStatusResponse | null | undefined,
): boolean {
  if (!onboarding) return false
  if (!isAccountStatusLoaded(onboarding.accountStatus)) return false
  if (!onboarding.billingSummaryLoaded) return false
  return true
}

// hasReactivatableManagedBilling returns true when Onboarding Status reports
// at least one managed billing account whose subscription_status is NOT NONE
// (for example canceled, past_due, lapsed). Used to decide whether to route
// inactive sessions to PlanSelectionPage (first-run or all-NONE) versus
// NoActiveBillingAccountPage (reactivatable BAs exist). Assumes the projection
// is ready (see isOnboardingReady).
export function hasReactivatableManagedBilling(
  onboarding: WatchOnboardingStatusResponse,
): boolean {
  const managedCount = onboarding.managedBaCount ?? 0
  if (managedCount === 0) return false
  const managedNoSubscriptionCount =
    onboarding.managedNoSubscriptionBaCount ?? 0
  return managedNoSubscriptionCount < managedCount
}
