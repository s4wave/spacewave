import { LuCircleAlert, LuCircleCheck, LuCloud } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'

import { ProgressBar } from './ProgressBar.js'
import { Spinner } from './Spinner.js'
import type { LoadingState, LoadingView } from './types.js'

interface LoadingCardProps {
  view: LoadingView
  className?: string
}

// LoadingCard is the primary loading surface. Glass card with an h-8 w-8 icon
// box, title, detail, optional progress, rate pills, last-activity footer,
// error box, and retry / cancel buttons. Four states: loading / active /
// synced / error.
export function LoadingCard({ view, className }: LoadingCardProps) {
  const { state } = view
  return (
    <div
      className={cn(
        'border-foreground/6 bg-background-card/30 rounded-lg border p-3.5 backdrop-blur-sm',
        className,
      )}
    >
      <div className="flex items-start gap-3">
        <div
          className={cn(
            'flex h-8 w-8 shrink-0 items-center justify-center rounded-md',
            state === 'loading' && 'bg-foreground/5 text-foreground-alt',
            state === 'active' && 'bg-brand/10 text-brand',
            state === 'synced' && 'bg-foreground/5 text-brand',
            state === 'error' && 'bg-destructive/10 text-destructive',
          )}
        >
          <LoadingCardIcon state={state} />
        </div>
        <div className="min-w-0 flex-1">
          <div className="text-foreground text-sm font-semibold tracking-tight">
            {view.title}
          </div>
          {view.detail ?
            <div className="text-foreground-alt/60 mt-0.5 text-xs leading-relaxed">
              {view.detail}
            </div>
          : null}
          {view.progress !== undefined ?
            <div className="mt-2.5">
              <ProgressBar
                value={view.progress * 100}
                rate={view.rate?.down ?? view.rate?.up}
              />
            </div>
          : null}
          {view.rate && view.progress === undefined ?
            <div className="mt-2 grid grid-cols-2 gap-2">
              <RatePill label="Up" value={view.rate.up ?? '0 B/s'} />
              <RatePill label="Down" value={view.rate.down ?? '0 B/s'} />
            </div>
          : null}
          {view.lastActivity ?
            <div className="text-foreground-alt/40 mt-2 text-[0.65rem]">
              {view.lastActivity}
            </div>
          : null}
          {view.error ?
            <div className="bg-destructive/5 border-destructive/15 text-destructive mt-2 rounded-md border px-2 py-1 text-[0.65rem] leading-relaxed">
              {view.error}
            </div>
          : null}
          {view.onRetry || view.onCancel ?
            <div className="mt-2.5 flex gap-2">
              {view.onRetry ?
                <button
                  type="button"
                  onClick={view.onRetry}
                  className="border-foreground/8 bg-foreground/5 hover:bg-foreground/10 hover:border-foreground/15 text-foreground-alt hover:text-foreground rounded-md border px-2 py-1 text-[0.65rem] font-medium transition-all duration-150"
                >
                  Retry
                </button>
              : null}
              {view.onCancel ?
                <button
                  type="button"
                  onClick={view.onCancel}
                  className="text-foreground-alt/60 hover:text-foreground-alt rounded-md px-2 py-1 text-[0.65rem] font-medium transition-colors"
                >
                  Cancel
                </button>
              : null}
            </div>
          : null}
        </div>
      </div>
    </div>
  )
}

function LoadingCardIcon({ state }: { state: LoadingState }) {
  if (state === 'error') {
    return <LuCircleAlert className="h-4 w-4" aria-hidden="true" />
  }
  if (state === 'synced') {
    return (
      <span
        className="relative flex h-4 w-4 items-center justify-center"
        aria-hidden="true"
      >
        <LuCloud className="h-3.5 w-3.5" />
        <LuCircleCheck className="text-brand absolute -right-1 -bottom-1 h-2.5 w-2.5" />
      </span>
    )
  }
  return <Spinner size="md" />
}

function RatePill({ label, value }: { label: string; value: string }) {
  return (
    <div className="border-foreground/6 bg-foreground/5 rounded-md border px-2 py-1">
      <div className="text-foreground-alt/50 text-[0.55rem] font-medium tracking-widest uppercase">
        {label}
      </div>
      <div className="text-foreground text-xs font-semibold tabular-nums">
        {value}
      </div>
    </div>
  )
}
