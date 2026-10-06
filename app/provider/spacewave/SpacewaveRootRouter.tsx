import { useEffect, useRef } from 'react'
import { toast } from '@s4wave/web/ui/toaster.js'

import { useNavigate } from '@s4wave/web/router/router.js'
import { Redirect } from '@s4wave/web/router/Redirect.js'
import { SpacewaveOnboardingContext } from '@s4wave/web/contexts/SpacewaveOnboardingContext.js'
import {
  AccountLifecycleState,
  type WatchOnboardingStatusResponse,
} from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import { BillingStateProvider } from '@s4wave/app/billing/BillingStateProvider.js'
import { SessionDashboardContainer } from '@s4wave/app/session/SessionDashboardContainer.js'
import { ProviderAccountStatus } from '@s4wave/core/provider/provider.pb.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import {
  hasReactivatableManagedBilling,
  isAccountStatusLoaded,
} from './account-status.js'

// RouterLoadingGate renders the shared session loading card for a router gate
// that is waiting on asynchronous state. Matches AppSession's loading shell so
// the card reads as a continuous transition into the router.
function RouterLoadingGate({ detail }: { detail: string }) {
  return (
    <div
      data-testid="session-loading"
      className="flex h-full min-h-0 w-full flex-1 items-center justify-center p-6"
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

interface RootRouteInputs {
  onboarding: WatchOnboardingStatusResponse | null
  isLapsed: boolean
  hasActiveBilling: boolean
  emailVerified: boolean
}

type RootRoute =
  | { kind: 'loading'; detail: string }
  | { kind: 'redirect'; to: string }
  | { kind: 'dashboard' }

/** isDormantAccount reports whether the cloud account tracker has idled. */
function isDormantAccount(onboarding: WatchOnboardingStatusResponse): boolean {
  return (
    onboarding.accountStatus ===
    ProviderAccountStatus.ProviderAccountStatus_DORMANT
  )
}

/** isCloudShellKept reports whether the account lifecycle keeps the cloud shell on screen. */
function isCloudShellKept(onboarding: WatchOnboardingStatusResponse): boolean {
  const lifecycleState = onboarding.lifecycleState
  return (
    lifecycleState ===
      AccountLifecycleState.AccountLifecycleState_PENDING_DELETE_READONLY ||
    lifecycleState ===
      AccountLifecycleState.AccountLifecycleState_DELETED_PENDING_PURGE ||
    lifecycleState === AccountLifecycleState.AccountLifecycleState_DELETED
  )
}

/**
 * shouldRedirectToLocal reports whether an inactive cloud session with a
 * linked local shell should move into the local session. It only fires once
 * the account snapshot has loaded so a transient pre-fetch snapshot
 * (subscription_status=UNKNOWN with hasLinkedLocal set) cannot trigger a
 * spurious navigation.
 */
function shouldRedirectToLocal({
  onboarding,
  isLapsed,
  hasActiveBilling,
}: RootRouteInputs): boolean {
  return (
    !!onboarding &&
    isAccountStatusLoaded(onboarding.accountStatus) &&
    !!onboarding.hasLinkedLocal &&
    !hasActiveBilling &&
    !isLapsed &&
    !isDormantAccount(onboarding) &&
    !isCloudShellKept(onboarding)
  )
}

/** resolveRootRoute applies the root gates in order and returns the first that matches. */
function resolveRootRoute(inputs: RootRouteInputs): RootRoute {
  const { onboarding, isLapsed, hasActiveBilling, emailVerified } = inputs

  // Hold until the cloud account snapshot has loaded. Without this gate a
  // pre-fetch Onboarding Status response would flash the plan page for
  // subscribed users whose subscription_status field has not yet been
  // populated.
  if (!onboarding || !isAccountStatusLoaded(onboarding.accountStatus)) {
    return { kind: 'loading', detail: 'Fetching account status.' }
  }

  // Dormant cloud session (tracker idled on subscription_required or
  // rbac_denied). Route to /plan/upgrade so UpgradeRouter can run the
  // reactivation checkout. When the reactivation completes and a linked
  // local session exists, UpgradeRouter forwards to /plan/migrate.
  if (isDormantAccount(onboarding)) {
    return { kind: 'redirect', to: 'plan/upgrade' }
  }

  // If redirecting to linked-local, show the loading card while navigation
  // occurs so the user sees a branded transition instead of a blank frame.
  if (shouldRedirectToLocal(inputs)) {
    return { kind: 'loading', detail: 'Switching to your local session.' }
  }

  // Lapsed subscription: show dashboard in read-only mode.
  if (isLapsed) return { kind: 'dashboard' }

  // Plan routing needs the managed billing account summary to decide
  // between /plan (no BAs or every BA is NONE) and /plan/no-active
  // (reactivatable BA exists). Hold until that summary is definitive so
  // first-run users do not flash through the wrong page.
  if (!hasActiveBilling) {
    if (!onboarding.billingSummaryLoaded) {
      return { kind: 'loading', detail: 'Checking subscription status.' }
    }

    // Keep first-run and "only no-subscription BA" accounts on /plan. Route
    // /plan/no-active only when there is a genuinely reactivatable managed
    // BA (for example canceled or past_due).
    return {
      kind: 'redirect',
      to: hasReactivatableManagedBilling(onboarding)
        ? 'plan/no-active'
        : 'plan',
    }
  }

  // Active subscription but email not verified: gate on verification.
  if (!emailVerified) return { kind: 'redirect', to: 'verify-email' }

  return { kind: 'dashboard' }
}

// SpacewaveRootRouter handles all cloud session root routing from Onboarding
// Status, the WatchOnboardingStatus route-status projection.
// Gates: loading -> dormant reactivation -> linked-local redirect -> lapsed
// -> email verification -> no-active-billing -> dashboard.
export function SpacewaveRootRouter() {
  const ctx = SpacewaveOnboardingContext.useContextSafe()
  const inputs: RootRouteInputs = {
    onboarding: ctx?.onboarding ?? null,
    isLapsed: ctx?.isLapsed ?? false,
    hasActiveBilling: ctx?.hasActiveBilling ?? false,
    emailVerified: ctx?.emailVerified ?? false,
  }
  const toastShown = useRef(false)
  const navigate = useNavigate()

  const redirectToLocal = shouldRedirectToLocal(inputs)
  const linkedLocalIndex = inputs.onboarding?.linkedLocalSessionIndex
  useEffect(() => {
    if (!redirectToLocal || toastShown.current) return
    toastShown.current = true
    toast.info('No subscription, using local session.')
    navigate({ path: `/u/${linkedLocalIndex}` })
  }, [redirectToLocal, linkedLocalIndex, navigate])

  const route = resolveRootRoute(inputs)
  if (route.kind === 'loading')
    return <RouterLoadingGate detail={route.detail} />
  if (route.kind === 'redirect') return <Redirect to={route.to} />

  return (
    <BillingStateProvider>
      <SessionDashboardContainer />
    </BillingStateProvider>
  )
}
