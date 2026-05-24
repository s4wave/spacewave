import { describe, expect, it } from 'vitest'

import { projectBrowserStartupView } from './browser-startup.js'
import { projectBrowserStartup } from './browser-startup-model.js'

describe('projectBrowserStartupView', () => {
  it('maps browser startup phases to the user-facing startup rail', () => {
    expect(
      projectBrowserStartupView({
        phase: 'runtime',
        detail: 'Connecting runtime...',
        state: 'loading',
      }),
    ).toEqual({
      state: 'loading',
      title: 'Spacewave',
      detail: 'Runtime: Starting the Spacewave runtime.',
      progress: 0.58,
    })
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
      title: 'Spacewave',
      detail:
        'App: Downloading the app bundle. This can take a while the first time.',
      progress: 0.84,
      progressIndeterminate: true,
    })
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
      title: 'Spacewave',
      detail: 'Done: Spacewave is ready.',
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
      title: 'Spacewave',
      detail: 'Runtime: Starting the Spacewave runtime.',
      progress: 0.58,
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
      title: 'Spacewave',
      detail: 'Prepare: Preparing browser files.',
      progress: 0.08,
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
      title: 'Spacewave',
      detail: 'Runtime: Starting the Spacewave runtime.',
      progress: 0.58,
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
