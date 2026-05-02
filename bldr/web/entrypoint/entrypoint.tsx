import React, { Suspense, useMemo } from 'react'
import { createRoot, hydrateRoot, type Root } from 'react-dom/client'
import {
  BldrRoot,
  IBldrRootProps,
  WebViewErrorBoundary,
} from '@aptre/bldr-react'
import { WebDocument as BldrWebDocument, WebDocumentOptions } from '@aptre/bldr'

import { setAppPath } from '@s4wave/web/router/app-path.js'
import {
  bootStatusEvent,
  canMutateBrowserBootStatusTarget,
} from '@s4wave/app/prerender/boot-status.js'

import { initBrowserReleaseAutoReload } from '../bldr/browser-release-update.js'

declare global {
  var __swDeferBoot: boolean | undefined
  var __swBoot: ((hash: string) => void) | undefined
  var __swBootStatus:
    | { phase: string; detail: string; state: 'loading' | 'error' }
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

const bldrRootProps: IBldrRootProps = { webDocumentOpts }

initBrowserReleaseAutoReload()

function setBrowserBootStatus(
  phase: string,
  detail: string,
  state: 'loading' | 'error' = 'loading',
) {
  const status = { phase, detail, state }
  globalThis.__swBootStatus = status
  const detailTarget = document.querySelector('[data-sw-boot-status]')
  if (canMutateBrowserBootStatusTarget(detailTarget)) {
    detailTarget.replaceChildren(detail)
  }
  const stateTarget = document.querySelector('[data-sw-boot-state]')
  if (canMutateBrowserBootStatusTarget(stateTarget)) {
    stateTarget.setAttribute('data-sw-boot-state', state)
  }
  window.dispatchEvent(new CustomEvent(bootStatusEvent, { detail: status }))
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
        <Suspense fallback={<div>Loading app...</div>}>
          {loadedComponent}
        </Suspense>
      </WebViewErrorBoundary>
    )
  }

  bldrRootProps.disableRootWebView = true
  bldrRootProps.children = <BldrWebStartupContainer />
}

function resolveDeferredBootReady() {
  const resolve = globalThis.__swReadyResolve
  if (!resolve) {
    return
  }
  globalThis.__swReadyResolve = undefined
  resolve()
}

function waitForWebRuntime(webDocument: BldrWebDocument) {
  setBrowserBootStatus('runtime', 'Connecting runtime...')
  void webDocument
    .waitConn()
    .then(() => {
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
