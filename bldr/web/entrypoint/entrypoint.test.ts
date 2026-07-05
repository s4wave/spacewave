import React, { act, type ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

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
  WebDocument: class WebDocument {
    waitConn() {
      return waitConnMock()
    }
  },
}))

vi.mock('../bldr/browser-release-update.js', () => ({
  initBrowserReleaseAutoReload: vi.fn(),
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

function createStartupModuleURL() {
  const source = [
    'globalThis.__swStartupModuleImportedFrom = import.meta.url;',
    'export default function StartupTestComponent() { return null }',
  ].join('\n')
  return `data:text/javascript;charset=utf-8,${encodeURIComponent(source)}`
}

function installStartupFetch(contentLength: string | null) {
  let controller: ReadableStreamDefaultController<Uint8Array> | undefined
  const body = new ReadableStream<Uint8Array>({
    start(nextController) {
      controller = nextController
    },
  })
  const headers = new Headers()
  if (contentLength !== null) {
    headers.set('content-length', contentLength)
  }
  const source = createStartupModuleURL()
  const fetchMock = vi.fn(() =>
    Promise.resolve(new Response(body, { status: 200, headers })),
  )
  vi.stubGlobal('BLDR_STARTUP_JS', source)
  vi.stubGlobal('fetch', fetchMock)
  if (!controller) {
    throw new Error('startup fetch stream controller was not initialized')
  }
  return { source, controller, fetchMock }
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
    renderedRootElements.length = 0
    globalThis.IS_REACT_ACT_ENVIRONMENT = true
    document.body.innerHTML = ''
    globalThis.__swDeferBoot = undefined
    globalThis.__swBoot = undefined
    globalThis.__swReady = undefined
    globalThis.__swBootStatus = undefined
    globalThis.__swReadyResolve = undefined
    globalThis.__swStartupMarks = undefined
    globalThis.__swStartupMarkSequence = undefined
    globalThis.__swStartupModuleImportedFrom = undefined
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    renderedRootElements.length = 0
    document.body.innerHTML = ''
    globalThis.__swDeferBoot = undefined
    globalThis.__swBoot = undefined
    globalThis.__swReady = undefined
    globalThis.__swBootStatus = undefined
    globalThis.__swReadyResolve = undefined
    globalThis.__swStartupMarks = undefined
    globalThis.__swStartupMarkSequence = undefined
    globalThis.__swStartupModuleImportedFrom = undefined
    globalThis.IS_REACT_ACT_ENVIRONMENT = undefined
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

  it('keeps deferred prerender boot gated on the web runtime', async () => {
    document.body.innerHTML =
      '<div id="bldr-root" data-prerendered="true"></div>'
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

    expect(globalThis.__swBoot).toEqual(expect.any(Function))
    expect(waitConnMock).toHaveBeenCalledTimes(1)
    expect(globalThis.__swReady).toBe(ready.promise)
    expect(globalThis.__swReadyResolve).toBeUndefined()
    expect(globalThis.__swBootStatus?.phase).toBe('ready')
    expect(resolved).toBe(true)
    expect(globalThis.__swStartupMarks?.map((mark) => mark.label)).toContain(
      'shell.deferred-boot-ready',
    )
  })

  it('preloads the startup module stream and reports determinate app download progress', async () => {
    document.body.innerHTML = `
      <div id="bldr-root"></div>
      <div data-sw-boot-progress role="progressbar" aria-valuemin="0" aria-valuemax="100"></div>
      <span data-sw-boot-progress-label></span>
    `
    const startup = installStartupFetch('10')

    await importEntrypoint()
    const root = await renderCapturedRoot()

    try {
      expect(startup.fetchMock).toHaveBeenCalledWith(startup.source, {
        method: 'GET',
        credentials: 'same-origin',
      })

      startup.controller.enqueue(new Uint8Array(4))
      await waitForAssertion(() => {
        expect(globalThis.__swBootStatus).toMatchObject({
          phase: 'app',
          detail:
            'Downloading the app bundle. This can take a while the first time.',
          state: 'loading',
        })
        expect(globalThis.__swBootStatus?.progress).toBeCloseTo(0.4)
      })
      const progress = document.querySelector('[data-sw-boot-progress]')
      if (!(progress instanceof HTMLElement)) {
        throw new Error('missing boot progress target')
      }
      expect(progress.style.width).toBe('40%')
      expect(progress.getAttribute('aria-valuenow')).toBe('40')

      startup.controller.enqueue(new Uint8Array(6))
      startup.controller.close()
      await waitForAssertion(() => {
        expect(globalThis.__swStartupModuleImportedFrom).toBe(startup.source)
      })

      expect(globalThis.__swBootStatus?.progress).toBe(1)
      expect(
        document.querySelector('[data-sw-boot-progress-label]')?.textContent,
      ).toBe('100%')
    } finally {
      await act(async () => {
        root.unmount()
      })
    }
  })

  it.each([
    { name: 'missing content-length', contentLength: null },
    { name: 'zero content-length', contentLength: '0' },
  ])(
    'keeps startup module preload progress indeterminate with $name',
    async ({ contentLength }) => {
      document.body.innerHTML = `
        <div id="bldr-root"></div>
        <div data-sw-boot-progress role="progressbar" aria-valuemin="0" aria-valuemax="100"></div>
        <span data-sw-boot-progress-label></span>
      `
      const startup = installStartupFetch(contentLength)

      await importEntrypoint()
      const root = await renderCapturedRoot()

      try {
        await waitForAssertion(() => {
          expect(globalThis.__swBootStatus).toEqual({
            phase: 'app',
            detail:
              'Downloading the app bundle. This can take a while the first time.',
            state: 'loading',
          })
        })

        startup.controller.enqueue(new Uint8Array(4))
        startup.controller.close()
        await drainMicrotasks()
        const progress = document.querySelector('[data-sw-boot-progress]')
        if (!(progress instanceof HTMLElement)) {
          throw new Error('missing boot progress target')
        }
        expect(
          progress.classList.contains('animate-progress-indeterminate'),
        ).toBe(true)
        expect(progress.style.width).toBe('33%')
        expect(progress.getAttribute('aria-valuenow')).toBeNull()
        expect(progress.getAttribute('aria-valuetext')).toBe('Loading')
        expect(
          document.querySelector('[data-sw-boot-progress-label]')?.textContent,
        ).toBe('')
      } finally {
        await act(async () => {
          root.unmount()
        })
      }
    },
  )
})
