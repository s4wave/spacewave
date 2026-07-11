import { afterEach, describe, expect, it, vi } from 'vitest'
import { act, cleanup, render, screen } from '@testing-library/react'
import {
  advanceBootDownload,
  beginBootDownload,
  bootProgressStallDelayMs,
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
      },
    },
  }
  return {
    initial,
    current: structuredClone(initial),
  }
})

vi.mock('@s4wave/app/landing/AnimatedLogo.js', () => ({
  default: ({ reduceMotion }: { reduceMotion?: boolean }) => (
    <div
      data-testid="animated-logo"
      data-reduce-motion={reduceMotion ? 'true' : undefined}
    />
  ),
}))

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

// stepState reads the rendered per-phase state marker (data-state on the <li>)
// and whether that step shows the active spinner rather than a static dot.
function stepState(container: HTMLElement, label: string) {
  const rail = container.querySelector('[aria-label="Startup phases"]')
  const li = Array.from(rail?.querySelectorAll('.swb-step') ?? []).find(
    (el) => el.querySelector('.swb-step-label')?.textContent === label,
  )
  return {
    state: li?.getAttribute('data-state'),
    spinner: !!li?.querySelector('.swb-spinner'),
    dot: !!li?.querySelector('.swb-dot'),
  }
}

describe('AppLoadingScreen', () => {
  it('renders the branded startup composition from the shared loading view', () => {
    const { container } = render(<AppLoadingScreen />)

    expect(screen.getByText('Starting the Spacewave runtime')).toBeDefined()
    expect(
      screen.getByText(
        'Runtime initialization: Connecting the Spacewave runtime.',
      ),
    ).toBeDefined()
    expect(screen.getByLabelText('Startup phases')).toBeDefined()
    expect(screen.getByText('Prepare')).toBeDefined()
    expect(screen.getByText('Connect')).toBeDefined()
    expect(screen.getByText('Runtime')).toBeDefined()
    expect(screen.getByText('App')).toBeDefined()
    expect(screen.getByText('Done')).toBeDefined()
    // The first-run hint tells new users the initial download can be slow.
    expect(
      screen.getByText(/first launch downloads the full app bundle/i),
    ).toBeDefined()
    // The surface is self-styled from the inlined boot critical stylesheet so it
    // paints branded before the Tailwind app.css loads.
    expect(container.querySelector('.swb-canvas')).not.toBeNull()
  })

  it('marks each phase with its state: complete dot, current spinner, pending muted', () => {
    const { container } = render(<AppLoadingScreen />)

    expect(stepState(container, 'Prepare')).toMatchObject({
      state: 'complete',
      dot: true,
      spinner: false,
    })
    expect(stepState(container, 'Runtime')).toMatchObject({
      state: 'current',
      spinner: true,
      dot: false,
    })
    expect(stepState(container, 'Done')).toMatchObject({
      state: 'pending',
      dot: true,
      spinner: false,
    })
  })

  it('renders app bundle progress as determinate frame progress', () => {
    mockProjection.current = {
      ...mockProjection.current,
      view: {
        state: 'loading',
        title: 'Loading the app',
        detail: 'Current app download: Opening the application.',
        progress: 0.42,
      },
      phase: {
        id: 'frame',
        label: 'App',
      },
      phases: mockProjection.current.phases.map((phase) => ({
        ...phase,
        state:
          phase.id === 'done'
            ? 'pending'
            : phase.id === 'frame'
              ? 'current'
              : 'complete',
      })),
      evidence: {
        ...mockProjection.current.evidence,
        status: {
          phase: 'app',
          detail: 'Downloading app bundle...',
          state: 'loading',
          progress: 0.42,
        },
      },
    }

    const { container } = render(<AppLoadingScreen />)

    expect(screen.getByText('Current download')).toBeDefined()
    expect(screen.getByText('42%')).toBeDefined()
    expect(screen.queryByText('84%')).toBeNull()
    expect(container.querySelector('.swb-bar-fill--indeterminate')).toBeNull()
    expect(stepState(container, 'App')).toMatchObject({
      state: 'current',
      spinner: true,
    })
  })

  it('shimmers only after a markless gap and returns to determinate progress on the next mark', () => {
    vi.useFakeTimers()
    const { container, rerender } = render(<AppLoadingScreen />)
    const progress = screen.getByRole('progressbar')

    act(() => {
      vi.advanceTimersByTime(bootProgressStallDelayMs - 1)
    })
    expect(container.querySelector('.swb-bar-fill--stalled')).toBeNull()

    act(() => {
      vi.advanceTimersByTime(1)
    })
    const stalled = container.querySelector('.swb-bar-fill--stalled')
    if (!(stalled instanceof HTMLElement)) {
      throw new Error(
        'Expected the stalled progress bar fill to be an HTMLElement',
      )
    }
    expect(stalled.style.width).toBe('58%')
    expect(progress.getAttribute('aria-valuenow')).toBe('58')
    expect(screen.getByText('58%')).toBeDefined()
    expect(container.querySelector('.swb-bar-fill--indeterminate')).toBeNull()
    expect(
      screen.getByText(
        'The runtime is still starting. First launch may take longer.',
      ),
    ).toBeDefined()

    mockProjection.current = {
      ...mockProjection.current,
      view: {
        ...mockProjection.current.view,
        detail: 'Runtime initialization: Runtime channel connected.',
        progress: 0.64,
      },
      evidence: {
        ...mockProjection.current.evidence,
        marks: [
          {
            name: 'spacewave.startup.runtime.client-channel-acked',
            label: 'runtime.client-channel-acked',
            sequence: 1,
            detail: {
              label: 'runtime.client-channel-acked',
              sequence: 1,
            },
          },
        ],
      },
    }
    rerender(<AppLoadingScreen />)

    expect(container.querySelector('.swb-bar-fill--stalled')).toBeNull()
    expect(progress.getAttribute('aria-valuenow')).toBe('64')
    expect(screen.getByText('64%')).toBeDefined()
    act(() => {
      vi.advanceTimersByTime(bootProgressStallDelayMs - 1)
    })
    expect(container.querySelector('.swb-bar-fill--stalled')).toBeNull()
    act(() => {
      vi.advanceTimersByTime(1)
    })
    expect(container.querySelector('.swb-bar-fill--stalled')).not.toBeNull()
  })

  it('clears a stalled shimmer on error and never restarts it in a terminal state', () => {
    vi.useFakeTimers()
    const loading = render(<AppLoadingScreen />)
    act(() => {
      vi.advanceTimersByTime(bootProgressStallDelayMs)
    })
    expect(
      loading.container.querySelector('.swb-bar-fill--stalled'),
    ).not.toBeNull()

    mockProjection.current = {
      ...mockProjection.current,
      view: {
        state: 'error',
        title: 'Starting the Spacewave runtime',
        detail: 'Runtime initialization: Connecting the Spacewave runtime.',
        progress: 0.31,
        error:
          'Startup did not finish. Check the browser console or startup marks for details.',
      },
    }
    loading.rerender(<AppLoadingScreen />)

    expect(loading.container.querySelector('.swb-bar-fill--stalled')).toBeNull()
    act(() => {
      vi.advanceTimersByTime(bootProgressStallDelayMs)
    })
    expect(loading.container.querySelector('.swb-bar-fill--stalled')).toBeNull()
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
    expect(screen.getByText('network error')).toBeDefined()
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
    expect(stepState(container, 'Connect')).toMatchObject({
      state: 'error',
      dot: true,
      spinner: false,
    })
  })

  it('keeps startup status, phases, and progress readable with reduced motion', () => {
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
    expect(screen.getByTestId('animated-logo').dataset.reduceMotion).toBe(
      'true',
    )
    expect(
      screen.getByText(
        'Runtime initialization: Connecting the Spacewave runtime.',
      ),
    ).toBeDefined()
    expect(screen.getByLabelText('Startup phases')).toBeDefined()
    expect(screen.getByText('58%')).toBeDefined()
  })
})
