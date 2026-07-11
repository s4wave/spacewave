import { describe, expect, it } from 'vitest'

import { projectBrowserStartupView } from './browser-startup.js'
import {
  browserStartupStallCopy,
  projectBrowserStartup,
} from './browser-startup-model.js'

describe('projectBrowserStartupView', () => {
  it('uses phase-specific startup titles and stall explanations', () => {
    const phaseViews = [
      [
        { phase: 'loading', detail: 'start', state: 'loading' as const },
        'Preparing Spacewave',
      ],
      [
        { phase: 'entrypoint', detail: 'start', state: 'loading' as const },
        'Connecting to your Space',
      ],
      [
        { phase: 'runtime', detail: 'start', state: 'loading' as const },
        'Starting the Spacewave runtime',
      ],
      [
        { phase: 'app', detail: 'start', state: 'loading' as const },
        'Loading the app',
      ],
    ] as const

    for (const [status, title] of phaseViews) {
      expect(projectBrowserStartupView(status).title).toBe(title)
    }
    expect(browserStartupStallCopy.frame).toBe(
      'Still loading; downloaded files are cached for the next launch.',
    )
    expect(browserStartupStallCopy.runtime).toBe(
      'The runtime is still starting. First launch may take longer.',
    )
  })

  it('maps browser startup phases to the user-facing startup rail', () => {
    expect(
      projectBrowserStartupView({
        phase: 'runtime',
        detail: 'Connecting runtime...',
        state: 'loading',
      }),
    ).toEqual({
      state: 'loading',
      title: 'Starting the Spacewave runtime',
      detail: 'Runtime initialization: Connecting the Spacewave runtime.',
      progress: 0.3,
    })
  })

  it('advances progress and the stage label on runtime startup marks', () => {
    const before = projectBrowserStartupView(
      { phase: 'runtime', detail: 'Starting...', state: 'loading' },
      [
        {
          name: 'spacewave.startup.runtime.worker-created',
          label: 'runtime.worker-created',
          sequence: 1,
          detail: { label: 'runtime.worker-created', sequence: 1 },
        },
      ],
    )
    const after = projectBrowserStartupView(
      { phase: 'runtime', detail: 'Starting...', state: 'loading' },
      [
        {
          name: 'spacewave.startup.runtime.worker-created',
          label: 'runtime.worker-created',
          sequence: 1,
          detail: { label: 'runtime.worker-created', sequence: 1 },
        },
        {
          name: 'spacewave.startup.runtime.client-channel-acked',
          label: 'runtime.client-channel-acked',
          sequence: 2,
          detail: { label: 'runtime.client-channel-acked', sequence: 2 },
        },
      ],
    )

    expect(before.detail).toBe(
      'Runtime initialization: Runtime worker started.',
    )
    expect(after.detail).toBe(
      'Runtime initialization: Runtime channel connected.',
    )
    expect(after.progress ?? 0).toBeGreaterThan(before.progress ?? 0)
  })

  it('uses selected startup marks to advance the projection', () => {
    expect(
      projectBrowserStartupView(
        {
          phase: 'runtime',
          detail: 'Starting...',
          state: 'loading',
        },
        [
          {
            name: 'spacewave.startup.shell.boot-requested',
            label: 'shell.boot-requested',
            sequence: 1,
            detail: { label: 'shell.boot-requested', sequence: 1 },
          },
        ],
      ),
    ).toEqual({
      state: 'loading',
      title: 'Loading the app',
      detail: 'Current app download: Opening the application.',
      progress: 0.8,
    })
  })

  it('maps app bundle download progress into the frame ladder window', () => {
    const view = projectBrowserStartupView({
      phase: 'app',
      detail: 'Downloading app bundle...',
      state: 'loading',
      progress: 0.5,
    })

    expect(view.state).toBe('loading')
    expect(view.detail).toBe('Current app download: Opening the application.')
    expect(view.progress).toBeCloseTo(0.89)
  })

  it('keeps mark-accumulated progress when the download fraction is invalid', () => {
    const view = projectBrowserStartupView({
      phase: 'app',
      detail: 'Downloading app bundle...',
      state: 'loading',
      progress: Number.NaN,
    })

    expect(view.progress).toBeCloseTo(0.8)
    expect(view).not.toHaveProperty('progressIndeterminate')
  })

  it('uses only startup-relevant WebView marks to advance the projection', () => {
    expect(
      projectBrowserStartupView(
        {
          phase: 'runtime',
          detail: 'Starting...',
          state: 'loading',
        },
        [
          {
            name: 'spacewave.startup.webview.revealed',
            label: 'webview.revealed',
            sequence: 1,
            detail: {
              label: 'webview.revealed',
              sequence: 1,
              source: 'webview',
              webViewId: 'nested-view',
              parentWebViewId: 'startup-view',
              startupRelevant: false,
            },
          },
          {
            name: 'spacewave.startup.webview.revealed',
            label: 'webview.revealed',
            sequence: 2,
            detail: {
              label: 'webview.revealed',
              sequence: 2,
              source: 'webview',
              webViewId: 'startup-view',
              startupRelevant: true,
            },
          },
        ],
      ),
    ).toEqual({
      state: 'synced',
      title: 'Opening your Space',
      detail: 'Startup complete. Rendering the first view…',
      progress: 1,
    })
  })

  it('ignores WebView marks from non-startup views', () => {
    expect(
      projectBrowserStartupView(
        {
          phase: 'runtime',
          detail: 'Starting...',
          state: 'loading',
        },
        [
          {
            name: 'spacewave.startup.webview.revealed',
            label: 'webview.revealed',
            sequence: 1,
            detail: {
              label: 'webview.revealed',
              sequence: 1,
              source: 'webview',
              webViewId: 'later-view',
              startupRelevant: false,
            },
          },
        ],
      ),
    ).toEqual({
      state: 'loading',
      title: 'Starting the Spacewave runtime',
      detail: 'Runtime initialization: Connecting the Spacewave runtime.',
      progress: 0.3,
    })
  })

  it('keeps raw boot error details out of user-facing startup copy', () => {
    expect(
      projectBrowserStartupView({
        phase: 'manifest-error',
        detail: 'failed to load browser release manifest: 500',
        state: 'error',
      }),
    ).toEqual({
      state: 'error',
      title: 'Preparing Spacewave',
      detail: 'Local initialization: Loading the app shell.',
      progress: 0.02,
      error:
        'Startup did not finish. Check the browser console or startup marks for details.',
    })
  })

  it('keeps error projections on the failing boot phase even when later marks exist', () => {
    expect(
      projectBrowserStartupView(
        {
          phase: 'runtime-error',
          detail: 'runtime failed',
          state: 'error',
        },
        [
          {
            name: 'spacewave.startup.webview.loading-surface-mounted',
            label: 'webview.loading-surface-mounted',
            sequence: 1,
            detail: {
              label: 'webview.loading-surface-mounted',
              sequence: 1,
              source: 'app',
            },
          },
        ],
      ),
    ).toEqual({
      state: 'error',
      title: 'Starting the Spacewave runtime',
      detail: 'Runtime initialization: Loading the application interface.',
      progress: 0.82,
      error:
        'Startup did not finish. Check the browser console or startup marks for details.',
    })
  })

  it('exposes typed runtime state as projection evidence', () => {
    const startup = projectBrowserStartup(
      {
        phase: 'runtime',
        detail: 'Starting...',
        state: 'loading',
      },
      [
        {
          name: 'spacewave.startup.runtime.client-channel-acked',
          label: 'runtime.client-channel-acked',
          sequence: 1,
          detail: { label: 'runtime.client-channel-acked', sequence: 1 },
        },
        {
          name: 'spacewave.startup.service-worker.control-ready',
          label: 'service-worker.control-ready',
          sequence: 2,
          detail: { label: 'service-worker.control-ready', sequence: 2 },
        },
      ],
    )

    expect(startup.evidence.runtime.runtimeClient.state).toBe('connected')
    expect(startup.evidence.runtime.serviceWorker.state).toBe('controlled')
    expect(startup.phase.id).toBe('runtime')
  })

  it('marks plugin terminal failure as a runtime phase error', () => {
    const startup = projectBrowserStartup(
      {
        phase: 'runtime',
        detail: 'Starting...',
        state: 'loading',
      },
      [
        {
          name: 'spacewave.startup.plugin.terminal-failure',
          label: 'plugin.terminal-failure',
          sequence: 1,
          detail: {
            label: 'plugin.terminal-failure',
            sequence: 1,
            reason: 'spacewave-core exited',
          },
        },
      ],
    )

    expect(startup.view.state).toBe('error')
    expect(startup.phase.id).toBe('runtime')
    expect(startup.phases.find((phase) => phase.id === 'runtime')?.state).toBe(
      'error',
    )
    expect(startup.evidence.runtime.terminalFailure?.owner).toBe(
      'plugin-generation',
    )
  })
})
