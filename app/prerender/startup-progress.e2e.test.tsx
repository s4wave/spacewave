import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { page } from 'vitest/browser'
import { cleanup, render } from 'vitest-browser-react'
import type { ReactNode } from 'react'

import {
  markBrowserStartupBoundary,
  resetBrowserStartupMarksForTest,
} from './boot-status.js'
import { resetBootDownloadsForTest, type BootDownload } from '@aptre/bldr'
import { Pricing } from '@s4wave/app/landing/Pricing.js'
import { AppLoadingScreen } from '@s4wave/app/loading/AppLoadingScreen.js'
import { QuickstartLoading } from '@s4wave/app/quickstart/QuickstartLoading.js'
import { RouterProvider } from '@s4wave/web/router/router.js'
import { StaticProvider } from './StaticContext.js'

function setRoute(path: string) {
  window.history.replaceState({}, '', path)
}

function setBootPhase(phase: string, state: 'loading' | 'error' = 'loading') {
  globalThis.__swBootStatus = {
    phase,
    detail: `${phase} detail`,
    state,
  }
}

function setBootPhaseProgress(phase: string, progress: number) {
  globalThis.__swBootStatus = {
    phase,
    detail: `${phase} detail`,
    state: 'loading',
    progress,
  }
}

const MiB = 1024 * 1024

function setBootDownloads(downloads: BootDownload[]) {
  globalThis.__swBootDownloads = downloads
}

async function renderSurface(children: ReactNode) {
  await render(
    <div className="bg-background text-foreground h-screen min-h-screen">
      {children}
    </div>,
  )
}

async function renderStaticSurface(path: string, children: ReactNode) {
  setRoute(path)
  await renderSurface(
    <RouterProvider path={path} onNavigate={() => {}}>
      <StaticProvider>{children}</StaticProvider>
    </RouterProvider>,
  )
}

async function captureStartupEvidence(name: string) {
  return page.screenshot({
    path: `__screenshots__/browser-startup/${name}.png`,
  })
}

function startupSurfaceBounds() {
  const surface = document.querySelector('.swb-canvas')
  if (!surface) return null
  const rect = surface.getBoundingClientRect()
  return {
    width: rect.width,
    height: rect.height,
    viewportWidth: window.innerWidth,
  }
}

describe('browser startup progress surfaces', () => {
  const originalMatchMedia = window.matchMedia

  beforeEach(async () => {
    await cleanup()
    localStorage.clear()
    window.location.hash = ''
    globalThis.__swBootStatus = undefined
    globalThis.__swBootDownloads = undefined
    resetBootDownloadsForTest()
    resetBrowserStartupMarksForTest()
    window.matchMedia = originalMatchMedia
    await page.viewport(1280, 800)
  })

  afterEach(async () => {
    await cleanup()
    localStorage.clear()
    globalThis.__swBootStatus = undefined
    globalThis.__swBootDownloads = undefined
    resetBootDownloadsForTest()
    resetBrowserStartupMarksForTest()
    window.matchMedia = originalMatchMedia
    setRoute('/')
    vi.restoreAllMocks()
  })

  it('renders returning-user boot progress in the browser', async () => {
    localStorage.setItem('spacewave-has-session', '1')
    setBootPhase('runtime')

    await renderSurface(<AppLoadingScreen />)

    await expect
      .element(page.getByText('App: Loading the application interface.'))
      .toBeInTheDocument()
    await expect
      .element(page.getByText('Prepare', { exact: true }))
      .toBeInTheDocument()
    await expect
      .element(page.getByText('Connect', { exact: true }))
      .toBeInTheDocument()
    await expect
      .element(page.getByText('Runtime', { exact: true }))
      .toBeInTheDocument()
    await expect
      .element(page.getByText('App', { exact: true }))
      .toBeInTheDocument()
    await expect
      .element(page.getByText('Done', { exact: true }))
      .toBeInTheDocument()
    const bounds = startupSurfaceBounds()
    expect(typeof bounds?.width).toBe('number')
    expect(typeof bounds?.height).toBe('number')

    await captureStartupEvidence('returning-user-runtime-desktop')
  })

  it('renders the cold-cache app download with real byte progress', async () => {
    localStorage.setItem('spacewave-has-session', '1')
    setBootPhaseProgress('app', 0.37)

    await renderSurface(<AppLoadingScreen />)

    await expect
      .element(page.getByText('App: Opening the application.'))
      .toBeInTheDocument()
    await expect.element(page.getByText('87%')).toBeInTheDocument()

    await captureStartupEvidence('frame-download-progress-desktop')
  })

  it('renders a live per-asset download breakdown mid-boot', async () => {
    localStorage.setItem('spacewave-has-session', '1')
    setBootPhase('app')
    setBootDownloads([
      {
        id: 'runtime',
        label: 'Runtime',
        loaded: 8 * MiB,
        total: 8 * MiB,
        state: 'complete',
      },
      {
        id: 'app',
        label: 'Application',
        loaded: 3355443,
        total: 8912896,
        state: 'active',
      },
      {
        id: '/b/pa/spacewave-app/v/b/fe/module.mjs',
        label: 'spacewave-app',
        loaded: 0,
        state: 'active',
      },
    ])

    await renderSurface(<AppLoadingScreen />)

    await expect
      .element(page.getByText('App: Loading the application interface.'))
      .toBeInTheDocument()
    // The plugin row's label is unique to the download list.
    await expect
      .element(page.getByText('spacewave-app', { exact: true }))
      .toBeInTheDocument()
    // App bundle shows streamed bytes against its known total.
    await expect
      .element(page.getByText('3.2 MiB / 8.5 MiB'))
      .toBeInTheDocument()

    // Completed downloads retire from the live list.
    expect(
      document.querySelectorAll('[data-sw-startup-download]'),
    ).toHaveLength(2)
    expect(
      document.querySelector('[data-sw-startup-download="runtime"]'),
    ).toBeNull()

    await captureStartupEvidence('per-asset-download-breakdown-desktop')
  })

  it('reaches the ready handoff at full progress', async () => {
    localStorage.setItem('spacewave-has-session', '1')
    setBootPhase('app')
    markBrowserStartupBoundary('webview.revealed', { startupRelevant: true })

    await renderSurface(<AppLoadingScreen />)

    await expect
      .element(page.getByText('Done: Spacewave is ready.'))
      .toBeInTheDocument()
    await expect.element(page.getByText('100%')).toBeInTheDocument()

    await captureStartupEvidence('frame-ready-handoff-desktop')
  })

  it('renders an early prepare phase on the quickstart handoff', async () => {
    setBootPhase('loading')

    await renderStaticSurface('/quickstart/drive', <QuickstartLoading />)

    await expect
      .element(page.getByText('Prepare: Loading the app shell.'))
      .toBeInTheDocument()
    await expect
      .element(page.getByText('Prepare', { exact: true }))
      .toBeInTheDocument()

    await captureStartupEvidence('quickstart-prepare-early')
  })

  it('keeps static routes on their prerendered handoff surface', async () => {
    await renderStaticSurface('/pricing', <Pricing />)

    await expect
      .element(page.getByText('Spacewave Pricing'))
      .toBeInTheDocument()
    expect(document.querySelector('[aria-label="Startup phases"]')).toBeNull()

    await captureStartupEvidence('static-route-pricing-handoff')
  })

  it('renders public Quickstart handoff progress and marks the handoff boundary', async () => {
    setBootPhase('entrypoint')

    await renderStaticSurface('/quickstart/drive', <QuickstartLoading />)

    await expect.element(page.getByText('Create a Drive')).toBeInTheDocument()
    await expect
      .element(page.getByText('Connect: Downloading the application.'))
      .toBeInTheDocument()
    await expect.element(page.getByText('Back to home')).toBeInTheDocument()

    await captureStartupEvidence('quickstart-drive-handoff')
  })

  it('renders startup errors with recoverable actions', async () => {
    localStorage.setItem('spacewave-has-session', '1')
    setBootPhase('runtime-error', 'error')

    await renderSurface(<AppLoadingScreen />)

    await expect
      .element(page.getByText('Runtime: Loading the application interface.'))
      .toBeInTheDocument()
    await expect
      .element(
        page.getByText(
          'Startup did not finish. Check the browser console or startup marks for details.',
        ),
      )
      .toBeInTheDocument()
    await expect.element(page.getByText('Retry')).toBeInTheDocument()
    await expect.element(page.getByText('Back')).toBeInTheDocument()

    await captureStartupEvidence('returning-user-error')
  })

  it('keeps reduced-motion startup progress readable', async () => {
    localStorage.setItem('spacewave-has-session', '1')
    setBootPhase('runtime')
    const reducedMatchMedia: typeof window.matchMedia = (query: string) => ({
      matches: query === '(prefers-reduced-motion: reduce)',
      media: query,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })
    window.matchMedia = reducedMatchMedia

    await renderSurface(<AppLoadingScreen />)

    await expect
      .element(page.getByText('App: Loading the application interface.'))
      .toBeInTheDocument()
    expect(
      document
        .querySelector('[data-sw-startup-reduced-motion]')
        ?.getAttribute('data-sw-startup-reduced-motion'),
    ).toBe('true')
    expect(document.querySelector('.shine-border-mask')).toBeNull()

    await captureStartupEvidence('returning-user-reduced-motion')
  })

  it('fits the returning-user startup layout on a mobile viewport', async () => {
    await page.viewport(390, 844)
    localStorage.setItem('spacewave-has-session', '1')
    setBootPhase('app')

    await renderSurface(<AppLoadingScreen />)

    await expect
      .element(page.getByText('App: Loading the application interface.'))
      .toBeInTheDocument()
    await expect
      .element(page.getByText('Done', { exact: true }))
      .toBeInTheDocument()

    const bounds = startupSurfaceBounds()
    expect(bounds).not.toBeNull()
    expect(bounds?.width).toBeLessThanOrEqual(bounds?.viewportWidth ?? 0)
    expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(
      window.innerWidth,
    )

    await captureStartupEvidence('returning-user-frame-mobile')
  })
})
