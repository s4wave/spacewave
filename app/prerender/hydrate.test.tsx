import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Root } from 'react-dom/client'

const mockHydrateRoot = vi.hoisted(() =>
  vi.fn(() => ({ render: vi.fn(), unmount: vi.fn() })),
)

vi.mock('react-dom/client', async () => {
  const actual =
    await vi.importActual<typeof import('react-dom/client')>('react-dom/client')
  return {
    ...actual,
    hydrateRoot: mockHydrateRoot,
  }
})

vi.mock('@s4wave/app/landing/Landing.js', () => ({
  Landing: function Landing() {
    return null
  },
}))

declare global {
  var __swReady: Promise<void> | undefined
  var __swBoot: ((hash: string) => void) | undefined
  var __swPrerenderRoot: Root | undefined
  var __swPrerenderContainer: HTMLElement | undefined
}

function renderRootShell() {
  document.body.innerHTML = `
    <div id="bldr-root" data-prerendered="true" role="main">
      <div id="sw-landing" style="display:flex"></div>
      <div id="sw-loading" style="display:none">
        <p data-sw-boot-status>Loading application...</p>
      </div>
    </div>
  `
}

function createReady() {
  const ready = { resolve: () => {} }
  const promise = new Promise<void>((resolve) => {
    ready.resolve = resolve
  })
  return { promise, resolve: () => ready.resolve() }
}

describe('hydrate root hash boot', () => {
  beforeEach(() => {
    vi.resetModules()
    mockHydrateRoot.mockClear()
    localStorage.clear()
    window.history.replaceState({}, '', '/')
    renderRootShell()
    globalThis.__swReady = undefined
    globalThis.__swBoot = undefined
    globalThis.__swPrerenderRoot = undefined
    globalThis.__swPrerenderContainer = undefined
  })

  afterEach(() => {
    document.body.innerHTML = ''
    window.history.replaceState({}, '', '/')
    globalThis.__swReady = undefined
    globalThis.__swBoot = undefined
    globalThis.__swPrerenderRoot = undefined
    globalThis.__swPrerenderContainer = undefined
  })

  it('boots a root hash link after the prerendered landing has loaded', async () => {
    const ready = createReady()
    globalThis.__swReady = ready.promise

    await import('./hydrate.js')
    expect(mockHydrateRoot).toHaveBeenCalledTimes(1)

    window.location.hash = '/login'
    window.dispatchEvent(new HashChangeEvent('hashchange'))

    expect(document.getElementById('sw-landing')?.style.display).toBe('none')
    expect(document.getElementById('sw-loading')?.style.display).toBe('')

    const boot = vi.fn()
    globalThis.__swBoot = boot
    ready.resolve()
    await globalThis.__swReady
    await Promise.resolve()

    expect(boot).toHaveBeenCalledWith('#/login')
  })
})
