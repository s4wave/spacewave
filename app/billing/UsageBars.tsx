import { useState, type ReactNode } from 'react'
import { cn } from '@s4wave/web/style/utils.js'
import { formatBytes } from '@s4wave/web/transform/TransformConfigDisplay.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { useBillingConsent } from '../provider/spacewave/useBillingConsent.js'
import {
  CLOUD_OFFER,
  formatStorageRate,
} from '../provider/spacewave/pricing.js'
import { useBillingStateContext } from './BillingStateProvider.js'

const SOFT_USAGE_ALERT_RATIO = 0.8

function formatCount(n: number): string {
  if (n < 1000) return String(n)
  if (n < 1_000_000) return `${(n / 1000).toFixed(1)}K`
  return `${(n / 1_000_000).toFixed(1)}M`
}

function thresholdBarColor(ratio: number): string {
  if (ratio < 0.7) return 'bg-green-500'
  if (ratio < 0.9) return 'bg-yellow-500'
  return 'bg-red-500'
}

function formatCurrency(amount: number): string {
  if (amount > 0 && amount < 0.01) return '<$0.01'
  return `$${amount.toFixed(2)}`
}

// UsageBars shows included storage, unbilled operation counts, and the
// payer's extra storage spending.
export function UsageBars(props: { actions?: ReactNode }) {
  const billingState = useBillingStateContext()
  const session = SessionContext.useContext().value
  const { requestConsent, consentDialog } = useBillingConsent()
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const usage = billingState.response?.usage
  if (!usage) return null

  const storageUsed = usage.storageBytes ?? 0
  const storageBaseline = usage.storageBaselineBytes ?? 1
  const writeOps = Number(usage.writeOps ?? 0n)
  const readOps = Number(usage.readOps ?? 0n)
  const overageLimit = (usage.overageLimitCents ?? 0) / 100
  const accrued = Number(usage.accruedOverageMicrodollars ?? 0n) / 1_000_000
  const periodStart = Number(usage.currentPeriodStart ?? 0n)
  const periodEnd = Number(usage.currentPeriodEnd ?? 0n)

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

  async function changeLimit() {
    if (!session || saving) return
    const consent = await requestConsent(
      usage?.overageLimitCents ?? 0,
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
  const meteredThroughAt = Number(usage.usageMeteredThroughAt ?? 0n)
  const storageRatio = storageBaseline > 0 ? storageUsed / storageBaseline : 0

  return (
    <div className="space-y-3">
      {consentDialog}
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
      <div className="text-foreground-alt space-y-2 rounded border p-3 text-xs">
        <div>
          Extra storage:{' '}
          {overageLimit
            ? `${formatCurrency(overageLimit)} monthly limit`
            : 'off'}
          . Service maximum:{' '}
          {formatCurrency(offer.monthlyPriceCents / 100 + overageLimit)} before
          tax.
        </div>
        <div>
          Accrued: {formatCurrency(accrued)} · Available:{' '}
          {formatCurrency(Math.max(0, overageLimit - accrued))}
        </div>
        {periodEnd > 0 && (
          <div>
            Subscription period: {new Date(periodStart).toLocaleDateString()} –{' '}
            {new Date(periodEnd).toLocaleDateString()}. Spending resets{' '}
            {new Date(periodEnd).toLocaleString()}.
          </div>
        )}
        <p>
          Storage above the included amount costs{' '}
          {formatStorageRate(offer.storageMicrodollarsPerGibMonth)}, measured
          hourly. Reads and writes are not billed; rate limits protect the
          service. Change or turn off the monthly spending limit below. One GiB
          is 1,073,741,824 bytes.
        </p>
        {accrued >= overageLimit && storageUsed >= storageBaseline && (
          <p>
            Accrued charges remain payable. Uploads are paused until the period
            renews or you raise the limit.
          </p>
        )}
        {billingState.selfServiceAllowed && (
          <button
            type="button"
            className="text-brand underline disabled:opacity-50"
            disabled={saving || !session}
            onClick={() => void changeLimit()}
          >
            {saving ? 'Saving…' : 'Change spending limit'}
          </button>
        )}
        {error && (
          <p className="text-destructive" role="alert">
            {error}
          </p>
        )}
      </div>
    </div>
  )
}

function formatMeteredThrough(value: number): string {
  return `${new Date(value).toISOString().replace('T', ' ').slice(0, 16)} UTC`
}

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
