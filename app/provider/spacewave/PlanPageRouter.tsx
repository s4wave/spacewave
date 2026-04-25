import { useSessionIndex } from '@s4wave/web/contexts/contexts.js'
import { SpacewaveOnboardingContext } from '@s4wave/web/contexts/SpacewaveOnboardingContext.js'
import { Redirect } from '@s4wave/web/router/Redirect.js'
import { PlanSelectionPage } from './PlanSelectionPage.js'
import {
  hasReactivatableManagedBilling,
  isOnboardingReady,
} from './account-status.js'

// PlanPageRouter handles the shared /plan route for both local and cloud
// sessions. Local sessions have no Spacewave onboarding context, so they
// always render PlanSelectionPage. Cloud sessions use WatchOnboardingStatus to
// determine whether to stay on PlanSelectionPage, redirect to the dashboard,
// or forward to /plan/no-active.
//
// Case 1: active subscription                 -> redirect to dashboard
// Case 2: no subscription + has_linked_cloud  -> redirect to cloud session
// Case 3: reactivatable managed BA exists     -> redirect to /plan/no-active
// Case 4: no subscription (no BAs or all NONE) -> show plan selection page
export function PlanPageRouter() {
  const sessionIndex = useSessionIndex()
  const ctx = SpacewaveOnboardingContext.useContextSafe()
  if (!ctx) {
    return <PlanSelectionPage />
  }
  const onboarding = ctx?.onboarding ?? null
  const hasActiveBilling = ctx?.hasActiveBilling ?? false

  // Hold the render until the onboarding snapshot (account status + billing
  // summary) is definitive. Otherwise the pre-fetch response flashes the
  // plan page for subscribed users or misroutes first-run users.
  if (!onboarding || !isOnboardingReady(onboarding)) {
    return null
  }

  if (hasActiveBilling) {
    return <Redirect to="../" />
  }

  if (
    onboarding.hasLinkedCloud &&
    onboarding.linkedCloudSessionIndex !== sessionIndex
  ) {
    return <Redirect to={`/u/${onboarding.linkedCloudSessionIndex}/plan`} />
  }

  if (hasReactivatableManagedBilling(onboarding)) {
    return <Redirect to="no-active" />
  }

  return <PlanSelectionPage />
}
