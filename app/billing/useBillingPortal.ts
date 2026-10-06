import { useCallback, useState } from 'react'

import { SessionContext } from '@s4wave/web/contexts/contexts.js'

import { useBillingStateContext } from './BillingStateProvider.js'

// useBillingPortal opens the Stripe billing portal of the billing account in
// a new tab, holding the request's progress and error.
export function useBillingPortal() {
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
