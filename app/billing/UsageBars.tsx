import type { ReactNode } from 'react'
import { LuGauge } from '@s4wave/web/images/icons.js'

import { cn } from '@s4wave/web/style/utils.js'
import { formatBytes } from '@s4wave/web/transform/TransformConfigDisplay.js'
import type { BillingUsageInfo } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'

import { formatStorageRate } from '../provider/spacewave/pricing.js'
import { useBillingStateContext } from './BillingStateProvider.js'
import { useSpendingLimit } from './useSpendingLimit.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  colors as themeColors,
  radii as themeRadii,
  utilityState as themeUtilityState,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule2: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-wider)',
    letterSpacing: 'var(--tracking-wider)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    textTransform: 'uppercase',
  },
  rule3: {
    marginTop: `calc(${themeLayout['--spacing']} * -2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 45%, transparent)`,
  },
  rule4: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-yellow-400']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-yellow-400']} 10%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
  },
  rule5: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule6: {
    marginTop: themeLayout['--spacing'],
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule7: {
    display: 'flex',
    justifyContent: 'space-between',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule8: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule9: {
    display: 'flex',
    alignItems: 'flex-start',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule10: { minWidth: 0 },
  rule11: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule12: {
    display: 'grid',
    gridTemplateColumns: 'repeat(3, minmax(0, 1fr))',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule13: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule14: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
  rule15: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule16: {
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 4%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule17: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    '--tw-numeric-spacing': 'tabular-nums',
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
  },
  rule18: {
    marginLeft: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-normal)',
    fontWeight: 'var(--font-weight-normal)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
  rule19: {
    marginBottom: themeLayout['--spacing'],
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
  },
  rule20: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule21: {
    height: `calc(${themeLayout['--spacing']} * 1.5)`,
    width: '100%',
    overflow: 'hidden',
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

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
    <div
      className={
        /* TODO(stylex): a descendant, group, peer or compound selector: space-y-3 */ 'space-y-3'
      }
    >
      <div sx={stylexStyles2.rule1}>
        <div sx={stylexStyles2.rule2}>Usage</div>
        {props.actions}
      </div>
      {meteredThroughAt > 0 && (
        <div sx={stylexStyles2.rule3}>
          Usage metered through {formatMeteredThrough(meteredThroughAt)}
        </div>
      )}
      {storageRatio >= SOFT_USAGE_ALERT_RATIO && (
        <div sx={stylexStyles2.rule4}>
          <div sx={stylexStyles2.rule5}>Included storage alert</div>
          <div sx={stylexStyles2.rule6}>
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
      <div sx={stylexStyles2.rule7}>
        <span>Writes {formatCount(writeOps)}</span>
        <span>Reads {formatCount(readOps)}</span>
        <span sx={stylexStyles2.rule8}>Not billed</span>
      </div>
      <ExtraStoragePanel usage={usage} />
    </div>
  )
}

// ExtraStoragePanel shows the payer's extra storage limit, its spending this
// period, and the control that changes the limit.
function ExtraStoragePanel({ usage }: { usage: BillingUsageInfo }) {
  const billingState = useBillingStateContext()
  const spending = useSpendingLimit(usage)
  const { offer } = spending

  // Read the limit, spending, and period of the payer.
  const storageUsed = usage.storageBytes ?? 0
  const storageBaseline = usage.storageBaselineBytes ?? 1
  const overageLimit = (usage.overageLimitCents ?? 0) / 100
  const accrued = Number(usage.accruedOverageMicrodollars ?? 0n) / 1_000_000
  const periodStart = Number(usage.currentPeriodStart ?? 0n)
  const periodEnd = Number(usage.currentPeriodEnd ?? 0n)
  const paused = accrued >= overageLimit && storageUsed >= storageBaseline

  return (
    <div
      className={
        /* TODO(stylex): a descendant, group, peer or compound selector: space-y-3 */ 'border-foreground/8 space-y-3 rounded-md border p-3'
      }
    >
      {spending.consentDialog}
      <div sx={stylexStyles2.rule9}>
        <div sx={stylexStyles2.rule10}>
          <div sx={stylexStyles2.rule5}>Extra storage</div>
          <div sx={stylexStyles2.rule11}>
            {overageLimit
              ? `${formatCurrency(overageLimit)} monthly limit`
              : 'Off'}{' '}
            · {formatStorageRate(offer.storageMicrodollarsPerGibMonth)}
          </div>
        </div>
        {billingState.selfServiceAllowed && (
          <DashboardButton
            icon={<LuGauge sx={stylexStyles.rule1} />}
            disabled={spending.saving || !spending.canChange}
            onClick={() => void spending.changeLimit()}
          >
            {spending.saving ? 'Saving…' : 'Change limit'}
          </DashboardButton>
        )}
      </div>
      <dl sx={stylexStyles2.rule12}>
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
        <div sx={stylexStyles2.rule13}>
          Period {formatPeriodDate(periodStart)} – {formatPeriodDate(periodEnd)}{' '}
          · Spending resets {formatResetTime(periodEnd)}
        </div>
      )}
      {paused && (
        <div sx={stylexStyles2.rule4}>
          Accrued charges remain payable. Uploads are paused until the period
          renews or you raise the limit.
        </div>
      )}
      <p sx={stylexStyles2.rule14}>
        Storage above the included amount is measured hourly. Reads and writes
        are not billed; rate limits protect the service. One GiB is
        1,073,741,824 bytes.
      </p>
      {spending.error && (
        <p sx={stylexStyles2.rule15} role="alert">
          {spending.error}
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
    <div sx={stylexStyles2.rule16}>
      <dt sx={stylexStyles2.rule11}>{props.label}</dt>
      <dd sx={stylexStyles2.rule17}>
        {props.value}
        {props.detail && <span sx={stylexStyles2.rule18}>{props.detail}</span>}
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
      <div sx={stylexStyles2.rule19}>
        <span sx={stylexStyles2.rule20}>{props.label}</span>
        <span sx={stylexStyles2.rule11}>
          {props.formatValue(props.used)} / {props.formatValue(props.baseline)}
        </span>
      </div>
      <div sx={stylexStyles2.rule21}>
        <div
          className={
            /* TODO(stylex): fallback class expression */ cn(
              'progress-width h-full rounded-full transition-all',
              barClassName,
            )
          }
          style={{ '--progress-width': `${pct}%` }}
        />
      </div>
    </div>
  )
}
