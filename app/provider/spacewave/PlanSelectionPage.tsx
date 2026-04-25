import { useCallback, useEffect, useMemo, useReducer, useRef } from 'react'
import {
  LuCheck,
  LuCloud,
  LuGlobe,
  LuServer,
  LuShield,
  LuUsers,
  LuZap,
} from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'
import {
  PLAN_PRICE_MONTHLY,
  OVERAGE_STORAGE_PER_GB,
  OVERAGE_WRITE_PER_MILLION,
  OVERAGE_READ_PER_MILLION,
} from '@s4wave/app/provider/spacewave/pricing.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import {
  SessionContext,
  useSessionIndex,
} from '@s4wave/web/contexts/contexts.js'
import { CheckoutStatus } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import {
  CloudConfirmationPage,
  FaqAccordion,
  FeatureGrid,
  PageFooter,
  PageWrapper,
} from './CloudConfirmationPage.js'
import { getCheckoutResultBaseUrl } from './checkout-url.js'
import { useCloudProviderConfig } from './useSpacewaveAuth.js'
import type { Session } from '@s4wave/sdk/session/session.js'
import { useSessionMetadata } from '@s4wave/app/hooks/useSessionMetadata.js'

// CheckoutState is the reducer state for the checkout flow.
export interface CheckoutState {
  cloudExpanded: boolean
  loading: boolean
  showRetry: boolean
  polling: boolean
  error: string | null
  checkoutUrl: string
}

export type CheckoutAction =
  | { type: 'expand' }
  | { type: 'start_checkout' }
  | { type: 'checkout_pending'; checkoutUrl?: string }
  | { type: 'checkout_error'; error: string }
  | { type: 'show_retry' }
  | { type: 'popup_blocked' }
  | { type: 'cancel' }
  | { type: 'reset' }

export function checkoutReducer(
  state: CheckoutState,
  action: CheckoutAction,
): CheckoutState {
  switch (action.type) {
    case 'expand':
      return { ...state, cloudExpanded: true }
    case 'start_checkout':
      return {
        ...state,
        loading: true,
        showRetry: false,
        error: null,
        checkoutUrl: '',
        cloudExpanded: true,
      }
    case 'checkout_pending':
      return {
        ...state,
        polling: true,
        checkoutUrl: action.checkoutUrl ?? state.checkoutUrl,
      }
    case 'checkout_error':
      return { ...state, loading: false, error: action.error }
    case 'show_retry':
      return { ...state, showRetry: true }
    case 'popup_blocked':
      return { ...state, showRetry: true, loading: false }
    case 'cancel':
      return {
        ...state,
        cloudExpanded: true,
        loading: false,
        showRetry: false,
        checkoutUrl: '',
        error: 'Checkout was not completed. You can try again.',
      }
    case 'reset':
      return {
        cloudExpanded: false,
        loading: false,
        showRetry: false,
        polling: false,
        error: null,
        checkoutUrl: '',
      }
  }
}

const CLOUD_FEATURES = [
  { icon: LuGlobe, text: 'Cloud sync and backup' },
  { icon: LuUsers, text: 'Shared Spaces with collaborators' },
  { icon: LuServer, text: '100 GB cloud storage included' },
  { icon: LuZap, text: '1M writes / 10M cloud reads per month' },
  { icon: LuGlobe, text: 'Always-on sync across all devices' },
  { icon: LuShield, text: 'End-to-end encrypted' },
]

const LOCAL_FEATURES = [
  'Store on your own devices',
  'Free and open-source',
  'No cloud account required',
]

const SHARED_FEATURES = [
  'The full local-first app',
  'Full plugin SDK and developer tools',
  'Peer-to-peer sync between devices',
  'Open-source, self-hostable',
]

const E2E_ENCRYPTION_LINK = (
  <a
    href="https://www.cloudflare.com/learning/privacy/what-is-end-to-end-encryption/"
    target="_blank"
    rel="noopener noreferrer"
    className="text-brand hover:underline"
    onClick={(e) => e.stopPropagation()}
  >
    a standard approach to data protection
  </a>
)

const PLAN_FAQ: { question: string; answer: React.ReactNode }[] = [
  {
    question: 'Can I switch between plans later?',
    answer: 'Yes. You can upgrade to Cloud or go back to Local at any time.',
  },
  {
    question: 'What happens to my data on Local?',
    answer:
      'Everything stays on your devices. Your files, notes, and settings live where you put them.',
  },
  {
    question: 'Do I need a credit card for Local?',
    answer:
      'No. Local is completely free with no account required. It stores data on your device.',
  },
  {
    question: 'What if I go over the Cloud baseline?',
    answer: `Overages at very low prices: $${OVERAGE_STORAGE_PER_GB.toFixed(2)}/GB-month storage, $${OVERAGE_WRITE_PER_MILLION.toFixed(2)}/million writes, $${OVERAGE_READ_PER_MILLION.toFixed(2)}/million cloud reads. You can monitor your usage anytime. Limit resets every month.`,
  },
  {
    question: 'Can I cancel my subscription?',
    answer:
      'Yes. Standard cancellation keeps your subscription active until the end of the current billing period. After that, your cloud data becomes read-only for 30 days so you can export what you need or re-subscribe. If you want to fully delete your account, that is handled separately and requires email verification.',
  },
  {
    question: 'Is my data encrypted?',
    answer: (
      <>
        Always. Both plans use end-to-end encryption by default. Your data is
        encrypted on your device before it goes anywhere, so only you and the
        people you share with can read it. On Cloud, even we cannot access your
        content. This is {E2E_ENCRYPTION_LINK} on the web.
      </>
    ),
  },
]

// PlanSelectionPage renders the post-signup plan selection screen.
// Users choose between Cloud ($8/mo) and Free Local storage.
export function PlanSelectionPage({
  checkoutResult,
  startCloud,
}: {
  checkoutResult?: 'success' | 'cancel'
  startCloud?: boolean
}) {
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const navigate = useNavigate()
  const sessionIdx = useSessionIndex()
  const metadata = useSessionMetadata(sessionIdx)
  const retryTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const cloudProviderConfig = useCloudProviderConfig()
  const checkoutResultBaseUrl = getCheckoutResultBaseUrl(cloudProviderConfig)

  const initialState: CheckoutState = useMemo(
    () => ({
      cloudExpanded:
        (startCloud ?? false) ||
        checkoutResult === 'success' ||
        checkoutResult === 'cancel',
      loading: checkoutResult === 'success',
      showRetry: false,
      polling: checkoutResult === 'success',
      error:
        checkoutResult === 'cancel' ?
          'Checkout was not completed. You can try again.'
        : null,
      checkoutUrl: '',
    }),
    // Only compute once on mount.
    // eslint-disable-next-line react-hooks/exhaustive-deps
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

  // Navigate back to the session root when checkout completes.
  const checkoutStatus = checkoutStatusResource.value?.status
  useEffect(() => {
    if (checkoutStatus === CheckoutStatus.CheckoutStatus_COMPLETED) {
      navigate({
        path: window.location.hash
          .replace(/^#/, '')
          .replace(/\/plan(\/.*)?$/, ''),
      })
    }
  }, [checkoutStatus, navigate])

  // Cloud path: create or resume checkout in a single atomic call.
  const handleStartCloud = useCallback(async () => {
    if (!session || !checkoutResultBaseUrl) return
    dispatch({ type: 'start_checkout' })
    if (retryTimerRef.current) clearTimeout(retryTimerRef.current)
    try {
      const sw = session.spacewave

      const successUrl = checkoutResultBaseUrl + '/checkout/success'
      const cancelUrl = checkoutResultBaseUrl + '/checkout/cancel'

      const resp = await sw.createCheckoutSession({ successUrl, cancelUrl })

      if (resp.status === CheckoutStatus.CheckoutStatus_COMPLETED) {
        navigate({
          path: window.location.hash
            .replace(/^#/, '')
            .replace(/\/plan(\/.*)?$/, ''),
        })
        return
      }

      const url = resp.checkoutUrl ?? ''
      if (url) {
        const win = window.open(url, '_blank')
        if (!win) {
          // Popup was blocked; show the button immediately so the user
          // can open Stripe with a direct click (user gesture).
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
    } catch (err) {
      if (err instanceof Error && err.message.includes('released resource')) {
        // Session resource was released during the async call; ignore.
        return
      }
      const msg =
        err instanceof Error ? err.message : 'Failed to create checkout'
      dispatch({ type: 'checkout_error', error: msg })
    }
  }, [checkoutResultBaseUrl, session, navigate])

  // Auto-start Stripe when entering expanded view or when the session
  // resource becomes available after a release/retry cycle.
  useEffect(() => {
    if (
      state.cloudExpanded &&
      !state.loading &&
      !state.polling &&
      session &&
      checkoutResultBaseUrl
    ) {
      void handleStartCloud()
    }
    // Re-trigger when session changes (e.g. after resource retry).
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [state.cloudExpanded, session, checkoutResultBaseUrl])

  // Clean up retry timer.
  useEffect(() => {
    return () => {
      if (retryTimerRef.current) clearTimeout(retryTimerRef.current)
    }
  }, [])

  // Free local path: navigate to the free local setup screen.
  // PlanSelectionPage renders at both /plan and /plan/upgrade, so derive the
  // plan base from the current hash to build the correct sibling path.
  const handleFreeLocal = useCallback(() => {
    const hashPath = window.location.hash.replace(/^#/, '')
    const sessionBase = hashPath.replace(/\/plan(\/.*)?$/, '')
    if (metadata?.providerId === 'local') {
      navigate({ path: `${sessionBase}/setup` })
      return
    }
    const planBase = hashPath.replace(/\/plan(\/.*)?$/, '/plan')
    navigate({ path: planBase + '/free' })
  }, [metadata?.providerId, navigate])

  if (state.cloudExpanded) {
    return (
      <CloudConfirmationPage
        loading={state.loading}
        polling={state.polling}
        showRetry={state.showRetry}
        error={state.error}
        root={!!session}
        checkoutUrl={state.checkoutUrl}
        onBack={() => {
          dispatch({ type: 'reset' })
          if (retryTimerRef.current) clearTimeout(retryTimerRef.current)
          // Cancel pending checkout in the background. If the subscription
          // activated during the race window, redirect to setup.
          if (session) {
            void (async () => {
              try {
                const sw = session.spacewave
                const resp = await sw.cancelCheckoutSession()
                if (resp.status === CheckoutStatus.CheckoutStatus_COMPLETED) {
                  navigate({
                    path: window.location.hash
                      .replace(/^#/, '')
                      .replace(/\/plan(\/.*)?$/, ''),
                  })
                }
              } catch {
                // Session resource may have been released during the async call.
              }
            })()
          }
        }}
        onRetry={() => void handleStartCloud()}
        onLoading={() => dispatch({ type: 'start_checkout' })}
      />
    )
  }

  return (
    <PageWrapper>
      {/* Header */}
      <div className="mt-4 flex flex-col items-center gap-2">
        <AnimatedLogo followMouse={false} />
        <h1 className="mt-2 text-xl font-bold tracking-wide">
          Welcome to Spacewave
        </h1>
      </div>

      {/* Cloud card */}
      <div className="border-brand/40 bg-background-card/50 hover:border-brand/60 hover:shadow-brand/5 relative overflow-hidden rounded-lg border p-6 backdrop-blur-sm transition-all duration-300 hover:shadow-md">
        <div className="mb-4 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <LuCloud className="text-brand h-5 w-5" />
            <h2 className="text-foreground text-lg font-bold">Cloud</h2>
          </div>
          <div className="flex items-baseline gap-1">
            <span className="text-foreground text-2xl font-bold">
              ${PLAN_PRICE_MONTHLY}
            </span>
            <span className="text-foreground-alt text-sm">/ month</span>
          </div>
        </div>

        <FeatureGrid features={CLOUD_FEATURES} />

        <button
          onClick={() =>
            navigate({
              path: window.location.hash
                .replace(/^#/, '')
                .replace(/\/plan(\/.*)?$/, '/plan/upgrade'),
            })
          }
          disabled={state.loading || !session}
          className={cn(
            'mt-6 flex w-full cursor-pointer items-center justify-center rounded-md border px-5 py-2.5 text-sm font-medium transition-all duration-300 select-none',
            'border-brand bg-brand/10 text-foreground hover:bg-brand/20',
            'disabled:cursor-not-allowed disabled:opacity-50',
          )}
        >
          Start with Cloud
        </button>
      </div>

      {state.error && (
        <p className="text-destructive text-center text-xs">{state.error}</p>
      )}

      {/* Free local option */}
      <div className="border-foreground/10 bg-background-card/30 hover:border-foreground/20 rounded-lg border p-6 backdrop-blur-sm transition-all duration-300 hover:shadow-md">
        <div className="mb-3 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <LuServer className="text-foreground-alt h-5 w-5" />
            <h2 className="text-foreground text-lg font-bold">Local</h2>
          </div>
          <span className="text-foreground-alt text-sm">free forever</span>
        </div>

        <div className="mb-4 space-y-2">
          {LOCAL_FEATURES.map((text) => (
            <div key={text} className="flex items-start gap-2">
              <LuCheck className="text-brand mt-0.5 h-4 w-4 shrink-0" />
              <span className="text-foreground-alt text-sm">{text}</span>
            </div>
          ))}
        </div>

        <button
          onClick={() => void handleFreeLocal()}
          disabled={state.loading || !session}
          className="border-foreground/20 bg-foreground/5 hover:bg-foreground/10 flex w-full cursor-pointer items-center justify-center rounded-md border px-5 py-2.5 text-sm font-medium transition-all duration-300 select-none disabled:cursor-not-allowed disabled:opacity-50"
        >
          Continue with local storage
        </button>
      </div>

      {/* Shared baseline */}
      <div className="border-foreground/8 bg-background-card/30 rounded-lg border p-4 backdrop-blur-sm">
        <p className="text-foreground-alt mb-3 text-center text-xs font-semibold tracking-wide uppercase">
          Both options include
        </p>
        <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
          {SHARED_FEATURES.map((feature) => (
            <div key={feature} className="flex items-start gap-2">
              <LuCheck className="text-brand mt-0.5 h-3 w-3 shrink-0" />
              <span className="text-foreground-alt text-xs">{feature}</span>
            </div>
          ))}
        </div>
      </div>

      {/* FAQ */}
      <FaqAccordion items={PLAN_FAQ} />

      <PageFooter />
    </PageWrapper>
  )
}
