import { useCallback, useEffect, useRef, useState } from 'react'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { LuRefreshCw, LuX } from 'react-icons/lu'
import { BillingStatus } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import { useBillingAccountCheckout } from '../provider/spacewave/useBillingAccountCheckout.js'
import { useBillingStateContext } from './BillingStateProvider.js'
import { isStatusActive } from './billing-utils.js'

function hasAutoReactivateIntent(): boolean {
  const hash =
    window.location.hash.startsWith('#') ?
      window.location.hash.slice(1)
    : window.location.hash
  const query = hash.split('?')[1] ?? ''
  return new URLSearchParams(query).get('reactivate') === '1'
}

function clearAutoReactivateIntent(): void {
  const hash =
    window.location.hash.startsWith('#') ?
      window.location.hash.slice(1)
    : window.location.hash
  const [path] = hash.split('?')
  const nextHash = path ? `#${path}` : '#/'
  window.history.replaceState(
    window.history.state,
    '',
    `${window.location.pathname}${window.location.search}${nextHash}`,
  )
}

// PlanControls provides cancel and reactivate actions.
export function PlanControls(props: {
  status?: BillingStatus
  cancelAt?: bigint | number
  showSelfService?: boolean
}) {
  const session = SessionContext.useContext().value
  const billingState = useBillingStateContext()
  const navigate = useNavigate()
  const checkout = useBillingAccountCheckout()
  const autoReactivate = useRef(hasAutoReactivateIntent())
  const autoTriggered = useRef(false)

  const [action, setAction] = useState<'idle' | 'reactivating'>('idle')
  const [error, setError] = useState<string | null>(null)

  const isActive = isStatusActive(props.status)
  const isCanceled = props.status === BillingStatus.BillingStatus_CANCELED
  const isCancelScheduled = isActive && !!props.cancelAt
  const cancelLabel =
    props.cancelAt ?
      new Date(Number(props.cancelAt)).toLocaleDateString()
    : null

  const handleCancel = useCallback(() => {
    navigate({ path: './cancel' })
  }, [navigate])

  const handleReactivate = useCallback(async () => {
    const baId = billingState.billingAccountId
    if (!session || !baId || action !== 'idle') return
    setAction('reactivating')
    setError(null)
    try {
      const resp = await session.spacewave.reactivateSubscription(baId)
      if (resp.needsCheckout) {
        await checkout.startCheckout(baId)
        return
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Reactivate failed')
    } finally {
      setAction('idle')
    }
  }, [session, action, billingState.billingAccountId, checkout])

  useEffect(() => {
    if (!autoReactivate.current || autoTriggered.current) return
    if (!props.showSelfService || !isCanceled) return
    if (!session || !billingState.billingAccountId || action !== 'idle') return
    autoTriggered.current = true
    clearAutoReactivateIntent()
    void handleReactivate()
  }, [
    action,
    billingState.billingAccountId,
    handleReactivate,
    isCanceled,
    props.showSelfService,
    session,
  ])

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
            icon={<LuX className="h-3 w-3" />}
            onClick={handleCancel}
            className="text-destructive hover:bg-destructive/10"
          >
            Cancel subscription
          </DashboardButton>
        )}
        {isCancelScheduled && (
          <DashboardButton
            icon={<LuRefreshCw className="h-3 w-3" />}
            onClick={() => void handleReactivate()}
            disabled={action !== 'idle' || checkout.polling}
          >
            {action === 'reactivating' ?
              'Keeping subscription...'
            : 'Keep subscription'}
          </DashboardButton>
        )}
        {isCanceled && (
          <DashboardButton
            icon={<LuRefreshCw className="h-3 w-3" />}
            onClick={() => void handleReactivate()}
            disabled={action !== 'idle' || checkout.polling}
          >
            {action === 'reactivating' ?
              'Reactivating...'
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
