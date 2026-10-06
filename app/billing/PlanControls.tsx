import { useCallback, useEffect, useRef, useState } from 'react'
import { LuExternalLink, LuRefreshCw, LuX } from 'react-icons/lu'

import { BillingStatus } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { useNavigate, usePath } from '@s4wave/web/router/router.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'

import { useBillingAccountCheckout } from '../provider/spacewave/useBillingAccountCheckout.js'
import { useBillingStateContext } from './BillingStateProvider.js'
import { isStatusActive } from './billing-utils.js'

// hasAutoReactivateIntent reports whether the path asks to reactivate on load.
function hasAutoReactivateIntent(path: string): boolean {
  const query = path.split('?')[1] ?? ''
  return new URLSearchParams(query).get('reactivate') === '1'
}

// clearAutoReactivateIntent replaces the path with one without its query.
function clearAutoReactivateIntent(
  path: string,
  navigate: (to: { path: string; replace?: boolean }) => void,
): void {
  const [cleanPath] = path.split('?')
  navigate({ path: cleanPath || '/', replace: true })
}

// useBillingPortal opens the Stripe billing portal of the billing account in
// a new tab, holding the request's progress and error.
function useBillingPortal() {
  const session = SessionContext.useContext().value
  const billingState = useBillingStateContext()
  const [opening, setOpening] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const open = useCallback(async () => {
    if (!session || opening) return
    setOpening(true)
    setError(null)
    try {
      const resp = await session.spacewave.createBillingPortal(
        billingState.billingAccountId,
      )
      if (resp.url) {
        window.open(resp.url, '_blank', 'noopener,noreferrer')
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to open portal')
    } finally {
      setOpening(false)
    }
  }, [session, opening, billingState.billingAccountId])

  return { open, opening, error, ready: !!session }
}

// PlanControls renders the Plan section: the Stripe billing portal, and the
// cancel and reactivate actions when the viewer may manage the subscription.
export function PlanControls(props: {
  status?: BillingStatus
  cancelAt?: bigint | number
  showSelfService?: boolean
}) {
  const session = SessionContext.useContext().value
  const billingState = useBillingStateContext()
  const navigate = useNavigate()
  const path = usePath()
  const checkout = useBillingAccountCheckout()
  const portal = useBillingPortal()
  const [autoReactivate] = useState(() => hasAutoReactivateIntent(path))
  const autoTriggered = useRef(false)

  const [reactivating, setReactivating] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const isActive = isStatusActive(props.status)
  const isCanceled = props.status === BillingStatus.BillingStatus_CANCELED
  const isCancelScheduled = isActive && !!props.cancelAt
  const cancelLabel = props.cancelAt
    ? new Date(Number(props.cancelAt)).toLocaleDateString()
    : null

  const handleCancel = useCallback(() => {
    navigate({ path: './cancel' })
  }, [navigate])

  const handleReactivate = useCallback(async () => {
    const baId = billingState.billingAccountId
    if (!session || !baId || reactivating) return
    setReactivating(true)
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
      setReactivating(false)
    }
  }, [session, reactivating, billingState.billingAccountId, checkout])

  useEffect(() => {
    if (!autoReactivate || autoTriggered.current) return
    if (!props.showSelfService || !isCanceled) return
    if (!session || !billingState.billingAccountId || reactivating) return
    autoTriggered.current = true
    clearAutoReactivateIntent(path, navigate)
    queueMicrotask(() => {
      void handleReactivate()
    })
  }, [
    autoReactivate,
    billingState.billingAccountId,
    handleReactivate,
    isCanceled,
    navigate,
    path,
    props.showSelfService,
    reactivating,
    session,
  ])

  const reactivateBusy = reactivating || checkout.polling

  return (
    <div className="space-y-3">
      {checkout.consentDialog}
      <div className="text-foreground-alt/60 text-xs font-medium tracking-wider uppercase select-none">
        Plan
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <DashboardButton
          icon={<LuExternalLink className="size-3" />}
          onClick={() => void portal.open()}
          disabled={!portal.ready || portal.opening}
        >
          {portal.opening ? 'Opening…' : 'Manage on Stripe'}
        </DashboardButton>
        {props.showSelfService && isActive && !isCancelScheduled && (
          <DashboardButton
            icon={<LuX className="size-3" />}
            onClick={handleCancel}
            variant="destructive"
          >
            Cancel subscription
          </DashboardButton>
        )}
        {props.showSelfService && isCancelScheduled && (
          <DashboardButton
            icon={<LuRefreshCw className="size-3" />}
            onClick={() => void handleReactivate()}
            disabled={reactivateBusy}
          >
            {reactivating ? 'Keeping subscription…' : 'Keep subscription'}
          </DashboardButton>
        )}
        {props.showSelfService && isCanceled && (
          <DashboardButton
            icon={<LuRefreshCw className="size-3" />}
            onClick={() => void handleReactivate()}
            disabled={reactivateBusy}
          >
            {reactivating ? 'Reactivating…' : 'Reactivate subscription'}
          </DashboardButton>
        )}
      </div>
      <div className="text-foreground-alt/50 text-xs">
        Payment methods, invoices, and billing history are on Stripe.
        {isCancelScheduled &&
          cancelLabel &&
          ` Cancellation is scheduled for ${cancelLabel}. You keep full access until then.`}
      </div>
      {checkout.polling && (
        <div className="text-foreground-alt/70 text-xs">
          Reactivation in progress. This page will update when Stripe confirms.
          {checkout.showRetry && (
            <button
              type="button"
              onClick={checkout.continueCheckout}
              className="text-brand hover:text-brand/80 ml-2 cursor-pointer transition-colors"
            >
              Continue with Stripe
            </button>
          )}
        </div>
      )}
      {(portal.error || error || checkout.error) && (
        <div className="text-destructive text-xs">
          {portal.error || error || checkout.error}
        </div>
      )}
    </div>
  )
}
