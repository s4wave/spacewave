import { useCallback, useRef, useState } from 'react'
import { LuRefreshCw, LuX } from 'react-icons/lu'
import { useAbortSignalEffect } from '@aptre/bldr-react'

import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { useNavigate, usePath } from '@s4wave/web/router/router.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { BillingStatus } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'

import { useBillingAccountCheckout } from '../provider/spacewave/useBillingAccountCheckout.js'
import { useBillingStateContext } from './BillingStateProvider.js'
import { isStatusActive } from './billing-utils.js'
import { useManagedBillingAccounts } from './useManagedBillingAccounts.js'

function hasAutoReactivateIntent(path: string): boolean {
  const query = path.split('?')[1] ?? ''
  return new URLSearchParams(query).get('reactivate') === '1'
}

function clearAutoReactivateIntent(
  path: string,
  navigate: (to: { path: string; replace?: boolean }) => void,
): void {
  const [cleanPath] = path.split('?')
  navigate({ path: cleanPath || '/', replace: true })
}

interface ReactivationContext {
  billingAccountId: string
  path: string
  session: NonNullable<ReturnType<typeof SessionContext.useContext>['value']>
}

interface ReactivationState {
  action: 'idle' | 'reactivating'
  context?: ReactivationContext
  error: string | null
}

function usePlanReactivation(
  billingAccountId: string | undefined,
  autoReactivate: boolean,
  path: string,
) {
  const session = SessionContext.useContext().value
  const navigate = useNavigate()
  const { store } = useManagedBillingAccounts()
  const checkout = useBillingAccountCheckout({
    onCompleted: () => void store?.refresh(),
  })
  const { startCheckout } = checkout
  const initialIntent = useRef(hasAutoReactivateIntent(path))
  const initialPath = useRef(path)
  const consumed = useRef(false)
  const generation = useRef<
    { pending: boolean; signal: AbortSignal } | undefined
  >(undefined)
  const routePath = path.split('?')[0] ?? ''
  const [state, setState] = useState<ReactivationState>({
    action: 'idle',
    error: null,
  })

  const reactivate = useCallback(
    async (signal = generation.current?.signal) => {
      const current = generation.current
      if (
        !signal ||
        signal.aborted ||
        !current ||
        current.signal !== signal ||
        current.pending ||
        !session ||
        !billingAccountId
      ) {
        return
      }

      const context = { billingAccountId, path: routePath, session }
      current.pending = true
      signal.throwIfAborted()
      setState({ action: 'reactivating', context, error: null })
      try {
        const response = await store?.reactivate(billingAccountId, signal)
        signal.throwIfAborted()
        if (response?.needsCheckout) {
          await startCheckout(billingAccountId)
        }
        signal.throwIfAborted()
        setState({ action: 'idle', context, error: null })
      } catch (cause) {
        if (!signal.aborted) {
          signal.throwIfAborted()
          setState({
            action: 'idle',
            context,
            error: cause instanceof Error ? cause.message : 'Reactivate failed',
          })
        }
      } finally {
        current.pending = false
      }
    },
    [billingAccountId, routePath, session, startCheckout, store],
  )

  useAbortSignalEffect(
    (signal) => {
      const current = { pending: false, signal }
      generation.current = current
      if (
        initialIntent.current &&
        !consumed.current &&
        autoReactivate &&
        session &&
        billingAccountId
      ) {
        consumed.current = true
        clearAutoReactivateIntent(initialPath.current, navigate)
        void reactivate(signal)
      }
      return () => {
        if (generation.current === current) generation.current = undefined
      }
    },
    [
      autoReactivate,
      billingAccountId,
      navigate,
      reactivate,
      routePath,
      session,
    ],
  )

  const currentState =
    state.context?.billingAccountId === billingAccountId &&
    state.context?.path === routePath &&
    state.context?.session === session
      ? state
      : { action: 'idle' as const, error: null }

  return {
    action: currentState.action,
    checkout,
    error: currentState.error,
    reactivate,
  }
}

// PlanControls provides cancel and reactivate actions.
export function PlanControls(props: {
  status?: BillingStatus
  cancelAt?: bigint | number
  showSelfService?: boolean
}) {
  const billingState = useBillingStateContext()
  const navigate = useNavigate()
  const path = usePath()
  const isActive = isStatusActive(props.status)
  const isCanceled = props.status === BillingStatus.BillingStatus_CANCELED
  const { action, checkout, error, reactivate } = usePlanReactivation(
    billingState.billingAccountId,
    !!props.showSelfService && isCanceled,
    path,
  )

  const isCancelScheduled = isActive && !!props.cancelAt
  const cancelLabel = props.cancelAt
    ? new Date(Number(props.cancelAt)).toLocaleDateString()
    : null

  const handleCancel = useCallback(() => {
    navigate({ path: './cancel' })
  }, [navigate])

  if (!props.showSelfService) {
    return null
  }

  return (
    <div className="space-y-3">
      <div className="text-foreground-alt/60 text-xs font-medium tracking-wider uppercase">
        Plan
      </div>
      <div className="flex flex-wrap gap-2">
        {isActive && !isCancelScheduled && (
          <DashboardButton
            icon={<LuX className="size-3" />}
            onClick={handleCancel}
            className="text-destructive hover:bg-destructive/10"
          >
            Cancel subscription
          </DashboardButton>
        )}
        {isCancelScheduled && (
          <DashboardButton
            icon={<LuRefreshCw className="size-3" />}
            onClick={() => void reactivate()}
            disabled={action !== 'idle' || checkout.polling}
          >
            {action === 'reactivating'
              ? 'Keeping subscription…'
              : 'Keep subscription'}
          </DashboardButton>
        )}
        {isCanceled && (
          <DashboardButton
            icon={<LuRefreshCw className="size-3" />}
            onClick={() => void reactivate()}
            disabled={action !== 'idle' || checkout.polling}
          >
            {action === 'reactivating'
              ? 'Reactivating…'
              : 'Reactivate subscription'}
          </DashboardButton>
        )}
      </div>
      {isCancelScheduled && cancelLabel && (
        <div className="text-foreground-alt/50 text-xs">
          Cancellation is scheduled for {cancelLabel}. You keep full access
          until then.
        </div>
      )}
      {checkout.polling && (
        <div className="text-foreground-alt/70 text-xs">
          Reactivation in progress. This page will update when Stripe confirms.
          {checkout.showRetry && (
            <button
              onClick={checkout.continueCheckout}
              className="text-brand hover:text-brand/80 ml-2 cursor-pointer transition-colors"
            >
              Continue with Stripe
            </button>
          )}
        </div>
      )}
      {(error || checkout.error) && (
        <div className="text-destructive text-xs">
          {error || checkout.error}
        </div>
      )}
    </div>
  )
}
