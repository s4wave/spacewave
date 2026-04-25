// ProgressBarProps describes the inputs to ProgressBar.
interface ProgressBarProps {
  // value is the determinate progress value, 0..100.
  value?: number
  // indeterminate switches the bar to the sweeping animated variant.
  indeterminate?: boolean
  // rate is an optional right-side label shown in place of the percent.
  rate?: string
}

// ProgressBar renders a slim progress track with an optional right-side label.
// Determinate mode shows percent or a rate label; indeterminate mode runs the
// animate-progress-indeterminate keyframe.
export function ProgressBar({ value, indeterminate, rate }: ProgressBarProps) {
  const pct =
    value === undefined ? 0 : Math.max(0, Math.min(100, Math.round(value)))
  return (
    <div className="flex w-full items-center gap-3">
      <div className="bg-foreground/8 relative h-1.5 flex-1 overflow-hidden rounded-full">
        {indeterminate ?
          <div className="bg-brand animate-progress-indeterminate absolute inset-y-0 w-1/3 rounded-full" />
        : <div
            className="bg-brand h-full rounded-full transition-[width] duration-200"
            style={{ width: `${pct}%` }}
          />
        }
      </div>
      {rate ?
        <span className="text-foreground-alt/70 font-mono text-[0.65rem] tabular-nums">
          {rate}
        </span>
      : !indeterminate ?
        <span className="text-foreground-alt/70 w-8 text-right font-mono text-[0.65rem] tabular-nums">
          {pct}%
        </span>
      : null}
    </div>
  )
}
