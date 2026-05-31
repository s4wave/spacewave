import React, { type ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const createRootMock = vi.hoisted(() =>
  vi.fn(() => ({ render: vi.fn(), unmount: vi.fn() })),
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
  BldrRoot: () => null,
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

async function importEntrypoint() {
  await import('./entrypoint.js')
}

describe('browser entrypoint boot readiness', () => {
  beforeEach(() => {
    vi.resetModules()
    createRootMock.mockClear()
    hydrateRootMock.mockClear()
    waitConnMock.mockClear()
    document.body.innerHTML = ''
    globalThis.__swDeferBoot = undefined
    globalThis.__swBoot = undefined
    globalThis.__swReady = undefined
    globalThis.__swBootStatus = undefined
    globalThis.__swReadyResolve = undefined
    globalThis.__swStartupMarks = undefined
    globalThis.__swStartupMarkSequence = undefined
  })

  afterEach(() => {
    document.body.innerHTML = ''
    globalThis.__swDeferBoot = undefined
    globalThis.__swBoot = undefined
    globalThis.__swReady = undefined
    globalThis.__swBootStatus = undefined
    globalThis.__swReadyResolve = undefined
    globalThis.__swStartupMarks = undefined
    globalThis.__swStartupMarkSequence = undefined
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
})
