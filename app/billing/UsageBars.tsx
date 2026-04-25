import type { ReactNode } from 'react'

import { cn } from '@s4wave/web/style/utils.js'
import { formatBytes } from '@s4wave/web/transform/TransformConfigDisplay.js'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@s4wave/web/ui/tooltip.js'

import { OVERAGE_STORAGE_PER_GB } from '../provider/spacewave/pricing.js'
import { useBillingStateContext } from './BillingStateProvider.js'

const BYTES_PER_GB = 1024 * 1024 * 1024

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

// UsageBars shows storage, write ops, and read ops progress bars.
export function UsageBars(props: { actions?: ReactNode }) {
  const billingState = useBillingStateContext()
  const usage = billingState.response?.usage
  if (!usage) return null

  const storageUsed = usage.storageBytes ?? 0
  const storageBaseline = usage.storageBaselineBytes ?? 1
  const writeOps = Number(usage.writeOps ?? 0n)
  const writeBaseline = Number(usage.writeOpsBaseline ?? 1n)
  const readOps = Number(usage.readOps ?? 0n)
  const readBaseline = Number(usage.readOpsBaseline ?? 1n)
  const extraStorageBytes =
    usage.storageOverageBytes ?? Math.max(storageUsed - storageBaseline, 0)
  const extraStorageGB = extraStorageBytes / BYTES_PER_GB
  const extraStorageCost =
    usage.storageOverageMonthlyCostEstimateUsd ??
    extraStorageGB * OVERAGE_STORAGE_PER_GB
  const monthToDateGbMonths = usage.storageOverageMonthToDateGbMonths ?? 0
  const monthToDateCost = usage.storageOverageMonthToDateCostEstimateUsd ?? 0
  const deletedGbMonths = usage.storageOverageDeletedGbMonths ?? 0
  const deletedCost = usage.storageOverageDeletedCostEstimateUsd ?? 0
  const meteredThroughAt = Number(usage.usageMeteredThroughAt ?? 0n)

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-2">
        <div className="text-foreground-alt/60 text-xs font-medium tracking-wider uppercase">
          Usage
        </div>
        {props.actions}
      </div>
      {meteredThroughAt > 0 && (
        <div className="text-foreground-alt/45 -mt-2 text-[0.6rem]">
          Usage metered through {formatMeteredThrough(meteredThroughAt)}
        </div>
      )}
      <div className="space-y-2">
        <UsageBar
          label="Storage"
          used={storageUsed}
          baseline={storageBaseline}
          formatValue={formatBytes}
          barClassName="bg-blue-500"
        />
        {extraStorageBytes > 0 && (
          <div className="rounded-md border border-blue-400/10 bg-blue-400/5 px-2.5 py-1.5 text-[0.6rem]">
            <div className="flex items-center justify-between gap-2">
              <div className="text-foreground-alt/60 flex min-w-0 items-center gap-1.5">
                <span className="h-1 w-1 shrink-0 rounded-full bg-blue-400/80" />
                <span>Extra storage</span>
              </div>
              <div className="text-foreground-alt/50 text-right">
                <span className="text-foreground">
                  {formatBytes(extraStorageBytes)}
                </span>{' '}
                @{' '}
                <span className="text-foreground">
                  ${OVERAGE_STORAGE_PER_GB.toFixed(2)}
                </span>{' '}
                per GB/mo ={' '}
                <span className="text-foreground font-medium">
                  {formatCurrency(extraStorageCost)}/mo
                </span>
              </div>
            </div>
            <UsageCostLine
              label="Month-to-date overage"
              tooltip="Accrued storage overage for this billing period so far. If usage drops later, future accrual slows or stops, but already accrued usage remains part of this period."
              value={`${formatGbMonths(monthToDateGbMonths)} = ${formatCurrency(monthToDateCost)} estimated`}
            />
            {deletedCost > 0 && (
              <UsageCostLine
                label="Already-deleted data"
                tooltip="Additional storage overage already accrued from data that is no longer stored."
                value={`${formatGbMonths(deletedGbMonths)} = +${formatCurrency(deletedCost)} estimated`}
              />
            )}
          </div>
        )}
      </div>
      <UsageBar
        label="Write Ops"
        used={writeOps}
        baseline={writeBaseline}
        formatValue={formatCount}
      />
      <UsageBar
        label="Cloud Reads"
        used={readOps}
        baseline={readBaseline}
        formatValue={formatCount}
      />
    </div>
  )
}

function formatGbMonths(value: number): string {
  if (value > 0 && value < 0.001) return '<0.001 GB-months'
  return `${value.toFixed(3)} GB-months`
}

function formatMeteredThrough(value: number): string {
  return `${new Date(value).toISOString().replace('T', ' ').slice(0, 16)} UTC`
}

function UsageCostLine(props: {
  label: string
  tooltip: string
  value: string
}) {
  return (
    <div className="border-foreground/5 mt-1.5 flex items-center justify-between gap-2 border-t pt-1.5">
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            type="button"
            className="text-foreground-alt/55 cursor-help underline decoration-dotted underline-offset-2"
          >
            {props.label}
          </button>
        </TooltipTrigger>
        <TooltipContent side="top" className="max-w-xs">
          {props.tooltip}
        </TooltipContent>
      </Tooltip>
      <span className="text-foreground-alt/50 text-right">{props.value}</span>
    </div>
  )
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
          className={cn('h-full rounded-full transition-all', barClassName)}
          style={{ width: `${pct}%` }}
        />
      </div>
    </div>
  )
}
