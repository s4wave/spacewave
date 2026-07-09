import { describe, expect, it } from 'vitest'

import { projectBootProgress, type BootProgressMark } from './boot-progress.js'

function mark(
  label: string,
  detail?: Record<string, unknown>,
): BootProgressMark {
  return { label, detail }
}

describe('projectBootProgress', () => {
  it('starts at the initial prepare step', () => {
    const step = projectBootProgress({ phase: 'loading', state: 'loading' })
    expect(step.progress).toBeCloseTo(0.02)
    expect(step.label).toBe('Loading the app shell.')
  })

  it('advances through the observed runtime marks and names the current step', () => {
    const sequence = [
      ['runtime.mode-selected', 0.32, 'Choosing the runtime mode.'],
      [
        'service-worker.register-start',
        0.34,
        'Registering the service worker.',
      ],
      ['runtime.client-open-start', 0.35, 'Opening the runtime channel.'],
      ['service-worker.register-ready', 0.36, 'Service worker registered.'],
      ['service-worker.update-ready', 0.38, 'Service worker updated.'],
      [
        'service-worker.control-ready',
        0.4,
        'Service worker controlling the app.',
      ],
      [
        'service-worker.port-started',
        0.42,
        'Starting the service worker bridge.',
      ],
      [
        'service-worker.port-sent',
        0.44,
        'Connecting the service worker bridge.',
      ],
      ['runtime.worker-create-start', 0.46, 'Starting the runtime worker.'],
      ['runtime.worker-created', 0.48, 'Runtime worker started.'],
      ['runtime.opfs-bridge-ready', 0.52, 'Preparing browser storage.'],
      ['runtime.client-open-sent', 0.56, 'Opening the runtime channel.'],
      ['runtime.client-channel-opened', 0.6, 'Runtime channel opened.'],
      ['runtime.client-channel-acked', 0.64, 'Runtime channel connected.'],
      ['runtime.connected', 0.68, 'Runtime connected.'],
      ['runtime.event-connected', 0.72, 'Runtime connected.'],
      ['runtime.wait-ready', 0.76, 'Runtime ready.'],
    ] as const
    const marks: BootProgressMark[] = []
    let previous = projectBootProgress(
      { phase: 'runtime', state: 'loading' },
      marks,
    )

    for (const [label, expectedProgress, expectedStepLabel] of sequence) {
      marks.push(mark(label))
      const next = projectBootProgress(
        { phase: 'runtime', state: 'loading' },
        marks,
      )

      expect(next.progress, label).toBeGreaterThan(previous.progress)
      expect(next.progress, label).toBeCloseTo(expectedProgress)
      expect(next.label, label).toBe(expectedStepLabel)
      previous = next
    }
  })

  it('labels the projection with the furthest observed step', () => {
    const step = projectBootProgress({ phase: 'runtime', state: 'loading' }, [
      mark('runtime.wait-start'),
      mark('runtime.opfs-bridge-ready'),
    ])
    expect(step.label).toBe('Preparing browser storage.')
    expect(step.progress).toBeCloseTo(0.52)
  })

  it('never regresses when the status phase lags observed marks', () => {
    const marks = [mark('runtime.client-channel-acked')]
    const step = projectBootProgress(
      { phase: 'entrypoint', state: 'loading' },
      marks,
    )
    expect(step.progress).toBeCloseTo(0.64)
    expect(step.label).toBe('Runtime channel connected.')
  })

  it('maps the entrypoint download fraction into its ladder window', () => {
    const start = projectBootProgress({
      phase: 'entrypoint',
      state: 'loading',
      progress: 0,
    })
    const half = projectBootProgress({
      phase: 'entrypoint',
      state: 'loading',
      progress: 0.5,
    })
    const full = projectBootProgress({
      phase: 'entrypoint',
      state: 'loading',
      progress: 1,
    })
    expect(start.progress).toBeCloseTo(0.08)
    expect(half.progress).toBeCloseTo(0.17)
    expect(full.progress).toBeCloseTo(0.26)
    expect(half.label).toBe('Downloading the application.')
  })

  it('retains partial entrypoint progress when the download errors', () => {
    const step = projectBootProgress(
      { phase: 'entrypoint-error', state: 'error' },
      [mark('boot-status.entrypoint', { progress: 0.4 })],
    )

    expect(step.progress).toBeCloseTo(0.152)
    expect(step.label).toBe('Downloading the application.')
  })

  it('maps the app frame download fraction into the frame window', () => {
    const step = projectBootProgress({
      phase: 'app',
      state: 'loading',
      progress: 0.5,
    })
    expect(step.progress).toBeCloseTo(0.89)
  })

  it('ignores marks from non-startup WebViews', () => {
    const step = projectBootProgress({ phase: 'runtime', state: 'loading' }, [
      mark('webview.revealed', {
        webViewId: 'nested-view',
        startupRelevant: false,
      }),
    ])
    expect(step.progress).toBeCloseTo(0.3)
  })

  it('completes on the startup WebView reveal', () => {
    const step = projectBootProgress({ phase: 'app', state: 'loading' }, [
      mark('webview.revealed', {
        webViewId: 'startup-view',
        startupRelevant: true,
      }),
    ])
    expect(step.progress).toBe(1)
    expect(step.label).toBe('Spacewave is ready.')
  })

  it('keeps accumulated progress on error and unknown marks', () => {
    const step = projectBootProgress(
      { phase: 'runtime-error', state: 'error' },
      [mark('runtime.worker-created'), mark('runtime.some-unknown-mark')],
    )
    expect(step.progress).toBeCloseTo(0.48)
    expect(step.label).toBe('Runtime worker started.')
  })
})
