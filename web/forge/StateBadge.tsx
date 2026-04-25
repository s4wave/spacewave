import { useMemo } from 'react'
import { cn } from '@s4wave/web/style/utils.js'

interface StateBadgeProps {
  state: number
  labels: Record<number, string>
}

// stateColor maps common state keywords to tailwind color classes.
function stateColor(label: string): string {
  const l = label.toLowerCase()
  if (l.includes('pending') || l.includes('unknown') || l.includes('stopped'))
    return 'bg-neutral-600'
  if (l.includes('running') || l.includes('checking') || l.includes('starting'))
    return 'bg-blue-600'
  if (l.includes('complete')) return 'bg-green-600'
  if (l.includes('stopping')) return 'bg-yellow-600'
  if (l.includes('retry') || l.includes('error')) return 'bg-red-600'
  return 'bg-neutral-500'
}

// StateBadge renders a colored badge for a proto enum state value.
export function StateBadge({ state, labels }: StateBadgeProps) {
  const label = useMemo(() => labels[state] ?? 'UNKNOWN', [state, labels])
  return (
    <span
      className={cn(
        'inline-flex items-center rounded px-1.5 py-0.5 text-xs font-medium text-white',
        stateColor(label),
      )}
    >
      {label}
    </span>
  )
}
