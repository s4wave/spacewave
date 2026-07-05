import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Root } from 'react-dom/client'

import { ROOT_LOADING_STYLE } from './root-loading-shell.js'

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

vi.mock('./static-pages.js', () => ({
  getStaticPageComponent(pathname: string) {
    if (pathname === '/quickstart/drive') {
      return function QuickstartLoading() {
        return null
      }
    }
    return null
  },
}))

vi.mock('@s4wave/app/blog/BlogPost.js', () => ({
  BlogPostPage: function BlogPostPage() {
    return null
  },
}))

vi.mock('@s4wave/app/blog/BlogIndex.js', () => ({
  BlogIndex: function BlogIndex() {
    return null
  },
}))

vi.mock('@s4wave/app/blog/BlogTagPage.js', () => ({
  BlogTagPage: function BlogTagPage() {
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
      <div id="sw-loading" style="${ROOT_LOADING_STYLE}">
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
    const loading = document.getElementById('sw-loading')
    expect(loading?.style.display).toBe('')
    expect(loading?.style.flex).toBe('1 1 0%')
    expect(loading?.style.width).toBe('100%')
    expect(loading?.style.minWidth).toBe('0')

    const boot = vi.fn()
    globalThis.__swBoot = boot
    ready.resolve()
    await ready.promise
    await Promise.resolve()

    expect(boot).toHaveBeenCalledWith('#/login')
  }, 15000)

  it('auto-boots a prerendered quickstart page without hydrating the transient loading DOM', async () => {
    const ready = createReady()
    globalThis.__swReady = ready.promise
    window.history.replaceState({}, '', '/quickstart/drive')
    const startupMark = vi.fn<(event: Event) => void>()
    window.addEventListener('spacewave-startup-mark', startupMark)

    await import('./hydrate.js')
    expect(mockHydrateRoot).not.toHaveBeenCalled()
    const startupMarkNames = startupMark.mock.calls.map(
      ([event]) => (event as CustomEvent<{ name: string }>).detail.name,
    )
    expect(startupMarkNames).toContain(
      'spacewave.startup.quickstart.static-handoff-requested',
    )

    const boot = vi.fn()
    globalThis.__swBoot = boot
    ready.resolve()
    await ready.promise
    await Promise.resolve()

    window.removeEventListener('spacewave-startup-mark', startupMark)
    expect(boot).toHaveBeenCalledWith('#/quickstart/drive')
  }, 15000)

  it('auto-boots display pathnames with query params after OTP hash wipe', async () => {
    const ready = createReady()
    globalThis.__swReady = ready.promise
    window.history.replaceState(
      {},
      '',
      '/display?path=docs%2Fhello&component=viewer.markdown#otp=secret',
    )

    await import('./hydrate.js')
    expect(mockHydrateRoot).not.toHaveBeenCalled()

    const boot = vi.fn()
    globalThis.__swBoot = boot
    ready.resolve()
    await ready.promise
    await Promise.resolve()

    expect(boot).toHaveBeenCalledWith(
      '/display?path=docs%2Fhello&component=viewer.markdown',
    )
  }, 15000)

  it('boots returning root visitors into the app without hydrating the landing page', async () => {
    const ready = createReady()
    globalThis.__swReady = ready.promise
    localStorage.setItem('spacewave-has-session', '1')

    await import('./hydrate.js')
    expect(mockHydrateRoot).not.toHaveBeenCalled()

    const boot = vi.fn()
    globalThis.__swBoot = boot
    ready.resolve()
    await ready.promise
    await Promise.resolve()

    expect(boot).toHaveBeenCalledWith('')
  }, 15000)
})
