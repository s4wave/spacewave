import { useMemo } from 'react'
import { cn } from '@s4wave/web/style/utils.js'

export type StateBadgeVariant = 'pill' | 'dot'

export type StateBadgeTone =
  | 'idle'
  | 'active'
  | 'success'
  | 'warning'
  | 'error'

interface StateBadgeProps {
  state: number
  labels: Record<number, string>
  variant?: StateBadgeVariant
}

// stateTone maps a state label keyword to a semantic tone bucket.
function stateTone(label: string): StateBadgeTone {
  const l = label.toLowerCase()
  if (l.includes('complete')) return 'success'
  if (l.includes('running') || l.includes('checking') || l.includes('starting'))
    return 'active'
  if (l.includes('stopping')) return 'warning'
  if (l.includes('retry') || l.includes('error')) return 'error'
  return 'idle'
}

const PILL_TONE: Record<StateBadgeTone, string> = {
  idle: 'border-foreground/15 bg-foreground/5 text-foreground-alt/70',
  active: 'border-blue-400/20 bg-blue-400/8 text-blue-300',
  success: 'border-emerald-400/20 bg-emerald-400/8 text-emerald-300',
  warning: 'border-amber-400/20 bg-amber-400/8 text-amber-300',
  error: 'border-destructive/30 bg-destructive/8 text-destructive',
}

const DOT_TONE: Record<StateBadgeTone, string> = {
  idle: 'bg-foreground/30',
  active: 'bg-blue-400',
  success: 'bg-emerald-400',
  warning: 'bg-amber-400',
  error: 'bg-destructive',
}

// StateBadge renders a tone-tinted pill or dot+label for a proto enum state.
export function StateBadge({ state, labels, variant = 'pill' }: StateBadgeProps) {
  const label = useMemo(() => labels[state] ?? 'UNKNOWN', [state, labels])
  const tone = useMemo(() => stateTone(label), [label])
  if (variant === 'dot') {
    return (
      <span className="text-foreground-alt/70 inline-flex items-center gap-1.5 text-[0.6rem] font-medium tracking-widest uppercase select-none">
        <span className={cn('h-1.5 w-1.5 rounded-full', DOT_TONE[tone])} />
        {label}
      </span>
    )
  }
  return (
    <span
      className={cn(
        'inline-flex items-center rounded-full border px-2 py-0.5 text-[0.6rem] font-medium tracking-widest uppercase select-none',
        PILL_TONE[tone],
      )}
    >
      {label}
    </span>
  )
}
