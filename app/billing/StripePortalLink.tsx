import { useCallback, useState } from 'react'
import { LuExternalLink } from 'react-icons/lu'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { useBillingStateContext } from './BillingStateProvider.js'

// StripePortalLink opens the Stripe billing portal in a new tab.
export function StripePortalLink() {
  const session = SessionContext.useContext().value
  const billingState = useBillingStateContext()
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handleClick = useCallback(async () => {
    if (!session || loading) return
    setLoading(true)
    setError(null)
    try {
      const resp = await session.spacewave.createBillingPortal(
        billingState.billingAccountId,
      )
      if (resp.url) {
        window.open(resp.url, '_blank')
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to open portal')
    } finally {
      setLoading(false)
    }
  }, [session, loading, billingState.billingAccountId])

  return (
    <div className="space-y-2">
      <DashboardButton
        icon={<LuExternalLink className="h-3 w-3" />}
        onClick={() => void handleClick()}
        disabled={loading}
      >
        {loading ? 'Opening...' : 'Manage on Stripe'}
      </DashboardButton>
      <div className="text-foreground-alt/40 text-xs">
        Payment methods, invoices, and billing history.
      </div>
      {error && <div className="text-destructive text-xs">{error}</div>}
    </div>
  )
}
