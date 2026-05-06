import { LuCheck, LuKeyRound, LuShield } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'
import { ProgressBar } from '@s4wave/web/ui/loading/ProgressBar.js'
import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'

interface AuthProgressCardProps {
  title: string
  detail: string
  steps: string[]
  className?: string
}

// AuthProgressCard shows honest indeterminate progress for auth flows whose
// crypto and provider work does not expose reliable percent completion.
export function AuthProgressCard({
  title,
  detail,
  steps,
  className,
}: AuthProgressCardProps) {
  return (
    <div
      className={cn(
        'border-foreground/6 bg-background-card/30 rounded-lg border p-3.5 backdrop-blur-sm',
        className,
      )}
      role="status"
      aria-live="polite"
    >
      <div className="flex items-start gap-3">
        <div className="bg-brand/10 text-brand flex h-8 w-8 shrink-0 items-center justify-center rounded-md">
          <Spinner size="md" />
        </div>
        <div className="min-w-0 flex-1">
          <div className="text-foreground text-sm font-semibold tracking-tight">
            {title}
          </div>
          <div className="text-foreground-alt/60 mt-0.5 text-xs leading-relaxed">
            {detail}
          </div>
          <div className="mt-2.5">
            <ProgressBar indeterminate />
          </div>
          <div className="mt-3 space-y-1.5">
            {steps.map((step, index) => {
              const active = index === 0
              return (
                <div
                  key={step}
                  className={cn(
                    'flex items-center gap-2 text-xs transition-colors',
                    active ? 'text-foreground' : 'text-foreground-alt/50',
                  )}
                >
                  <div
                    className={cn(
                      'flex h-5 w-5 shrink-0 items-center justify-center rounded-md',
                      active ? 'bg-brand/10 text-brand' : 'bg-foreground/5',
                    )}
                  >
                    {active ?
                      <LuKeyRound className="h-3 w-3" aria-hidden="true" />
                    : index === steps.length - 1 ?
                      <LuCheck className="h-3 w-3" aria-hidden="true" />
                    : <LuShield className="h-3 w-3" aria-hidden="true" />}
                  </div>
                  <span>{step}</span>
                </div>
              )
            })}
          </div>
        </div>
      </div>
    </div>
  )
}
