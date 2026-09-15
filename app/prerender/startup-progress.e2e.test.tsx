import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { page } from 'vitest/browser'
import { cleanup, render } from 'vitest-browser-react'

import { resetBootDownloadsForTest } from '@aptre/bldr'
import { Pricing } from '@s4wave/app/landing/Pricing.js'
import { RouterProvider } from '@s4wave/web/router/router.js'
import { StaticProvider } from './StaticContext.js'
import { AppLoadingScreen } from '@s4wave/app/loading/AppLoadingScreen.js'
import { LoadingScreen as QuickstartSetupScreen } from '@s4wave/app/quickstart/LoadingScreen.js'
import { LoadingScreen } from '@s4wave/web/ui/loading/LoadingScreen.js'

import { resetBrowserStartupMarksForTest } from './boot-status.js'

beforeEach(() => {
  globalThis.__swBootStatus = undefined
  resetBootDownloadsForTest()
  resetBrowserStartupMarksForTest()
})

afterEach(async () => {
  await cleanup()
  globalThis.__swBootStatus = undefined
  resetBootDownloadsForTest()
  resetBrowserStartupMarksForTest()
  vi.restoreAllMocks()
})

describe('startup surfaces', () => {
  it.each([
    { width: 1440, height: 900 },
    { width: 390, height: 844 },
    { width: 320, height: 568 },
  ])(
    'shows actual phases and the emblem at $width × $height',
    async ({ width, height }) => {
      await page.viewport(width, height)
      globalThis.__swBootStatus = {
        phase: 'wasm',
        state: 'loading',
        detail: '',
      }
      await render(
        <div className="fixed inset-0 flex">
          <AppLoadingScreen />
        </div>,
      )
      await expect.element(page.getByRole('heading')).toBeVisible()
      await expect
        .element(page.getByRole('list', { name: 'Startup phases' }))
        .toBeVisible()
      expect(document.querySelector('[aria-current="step"]')?.textContent).toBe(
        'App',
      )
      expect(document.querySelector('details')).toBeNull()
      expect(document.querySelector('[role="progressbar"]')).toBeNull()
      expect(document.querySelector('canvas')).toBeNull()
      expect(document.querySelector('.swl-emblem img')).not.toBeNull()
      expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(width)
      const consoleBounds = document
        .querySelector('.swl-console')!
        .getBoundingClientRect()
      expect(consoleBounds.bottom).toBeLessThanOrEqual(height)
      await page.screenshot({
        path: `__screenshots__/browser-startup/current-pulse-${width}.png`,
      })
      await cleanup()
    },
  )

  it('never presents a readiness milestone as downloaded bytes', async () => {
    globalThis.__swBootStatus = {
      phase: 'app',
      state: 'loading',
      detail: '',
      progress: 0.37,
    }
    await render(<AppLoadingScreen />)
    expect(document.querySelector('[role="progressbar"]')).toBeNull()
    await expect.element(page.getByText('87%')).not.toBeInTheDocument()
  })

  it('shows quickstart progress without requiring a disclosure', async () => {
    await page.viewport(320, 568)
    await render(
      <div className="fixed inset-0 flex">
        <QuickstartSetupScreen
          quickstartId="drive"
          progress={{
            step: 'content',
            stepIndex: 4,
            stepCount: 4,
            detail: 'Adding starter content',
          }}
        />
      </div>,
    )
    await expect
      .element(page.getByRole('heading', { name: 'Creating your space' }))
      .toBeVisible()
    expect(document.querySelector('[aria-current="step"]')?.textContent).toBe(
      'Add starter content',
    )
    expect(
      document.querySelector('.swl-console')!.getBoundingClientRect().bottom,
    ).toBeLessThanOrEqual(568)
  })

  it('retains visible recovery when an interface download fails', async () => {
    globalThis.__swBootStatus = { phase: 'app', state: 'loading', detail: '' }
    globalThis.__swBootDownloads = [
      {
        id: 'frame',
        label: 'Interface frame',
        loaded: 0,
        state: 'error',
        error: 'Interface did not become ready.',
      },
    ]
    await render(<AppLoadingScreen />)
    await expect
      .element(page.getByRole('alert'))
      .toHaveTextContent('Interface did not become ready.')
    await expect
      .element(page.getByRole('button', { name: 'Retry' }))
      .toBeVisible()
    await expect
      .element(page.getByRole('button', { name: 'Back', exact: true }))
      .toBeVisible()
  })

  it('keeps recovery usable when WebGL is unavailable', async () => {
    vi.stubGlobal('WebGL2RenderingContext', undefined)
    const retry = vi.fn()
    await render(
      <div className="fixed inset-0 flex">
        <LoadingScreen
          view={{
            state: 'error',
            title: 'Unable to open Spacewave',
            error: 'Connection failed.',
            onRetry: retry,
          }}
        />
      </div>,
    )
    await page.getByRole('button', { name: 'Retry' }).click()
    expect(retry).toHaveBeenCalledOnce()
    await expect.element(page.getByRole('alert')).toBeVisible()
    vi.unstubAllGlobals()
  })
})

// App readiness can advance while a transfer is still active; percentages
// describe the transfer's byte counters, independently of that phase.
it('reports only measured current-download bytes', async () => {
  globalThis.__swBootStatus = {
    phase: 'app',
    state: 'loading',
    detail: '',
    progress: 0.8,
  }
  globalThis.__swBootDownloads = [
    {
      id: 'app',
      label: 'Application',
      loaded: 42,
      total: 100,
      state: 'active',
    },
  ]
  await render(
    <div className="fixed inset-0 flex">
      <AppLoadingScreen />
    </div>,
  )
  await expect
    .element(page.getByRole('progressbar', { name: 'Current download' }))
    .toHaveAttribute('aria-valuenow', '42')
})

it('keeps static routes on their prerendered surface', async () => {
  await render(
    <RouterProvider path="/pricing" onNavigate={() => {}}>
      <StaticProvider>
        <Pricing />
      </StaticProvider>
    </RouterProvider>,
  )
  await expect.element(page.getByText('Spacewave Pricing')).toBeVisible()
  expect(document.querySelector('.swl-canvas')).toBeNull()
})

it('retains startup feedback under reduced motion', async () => {
  const originalMatchMedia = window.matchMedia.bind(window)
  vi.spyOn(window, 'matchMedia').mockImplementation((query) => {
    const media = originalMatchMedia(query)
    if (query === '(prefers-reduced-motion: reduce)')
      Object.defineProperty(media, 'matches', { value: true })
    return media
  })
  await page.viewport(390, 844)
  await render(
    <div className="fixed inset-0 flex">
      <AppLoadingScreen />
    </div>,
  )
  expect(document.querySelector('canvas')).toBeNull()
  await expect.element(page.getByRole('heading')).toBeVisible()
  expect(
    document
      .querySelector('[data-sw-reduced-motion]')
      ?.getAttribute('data-sw-reduced-motion'),
  ).toBe('true')
})

it('keeps recovery reachable when an error wraps on a tiny screen', async () => {
  await page.viewport(320, 568)
  const back = vi.fn()
  await render(
    <div className="fixed inset-0 flex">
      <LoadingScreen
        view={{
          state: 'error',
          title: 'Unable to open Spacewave',
          error:
            'The web view did not become ready within 90 seconds. The application interface could not finish loading. Retry to reconnect, or return to the home page.',
          onCancel: back,
        }}
      />
    </div>,
  )
  await page.getByRole('button', { name: 'Back', exact: true }).click()
  expect(back).toHaveBeenCalledOnce()
})
