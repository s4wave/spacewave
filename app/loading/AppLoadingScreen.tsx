import { useEffect } from 'react'

import {
  bootDownloadFraction,
  formatBytes,
  type BootDownload,
} from '@aptre/bldr'

import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { useBrowserStartupProjection } from '@s4wave/app/loading/status/browser-startup.js'
import { useBootDownloads } from '@s4wave/app/loading/status/browser-downloads.js'
import type { BrowserStartupPhaseView } from '@s4wave/app/loading/status/browser-startup-model.js'
import { markBrowserStartupBoundary } from '@s4wave/app/prerender/boot-status.js'
import { useReducedMotion } from '@s4wave/web/ui/loading/index.js'
import type { LoadingView } from '@s4wave/web/ui/loading/types.js'

import { BootLoadingCriticalStyle } from './boot-loading-critical.js'

// AppLoadingScreen renders the full-screen browser boot state. It is the node
// the root WebView shows while the plugin bundle and its stylesheets download,
// so it is styled entirely from the inlined boot critical CSS (semantic .swb-*
// classes) instead of the not-yet-loaded Tailwind app.css.
export function AppLoadingScreen() {
  const startup = useBrowserStartupProjection()
  const reducedMotion = useReducedMotion()
  const view = withBrowserStartupErrorActions(startup.view)
  return (
    <BrowserStartupRevealProbe>
      <BootLoadingCriticalStyle />
      <div
        className="swb-canvas"
        data-sw-startup-reduced-motion={reducedMotion ? 'true' : undefined}
      >
        <div className="swb-col">
          <div className="swb-logo">
            <AnimatedLogo
              followMouse={false}
              fixedSize="5rem"
              reduceMotion={reducedMotion}
            />
          </div>

          <div className="swb-head" aria-live="polite">
            <h1 className="swb-title">{view.title}</h1>
            {view.detail ? <p className="swb-detail">{view.detail}</p> : null}
            {view.progress !== undefined || view.progressIndeterminate ? (
              <BrowserStartupProgress
                value={
                  view.progress === undefined ? undefined : view.progress * 100
                }
                indeterminate={view.progressIndeterminate}
              />
            ) : null}
            {view.error ? <p className="swb-error">{view.error}</p> : null}
            {view.onRetry || view.onCancel ? (
              <div className="swb-actions">
                {view.onRetry ? (
                  <button
                    type="button"
                    className="swb-btn swb-btn--primary"
                    onClick={view.onRetry}
                  >
                    Retry
                  </button>
                ) : null}
                {view.onCancel ? (
                  <button
                    type="button"
                    className="swb-btn swb-btn--ghost"
                    onClick={view.onCancel}
                  >
                    Back
                  </button>
                ) : null}
              </div>
            ) : null}
          </div>

          <BrowserStartupPhaseRail phases={startup.phases} />
          <BrowserStartupDownloadList />
          <p className="swb-hint">
            The first launch downloads the full app bundle and can take a while.
            It is cached for instant loads afterward.
          </p>
        </div>
      </div>
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

function BrowserStartupRevealProbe({
  children,
}: {
  children: React.ReactNode
}) {
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

// BrowserStartupProgress renders the determinate boot progress bar with a mono
// percent readout, or a sweeping indeterminate bar while byte totals are
// unknown.
function BrowserStartupProgress({
  value,
  indeterminate,
}: {
  value?: number
  indeterminate?: boolean
}) {
  const pct =
    value === undefined ? 0 : Math.max(0, Math.min(100, Math.round(value)))
  return (
    <div className="swb-progress-wrap">
      <div
        className="swb-bar"
        role="progressbar"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={indeterminate ? undefined : pct}
      >
        {indeterminate ? (
          <div className="swb-bar-fill swb-bar-fill--indeterminate" />
        ) : (
          <div className="swb-bar-fill" style={{ width: `${pct}%` }} />
        )}
      </div>
      {indeterminate ? null : (
        <span className="swb-mono swb-progress-label">{pct}%</span>
      )}
    </div>
  )
}

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
          className={
            failed ? 'swb-rail-fill swb-rail-fill--error' : 'swb-rail-fill'
          }
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

// BrowserStartupDownloadList renders the live per-asset download breakdown from
// the boot download registry: one labeled progress bar per boot asset (runtime,
// app bundle, and any plugin modules) with streamed bytes and percent. It
// renders nothing until a producer registers a download, so screens that never
// stream bytes stay unchanged.
export function BrowserStartupDownloadList() {
  const downloads = useBootDownloads()
  if (downloads.length === 0) return null
  return (
    <ul
      aria-label="Downloads"
      data-sw-startup-downloads
      className="swb-downloads"
    >
      {downloads.map((download) => (
        <li key={download.id} data-sw-startup-download={download.id}>
          <BrowserStartupDownloadRow download={download} />
        </li>
      ))}
    </ul>
  )
}

function BrowserStartupDownloadRow({ download }: { download: BootDownload }) {
  const fraction = bootDownloadFraction(download)
  const failed = download.state === 'error'
  const pct =
    fraction === undefined
      ? undefined
      : Math.max(0, Math.min(100, Math.round(fraction * 100)))
  return (
    <div className="swb-dl-row">
      <div className="swb-dl-head">
        <span className="swb-dl-label">{download.label}</span>
        <span
          data-sw-startup-download-detail
          className={
            failed
              ? 'swb-mono swb-dl-detail swb-dl-detail--error'
              : 'swb-mono swb-dl-detail'
          }
        >
          {downloadDetailText(download)}
        </span>
      </div>
      <div
        className="swb-bar"
        role="progressbar"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={pct}
      >
        {pct === undefined && !failed ? (
          <div className="swb-bar-fill swb-bar-fill--indeterminate" />
        ) : (
          <div className="swb-bar-fill" style={{ width: `${pct ?? 0}%` }} />
        )}
      </div>
    </div>
  )
}

// downloadDetailText renders the streamed byte counter, or the error message
// when the download failed. A known total shows "loaded / total"; an unknown
// total shows the bytes seen so far without a faked denominator.
function downloadDetailText(download: BootDownload): string {
  if (download.state === 'error') return download.error ?? 'Failed'
  if (download.total !== undefined) {
    return `${formatBytes(download.loaded, 1)} / ${formatBytes(download.total, 1)}`
  }
  if (download.loaded > 0) return formatBytes(download.loaded, 1)
  return ''
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
