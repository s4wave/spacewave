import { cn } from '@s4wave/web/style/utils.js'

import type { BrowserStartupPhaseView } from './status/browser-startup-model.js'

import { BootLoadingCriticalStyle } from './boot-loading-critical.js'

// BrowserStartupPhaseRail renders the five boot phases as a connected rail. The
// filled track reaches the current dot so the rail reads as one journey; the
// current phase shows a spinner, completed phases a filled dot, pending phases a
// muted ring, and an errored phase a destructive marker. Shared with the
// quickstart loading page, so it inlines the boot critical style itself.
export function BrowserStartupPhaseRail({
  phases,
}: {
  phases: BrowserStartupPhaseView[]
}) {
  const failed = phases.some((phase) => phase.state === 'error')
  const activeIndex = phaseRailActiveIndex(phases)
  const fillPct =
    phases.length > 1 ? (activeIndex / (phases.length - 1)) * 100 : 0
  return (
    <div className="swb-rail">
      <BootLoadingCriticalStyle />
      <div aria-hidden="true" className="swb-rail-track">
        <div
          className={cn('swb-rail-fill', failed && 'swb-rail-fill--error')}
          style={{ width: `${fillPct}%` }}
        />
      </div>
      <ol className="swb-steps" aria-label="Startup phases">
        {phases.map((phase) => (
          <li key={phase.id} className="swb-step" data-state={phase.state}>
            <div className="swb-step-mark">
              {phase.state === 'current' ? (
                <span className="swb-spinner" aria-hidden="true" />
              ) : (
                <span className="swb-dot" aria-hidden="true" />
              )}
            </div>
            <div className="swb-step-label">{phase.label}</div>
          </li>
        ))}
      </ol>
    </div>
  )
}

// phaseRailActiveIndex returns the index the connecting track should fill to:
// the current or errored phase when present, otherwise the last completed
// phase (0 while nothing has completed yet).
function phaseRailActiveIndex(phases: BrowserStartupPhaseView[]): number {
  const current = phases.findIndex(
    (phase) => phase.state === 'current' || phase.state === 'error',
  )
  if (current >= 0) return current
  let lastComplete = 0
  for (const [index, phase] of phases.entries()) {
    if (phase.state === 'complete') lastComplete = index
  }
  return lastComplete
}
