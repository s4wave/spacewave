import { useCallback } from 'react'
import { LuCreditCard } from 'react-icons/lu'
import { cn } from '@s4wave/web/style/utils.js'
import { useSessionNavigate } from '@s4wave/web/contexts/contexts.js'
import { BillingStatus } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import { useBillingStateContext } from './BillingStateProvider.js'
import {
  statusLabel,
  intervalLabel,
  isStatusActive,
  isStatusPastDue,
} from './billing-utils.js'

// BillingSummary is a compact billing card for the session details overlay.
export function BillingSummary() {
  const billingState = useBillingStateContext()
  const navigateSession = useSessionNavigate()
  const billing = billingState.response?.billingAccount
  const baId = billing?.id
  const handleManage = useCallback(() => {
    if (baId) {
      navigateSession({ path: `billing/${baId}` })
      return
    }
    navigateSession({ path: 'billing' })
  }, [navigateSession, baId])

  if (!billing) return null

  const status = billing.status
  const stColor =
    isStatusActive(status) ? 'text-green-500'
    : isStatusPastDue(status) ? 'text-yellow-500'
    : status === BillingStatus.BillingStatus_CANCELED ? 'text-destructive'
    : 'text-foreground-alt/50'
  const intLabel = intervalLabel(billing.billingInterval)
  const nextDate =
    billing.currentPeriodEnd ?
      new Date(Number(billing.currentPeriodEnd)).toLocaleDateString()
    : ''

  return (
    <button
      onClick={handleManage}
      className="border-foreground/10 bg-foreground/5 hover:border-brand/30 hover:bg-brand/5 group flex w-full cursor-pointer items-center gap-3 rounded-md border p-2.5 text-left transition-colors"
    >
      <div className="bg-foreground/10 group-hover:bg-brand/10 flex h-8 w-8 shrink-0 items-center justify-center rounded-md transition-colors">
        <LuCreditCard className="text-foreground-alt group-hover:text-brand h-3.5 w-3.5 transition-colors" />
      </div>
      <div className="flex min-w-0 flex-1 flex-col">
        <div className="flex items-center gap-2">
          <h4 className="text-foreground text-xs font-medium select-none">
            Billing
          </h4>
          <span className={cn('text-[10px] font-semibold', stColor)}>
            {statusLabel(status)}
          </span>
        </div>
        <p className="text-foreground-alt text-xs select-none">
          {[intLabel, nextDate && `Next: ${nextDate}`]
            .filter(Boolean)
            .join(' \u00b7 ') || 'Manage your subscription'}
        </p>
      </div>
    </button>
  )
}
