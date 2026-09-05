import { afterEach, describe, expect, it, vi } from 'vitest'
import { act, cleanup, render, screen } from '@testing-library/react'
import {
  advanceBootDownload,
  beginBootDownload,
  completeBootDownload,
  failBootDownload,
  resetBootDownloadsForTest,
} from '@aptre/bldr'

import { AppLoadingScreen } from './AppLoadingScreen.js'
import type { BrowserStartupProjection } from './status/browser-startup-model.js'

const mockProjection = vi.hoisted<{
  initial: BrowserStartupProjection
  current: BrowserStartupProjection
}>(() => {
  const initial: BrowserStartupProjection = {
    view: {
      state: 'loading',
      title: 'Starting the Spacewave runtime',
      detail: 'Runtime initialization: Connecting the Spacewave runtime.',
      progress: 0.58,
    },
    phase: {
      id: 'runtime',
      label: 'Runtime',
    },
    phases: [
      { id: 'prepare', label: 'Prepare', state: 'complete' },
      { id: 'connect', label: 'Connect', state: 'complete' },
      { id: 'runtime', label: 'Runtime', state: 'current' },
      { id: 'frame', label: 'App', state: 'pending' },
      { id: 'done', label: 'Done', state: 'pending' },
    ],
    evidence: {
      status: {
        phase: 'runtime',
        detail: 'Connecting runtime...',
        state: 'loading',
        progress: 0.58,
      },
      marks: [],
      runtime: {
        startup: { phase: 'runtime' },
        document: { state: 'unknown' },
        runtimeClient: { state: 'opening' },
        serviceWorker: { state: 'unknown' },
        pluginGeneration: { state: 'idle' },
        frame: { state: 'idle' },
        warmProjection: {
          state: 'cold',
          connection: false,
          neutralFrame: false,
          finalReveal: false,
        },
      },
    },
  }
  return {
    initial,
    current: structuredClone(initial),
  }
})

vi.mock('@s4wave/app/loading/status/browser-startup.js', () => ({
  useBrowserStartupProjection: () => mockProjection.current,
}))

afterEach(() => {
  cleanup()
  resetBootDownloadsForTest()
  mockProjection.current = structuredClone(mockProjection.initial)
  vi.useRealTimers()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('AppLoadingScreen', () => {
  it('keeps phase percentages out of the default screen and exposes diagnostics on demand', () => {
    const { container } = render(<AppLoadingScreen />)
    expect(screen.getByText('Opening Spacewave')).toBeDefined()
    expect(screen.getByText('Starting the app')).toBeDefined()
    expect(
      screen.getByRole('progressbar').getAttribute('aria-valuenow'),
    ).toBeNull()
    expect(screen.queryByText('58%')).toBeNull()
    expect(screen.queryByLabelText('Startup phases')).toBeNull()
    const details = container.querySelector('details')
    expect(details?.open).toBe(false)
    expect(details?.textContent).toContain(
      'Runtime initialization: Connecting the Spacewave runtime.',
    )
    expect(
      screen.getByText('Downloaded files are saved on this device.'),
    ).toBeDefined()
  })

  it('retires a download when it completes while active and failed rows remain', () => {
    beginBootDownload('app', 'Application', 100)
    beginBootDownload('plugin', 'Plugin', 200)
    advanceBootDownload('plugin', 50, 200)
    beginBootDownload('styles', 'Styles', 10)

    const { container } = render(<AppLoadingScreen />)
    expect(
      container.querySelector('[data-sw-startup-download="app"]'),
    ).not.toBeNull()

    act(() => {
      completeBootDownload('app')
      failBootDownload('styles', 'network error')
    })

    expect(
      container.querySelector('[data-sw-startup-download="app"]'),
    ).toBeNull()
    expect(
      container.querySelector('[data-sw-startup-download="plugin"]'),
    ).not.toBeNull()
    expect(
      container.querySelector('[data-sw-startup-download="styles"]'),
    ).not.toBeNull()
    expect(screen.getByRole('alert').textContent).toBe('network error')
    expect(screen.getByRole('button', { name: 'Retry' })).toBeDefined()
    expect(container.querySelector('.swb-activity')).toBeNull()
  })

  it('renders retry and back affordances for startup errors', () => {
    mockProjection.current = {
      ...mockProjection.current,
      view: {
        state: 'error',
        title: 'Connecting to your Space',
        detail: 'Session connection: Downloading the application.',
        progress: 0.3,
        error:
          'Startup did not finish. Check the browser console or startup marks for details.',
      },
      phase: {
        id: 'connect',
        label: 'Connect',
      },
      phases: mockProjection.current.phases.map((phase) => ({
        ...phase,
        state:
          phase.id === 'prepare'
            ? 'complete'
            : phase.id === 'connect'
              ? 'error'
              : 'pending',
      })),
    }

    const { container } = render(<AppLoadingScreen />)

    expect(screen.getByText('Retry')).toBeDefined()
    expect(screen.getByText('Back')).toBeDefined()
    expect(
      screen.getByText(
        'Startup did not finish. Check the browser console or startup marks for details.',
      ),
    ).toBeDefined()
    expect(screen.getByRole('alert')).toBeDefined()
    expect(container.querySelector('.swb-activity')).toBeNull()
  })

  it('retains startup diagnostics with reduced motion', () => {
    vi.stubGlobal('matchMedia', (query: string) => ({
      matches: query === '(prefers-reduced-motion: reduce)',
      media: query,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    }))

    const { container } = render(<AppLoadingScreen />)

    expect(
      container
        .querySelector('[data-sw-startup-reduced-motion]')
        ?.getAttribute('data-sw-startup-reduced-motion'),
    ).toBe('true')
    expect(
      screen.getByText(
        'Runtime initialization: Connecting the Spacewave runtime.',
      ),
    ).toBeDefined()
    expect(
      screen.getByRole('progressbar').getAttribute('aria-valuenow'),
    ).toBeNull()
  })
})
