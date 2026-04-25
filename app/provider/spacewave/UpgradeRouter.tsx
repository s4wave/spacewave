import { useCallback, useEffect, useMemo, useReducer, useRef } from 'react'

import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { Redirect } from '@s4wave/web/router/Redirect.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { SpacewaveOnboardingContext } from '@s4wave/web/contexts/SpacewaveOnboardingContext.js'
import { CheckoutStatus } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import { CloudConfirmationPage } from './CloudConfirmationPage.js'
import { getCheckoutResultBaseUrl } from './checkout-url.js'
import { checkoutReducer } from './PlanSelectionPage.js'
import type { CheckoutState } from './PlanSelectionPage.js'
import { useCloudProviderConfig } from './useSpacewaveAuth.js'
import { isAccountStatusLoaded } from './account-status.js'
import type { Session } from '@s4wave/sdk/session/session.js'

// UpgradeRouter is a thin router at /plan/upgrade that watches onboarding
// status and routes to the appropriate page. If the user already has a
// subscription, it redirects to /plan/migrate (non-empty local) or the
// dashboard (no local / empty local). If no subscription exists, it renders
// the cloud confirmation checkout flow with auto-start.
export function UpgradeRouter() {
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const ctx = SpacewaveOnboardingContext.useContextSafe()
  const onboarding = ctx?.onboarding ?? null
  const navigate = useNavigate()
  const retryTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const cloudProviderConfig = useCloudProviderConfig()
  const checkoutResultBaseUrl = getCheckoutResultBaseUrl(cloudProviderConfig)

  // Detect session type. Non-cloud sessions redirect to login first.
  const { providerId, isCloud: isCloudSession } = useSessionInfo(session)

  // The onboarding payload is trustworthy once the account status has
  // advanced past the pre-fetch placeholder. UpgradeRouter only reads
  // hasSubscription / hasLinkedLocal / linkedLocalHasContent so the
  // managed BA summary is not required here.
  const noSubscription =
    !!onboarding &&
    isAccountStatusLoaded(onboarding.accountStatus) &&
    !onboarding.hasSubscription

  // Checkout reducer for the no-subscription case.
  const initialState: CheckoutState = useMemo(
    () => ({
      cloudExpanded: false,
      loading: false,
      showRetry: false,
      polling: false,
      error: null,
      checkoutUrl: '',
    }),
    [],
  )
  const [state, dispatch] = useReducer(checkoutReducer, initialState)

  // Watch checkout status via streaming RPC when polling.
  const checkoutStatusResource = useStreamingResource(
    sessionResource,
    useCallback(
      (sess: NonNullable<Session>, signal: AbortSignal) => {
        if (!state.polling) return (async function* () {})()
        return sess.spacewave.watchCheckoutStatus(signal)
      },
      [state.polling],
    ),
    [state.polling],
  )

  // Navigate to setup when checkout completes.
  const checkoutStatus = checkoutStatusResource.value?.status
  useEffect(() => {
    if (checkoutStatus === CheckoutStatus.CheckoutStatus_COMPLETED) {
      navigate({
        path: window.location.hash
          .replace(/^#/, '')
          .replace(/\/plan(\/.*)?$/, '/setup'),
      })
    }
  }, [checkoutStatus, navigate])

  // Create or resume a Stripe checkout session.
  // Retries on "released" errors (session resource still mounting after creation).
  const handleStartCloud = useCallback(async () => {
    if (!session || !checkoutResultBaseUrl) return
    dispatch({ type: 'start_checkout' })
    if (retryTimerRef.current) clearTimeout(retryTimerRef.current)

    const maxRetries = 10
    for (let attempt = 0; attempt <= maxRetries; attempt++) {
      try {
        const sw = session.spacewave

        const successUrl = checkoutResultBaseUrl + '/checkout/success'
        const cancelUrl = checkoutResultBaseUrl + '/checkout/cancel'

        const resp = await sw.createCheckoutSession({ successUrl, cancelUrl })

        if (resp.status === CheckoutStatus.CheckoutStatus_COMPLETED) {
          navigate({
            path: window.location.hash
              .replace(/^#/, '')
              .replace(/\/plan(\/.*)?$/, '/setup'),
          })
          return
        }

        const url = resp.checkoutUrl ?? ''
        if (url) {
          const win = window.open(url, '_blank')
          if (!win) {
            dispatch({ type: 'checkout_pending', checkoutUrl: url })
            dispatch({ type: 'popup_blocked' })
            return
          }
        }
        dispatch({ type: 'checkout_pending', checkoutUrl: url })
        retryTimerRef.current = setTimeout(
          () => dispatch({ type: 'show_retry' }),
          4000,
        )
        return
      } catch (err) {
        const msg =
          err instanceof Error ? err.message : 'Failed to create checkout'
        if (msg.includes('released') && attempt < maxRetries) {
          await new Promise((r) => setTimeout(r, 100))
          continue
        }
        if (msg.includes('no active spacewave session')) {
          navigate({ path: '/login' })
          return
        }
        dispatch({ type: 'checkout_error', error: msg })
        return
      }
    }
  }, [checkoutResultBaseUrl, session, navigate])

  // Auto-start Stripe once onboarding confirms the caller has no
  // subscription. Holding until onboardingLoaded prevents firing a
  // createCheckoutSession RPC while the cloud account snapshot is still
  // loading, which would otherwise push subscribed users through Stripe.
  useEffect(() => {
    if (!noSubscription) return
    if (!checkoutResultBaseUrl) return
    if (state.loading || state.polling) return
    void handleStartCloud()
    // Only re-run when the trigger conditions change; state.loading /
    // state.polling are checked each render but should not re-drive the
    // effect themselves.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [noSubscription, checkoutResultBaseUrl])

  // Clean up retry timer.
  useEffect(() => {
    return () => {
      if (retryTimerRef.current) clearTimeout(retryTimerRef.current)
    }
  }, [])

  // Callbacks must be above conditional returns to satisfy rules of hooks.
  const handleBack = useCallback(() => {
    dispatch({ type: 'reset' })
    if (retryTimerRef.current) clearTimeout(retryTimerRef.current)
    if (session) {
      void (async () => {
        const sw = session.spacewave
        const resp = await sw.cancelCheckoutSession()
        if (resp.status === CheckoutStatus.CheckoutStatus_COMPLETED) {
          navigate({
            path: window.location.hash
              .replace(/^#/, '')
              .replace(/\/plan(\/.*)?$/, '/setup'),
          })
        }
      })()
    }
    // Navigate back to plan selection.
    navigate({ path: '../' })
  }, [session, navigate, retryTimerRef])

  const handleRetry = useCallback(() => {
    void handleStartCloud()
  }, [handleStartCloud])

  // Non-cloud sessions need to create a cloud account first.
  if (providerId && !isCloudSession) {
    return <Redirect to="login" />
  }

  // Wait for the cloud account snapshot to load before deciding anything.
  // Without this gate a pre-fetch onboarding response would fall through to
  // the checkout flow below and auto-start a Stripe session for a user we
  // already know is subscribed.
  if (!onboarding || !isAccountStatusLoaded(onboarding.accountStatus)) {
    return null
  }

  // Subscribed callers exit the upgrade flow: either to the migration
  // wizard (if they still have a non-empty linked local session) or to the
  // session dashboard.
  if (onboarding.hasSubscription) {
    if (onboarding.hasLinkedLocal && onboarding.linkedLocalHasContent) {
      return <Redirect to="../migrate" />
    }
    return <Redirect to="../../" />
  }

  // No subscription: render the checkout confirmation. The auto-start
  // effect above will fire a single createCheckoutSession RPC.
  return (
    <CloudConfirmationPage
      loading={state.loading}
      polling={state.polling}
      showRetry={state.showRetry}
      error={state.error}
      root={!!session}
      checkoutUrl={state.checkoutUrl}
      onBack={handleBack}
      onRetry={handleRetry}
      onLoading={() => dispatch({ type: 'start_checkout' })}
    />
  )
}
