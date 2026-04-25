import { ProviderAccountStatus } from '@s4wave/core/provider/provider.pb.js'
import type { WatchOnboardingStatusResponse } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'

// isAccountStatusLoaded returns true once the cloud account status has
// advanced past the pre-fetch placeholder states. WatchOnboardingStatus
// responses with NONE or PENDING indicate the server has not yet loaded
// the cloud account snapshot and should not be used to drive routing
// decisions (specifically: the subscription_status field is still its
// zero value and cannot be trusted).
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

// isOnboardingReady returns true when the onboarding response is populated,
// the account status is out of the pre-fetch placeholder states, AND the
// managed billing account summary has been loaded. Use this as the gate
// before any routing decision that reads subscription_status or the
// managed_ba_count fields, so loading windows never drive a redirect.
export function isOnboardingReady(
  onboarding: WatchOnboardingStatusResponse | null | undefined,
): boolean {
  if (!onboarding) return false
  if (!isAccountStatusLoaded(onboarding.accountStatus)) return false
  if (!onboarding.billingSummaryLoaded) return false
  return true
}

// hasReactivatableManagedBilling returns true when the caller has at least
// one managed billing account whose subscription_status is NOT NONE (for
// example canceled, past_due, lapsed). Used to decide whether to route
// inactive sessions to PlanSelectionPage (first-run or all-NONE) versus
// NoActiveBillingAccountPage (reactivatable BAs exist). Assumes the
// onboarding snapshot is ready (see isOnboardingReady).
export function hasReactivatableManagedBilling(
  onboarding: WatchOnboardingStatusResponse,
): boolean {
  const managedCount = onboarding.managedBaCount ?? 0
  if (managedCount === 0) return false
  const managedNoSubscriptionCount =
    onboarding.managedNoSubscriptionBaCount ?? 0
  return managedNoSubscriptionCount < managedCount
}
