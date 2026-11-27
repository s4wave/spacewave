import * as React from 'react'

import { cn } from '@s4wave/web/style/utils.js'

/** contextUsage clamps a negative used count and caps the meter at 100 percent. */
function contextUsage(used: number, limit: number) {
  const shown = Math.max(0, used)
  const percent =
    limit > 0 ? Math.min(100, Math.round((shown / limit) * 100)) : 0
  return { shown, percent }
}

/**
 * Context renders used context as a meter. A zero limit gives an empty meter,
 * not an error.
 */
function Context({
  used,
  limit,
  className,
  ...props
}: React.ComponentProps<'div'> & {
  used: number
  limit: number
}) {
  const { shown, percent } = contextUsage(used, limit)

  return (
    <div
      data-slot="context"
      role="progressbar"
      aria-label="Context used"
      aria-valuemax={100}
      aria-valuemin={0}
      aria-valuenow={percent}
      aria-valuetext={`${shown} of ${limit}`}
      className={cn('flex w-full min-w-0 flex-col gap-1 text-sm', className)}
      {...props}
    >
      <div className="flex items-center justify-between gap-3">
        <span className="font-medium">{percent}%</span>
        <span className="text-muted-foreground tabular-nums">
          {shown} / {limit}
        </span>
      </div>
      <div className="bg-muted h-1.5 overflow-hidden rounded-full">
        <div
          data-slot="context-meter"
          className="bg-foreground h-full w-(--context-used)"
          style={{ '--context-used': `${percent}%` }}
        />
      </div>
    </div>
  )
}

export { Context }
