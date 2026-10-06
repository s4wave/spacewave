import { useCallback } from 'react'
import { LuExternalLink, LuRefreshCw, LuX } from 'react-icons/lu'

import { BillingStatus } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'

import type { useBillingAccountCheckout } from '../provider/spacewave/useBillingAccountCheckout.js'
import { isStatusActive } from './billing-utils.js'
import { useBillingPortal } from './useBillingPortal.js'
import { usePlanReactivation } from './usePlanReactivation.js'

// PlanControls renders the Plan section: the Stripe billing portal, and the
// cancel and reactivate actions when the viewer may manage the subscription.
export function PlanControls(props: {
  status?: BillingStatus
  cancelAt?: bigint | number
  showSelfService?: boolean
}) {
  const navigate = useNavigate()
  const portal = useBillingPortal()

  const isActive = isStatusActive(props.status)
  const isCanceled = props.status === BillingStatus.BillingStatus_CANCELED
  const isCancelScheduled = isActive && !!props.cancelAt
  const cancelLabel = props.cancelAt
    ? new Date(Number(props.cancelAt)).toLocaleDateString()
    : null
  const reactivation = usePlanReactivation(
    !!props.showSelfService && isCanceled,
  )
  const { checkout } = reactivation

  const handleCancel = useCallback(() => {
    navigate({ path: './cancel' })
  }, [navigate])

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
        {props.showSelfService && (
          <SubscriptionActions
            isActive={isActive}
            isCanceled={isCanceled}
            isCancelScheduled={isCancelScheduled}
            reactivating={reactivation.reactivating}
            reactivateBusy={reactivation.busy}
            onCancel={handleCancel}
            onReactivate={() => void reactivation.reactivate()}
          />
        )}
      </div>
      <div className="text-foreground-alt/50 text-xs">
        Payment methods, invoices, and billing history are on Stripe.
        {isCancelScheduled &&
          cancelLabel &&
          ` Cancellation is scheduled for ${cancelLabel}. You keep full access until then.`}
      </div>
      <ReactivationProgress checkout={checkout} />
      {(portal.error || reactivation.error || checkout.error) && (
        <div className="text-destructive text-xs">
          {portal.error || reactivation.error || checkout.error}
        </div>
      )}
    </div>
  )
}

// SubscriptionActions renders the cancel or reactivate button that fits the
// subscription's state.
function SubscriptionActions(props: {
  isActive: boolean
  isCanceled: boolean
  isCancelScheduled: boolean
  reactivating: boolean
  reactivateBusy: boolean
  onCancel: () => void
  onReactivate: () => void
}) {
  if (props.isCancelScheduled) {
    return (
      <DashboardButton
        icon={<LuRefreshCw className="size-3" />}
        onClick={props.onReactivate}
        disabled={props.reactivateBusy}
      >
        {props.reactivating ? 'Keeping subscription…' : 'Keep subscription'}
      </DashboardButton>
    )
  }
  if (props.isActive) {
    return (
      <DashboardButton
        icon={<LuX className="size-3" />}
        onClick={props.onCancel}
        variant="destructive"
      >
        Cancel subscription
      </DashboardButton>
    )
  }
  if (props.isCanceled) {
    return (
      <DashboardButton
        icon={<LuRefreshCw className="size-3" />}
        onClick={props.onReactivate}
        disabled={props.reactivateBusy}
      >
        {props.reactivating ? 'Reactivating…' : 'Reactivate subscription'}
      </DashboardButton>
    )
  }
  return null
}

// ReactivationProgress tells the viewer that Stripe checkout is pending and
// offers to reopen it.
function ReactivationProgress(props: {
  checkout: ReturnType<typeof useBillingAccountCheckout>
}) {
  const { checkout } = props
  if (!checkout.polling) return null
  return (
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
  )
}
