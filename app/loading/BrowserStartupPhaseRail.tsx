import { cn } from '@s4wave/web/style/utils.js'

import type { BrowserStartupPhaseView } from './status/browser-startup-model.js'

import { BootLoadingCriticalStyle } from './boot-loading-critical.js'

import * as stylex from '@stylexjs/stylex'

const stylexStyles = stylex.create({
  rule1: { position: 'relative', width: '100%' },
  rule2: {
    position: 'relative',
    display: 'grid',
    gridTemplateColumns: 'repeat(5, minmax(0, 1fr))',
    gap: '0.5rem',
    margin: 0,
    padding: 0,
    listStyle: 'none',
  },
  rule3: {
    display: 'flex',
    height: '1.5rem',
    alignItems: 'center',
    justifyContent: 'center',
  },
  rule4: {
    overflow: 'hidden',
    fontSize: '0.65rem',
    fontWeight: 500,
    textAlign: 'center',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
})

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
    <div sx={stylexStyles.rule1}>
      <BootLoadingCriticalStyle />
      <div
        aria-hidden="true"
        className={
          /* TODO(stylex): global or unknown class: swb-rail-track */ 'swb-rail-track'
        }
      >
        <div
          className={
            /* TODO(stylex): global or unknown class: swb-rail-fill */ cn(
              'swb-rail-fill browser-progress-width',
              failed && 'swb-rail-fill--error',
            )
          }
          style={{ '--browser-progress-width': `${fillPct}%` }}
        />
      </div>
      <ol sx={stylexStyles.rule2} aria-label="Startup phases">
        {phases.map((phase) => (
          <li
            key={phase.id}
            className={
              /* TODO(stylex): global or unknown class: swb-step */ 'swb-step'
            }
            data-state={phase.state}
            aria-current={phase.state === 'current' ? 'step' : undefined}
          >
            <div sx={stylexStyles.rule3}>
              {phase.state === 'current' ? (
                <span
                  className={
                    /* TODO(stylex): global or unknown class: swb-spinner */ 'swb-spinner'
                  }
                  aria-hidden="true"
                />
              ) : (
                <span
                  className={
                    /* TODO(stylex): global or unknown class: swb-dot */ 'swb-dot'
                  }
                  aria-hidden="true"
                />
              )}
            </div>
            <div sx={stylexStyles.rule4}>{phase.label}</div>
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
