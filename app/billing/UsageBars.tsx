import { useState, type ReactNode } from 'react'
import { LuGauge } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'
import { formatBytes } from '@s4wave/web/transform/TransformConfigDisplay.js'
import type { BillingUsageInfo } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'

import { useBillingConsent } from '../provider/spacewave/useBillingConsent.js'
import {
  CLOUD_OFFER,
  formatStorageRate,
} from '../provider/spacewave/pricing.js'
import { useBillingStateContext } from './BillingStateProvider.js'

const SOFT_USAGE_ALERT_RATIO = 0.8

// formatCount abbreviates an operation count.
function formatCount(n: number): string {
  if (n < 1000) return String(n)
  if (n < 1_000_000) return `${(n / 1000).toFixed(1)}K`
  return `${(n / 1_000_000).toFixed(1)}M`
}

// thresholdBarColor colors a usage bar by how full it is.
function thresholdBarColor(ratio: number): string {
  if (ratio < 0.7) return 'bg-green-500'
  if (ratio < 0.9) return 'bg-yellow-500'
  return 'bg-red-500'
}

// formatCurrency formats a dollar amount, showing a nonzero sub-cent amount.
function formatCurrency(amount: number): string {
  if (amount > 0 && amount < 0.01) return '<$0.01'
  return `$${amount.toFixed(2)}`
}

// UsageBars shows included storage, unbilled operation counts, and the
// payer's extra storage spending.
export function UsageBars(props: { actions?: ReactNode }) {
  const billingState = useBillingStateContext()
  const usage = billingState.response?.usage
  if (!usage) return null

  // Read the storage and operation counts of the payer.
  const storageUsed = usage.storageBytes ?? 0
  const storageBaseline = usage.storageBaselineBytes ?? 1
  const writeOps = Number(usage.writeOps ?? 0n)
  const readOps = Number(usage.readOps ?? 0n)
  const meteredThroughAt = Number(usage.usageMeteredThroughAt ?? 0n)
  const storageRatio = storageBaseline > 0 ? storageUsed / storageBaseline : 0

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-2">
        <div className="text-foreground-alt/60 text-xs font-medium tracking-wider uppercase">
          Usage
        </div>
        {props.actions}
      </div>
      {meteredThroughAt > 0 && (
        <div className="text-foreground-alt/45 -mt-2 text-xs">
          Usage metered through {formatMeteredThrough(meteredThroughAt)}
        </div>
      )}
      {storageRatio >= SOFT_USAGE_ALERT_RATIO && (
        <div className="rounded-md border border-yellow-400/20 bg-yellow-400/10 px-2.5 py-2 text-xs leading-relaxed">
          <div className="text-foreground text-xs font-medium">
            Included storage alert
          </div>
          <div className="text-foreground-alt/60 mt-1">
            Storage has reached {Math.round(storageRatio * 100)}% of included
            storage.
          </div>
        </div>
      )}
      <UsageBar
        label="Storage"
        used={storageUsed}
        baseline={storageBaseline}
        formatValue={formatBytes}
        barClassName="bg-blue-500"
      />
      <div className="text-foreground-alt/70 flex justify-between text-xs">
        <span>Writes {formatCount(writeOps)}</span>
        <span>Reads {formatCount(readOps)}</span>
        <span className="text-foreground-alt/50">Not billed</span>
      </div>
      <ExtraStoragePanel usage={usage} />
    </div>
  )
}

// ExtraStoragePanel shows the payer's extra storage limit, its spending this
// period, and the control that changes the limit.
function ExtraStoragePanel({ usage }: { usage: BillingUsageInfo }) {
  const billingState = useBillingStateContext()
  const session = SessionContext.useContext().value
  const { requestConsent, consentDialog } = useBillingConsent()
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Read the limit, spending, and period of the payer.
  const storageUsed = usage.storageBytes ?? 0
  const storageBaseline = usage.storageBaselineBytes ?? 1
  const overageLimit = (usage.overageLimitCents ?? 0) / 100
  const accrued = Number(usage.accruedOverageMicrodollars ?? 0n) / 1_000_000
  const periodStart = Number(usage.currentPeriodStart ?? 0n)
  const periodEnd = Number(usage.currentPeriodEnd ?? 0n)
  const paused = accrued >= overageLimit && storageUsed >= storageBaseline

  // Describe the payer's accepted offer for the spending limit consent.
  const offer = {
    ...CLOUD_OFFER,
    version: usage.offerVersion ?? CLOUD_OFFER.version,
    policyVersion: usage.policyVersion ?? CLOUD_OFFER.policyVersion,
    monthlyPriceCents: usage.monthlyPriceCents ?? CLOUD_OFFER.monthlyPriceCents,
    storageMicrodollarsPerGibMonth:
      usage.storageMicrodollarsPerGibMonth ??
      CLOUD_OFFER.storageMicrodollarsPerGibMonth,
    storageBytes: storageBaseline,
  }

  // changeLimit asks for consent to a new spending limit and stores it.
  async function changeLimit() {
    if (!session || saving) return
    const consent = await requestConsent(
      usage.overageLimitCents ?? 0,
      true,
      offer,
    )
    if (!consent) return
    setSaving(true)
    setError(null)
    try {
      await session.spacewave.setBillingSpendingLimit(
        consent,
        billingState.billingAccountId,
      )
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="border-foreground/8 space-y-3 rounded-md border p-3">
      {consentDialog}
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <div className="text-foreground text-xs font-medium">
            Extra storage
          </div>
          <div className="text-foreground-alt/50 text-xs">
            {overageLimit
              ? `${formatCurrency(overageLimit)} monthly limit`
              : 'Off'}{' '}
            · {formatStorageRate(offer.storageMicrodollarsPerGibMonth)}
          </div>
        </div>
        {billingState.selfServiceAllowed && (
          <DashboardButton
            icon={<LuGauge className="size-3" />}
            disabled={saving || !session}
            onClick={() => void changeLimit()}
          >
            {saving ? 'Saving…' : 'Change limit'}
          </DashboardButton>
        )}
      </div>
      <dl className="grid grid-cols-3 gap-2">
        <SpendingStat label="Accrued" value={formatCurrency(accrued)} />
        <SpendingStat
          label="Available"
          value={formatCurrency(Math.max(0, overageLimit - accrued))}
        />
        <SpendingStat
          label="Maximum"
          value={formatCurrency(offer.monthlyPriceCents / 100 + overageLimit)}
          detail="before tax"
        />
      </dl>
      {periodEnd > 0 && (
        <div className="text-foreground-alt/60 text-xs">
          Period {formatPeriodDate(periodStart)} – {formatPeriodDate(periodEnd)}{' '}
          · Spending resets {formatResetTime(periodEnd)}
        </div>
      )}
      {paused && (
        <div className="rounded-md border border-yellow-400/20 bg-yellow-400/10 px-2.5 py-2 text-xs leading-relaxed">
          Accrued charges remain payable. Uploads are paused until the period
          renews or you raise the limit.
        </div>
      )}
      <p className="text-foreground-alt/40 text-xs leading-relaxed">
        Storage above the included amount is measured hourly. Reads and writes
        are not billed; rate limits protect the service. One GiB is
        1,073,741,824 bytes.
      </p>
      {error && (
        <p className="text-destructive text-xs" role="alert">
          {error}
        </p>
      )}
    </div>
  )
}

// formatPeriodDate formats a period boundary as a short date.
function formatPeriodDate(value: number): string {
  return new Date(value).toLocaleDateString(undefined, {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  })
}

// formatResetTime formats the spending reset as a date and time.
function formatResetTime(value: number): string {
  return new Date(value).toLocaleString(undefined, {
    dateStyle: 'medium',
    timeStyle: 'short',
  })
}

// formatMeteredThrough formats the metering watermark in UTC.
function formatMeteredThrough(value: number): string {
  return `${new Date(value).toISOString().replace('T', ' ').slice(0, 16)} UTC`
}

// SpendingStat renders one labeled amount of the extra storage panel.
function SpendingStat(props: {
  label: string
  value: string
  detail?: string
}) {
  return (
    <div className="bg-foreground/4 rounded-md px-2.5 py-2">
      <dt className="text-foreground-alt/50 text-xs">{props.label}</dt>
      <dd className="text-foreground text-sm font-medium tabular-nums">
        {props.value}
        {props.detail && (
          <span className="text-foreground-alt/40 ml-1 text-xs font-normal">
            {props.detail}
          </span>
        )}
      </dd>
    </div>
  )
}

// UsageBar renders one usage meter against its included amount.
function UsageBar(props: {
  label: string
  used: number
  baseline: number
  formatValue: (n: number) => string
  barClassName?: string
}) {
  const ratio = props.baseline > 0 ? props.used / props.baseline : 0
  const pct = Math.min(ratio * 100, 100)
  const barClassName = props.barClassName ?? thresholdBarColor(ratio)

  return (
    <div>
      <div className="mb-1 flex items-center justify-between">
        <span className="text-foreground-alt/70 text-xs">{props.label}</span>
        <span className="text-foreground-alt/50 text-xs">
          {props.formatValue(props.used)} / {props.formatValue(props.baseline)}
        </span>
      </div>
      <div className="bg-foreground/8 h-1.5 w-full overflow-hidden rounded-full">
        <div
          className={cn(
            'progress-width h-full rounded-full transition-all',
            barClassName,
          )}
          style={{ '--progress-width': `${pct}%` }}
        />
      </div>
    </div>
  )
}
