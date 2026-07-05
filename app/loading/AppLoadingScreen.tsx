import { useEffect, type ReactNode } from 'react'

import '@s4wave/web/style/app.css'
import { LoadingScreen } from '@s4wave/web/ui/loading/LoadingScreen.js'
import { cn } from '@s4wave/web/style/utils.js'
import { useReducedMotion } from '@s4wave/web/ui/loading/index.js'

import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { useBrowserStartupProjection } from '@s4wave/app/loading/status/browser-startup.js'
import type {
  BrowserStartupPhaseID,
  BrowserStartupPhaseView,
} from '@s4wave/app/loading/status/browser-startup-model.js'
import { markBrowserStartupBoundary } from '@s4wave/app/prerender/boot-status.js'
import type { LoadingView } from '@s4wave/web/ui/loading/types.js'

// AppLoadingScreen renders the full-screen boot state for returning users.
// Wraps the new LoadingScreen primitive with the animated Spacewave logo.
export function AppLoadingScreen() {
  const startup = useBrowserStartupProjection()
  const reducedMotion = useReducedMotion()
  const view = withBrowserStartupErrorActions(startup.view)
  return (
    <BrowserStartupRevealProbe>
      <LoadingScreen
        view={view}
        logo={
          <AnimatedLogo
            followMouse={false}
            fixedSize="5rem"
            reduceMotion={reducedMotion}
          />
        }
        showShineBorder={false}
      >
        <div
          className="flex w-[min(30rem,calc(100vw-2rem))] flex-col items-center gap-5"
          data-sw-startup-reduced-motion={reducedMotion ? 'true' : undefined}
        >
          <BrowserStartupPhaseRail phases={startup.phases} />
          <BrowserStartupPreviewSurface phase={startup.phase.id} />
        </div>
      </LoadingScreen>
    </BrowserStartupRevealProbe>
  )
}

function withBrowserStartupErrorActions(view: LoadingView): LoadingView {
  if (view.state !== 'error') return view
  return {
    ...view,
    onRetry: retryBrowserStartup,
    onCancel: leaveBrowserStartup,
  }
}

function retryBrowserStartup() {
  markBrowserStartupBoundary('webview.loading-surface-retry', {
    source: 'app',
  })
  window.location.reload()
}

function leaveBrowserStartup() {
  markBrowserStartupBoundary('webview.loading-surface-back', {
    source: 'app',
  })
  if (window.history.length > 1) {
    window.history.back()
    return
  }
  localStorage.removeItem('spacewave-has-session')
  window.location.assign('/')
}

function BrowserStartupRevealProbe({ children }: { children: ReactNode }) {
  useEffect(() => {
    markBrowserStartupBoundary('webview.loading-surface-mounted', {
      source: 'app',
    })
    return () => {
      markBrowserStartupBoundary('webview.loading-surface-revealed', {
        source: 'app',
      })
    }
  }, [])

  return children
}

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
    <div className="relative w-full">
      {/* Connecting track: the filled portion reaches the current dot so the
          rail reads as one continuous journey (goal-gradient), not five
          disconnected lamps. Ends inset to sit under the outer dot centers. */}
      <div
        aria-hidden="true"
        className="bg-foreground/10 pointer-events-none absolute inset-x-[10%] top-3 h-px -translate-y-1/2 overflow-hidden rounded-full"
      >
        <div
          className={cn(
            'h-full rounded-full transition-[width] duration-500 ease-out motion-reduce:transition-none',
            failed ? 'bg-destructive/70' : 'bg-brand',
          )}
          style={{ width: `${fillPct}%` }}
        />
      </div>
      <ol
        className="relative grid grid-cols-5 gap-2"
        aria-label="Startup phases"
      >
        {phases.map((phase) => (
          <li key={phase.id} className="min-w-0">
            <div className="flex h-6 items-center justify-center">
              <span
                className={cn(
                  'block rounded-full transition-all duration-300 motion-reduce:transition-none',
                  phase.state === 'complete' && 'bg-brand size-2.5',
                  phase.state === 'current' &&
                    'bg-brand ring-brand/25 animate-pulse-subtle size-3 ring-4 motion-reduce:animate-none',
                  phase.state === 'error' &&
                    'bg-destructive ring-destructive/25 size-3 ring-4',
                  phase.state === 'pending' &&
                    'border-foreground/25 bg-background size-2.5 border',
                )}
              />
            </div>
            <div
              className={cn(
                'truncate text-center text-[0.65rem] font-medium transition-colors motion-reduce:transition-none',
                phase.state === 'pending' && 'text-foreground-alt/55',
                phase.state === 'complete' && 'text-foreground-alt/85',
                phase.state === 'current' && 'text-foreground',
                phase.state === 'error' && 'text-destructive',
              )}
            >
              {phase.label}
            </div>
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

function BrowserStartupPreviewSurface({
  phase,
}: {
  phase: BrowserStartupPhaseID
}) {
  const phaseIndex = previewPhaseIndex[phase]
  return (
    <div
      aria-hidden="true"
      data-sw-startup-preview
      data-sw-startup-preview-phase={phase}
      className="border-foreground/8 bg-background-card/35 shadow-background-dark/45 relative h-30 w-full overflow-hidden rounded-lg border shadow-2xl backdrop-blur-sm"
    >
      <div className="border-foreground/8 bg-background-card/70 flex h-7 items-center gap-1.5 border-b px-3">
        <span className="bg-foreground/15 size-2 rounded-full" />
        <span className="bg-foreground/15 size-2 rounded-full" />
        <span className="bg-foreground/15 size-2 rounded-full" />
        <div className="bg-foreground/8 ml-2 h-2 w-18 rounded-full" />
      </div>

      <div className="grid h-[calc(100%-1.75rem)] grid-cols-[4.25rem_1fr]">
        <div className="border-foreground/6 bg-background-dark/35 flex flex-col gap-2 border-r p-2">
          {previewNavItems.map((item) => (
            // Neutral chrome: the sidebar stays inert gray so the eye rests on
            // the progress UI, not a second brand-colored focal point.
            <span key={item} className="bg-foreground/12 h-2 rounded-full" />
          ))}
        </div>

        <div className="relative grid grid-cols-[1fr_5.5rem] gap-3 p-3">
          <div className="space-y-2">
            <div className="bg-foreground/20 h-2.5 w-24 rounded-full" />
            <div className="grid grid-cols-3 gap-2">
              {previewTiles.map((item, index) => (
                <span
                  key={item}
                  className={cn(
                    'h-12 rounded-md border transition-colors duration-300 motion-reduce:transition-none',
                    index <= phaseIndex
                      ? 'border-brand/20 bg-brand/15'
                      : 'border-foreground/6 bg-foreground/[0.04]',
                  )}
                />
              ))}
            </div>
            <div className="bg-foreground/10 h-2 w-42 rounded-full" />
          </div>

          <div className="border-foreground/6 bg-background/35 flex flex-col justify-end gap-1.5 rounded-md border p-2">
            {previewMeters.map((item, index) => (
              <span
                key={item}
                className={cn(
                  'h-1.5 rounded-full transition-colors duration-300 motion-reduce:transition-none',
                  index <= phaseIndex ? 'bg-brand/65' : 'bg-foreground/10',
                )}
                style={{ width: previewMeterWidths[index] }}
              />
            ))}
          </div>
        </div>
      </div>

      <span className="bg-brand/60 absolute right-4 bottom-4 size-2 rounded-full shadow-[0_0_18px_var(--color-brand)]" />
    </div>
  )
}

const previewPhaseIndex: Record<BrowserStartupPhaseID, number> = {
  prepare: 0,
  connect: 1,
  runtime: 2,
  frame: 3,
  done: 4,
}

const previewNavItems = ['root', 'spaces', 'activity', 'settings']
const previewTiles = ['session', 'objects', 'frame']
const previewMeters = ['sync', 'runtime', 'frame']
const previewMeterWidths = ['72%', '55%', '88%']
