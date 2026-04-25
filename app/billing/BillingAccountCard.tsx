import { cn } from '@s4wave/web/style/utils.js'
import {
  BillingStatus,
  type BillingAccountInfo,
} from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import {
  statusLabel,
  intervalLabel,
  isStatusActive,
  isStatusPastDue,
} from './billing-utils.js'

export interface BillingAccountCardProps {
  label: string
  billing: BillingAccountInfo
  onManage: () => void
}

// BillingAccountCard renders a compact billing account card with status badge,
// interval, next date, and manage action.
export function BillingAccountCard({
  label,
  billing,
  onManage,
}: BillingAccountCardProps) {
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

  const detail = [intLabel, nextDate && `Next: ${nextDate}`]
    .filter(Boolean)
    .join(' \u00b7 ')

  return (
    <button
      onClick={onManage}
      className="border-foreground/6 bg-background-card/20 hover:border-foreground/12 group flex w-full cursor-pointer items-center gap-3 rounded-md border p-2.5 text-left transition-colors"
    >
      <div className="flex min-w-0 flex-1 flex-col">
        <div className="flex items-center gap-2">
          <span className="text-foreground text-xs font-medium">{label}</span>
          <span className={cn('text-[10px] font-semibold', stColor)}>
            {statusLabel(status)}
          </span>
        </div>
        {detail && (
          <p className="text-foreground-alt/50 mt-0.5 text-[0.6rem]">
            {detail}
          </p>
        )}
      </div>
      <span className="text-brand/60 shrink-0 self-center text-xs">Manage</span>
    </button>
  )
}
