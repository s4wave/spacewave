import React, { Suspense, useMemo } from 'react'
import { createRoot, hydrateRoot, type Root } from 'react-dom/client'
import {
  BldrRoot,
  IBldrRootProps,
  WebViewErrorBoundary,
} from '@aptre/bldr-react'
import { isDesktop, WebDocument as BldrWebDocument } from '@aptre/bldr'
import type { WebDocumentOptions } from '@aptre/bldr'

import { initBrowserReleaseAutoReload } from '../bldr/browser-release-update.js'
import { markStartupBoundary } from '../bldr/startup-marks.js'
import { setAppPath } from './app-path.js'
import { writeBrowserBootStatus } from './boot-status.js'

declare global {
  var __swDeferBoot: boolean | undefined
  var __swBoot: ((hash: string) => void) | undefined
  var __swReady: Promise<void> | undefined
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
  var BLDR_RUNTIME_WASM_ENV: Record<string, string> | undefined
}

function installBrowserRuntimeEnv(): void {
  if (location.hostname !== 'staging.spacewave.app') {
    return
  }
  globalThis.BLDR_RUNTIME_WASM_ENV = {
    ...globalThis.BLDR_RUNTIME_WASM_ENV,
    SPACEWAVE_CDN_BASE_URL:
      globalThis.BLDR_RUNTIME_WASM_ENV?.SPACEWAVE_CDN_BASE_URL ??
      'https://cdn-staging.spacewave.app',
  }
}

installBrowserRuntimeEnv()

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
// This worker wraps plugin modules with Bldr runtime lifecycle params.
declare const BLDR_SHW_JS: string | undefined
if (typeof BLDR_SW_JS === 'string') {
  webDocumentOpts.serviceWorkerPath = BLDR_SW_JS
}

if (typeof BLDR_SHW_JS === 'string') {
  webDocumentOpts.sharedWorkerPath = BLDR_SHW_JS
}

// BLDR_OPFS_WORKER_JS is an injected variable with the path to the OPFS worker.
declare const BLDR_OPFS_WORKER_JS: string | undefined
if (typeof BLDR_OPFS_WORKER_JS === 'string') {
  webDocumentOpts.opfsWorkerPath = BLDR_OPFS_WORKER_JS
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

if (!isDesktop) {
  initBrowserReleaseAutoReload()
}
markStartupBoundary('shell.entrypoint-loaded', { source: 'browser' })

type StartupModule = {
  default: React.LazyExoticComponent<React.ComponentType>
}

function setBrowserBootStatus(
  phase: string,
  detail: string,
  state: 'loading' | 'error' = 'loading',
  progress?: number,
) {
  writeBrowserBootStatus({ phase, detail, state, progress })
}

// BLDR_STARTUP_JS is build-injected per bundle with the startup component
// module specifier, so it is not known at author time.
declare const BLDR_STARTUP_JS: string | undefined
if (typeof BLDR_STARTUP_JS === 'string') {
  const importStartupModule = async (): Promise<StartupModule> =>
    (await import(BLDR_STARTUP_JS)) as StartupModule
  const BldrWebStartupContainer: React.FC = () => {
    const LoadedComponent = useMemo(
      () =>
        React.lazy(async (): Promise<StartupModule> => importStartupModule()),
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

function resolveBootReady(label: string) {
  markStartupBoundary(label, { source: 'browser' })
  const resolve = globalThis.__swReadyResolve
  if (!resolve) {
    return
  }
  globalThis.__swReadyResolve = undefined
  resolve()
}

function resolveImmediateBootReady() {
  globalThis.__swDeferBoot = undefined
  setBrowserBootStatus('ready', 'Application ready.')
  resolveBootReady('shell.immediate-boot-ready')
  globalThis.__swReady = undefined
}

function waitForWebRuntime(webDocument: BldrWebDocument): Promise<boolean> {
  markStartupBoundary('runtime.wait-start', { source: 'browser' })
  setBrowserBootStatus('runtime', 'Connecting runtime...')
  return webDocument
    .waitConn()
    .then(() => {
      markStartupBoundary('runtime.wait-ready', { source: 'browser' })
      setBrowserBootStatus('ready', 'Application ready.')
      resolveBootReady('shell.deferred-boot-ready')
      return true
    })
    .catch((err: unknown) => {
      const detail = err instanceof Error ? err.message : String(err)
      setBrowserBootStatus('runtime-error', detail, 'error')
      console.error('entrypoint: failed to connect runtime', err)
      return false
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
  let root: Root | null = null
  let runtimeReady = false
  let pendingBoot = false
  let bootedRootRendered = false
  const renderBootedRoot = () => {
    if (bootedRootRendered) {
      return
    }
    bootedRootRendered = true
    pendingBoot = false
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

  const runtimeReadyPromise = waitForWebRuntime(webDocument).then((ready) => {
    runtimeReady = ready
    if (runtimeReady && pendingBoot) {
      renderBootedRoot()
    }
  })

  globalThis.__swBoot = (hash: string) => {
    markStartupBoundary('shell.boot-requested', { source: 'browser' })
    setBrowserBootStatus('app', 'Opening application...')
    setAppPath(hash)
    pendingBoot = true
    if (runtimeReady) {
      renderBootedRoot()
      return
    }
    void runtimeReadyPromise
  }
} else if (container?.hasAttribute('data-prerendered')) {
  container.removeAttribute('data-prerendered')
  hydrateRoot(container, <BldrRoot {...bldrRootProps} />)
  resolveImmediateBootReady()
} else {
  const root = createRoot(container!)
  root.render(<BldrRoot {...bldrRootProps} />)
  resolveImmediateBootReady()
}
