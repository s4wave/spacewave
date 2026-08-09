import { useCallback, useState } from 'react'

import { useNavigate } from '@s4wave/web/router/router.js'
import { AccountLifecycleState } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'

import { useBillingAccountCheckout } from '../provider/spacewave/useBillingAccountCheckout.js'
import { BillingCancellationContent } from './BillingCancellationContent.js'
import { useBillingStateContext } from './BillingStateProvider.js'
import { useManagedBillingAccounts } from './useManagedBillingAccounts.js'

// BillingCancelPage coordinates subscription cancellation and reactivation.
export function BillingCancelPage() {
  const navigate = useNavigate()
  const billingState = useBillingStateContext()
  const { session, store } = useManagedBillingAccounts()
  const checkout = useBillingAccountCheckout({
    onCompleted: () => {
      void store?.refresh()
      navigate({ path: '../' })
    },
  })
  const [action, setAction] = useState<'idle' | 'canceling' | 'reactivating'>(
    'idle',
  )
  const [error, setError] = useState<string | null>(null)

  const billing = billingState.response?.billingAccount
  const lifecycleState = billing?.lifecycleState
  const endAt = billing?.cancelAt || billing?.currentPeriodEnd
  const isCancelScheduled =
    lifecycleState ===
    AccountLifecycleState.AccountLifecycleState_ACTIVE_WITH_CANCEL_AT_PERIOD_END
  const isGrace =
    lifecycleState ===
    AccountLifecycleState.AccountLifecycleState_CANCELED_GRACE_READONLY
  const canScheduleCancel =
    lifecycleState === AccountLifecycleState.AccountLifecycleState_ACTIVE ||
    isCancelScheduled
  const endLabel = endAt
    ? new Date(Number(endAt)).toLocaleDateString(undefined, {
        month: 'long',
        day: 'numeric',
        year: 'numeric',
      })
    : null
  const title = isCancelScheduled
    ? endLabel
      ? `Your plan will already cancel on ${endLabel}`
      : 'Your plan is already set to cancel'
    : isGrace
      ? 'Your plan is in the 30-day export window'
      : 'Cancel your Spacewave Cloud plan?'
  const subtitle = isCancelScheduled
    ? 'Nothing else needs to happen. You still have full access until then. If you changed your mind, you can keep the plan active.'
    : isGrace
      ? 'Your subscription has already ended. Cloud data is read-only for 30 days so you can export what you need or start a new subscription.'
      : 'This keeps your plan active until the end of the current billing period. After that, your cloud data becomes read-only for 30 days so you can export it.'

  const handleBack = useCallback(() => navigate({ path: '../' }), [navigate])
  const handleCancel = useCallback(async () => {
    const baId = billingState.billingAccountId
    if (!session || !store || !baId || action !== 'idle') return
    setAction('canceling')
    setError(null)
    try {
      await store.cancel(baId)
      navigate({ path: '../' })
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Cancel failed')
      setAction('idle')
    }
  }, [action, billingState.billingAccountId, navigate, session, store])
  const handleKeep = useCallback(async () => {
    const baId = billingState.billingAccountId
    if (!session || !store || !baId || action !== 'idle') return
    setAction('reactivating')
    setError(null)
    try {
      const response = await store.reactivate(baId)
      if (response.needsCheckout) {
        await checkout.startCheckout(baId)
        return
      }
      navigate({ path: '../' })
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Reactivate failed')
      setAction('idle')
    }
  }, [
    action,
    billingState.billingAccountId,
    checkout,
    navigate,
    session,
    store,
  ])

  return (
    <BillingCancellationContent
      billing={billing}
      loading={billingState.loading}
      isCancelScheduled={isCancelScheduled}
      isGrace={isGrace}
      canScheduleCancel={canScheduleCancel}
      endLabel={endLabel}
      title={title}
      subtitle={subtitle}
      action={action}
      error={error}
      checkout={checkout}
      onBack={handleBack}
      onCancel={() => void handleCancel()}
      onKeep={() => void handleKeep()}
    />
  )
}
