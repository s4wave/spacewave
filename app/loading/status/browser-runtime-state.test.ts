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
  it('does not classify relay failure as warm evidence', () => {
    const state = buildBrowserRuntimeState(
      {
        phase: 'runtime',
        detail: 'Connecting runtime...',
        state: 'loading',
      },
      [
        mark('dedicated-host.attach-open-failed', 1, {
          hostGeneration: 'generation-1',
          reason: 'relay unavailable',
        }),
      ],
    )

    expect(state.warmProjection).toMatchObject({
      state: 'cold',
      connection: false,
    })
  })

  it('replaces a promoted generation and rejects obsolete frame facts', () => {
    const state = buildBrowserRuntimeState(
      {
        phase: 'runtime',
        detail: 'Connecting runtime...',
        state: 'loading',
      },
      [
        mark('dedicated-host.attach-open-ready', 1, {
          hostGeneration: 'generation-1',
          connectionMode: 'dedicated-attached',
          hostDocumentId: 'host-a',
        }),
        mark('runtime.connected', 2, {
          runtimeGeneration: 'generation-1',
          connectionMode: 'dedicated-attached',
        }),
        mark('webview.neutral-frame', 3, {
          runtimeGeneration: 'generation-1',
          startupRelevant: true,
        }),
        mark('webview.revealed', 4, {
          runtimeGeneration: 'generation-1',
          startupRelevant: true,
        }),
        mark('dedicated-host.promoted', 5, {
          generation: 'generation-2',
        }),
        mark('runtime.connected', 6, {
          runtimeGeneration: 'generation-1',
          connectionMode: 'dedicated-attached',
        }),
        mark('webview.neutral-frame', 7, {
          runtimeGeneration: 'generation-1',
          startupRelevant: true,
        }),
        mark('webview.revealed', 8, {
          runtimeGeneration: 'generation-1',
          startupRelevant: true,
        }),
      ],
    )

    expect(state.warmProjection).toMatchObject({
      state: 'invalidated',
      generation: 'generation-2',
      connection: false,
      neutralFrame: false,
      finalReveal: false,
    })
    expect(state.frame.state).toBe('idle')
  })

  it('requires neutral frame before a matching generation reveal', () => {
    const state = buildBrowserRuntimeState(
      {
        phase: 'runtime',
        detail: 'Connecting runtime...',
        state: 'loading',
      },
      [
        mark('dedicated-host.attach-open-ready', 1, {
          hostGeneration: 'generation-1',
          connectionMode: 'dedicated-attached',
        }),
        mark('runtime.connected', 2, {
          runtimeGeneration: 'generation-1',
          connectionMode: 'dedicated-attached',
        }),
        mark('webview.revealed', 3, {
          runtimeGeneration: 'generation-1',
          startupRelevant: true,
        }),
        mark('webview.neutral-frame', 4, {
          runtimeGeneration: 'generation-1',
          startupRelevant: true,
        }),
        mark('webview.revealed', 5, {
          runtimeGeneration: 'generation-1',
          startupRelevant: true,
        }),
      ],
    )

    expect(state.warmProjection).toMatchObject({
      generation: 'generation-1',
      neutralFrame: true,
      finalReveal: true,
    })
    expect(state.frame.state).toBe('revealed')
  })

  it('invalidates warm state on host loss and ignores stale reconnect facts', () => {
    const state = buildBrowserRuntimeState(
      {
        phase: 'runtime',
        detail: 'Connecting runtime...',
        state: 'loading',
      },
      [
        mark('dedicated-host.attach-open-ready', 1, {
          hostGeneration: 'generation-1',
          connectionMode: 'dedicated-attached',
        }),
        mark('runtime.connected', 2, {
          runtimeGeneration: 'generation-1',
          connectionMode: 'dedicated-attached',
        }),
        mark('dedicated-host.lost', 3, {
          hostGeneration: 'generation-1',
        }),
        mark('runtime.connected', 4, {
          runtimeGeneration: 'generation-1',
          connectionMode: 'dedicated-attached',
        }),
      ],
    )

    expect(state.warmProjection).toMatchObject({
      state: 'invalidated',
      generation: 'generation-1',
      connection: false,
    })
  })

  it('ignores obsolete invalidation marks after a replacement connects', () => {
    const state = buildBrowserRuntimeState(
      {
        phase: 'runtime',
        detail: 'Connecting runtime...',
        state: 'loading',
      },
      [
        mark('dedicated-host.attach-open-ready', 1, {
          hostGeneration: 'generation-1',
          connectionMode: 'dedicated-attached',
          hostDocumentId: 'host-a',
        }),
        mark('runtime.connected', 2, {
          runtimeGeneration: 'generation-1',
          connectionMode: 'dedicated-attached',
        }),
        mark('dedicated-host.attach-open-ready', 3, {
          hostGeneration: 'generation-2',
          connectionMode: 'dedicated-attached',
          hostDocumentId: 'host-b',
        }),
        mark('runtime.connected', 4, {
          runtimeGeneration: 'generation-2',
          connectionMode: 'dedicated-attached',
        }),
        mark('dedicated-host.lost', 5, {
          hostGeneration: 'generation-1',
          hostDocumentId: 'host-a',
        }),
        mark('runtime.connection-invalidated', 6, {
          runtimeGeneration: 'generation-1',
        }),
      ],
    )

    expect(state.warmProjection).toMatchObject({
      state: 'warm',
      generation: 'generation-2',
      hostDocumentId: 'host-b',
      connection: true,
    })
    expect(state.runtimeClient.state).toBe('connected')
  })
  it('replaces warm state on reroute and runtime disconnect', () => {
    const state = buildBrowserRuntimeState(
      {
        phase: 'runtime',
        detail: 'Connecting runtime...',
        state: 'loading',
      },
      [
        mark('dedicated-host.attach-open-ready', 1, {
          hostGeneration: 'generation-1',
          connectionMode: 'dedicated-attached',
        }),
        mark('runtime.connected', 2, {
          runtimeGeneration: 'generation-1',
          connectionMode: 'dedicated-attached',
        }),
        mark('runtime.client-channel-reroute-start', 3),
        mark('webview.neutral-frame', 4, {
          runtimeGeneration: 'generation-1',
          startupRelevant: true,
        }),
        mark('runtime.connection-invalidated', 5, {
          runtimeGeneration: 'generation-1',
        }),
      ],
    )

    expect(state.warmProjection).toMatchObject({
      state: 'invalidated',
      generation: 'generation-1',
      connection: false,
      neutralFrame: false,
      finalReveal: false,
    })
    expect(state.frame.state).toBe('idle')
  })
})
