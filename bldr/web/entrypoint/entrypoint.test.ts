import React, { act, type ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { writeBrowserBootStatus } from './boot-status.js'

const renderedRootElements = vi.hoisted<unknown[]>(() => [])

const createRootMock = vi.hoisted(() =>
  vi.fn(() => ({
    render: vi.fn((element: unknown) => {
      renderedRootElements.push(element)
    }),
    unmount: vi.fn(),
  })),
)
const hydrateRootMock = vi.hoisted(() =>
  vi.fn(() => ({ render: vi.fn(), unmount: vi.fn() })),
)
const waitConnMock = vi.hoisted(() => vi.fn(() => Promise.resolve()))
const bldrRuntimeMock = vi.hoisted(() => ({ isDesktop: false }))
const initBrowserReleaseAutoReloadMock = vi.hoisted(() => vi.fn())

vi.mock('react-dom/client', () => ({
  createRoot: createRootMock,
  hydrateRoot: hydrateRootMock,
}))

vi.mock('@aptre/bldr-react', () => ({
  BldrRoot: ({ children }: { children?: ReactNode }) =>
    React.createElement(React.Fragment, null, children),
  WebViewErrorBoundary: ({ children }: { children: ReactNode }) =>
    React.createElement(React.Fragment, null, children),
}))

vi.mock('@aptre/bldr', () => ({
  get isDesktop() {
    return bldrRuntimeMock.isDesktop
  },
  WebDocument: class WebDocument {
    waitConn() {
      return waitConnMock()
    }
  },
}))

vi.mock('../bldr/browser-release-update.js', () => ({
  initBrowserReleaseAutoReload: initBrowserReleaseAutoReloadMock,
}))

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
  var __swReadyResolve: (() => void) | undefined
  var __swStartupModuleImportedFrom: string | undefined
  var BLDR_STARTUP_JS: string | undefined
  var BLDR_RUNTIME_WASM_ENV: Record<string, string> | undefined
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined
}

function createReady() {
  let resolveReady = () => {}
  const promise = new Promise<void>((resolve) => {
    resolveReady = resolve
  })
  return {
    promise,
    resolve: resolveReady,
  }
}

let startupModuleURLIndex = 0

function createStartupModuleURL() {
  startupModuleURLIndex += 1
  const source = [
    `globalThis.__swStartupModuleImportedFrom = import.meta.url; // ${startupModuleURLIndex}`,
    'export default function StartupTestComponent() { return null }',
  ].join('\n')
  return `data:text/javascript;charset=utf-8,${encodeURIComponent(source)}`
}

async function drainMicrotasks(count = 5) {
  for (let i = 0; i < count; i += 1) {
    await Promise.resolve()
  }
}

async function waitForAssertion(assertion: () => void) {
  let lastError: unknown
  for (let attempt = 0; attempt < 20; attempt += 1) {
    try {
      assertion()
      return
    } catch (err: unknown) {
      lastError = err
      await Promise.resolve()
    }
  }
  if (lastError) throw lastError
  assertion()
}

function getRenderedRootElement(): ReactNode {
  const element = renderedRootElements.at(-1)
  if (!React.isValidElement(element)) {
    throw new Error('entrypoint did not render a React root element')
  }
  return element
}

async function renderCapturedRoot() {
  const actualReactDOM = await vi.importActual<{
    createRoot: (container: Element | DocumentFragment) => {
      render: (element: ReactNode) => void
      unmount: () => void
    }
  }>('react-dom/client')
  const mount = document.createElement('div')
  document.body.appendChild(mount)
  const root = actualReactDOM.createRoot(mount)
  await act(async () => {
    root.render(getRenderedRootElement())
    await drainMicrotasks()
  })
  return root
}

async function importEntrypoint() {
  await import('./entrypoint.js')
}

describe('browser entrypoint boot readiness', () => {
  beforeEach(() => {
    vi.resetModules()
    createRootMock.mockClear()
    hydrateRootMock.mockClear()
    waitConnMock.mockClear()
    bldrRuntimeMock.isDesktop = false
    initBrowserReleaseAutoReloadMock.mockClear()
    renderedRootElements.length = 0
    globalThis.IS_REACT_ACT_ENVIRONMENT = true
    document.body.innerHTML = ''
    window.location.href = 'http://localhost/'
    globalThis.__swDeferBoot = undefined
    globalThis.__swBoot = undefined
    globalThis.__swReady = undefined
    globalThis.__swBootStatus = undefined
    globalThis.__swReadyResolve = undefined
    globalThis.__swStartupMarks = undefined
    globalThis.__swStartupMarkSequence = undefined
    globalThis.__swStartupModuleImportedFrom = undefined
    globalThis.BLDR_RUNTIME_WASM_ENV = undefined
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    bldrRuntimeMock.isDesktop = false
    initBrowserReleaseAutoReloadMock.mockClear()
    renderedRootElements.length = 0
    document.body.innerHTML = ''
    window.location.href = 'http://localhost/'
    globalThis.__swDeferBoot = undefined
    globalThis.__swBoot = undefined
    globalThis.__swReady = undefined
    globalThis.__swBootStatus = undefined
    globalThis.__swReadyResolve = undefined
    globalThis.__swStartupMarks = undefined
    globalThis.__swStartupMarkSequence = undefined
    globalThis.__swStartupModuleImportedFrom = undefined
    globalThis.BLDR_RUNTIME_WASM_ENV = undefined
    globalThis.IS_REACT_ACT_ENVIRONMENT = undefined
  })

  it('skips browser release auto reload in desktop runtime', async () => {
    bldrRuntimeMock.isDesktop = true
    document.body.innerHTML = '<div id="bldr-root"></div>'

    await importEntrypoint()

    expect(initBrowserReleaseAutoReloadMock).not.toHaveBeenCalled()
  })

  it('initializes browser release auto reload in browser runtime', async () => {
    bldrRuntimeMock.isDesktop = false
    document.body.innerHTML = '<div id="bldr-root"></div>'

    await importEntrypoint()

    expect(initBrowserReleaseAutoReloadMock).toHaveBeenCalledTimes(1)
  })

  it('publishes staging CDN runtime env while preserving existing runtime keys', async () => {
    window.location.href = 'https://staging.spacewave.app/'
    globalThis.BLDR_RUNTIME_WASM_ENV = {
      UNRELATED_EXISTING_KEY: 'keep-me',
    }
    document.body.innerHTML = '<div id="bldr-root"></div>'

    await importEntrypoint()

    expect(globalThis.BLDR_RUNTIME_WASM_ENV).toEqual({
      UNRELATED_EXISTING_KEY: 'keep-me',
      SPACEWAVE_CDN_BASE_URL: 'https://cdn-staging.spacewave.app',
    })
  })

  it('does not publish a staging CDN override on production origin', async () => {
    window.location.href = 'https://spacewave.app/'
    document.body.innerHTML = '<div id="bldr-root"></div>'

    await importEntrypoint()

    expect(
      globalThis.BLDR_RUNTIME_WASM_ENV?.SPACEWAVE_CDN_BASE_URL,
    ).toBeUndefined()
  })

  it('resolves boot readiness after immediate render', async () => {
    document.body.innerHTML = '<div id="bldr-root"></div>'
    const ready = createReady()
    let resolved = false
    ready.promise.then(() => {
      resolved = true
    })
    globalThis.__swDeferBoot = true
    globalThis.__swReady = ready.promise
    globalThis.__swReadyResolve = ready.resolve

    await importEntrypoint()
    await Promise.resolve()

    expect(createRootMock).toHaveBeenCalledTimes(1)
    expect(hydrateRootMock).not.toHaveBeenCalled()
    expect(waitConnMock).not.toHaveBeenCalled()
    expect(globalThis.__swDeferBoot).toBeUndefined()
    expect(globalThis.__swReady).toBeUndefined()
    expect(globalThis.__swReadyResolve).toBeUndefined()
    expect(globalThis.__swBootStatus?.phase).toBe('ready')
    expect(resolved).toBe(true)
    expect(globalThis.__swStartupMarks?.map((mark) => mark.label)).toContain(
      'shell.immediate-boot-ready',
    )
  })

  it('resolves boot readiness after non-deferred prerender hydration', async () => {
    document.body.innerHTML =
      '<div id="bldr-root" data-prerendered="true"></div>'
    const ready = createReady()
    let resolved = false
    ready.promise.then(() => {
      resolved = true
    })
    globalThis.__swReady = ready.promise
    globalThis.__swReadyResolve = ready.resolve

    await importEntrypoint()
    await Promise.resolve()

    expect(hydrateRootMock).toHaveBeenCalledTimes(1)
    expect(createRootMock).not.toHaveBeenCalled()
    expect(waitConnMock).not.toHaveBeenCalled()
    expect(
      document.getElementById('bldr-root')?.hasAttribute('data-prerendered'),
    ).toBe(false)
    expect(globalThis.__swReady).toBeUndefined()
    expect(globalThis.__swReadyResolve).toBeUndefined()
    expect(globalThis.__swBootStatus?.phase).toBe('ready')
    expect(resolved).toBe(true)
    expect(globalThis.__swStartupMarks?.map((mark) => mark.label)).toContain(
      'shell.immediate-boot-ready',
    )
  })

  it('defers prerender __swBoot rendering until waitConn resolves', async () => {
    document.body.innerHTML =
      '<div id="bldr-root" data-prerendered="true"></div>'
    const runtimeReady = createReady()
    waitConnMock.mockReturnValueOnce(runtimeReady.promise)
    globalThis.__swDeferBoot = true

    await importEntrypoint()
    await Promise.resolve()

    expect(globalThis.__swBoot).toEqual(expect.any(Function))
    expect(waitConnMock).toHaveBeenCalledTimes(1)

    const boot = globalThis.__swBoot
    if (!boot) {
      throw new Error('deferred boot callback was not installed')
    }
    boot('/quickstart/deferred')
    await drainMicrotasks()

    expect(window.location.hash).toBe('#/quickstart/deferred')
    expect(createRootMock).not.toHaveBeenCalled()
    expect(hydrateRootMock).not.toHaveBeenCalled()
    expect(renderedRootElements).toHaveLength(0)
    expect(
      document.getElementById('bldr-root')?.hasAttribute('data-prerendered'),
    ).toBe(true)

    runtimeReady.resolve()
    await drainMicrotasks()

    expect(createRootMock).toHaveBeenCalledTimes(1)
    expect(hydrateRootMock).not.toHaveBeenCalled()
    expect(renderedRootElements).toHaveLength(1)
    expect(React.isValidElement(renderedRootElements[0])).toBe(true)
    expect(
      document.getElementById('bldr-root')?.hasAttribute('data-prerendered'),
    ).toBe(false)
  })

  it('coalesces deferred __swBoot requests to one render at the latest path', async () => {
    document.body.innerHTML =
      '<div id="bldr-root" data-prerendered="true"></div>'
    const runtimeReady = createReady()
    waitConnMock.mockReturnValueOnce(runtimeReady.promise)
    globalThis.__swDeferBoot = true

    await importEntrypoint()
    await Promise.resolve()

    const boot = globalThis.__swBoot
    if (!boot) {
      throw new Error('deferred boot callback was not installed')
    }
    boot('/quickstart/first')
    boot('/quickstart/latest')
    await drainMicrotasks()

    expect(window.location.hash).toBe('#/quickstart/latest')
    expect(createRootMock).not.toHaveBeenCalled()
    expect(renderedRootElements).toHaveLength(0)

    runtimeReady.resolve()
    await drainMicrotasks()

    expect(window.location.hash).toBe('#/quickstart/latest')
    expect(createRootMock).toHaveBeenCalledTimes(1)
    expect(renderedRootElements).toHaveLength(1)
    expect(
      document.getElementById('bldr-root')?.hasAttribute('data-prerendered'),
    ).toBe(false)
  })

  it('imports injected startup module without fetching BLDR_STARTUP_JS from the entrypoint', async () => {
    document.body.innerHTML = '<div id="bldr-root"></div>'
    const source = createStartupModuleURL()
    const fetchMock = vi.fn(() =>
      Promise.reject(new Error('entrypoint must not fetch startup module')),
    )
    vi.stubGlobal('BLDR_STARTUP_JS', source)
    vi.stubGlobal('fetch', fetchMock)

    await importEntrypoint()
    const root = await renderCapturedRoot()

    try {
      await waitForAssertion(() => {
        expect(globalThis.__swStartupModuleImportedFrom).toBe(source)
      })
      expect(fetchMock).not.toHaveBeenCalled()
    } finally {
      await act(async () => {
        root.unmount()
      })
    }
  })

  it('clamps determinate app progress UI between 0 and 100', () => {
    document.body.innerHTML = `
      <div id="bldr-root"></div>
      <div data-sw-boot-progress role="progressbar" aria-valuemin="0" aria-valuemax="100"></div>
      <span data-sw-boot-progress-label></span>
    `

    writeBrowserBootStatus({
      phase: 'app',
      detail:
        'Downloading the app bundle. This can take a while the first time.',
      state: 'loading',
      progress: -0.25,
    })

    const progress = document.querySelector('[data-sw-boot-progress]')
    if (!(progress instanceof HTMLElement)) {
      throw new Error('missing boot progress target')
    }
    expect(progress.style.width).toBe('0%')
    expect(progress.getAttribute('aria-valuenow')).toBe('0')
    expect(
      document.querySelector('[data-sw-boot-progress-label]')?.textContent,
    ).toBe('0%')
    expect(globalThis.__swBootStatus?.progress).toBe(0)

    writeBrowserBootStatus({
      phase: 'app',
      detail:
        'Downloading the app bundle. This can take a while the first time.',
      state: 'loading',
      progress: 1.25,
    })

    expect(progress.style.width).toBe('100%')
    expect(progress.getAttribute('aria-valuenow')).toBe('100')
    expect(
      document.querySelector('[data-sw-boot-progress-label]')?.textContent,
    ).toBe('100%')
    expect(globalThis.__swBootStatus?.progress).toBe(1)
  })

  it('keeps app progress indeterminate with no percent label until a positive total is known', () => {
    document.body.innerHTML = `
      <div id="bldr-root"></div>
      <div data-sw-boot-progress role="progressbar" aria-valuemin="0" aria-valuemax="100"></div>
      <span data-sw-boot-progress-label></span>
    `

    writeBrowserBootStatus({
      phase: 'app',
      detail:
        'Downloading the app bundle. This can take a while the first time.',
      state: 'loading',
    })

    expect(globalThis.__swBootStatus).toEqual({
      phase: 'app',
      detail:
        'Downloading the app bundle. This can take a while the first time.',
      state: 'loading',
    })
    const progress = document.querySelector('[data-sw-boot-progress]')
    if (!(progress instanceof HTMLElement)) {
      throw new Error('missing boot progress target')
    }
    expect(progress.classList.contains('animate-progress-indeterminate')).toBe(
      true,
    )
    expect(progress.style.width).toBe('33%')
    expect(progress.getAttribute('aria-valuenow')).toBeNull()
    expect(progress.getAttribute('aria-valuetext')).toBe('Loading')
    expect(
      document.querySelector('[data-sw-boot-progress-label]')?.textContent,
    ).toBe('')
  })
})
