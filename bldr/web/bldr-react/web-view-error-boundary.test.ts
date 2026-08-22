import React from 'react'
import { act, cleanup, fireEvent, render } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { WebViewErrorBoundary } from './web-view-error-boundary.js'
import { WebViewRootAssetLoadError } from './web-view-module-loader.js'

function ThrowError({ error }: { error: Error }): React.ReactElement | null {
  throw error
}

describe('WebViewErrorBoundary module load diagnostics', () => {
  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
  })

  it('reports root plugin asset fetch results separately', () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})

    const error = new WebViewRootAssetLoadError({
      scriptPath: '/b/pa/spacewave-app/v/app/App-old.mjs',
      status: 404,
      ok: false,
      fetchSource: 'plugin-assets',
      runtimeError: 'plugin-asset-missing',
      pluginAssetResult: 'missing',
      contentType: 'text/plain',
      classification: 'missing',
      bodyPrefix: 'missing',
    })

    const rendered = render(
      React.createElement(
        WebViewErrorBoundary,
        {},
        React.createElement(ThrowError, { error }),
      ),
    )

    expect(rendered.container.textContent).toContain(
      'Failed to load plugin asset',
    )
    expect(rendered.container.textContent).toContain(
      '/b/pa/spacewave-app/v/app/App-old.mjs 404 missing source=plugin-assets runtime=plugin-asset-missing plugin=missing',
    )
  })

  it('keeps nested dynamic import failures on the module diagnostic path', () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})

    const error = new Error(
      'Failed to fetch dynamically imported module: /b/pa/spacewave-app/v/chunk-missing.mjs',
    )

    const rendered = render(
      React.createElement(
        WebViewErrorBoundary,
        {},
        React.createElement(ThrowError, { error }),
      ),
    )

    expect(rendered.container.textContent).toContain('Failed to load module')
    expect(rendered.container.textContent).toContain(
      '/b/pa/spacewave-app/v/chunk-missing.mjs',
    )
    expect(rendered.container.textContent).not.toContain(
      'Failed to load plugin asset',
    )
  })

  it('notifies before manually retrying recoverable root asset failures', () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    vi.spyOn(console, 'log').mockImplementation(() => {})

    const onRecoverableRetry = vi.fn()
    const error = new WebViewRootAssetLoadError({
      scriptPath: '/b/pa/spacewave-app/v/app/App-bypass.mjs',
      status: 200,
      ok: true,
      classification: 'bypass',
    })

    const rendered = render(
      React.createElement(
        WebViewErrorBoundary,
        { onRecoverableRetry },
        React.createElement(ThrowError, { error }),
      ),
    )

    fireEvent.click(rendered.getByText('Retry now'))

    expect(onRecoverableRetry).toHaveBeenCalledOnce()
  })

  it('recovers a root-changed 409 classification through the bounded retry path', async () => {
    vi.useFakeTimers()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    vi.spyOn(console, 'log').mockImplementation(() => {})

    const onRecoverableRetry = vi.fn()
    const error = new WebViewRootAssetLoadError({
      scriptPath: '/b/pd/spacewave-app/backend.mjs',
      status: 409,
      ok: false,
      fetchSource: 'plugin-dist',
      runtimeError: 'plugin-root-changed',
      pluginAssetResult: 'root-changed',
      contentType: 'application/json',
      classification: 'root-changed',
      bodyPrefix: '{"code":"plugin-root-changed"',
    })

    const rendered = render(
      React.createElement(
        WebViewErrorBoundary,
        { onRecoverableRetry },
        React.createElement(ThrowError, { error }),
      ),
    )

    // The classified failure enters the established exponential-backoff
    // recovery instead of the manual-only diagnostic path.
    expect(rendered.container.textContent).toContain('Retrying in 2s…')
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000)
    })
    expect(rendered.container.textContent).toContain('Retrying in 1s…')
    expect(onRecoverableRetry).not.toHaveBeenCalled()

    // The bounded recovery retries exactly once, then backoff escalates for
    // any repeat failure instead of retrying immediately.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000)
    })
    expect(onRecoverableRetry).toHaveBeenCalledOnce()
    expect(rendered.container.textContent).toContain('Retrying in 4s…')
  })
})
