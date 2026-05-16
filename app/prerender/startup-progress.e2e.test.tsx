import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { page } from 'vitest/browser'
import { cleanup, render } from 'vitest-browser-react'
import type { ReactNode } from 'react'

import { resetBrowserStartupMarksForTest } from './boot-status.js'
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

async function renderSurface(children: ReactNode) {
  await render(
    <div className="bg-background text-foreground h-full min-h-0">
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
  const surface = document.querySelector('[data-sw-startup-preview]')
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
    resetBrowserStartupMarksForTest()
    window.matchMedia = originalMatchMedia
    await page.viewport(1280, 800)
  })

  afterEach(async () => {
    await cleanup()
    localStorage.clear()
    globalThis.__swBootStatus = undefined
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
      .element(page.getByText('Frame: Opening the app frame.'))
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
      .element(page.getByText('Frame', { exact: true }))
      .toBeInTheDocument()
    await expect
      .element(page.getByText('Done', { exact: true }))
      .toBeInTheDocument()
    const bounds = startupSurfaceBounds()
    expect(typeof bounds?.width).toBe('number')
    expect(typeof bounds?.height).toBe('number')

    await captureStartupEvidence('returning-user-runtime-desktop')
  })

  it('keeps static routes on their prerendered handoff surface', async () => {
    await renderStaticSurface('/pricing', <Pricing />)

    await expect
      .element(page.getByText('Spacewave Pricing'))
      .toBeInTheDocument()
    await expect
      .element(page.getByText('Runtime: Starting the Spacewave runtime.'))
      .not.toBeInTheDocument()

    await captureStartupEvidence('static-route-pricing-handoff')
  })

  it('renders public Quickstart handoff progress and marks the handoff boundary', async () => {
    setBootPhase('entrypoint')

    await renderStaticSurface('/quickstart/drive', <QuickstartLoading />)

    await expect.element(page.getByText('Create a Drive')).toBeInTheDocument()
    await expect
      .element(page.getByText('Connect: Connecting the app shell.'))
      .toBeInTheDocument()
    await expect.element(page.getByText('Back to home')).toBeInTheDocument()

    await captureStartupEvidence('quickstart-drive-handoff')
  })

  it('renders startup errors with recoverable actions', async () => {
    localStorage.setItem('spacewave-has-session', '1')
    setBootPhase('runtime-error', 'error')

    await renderSurface(<AppLoadingScreen />)

    await expect
      .element(page.getByText('Runtime: Starting the Spacewave runtime.'))
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
      .element(page.getByText('Frame: Opening the app frame.'))
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
      .element(page.getByText('Frame: Opening the app frame.'))
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
