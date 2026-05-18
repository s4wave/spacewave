import { describe, expect, it } from 'vitest'

import {
  buildBrowserRuntimeState,
  projectBrowserRuntimeStartupPhase,
  type BrowserRuntimeStartupMark,
} from './browser-runtime-state.js'

function mark(
  label: string,
  sequence: number,
  detail: Record<string, unknown> = {},
): BrowserRuntimeStartupMark {
  return {
    label,
    sequence,
    detail: {
      label,
      sequence,
      ...detail,
    },
  }
}

describe('browser runtime state', () => {
  it('projects runtime, service worker, plugin, and frame marks into typed state', () => {
    const state = buildBrowserRuntimeState(
      {
        phase: 'runtime',
        detail: 'Connecting runtime...',
        state: 'loading',
      },
      [
        mark('service-worker.control-ready', 1),
        mark('runtime.client-channel-acked', 2),
        mark('worker.ready', 3),
        mark('webview.component-ready', 4, {
          webViewId: 'startup-view',
          startupRelevant: true,
        }),
      ],
    )

    expect(state.serviceWorker.state).toBe('controlled')
    expect(state.runtimeClient.state).toBe('connected')
    expect(state.pluginGeneration.state).toBe('frontend-ready')
    expect(state.frame.state).toBe('component-ready')
    expect(projectBrowserRuntimeStartupPhase(state)).toBe('frame')
  })

  it('keeps delayed runtime startup before frame readiness', () => {
    const state = buildBrowserRuntimeState(
      {
        phase: 'entrypoint',
        detail: 'Loading application entrypoint...',
        state: 'loading',
      },
      [mark('runtime.wait-start', 1)],
    )

    expect(state.runtimeClient.state).toBe('opening')
    expect(projectBrowserRuntimeStartupPhase(state)).toBe('runtime')
  })

  it('keeps retained resume state current without regressing startup phase', () => {
    const state = buildBrowserRuntimeState(
      {
        phase: 'ready',
        detail: 'Application ready.',
        state: 'loading',
      },
      [
        mark('web-document.resume-ready', 1),
        mark('web-document.resume-not-ready', 2, {
          reason: 'visibility hidden',
        }),
      ],
    )

    expect(state.document.state).toBe('hidden')
    expect(state.runtimeClient.state).toBe('connected')
    expect(projectBrowserRuntimeStartupPhase(state)).toBe('runtime')
  })

  it('projects terminal plugin failure as runtime failure state', () => {
    const state = buildBrowserRuntimeState(
      {
        phase: 'runtime',
        detail: 'Starting plugins...',
        state: 'loading',
      },
      [
        mark('worker.ready', 1),
        mark('plugin.terminal-failure', 2, {
          reason: 'spacewave-core exited',
        }),
      ],
    )

    expect(state.pluginGeneration.state).toBe('terminal-failure')
    expect(state.terminalFailure).toEqual({
      owner: 'plugin-generation',
      phase: 'runtime',
      detail: 'spacewave-core exited',
    })
    expect(projectBrowserRuntimeStartupPhase(state)).toBe('runtime')
  })

  it('does not let unrelated WebView marks complete startup', () => {
    const state = buildBrowserRuntimeState(
      {
        phase: 'runtime',
        detail: 'Starting runtime...',
        state: 'loading',
      },
      [
        mark('webview.revealed', 1, {
          webViewId: 'nested-view',
          startupRelevant: false,
        }),
      ],
    )

    expect(state.frame.state).toBe('idle')
    expect(projectBrowserRuntimeStartupPhase(state)).toBe('runtime')
  })
})
