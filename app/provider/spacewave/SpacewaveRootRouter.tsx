import { useEffect, useRef } from 'react'
import { toast } from '@s4wave/web/ui/toaster.js'

import { useNavigate } from '@s4wave/web/router/router.js'
import { Redirect } from '@s4wave/web/router/Redirect.js'
import { SpacewaveOnboardingContext } from '@s4wave/web/contexts/SpacewaveOnboardingContext.js'
import {
  AccountLifecycleState,
  SelfEnrollmentGateState,
} from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import { BillingStateProvider } from '@s4wave/app/billing/BillingStateProvider.js'
import { SessionDashboardContainer } from '@s4wave/app/session/SessionDashboardContainer.js'
import { ProviderAccountStatus } from '@s4wave/core/provider/provider.pb.js'
import { useStateAtom } from '@s4wave/web/state/persist.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import {
  hasReactivatableManagedBilling,
  isAccountStatusLoaded,
} from './account-status.js'
import { SessionSelfEnrollmentInterstitial } from './SessionSelfEnrollmentInterstitial.js'
import {
  defaultSelfEnrollmentSkip,
  selfEnrollmentSkipAtomKey,
} from './self-enrollment-skip.js'

// RouterLoadingGate renders the shared session loading card for a router gate
// that is waiting on asynchronous state. Matches AppSession's loading shell so
// the card reads as a continuous transition into the router.
function RouterLoadingGate({ detail }: { detail: string }) {
  return (
    <div
      data-testid="session-loading"
      className="flex min-h-screen w-full items-center justify-center p-6"
    >
      <div className="w-full max-w-sm">
        <LoadingCard
          view={{
            state: 'loading',
            title: 'Loading session',
            detail,
          }}
        />
      </div>
    </div>
  )
}

// SpacewaveRootRouter handles all cloud session root routing.
// Gates: loading -> dormant reactivation -> linked-local redirect -> lapsed
// -> email verification -> no-active-billing -> dashboard.
export function SpacewaveRootRouter() {
  const ctx = SpacewaveOnboardingContext.useContextSafe()
  const onboarding = ctx?.onboarding ?? null
  const isLapsed = ctx?.isLapsed ?? false
  const hasActiveBilling = ctx?.hasActiveBilling ?? false
  const emailVerified = ctx?.emailVerified ?? false
  const toastShown = useRef(false)
  const navigate = useNavigate()
  const [selfEnrollmentSkip] = useStateAtom(
    null,
    selfEnrollmentSkipAtomKey,
    defaultSelfEnrollmentSkip,
  )

  const accountLoaded =
    !!onboarding && isAccountStatusLoaded(onboarding.accountStatus)
  const billingLoaded = accountLoaded && !!onboarding?.billingSummaryLoaded
  const isDormant =
    onboarding?.accountStatus ===
    ProviderAccountStatus.ProviderAccountStatus_DORMANT
  const lifecycleState = onboarding?.lifecycleState
  const keepCloudShell =
    lifecycleState ===
      AccountLifecycleState.AccountLifecycleState_PENDING_DELETE_READONLY ||
    lifecycleState ===
      AccountLifecycleState.AccountLifecycleState_DELETED_PENDING_PURGE ||
    lifecycleState === AccountLifecycleState.AccountLifecycleState_DELETED

  // Redirect inactive cloud sessions with a linked local shell into the
  // local session. Only fire once the account snapshot has loaded so a
  // transient pre-fetch snapshot (subscription_status=UNKNOWN with
  // hasLinkedLocal set) cannot trigger a spurious navigation.
  const linkedLocalIndex = onboarding?.linkedLocalSessionIndex
  const shouldRedirectToLocal =
    accountLoaded &&
    !!onboarding?.hasLinkedLocal &&
    !hasActiveBilling &&
    !isLapsed &&
    !isDormant &&
    !keepCloudShell
  useEffect(() => {
    if (!shouldRedirectToLocal || toastShown.current) return
    toastShown.current = true
    toast.info('No subscription, using local session.')
    navigate({ path: `/u/${linkedLocalIndex}` })
  }, [shouldRedirectToLocal, linkedLocalIndex, navigate])

  // Hold until the cloud account snapshot has loaded. Without this gate a
  // pre-fetch onboarding response would flash the plan page for subscribed
  // users whose subscription_status field has not yet been populated.
  if (!onboarding || !accountLoaded) {
    return <RouterLoadingGate detail="Fetching account status." />
  }

  // Dormant cloud session (tracker idled on subscription_required or
  // rbac_denied). Route to /plan/upgrade so UpgradeRouter can run the
  // reactivation checkout. When the reactivation completes and a linked
  // local session exists, UpgradeRouter forwards to /plan/migrate.
  if (isDormant) {
    return <Redirect to="plan/upgrade" />
  }

  // If redirecting to linked-local, render the loading card while navigation
  // occurs so the user sees a branded transition instead of a blank frame.
  if (shouldRedirectToLocal) {
    return <RouterLoadingGate detail="Switching to your local session." />
  }

  // Lapsed subscription: show dashboard in read-only mode.
  if (isLapsed) {
    return (
      <BillingStateProvider>
        <SessionDashboardContainer />
      </BillingStateProvider>
    )
  }

  // Plan routing needs the managed billing account summary to decide
  // between /plan (no BAs or every BA is NONE) and /plan/no-active
  // (reactivatable BA exists). Hold until that summary is definitive so
  // first-run users do not flash through the wrong page.
  if (!hasActiveBilling && !billingLoaded) {
    return <RouterLoadingGate detail="Checking subscription status." />
  }

  // No active billing: keep first-run and "only no-subscription BA" accounts
  // on /plan. Route /plan/no-active only when there is a genuinely
  // reactivatable managed BA (for example canceled or past_due).
  if (!hasActiveBilling) {
    if (hasReactivatableManagedBilling(onboarding)) {
      return <Redirect to="plan/no-active" />
    }
    return <Redirect to="plan" />
  }

  // Active subscription but email not verified: gate on verification.
  if (!emailVerified) {
    return <Redirect to="verify-email" />
  }

  if (
    onboarding.selfEnrollmentGateState ===
      SelfEnrollmentGateState.ACTION_REQUIRED &&
    onboarding.sessionSelfEnrollmentGenerationKey &&
    selfEnrollmentSkip?.skippedKey !==
      onboarding.sessionSelfEnrollmentGenerationKey
  ) {
    return <SessionSelfEnrollmentInterstitial />
  }

  return (
    <BillingStateProvider>
      <SessionDashboardContainer />
    </BillingStateProvider>
  )
}
