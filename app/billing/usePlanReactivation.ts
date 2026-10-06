import { useCallback, useEffect, useRef, useState } from 'react'

import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { useNavigate, usePath } from '@s4wave/web/router/router.js'

import { useBillingAccountCheckout } from '../provider/spacewave/useBillingAccountCheckout.js'
import { useBillingStateContext } from './BillingStateProvider.js'

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

// usePlanReactivation reactivates the billing account's subscription, sending
// the viewer through Stripe checkout when the subscription needs one. When the
// page loads with the reactivate intent and canReactivate holds, it starts the
// reactivation once and clears the intent from the path.
export function usePlanReactivation(canReactivate: boolean) {
  const session = SessionContext.useContext().value
  const billingState = useBillingStateContext()
  const navigate = useNavigate()
  const path = usePath()
  const checkout = useBillingAccountCheckout()
  const [autoReactivate] = useState(() => hasAutoReactivateIntent(path))
  const autoTriggered = useRef(false)
  const [reactivating, setReactivating] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const reactivate = useCallback(async () => {
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
    if (!autoReactivate || autoTriggered.current || !canReactivate) return
    if (!session || !billingState.billingAccountId || reactivating) return
    autoTriggered.current = true
    clearAutoReactivateIntent(path, navigate)
    queueMicrotask(() => {
      void reactivate()
    })
  }, [
    autoReactivate,
    billingState.billingAccountId,
    canReactivate,
    navigate,
    path,
    reactivate,
    reactivating,
    session,
  ])

  return {
    reactivate,
    reactivating,
    busy: reactivating || checkout.polling,
    error,
    checkout,
  }
}
