import { useEffect } from 'react'

import { useBrowserStartupProjection } from '@s4wave/app/loading/status/browser-startup.js'
import { useBrowserBootWatchdog } from '@s4wave/app/loading/status/browser-boot-watchdog.js'
import { markBrowserStartupBoundary } from '@s4wave/app/prerender/boot-status.js'
import spacewaveIcon from '@s4wave/web/images/spacewave-icon.png'
import { useReducedMotion } from '@s4wave/web/ui/loading/index.js'

import { BootLoadingCriticalStyle } from './boot-loading-critical.js'
import { BrowserStartupDownloadList } from './BrowserStartupDownloadList.js'
import { useBootDownloads } from './status/browser-downloads.js'

// AppLoadingScreen presents startup activity before the application stylesheet
// arrives. Readiness and download evidence stay in the startup projection.
export function AppLoadingScreen() {
  const startup = useBrowserStartupProjection()
  useBrowserBootWatchdog()
  const reducedMotion = useReducedMotion()
  const view = startup.view
  const downloads = useBootDownloads()
  const failedDownload = downloads.find(
    (download) => download.state === 'error',
  )
  const failed = view.state === 'error' || failedDownload !== undefined
  const error =
    view.error ??
    failedDownload?.error ??
    (failedDownload
      ? `${failedDownload.label} could not be loaded.`
      : undefined)

  return (
    <BrowserStartupRevealProbe>
      <BootLoadingCriticalStyle />
      <div
        className="swb-canvas"
        data-sw-startup-reduced-motion={reducedMotion ? 'true' : undefined}
      >
        <div className="swb-col">
          <img
            className="swb-logo"
            src={spacewaveIcon}
            alt=""
            width={120}
            height={120}
          />
          <div className="swb-head" aria-live="polite">
            <h1 className="swb-title">
              {failed ? 'Unable to open Spacewave' : 'Opening Spacewave'}
            </h1>
            {!failed ? <p className="swb-detail">Starting the app</p> : null}
          </div>
          {!failed ? (
            <>
              <div
                className="swb-activity swb-bar"
                role="progressbar"
                aria-label="Opening Spacewave"
              >
                <div className="swb-bar-fill swb-bar-fill--indeterminate" />
              </div>
              <p className="swb-hint">
                Downloaded files are saved on this device.
              </p>
            </>
          ) : null}
          {error ? (
            <p className="swb-error" role="alert">
              {error}
            </p>
          ) : null}
          {failed ? (
            <div className="swb-actions">
              <button
                type="button"
                className="swb-btn swb-btn--primary"
                onClick={retryBrowserStartup}
              >
                Retry
              </button>
              <button
                type="button"
                className="swb-btn swb-btn--ghost"
                onClick={leaveBrowserStartup}
              >
                Back
              </button>
            </div>
          ) : null}
          <details className="swb-disclosure">
            <summary>Show details</summary>
            <div className="swb-diagnostics">
              <p className="swb-status">{view.detail ?? startup.phase.label}</p>
              <BrowserStartupDownloadList downloads={downloads} />
            </div>
          </details>
        </div>
      </div>
    </BrowserStartupRevealProbe>
  )
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
