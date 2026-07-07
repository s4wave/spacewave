import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'

import { AppLoadingScreen } from './AppLoadingScreen.js'
import type { BrowserStartupProjection } from './status/browser-startup-model.js'

const mockProjection = vi.hoisted<{
  initial: BrowserStartupProjection
  current: BrowserStartupProjection
}>(() => {
  const initial: BrowserStartupProjection = {
    view: {
      state: 'loading',
      title: 'Spacewave',
      detail: 'Runtime: Starting the Spacewave runtime.',
      progress: 0.58,
    },
    phase: {
      id: 'runtime',
      label: 'Runtime',
      detail: 'Starting the Spacewave runtime.',
      progress: 0.58,
    },
    phases: [
      {
        id: 'prepare',
        label: 'Prepare',
        detail: 'Preparing browser files.',
        progress: 0.08,
        state: 'complete',
      },
      {
        id: 'connect',
        label: 'Connect',
        detail: 'Connecting the app shell.',
        progress: 0.3,
        state: 'complete',
      },
      {
        id: 'runtime',
        label: 'Runtime',
        detail: 'Starting the Spacewave runtime.',
        progress: 0.58,
        state: 'current',
      },
      {
        id: 'frame',
        label: 'App',
        detail:
          'Downloading the app bundle. This can take a while the first time.',
        progress: 0.84,
        state: 'pending',
      },
      {
        id: 'done',
        label: 'Done',
        detail: 'Spacewave is ready.',
        progress: 1,
        state: 'pending',
      },
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
  mockProjection.current = structuredClone(mockProjection.initial)
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

    expect(screen.getByText('Spacewave')).toBeDefined()
    expect(
      screen.getByText('Runtime: Starting the Spacewave runtime.'),
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
        title: 'Spacewave',
        detail:
          'App: Downloading the app bundle. This can take a while the first time.',
        progress: 0.42,
      },
      phase: {
        id: 'frame',
        label: 'App',
        detail:
          'Downloading the app bundle. This can take a while the first time.',
        progress: 0.84,
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

    expect(screen.getByText('42%')).toBeDefined()
    expect(screen.queryByText('84%')).toBeNull()
    expect(container.querySelector('.swb-bar-fill--indeterminate')).toBeNull()
    expect(stepState(container, 'App')).toMatchObject({
      state: 'current',
      spinner: true,
    })
  })

  it('renders retry and back affordances for startup errors', () => {
    mockProjection.current = {
      ...mockProjection.current,
      view: {
        state: 'error',
        title: 'Spacewave',
        detail: 'Connect: Connecting the app shell.',
        progress: 0.3,
        error:
          'Startup did not finish. Check the browser console or startup marks for details.',
      },
      phase: {
        id: 'connect',
        label: 'Connect',
        detail: 'Connecting the app shell.',
        progress: 0.3,
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
      screen.getByText('Runtime: Starting the Spacewave runtime.'),
    ).toBeDefined()
    expect(screen.getByLabelText('Startup phases')).toBeDefined()
    expect(screen.getByText('58%')).toBeDefined()
  })
})
