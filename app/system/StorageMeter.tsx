import { cn } from '@s4wave/web/style/utils.js'

import { formatBytes } from './format.js'

// StorageMeter draws browser storage usage against its quota. It renders
// nothing when the browser did not report both numbers.
export function StorageMeter({
  usage,
  quota,
  compact = false,
}: {
  usage: number | null
  quota: number | null
  compact?: boolean
}) {
  if (usage == null || quota == null || quota <= 0) {
    return null
  }

  const ratio = Math.min(usage / quota, 1)
  const label = `${formatBytes(usage)} of ${formatBytes(quota)} browser quota`
  return (
    <div>
      <div
        role="meter"
        aria-label="Browser storage used"
        aria-valuemin={0}
        aria-valuemax={quota}
        aria-valuenow={usage}
        aria-valuetext={label}
        className={cn(
          'bg-foreground/8 overflow-hidden rounded-full',
          compact ? 'h-1' : 'h-1.5',
        )}
      >
        <div
          className={cn(
            'progress-width progress-width-transition-slow h-full rounded-full motion-reduce:transition-none',
            ratio >= 0.9 ? 'bg-warning' : 'bg-foreground-alt/60',
          )}
          style={{ '--progress-width': `${Math.max(ratio * 100, 1)}%` }}
        />
      </div>
      {!compact && (
        <p className="text-foreground-alt/60 mt-1.5 font-mono text-xs tabular-nums">
          {label}
        </p>
      )}
    </div>
  )
}
