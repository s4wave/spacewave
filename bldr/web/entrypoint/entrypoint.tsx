import React, { Suspense, useMemo } from 'react'
import { createRoot, hydrateRoot, type Root } from 'react-dom/client'
import {
  BldrRoot,
  IBldrRootProps,
  WebViewErrorBoundary,
} from '@aptre/bldr-react'
import { WebDocument as BldrWebDocument, WebDocumentOptions } from '@aptre/bldr'

import { initBrowserReleaseAutoReload } from '../bldr/browser-release-update.js'
import { markStartupBoundary } from '../bldr/startup-marks.js'
import { setAppPath } from './app-path.js'
import { writeBrowserBootStatus } from './boot-status.js'

declare global {
  var __swDeferBoot: boolean | undefined
  var __swBoot: ((hash: string) => void) | undefined
  var __swBootStatus:
    | {
        phase: string
        detail: string
        state: 'loading' | 'error'
        progress?: number
      }
    | undefined
  var __swPrerenderRoot: Root | undefined
  var __swPrerenderContainer: HTMLElement | undefined
  var __swReadyResolve: (() => void) | undefined
}

const webDocumentOpts: WebDocumentOptions = {}

// Extract webDocumentId from URL query parameters (for Electron)
const urlParams = new URLSearchParams(window.location.search)
const webDocumentId = urlParams.get('webDocumentId')
if (webDocumentId) {
  webDocumentOpts.webDocumentId = webDocumentId
}

// BLDR_RUNTIME_JS is an injected variable with the path to the runtime.js
declare const BLDR_RUNTIME_JS: string | undefined
if (typeof BLDR_RUNTIME_JS === 'string') {
  webDocumentOpts.runtimeWorkerPath = BLDR_RUNTIME_JS
}

// BLDR_SW_JS is an injected variable with the path to the sw.mjs
declare const BLDR_SW_JS: string | undefined
// BLDR_SHW_JS is an injected variable with the path to the shw.mjs
// This unified worker handles both native and QuickJS plugins via URL params.
declare const BLDR_SHW_JS: string | undefined
if (typeof BLDR_SW_JS === 'string') {
  webDocumentOpts.serviceWorkerPath = BLDR_SW_JS
}

if (typeof BLDR_SHW_JS === 'string') {
  webDocumentOpts.sharedWorkerPath = BLDR_SHW_JS
}

// BLDR_FORCE_DEDICATED_WORKERS is an injected variable to force dedicated workers.
declare const BLDR_FORCE_DEDICATED_WORKERS: boolean | undefined
if (
  typeof BLDR_FORCE_DEDICATED_WORKERS === 'boolean' &&
  BLDR_FORCE_DEDICATED_WORKERS
) {
  webDocumentOpts.forceDedicatedWorkers = true
}

// BLDR_FORCE_MESSAGEPORT_WORKER_COMMS is injected for runtimes that need the
// conservative MessagePort worker transport.
declare const BLDR_FORCE_MESSAGEPORT_WORKER_COMMS: boolean | undefined
if (
  typeof BLDR_FORCE_MESSAGEPORT_WORKER_COMMS === 'boolean' &&
  BLDR_FORCE_MESSAGEPORT_WORKER_COMMS
) {
  webDocumentOpts.forceMessagePortWorkerComms = true
}

const bldrRootProps: IBldrRootProps = { webDocumentOpts }

initBrowserReleaseAutoReload()
markStartupBoundary('shell.entrypoint-loaded', { source: 'browser' })

function setBrowserBootStatus(
  phase: string,
  detail: string,
  state: 'loading' | 'error' = 'loading',
) {
  writeBrowserBootStatus({ phase, detail, state })
}

// BLDR_STARTUP_JS is an injected variable with the path to the startup js component
declare const BLDR_STARTUP_JS: string | undefined
if (typeof BLDR_STARTUP_JS === 'string') {
  const BldrWebStartupContainer: React.FC = () => {
    const LoadedComponent = useMemo(
      () =>
        React.lazy(
          async (): Promise<{
            default: React.LazyExoticComponent<React.ComponentType>
          }> =>
            (await import(BLDR_STARTUP_JS)) as {
              default: React.LazyExoticComponent<React.ComponentType>
            },
        ),
      [],
    )

    const loadedComponent = useMemo(
      () => <LoadedComponent />,
      [LoadedComponent],
    )

    return (
      <WebViewErrorBoundary>
        <Suspense fallback={<div>Loading app…</div>}>
          {loadedComponent}
        </Suspense>
      </WebViewErrorBoundary>
    )
  }

  bldrRootProps.disableRootWebView = true
  bldrRootProps.children = <BldrWebStartupContainer />
}

function resolveDeferredBootReady() {
  markStartupBoundary('shell.deferred-boot-ready', { source: 'browser' })
  const resolve = globalThis.__swReadyResolve
  if (!resolve) {
    return
  }
  globalThis.__swReadyResolve = undefined
  resolve()
}

function waitForWebRuntime(webDocument: BldrWebDocument) {
  markStartupBoundary('runtime.wait-start', { source: 'browser' })
  setBrowserBootStatus('runtime', 'Connecting runtime...')
  void webDocument
    .waitConn()
    .then(() => {
      markStartupBoundary('runtime.wait-ready', { source: 'browser' })
      setBrowserBootStatus('ready', 'Application ready.')
      resolveDeferredBootReady()
    })
    .catch((err: unknown) => {
      const detail = err instanceof Error ? err.message : String(err)
      setBrowserBootStatus('runtime-error', detail, 'error')
      console.error('entrypoint: failed to connect runtime', err)
    })
}

// initialize react and Bldr
const container = document.getElementById('bldr-root')
const deferBoot =
  !!container?.hasAttribute('data-prerendered') && !!globalThis.__swDeferBoot
markStartupBoundary('shell.container-resolved', {
  source: 'browser',
  deferred: deferBoot,
  prerendered: !!container?.hasAttribute('data-prerendered'),
})

if (container && deferBoot) {
  const webDocument = new BldrWebDocument(webDocumentOpts)
  let root: ReturnType<typeof createRoot> | null = null

  const renderBootedRoot = () => {
    container.removeAttribute('data-prerendered')

    if (
      globalThis.__swPrerenderRoot &&
      globalThis.__swPrerenderContainer === container
    ) {
      globalThis.__swPrerenderRoot.render(
        <BldrRoot {...bldrRootProps} webDocument={webDocument} />,
      )
      globalThis.__swPrerenderRoot = undefined
      globalThis.__swPrerenderContainer = undefined
      return
    }

    globalThis.__swPrerenderRoot?.unmount()
    globalThis.__swPrerenderRoot = undefined
    globalThis.__swPrerenderContainer = undefined

    if (!root) {
      root = createRoot(container)
    }
    root.render(<BldrRoot {...bldrRootProps} webDocument={webDocument} />)
  }

  globalThis.__swBoot = (hash: string) => {
    markStartupBoundary('shell.boot-requested', { source: 'browser' })
    setBrowserBootStatus('app', 'Opening application...')
    setAppPath(hash)
    renderBootedRoot()
  }
  waitForWebRuntime(webDocument)
} else if (container?.hasAttribute('data-prerendered')) {
  container.removeAttribute('data-prerendered')
  hydrateRoot(container, <BldrRoot {...bldrRootProps} />)
} else {
  const root = createRoot(container!)
  root.render(<BldrRoot {...bldrRootProps} />)
}
