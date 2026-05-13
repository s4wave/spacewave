import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'

import { LoadingScreen } from './LoadingScreen.js'

describe('LoadingScreen', () => {
  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
  })

  it('renders retry and back actions from the shared loading view', () => {
    const onRetry = vi.fn()
    const onCancel = vi.fn()

    render(
      <LoadingScreen
        view={{
          state: 'error',
          title: 'Spacewave',
          detail: 'Connect: Connecting the app shell.',
          error: 'Startup did not finish.',
          onRetry,
          onCancel,
        }}
        showShineBorder={false}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    fireEvent.click(screen.getByRole('button', { name: 'Back' }))

    expect(onRetry).toHaveBeenCalledOnce()
    expect(onCancel).toHaveBeenCalledOnce()
  })

  it('omits actions when callbacks are absent', () => {
    render(
      <LoadingScreen
        view={{
          state: 'loading',
          title: 'Spacewave',
          detail: 'Runtime: Starting the Spacewave runtime.',
        }}
        showShineBorder={false}
      />,
    )

    expect(screen.queryByRole('button', { name: 'Retry' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Back' })).toBeNull()
  })

  it('disables the decorative shine border when reduced motion is requested', () => {
    vi.stubGlobal('matchMedia', (query: string) => ({
      matches: query === '(prefers-reduced-motion: reduce)',
      media: query,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    }))

    const { container } = render(
      <LoadingScreen
        view={{
          state: 'loading',
          title: 'Spacewave',
          detail: 'Runtime: Starting the Spacewave runtime.',
          progress: 0.58,
        }}
      />,
    )

    expect(
      container
        .querySelector('[data-sw-reduced-motion]')
        ?.getAttribute('data-sw-reduced-motion'),
    ).toBe('true')
    expect(container.querySelector('.shine-border-mask')).toBeNull()
    expect(
      screen.getByText('Runtime: Starting the Spacewave runtime.'),
    ).toBeDefined()
    expect(screen.getByText('58%')).toBeDefined()
  })
})
