import type { ReactNode } from 'react'
import { cn } from '@s4wave/web/style/utils.js'

// StatItem represents a single statistic to display.
export interface StatItem {
  // label is the stat label text.
  label: string
  // value is the stat value (can be string, number, or ReactNode).
  value: ReactNode
  // valueClassName is an optional class for the value element.
  valueClassName?: string
}

// StatsBarProps are the props for the StatsBar component.
export interface StatsBarProps {
  // stats is the array of statistics to display.
  stats: StatItem[]
  // className is an optional CSS class for the container.
  className?: string
}

// StatsBar displays a horizontal bar of key-value statistics.
export function StatsBar({ stats, className }: StatsBarProps) {
  if (stats.length === 0) return null

  return (
    <div
      className={cn(
        'bg-background-secondary border-ui-outline flex items-center gap-4 rounded border px-2 py-1 text-xs',
        className,
      )}
    >
      {stats.map((stat, i) => (
        <span key={i} className="text-foreground-alt">
          {stat.label}:{' '}
          <span className={stat.valueClassName ?? 'text-foreground'}>
            {stat.value}
          </span>
        </span>
      ))}
    </div>
  )
}
