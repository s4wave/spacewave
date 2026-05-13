import { describe, expect, it } from 'vitest'

import { projectBrowserStartupView } from './browser-startup.js'

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
      detail: 'Frame: Opening the app frame.',
      progress: 0.84,
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
})
