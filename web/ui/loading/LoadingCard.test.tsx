import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'

import { LoadingCard } from './LoadingCard.js'

describe('LoadingCard', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders the title and detail', () => {
    render(
      <LoadingCard
        view={{
          state: 'loading',
          title: 'Checking sync status',
          detail: 'Waiting for the session sync watcher.',
        }}
      />,
    )
    expect(screen.getByText('Checking sync status')).toBeTruthy()
    expect(
      screen.getByText('Waiting for the session sync watcher.'),
    ).toBeTruthy()
  })

  it('applies the active state icon treatment', () => {
    const { container } = render(
      <LoadingCard view={{ state: 'active', title: 'Uploading changes' }} />,
    )
    const iconBox = container.querySelector('div.bg-brand\\/10')
    expect(iconBox).toBeTruthy()
  })

  it('applies the synced state icon treatment', () => {
    const { container } = render(
      <LoadingCard view={{ state: 'synced', title: 'Synced' }} />,
    )
    // Synced pairs a cloud icon with a brand-colored check.
    expect(container.querySelector('.text-brand')).toBeTruthy()
  })

  it('renders the error box when error is set', () => {
    render(
      <LoadingCard
        view={{
          state: 'error',
          title: 'Sync needs attention',
          error: 'WebSocket closed (code 1006).',
        }}
      />,
    )
    expect(screen.getByText('WebSocket closed (code 1006).')).toBeTruthy()
  })

  it('renders a determinate progress bar when progress is set', () => {
    const { container } = render(
      <LoadingCard
        view={{
          state: 'active',
          title: 'Cloning repository',
          progress: 0.62,
          rate: { down: '4.8 MiB/s' },
        }}
      />,
    )
    const fill = container.querySelector('div.bg-brand') as HTMLElement | null
    expect(fill?.style.width).toBe('62%')
    expect(screen.getByText('4.8 MiB/s')).toBeTruthy()
  })

  it('renders rate pills when progress is absent but rate is present', () => {
    render(
      <LoadingCard
        view={{
          state: 'active',
          title: 'Uploading',
          rate: { up: '1.5 MiB/s', down: '24 KiB/s' },
        }}
      />,
    )
    expect(screen.getByText('1.5 MiB/s')).toBeTruthy()
    expect(screen.getByText('24 KiB/s')).toBeTruthy()
  })

  it('invokes retry and cancel callbacks when their buttons are clicked', () => {
    const onRetry = vi.fn()
    const onCancel = vi.fn()
    render(
      <LoadingCard
        view={{
          state: 'error',
          title: 'Sync failed',
          onRetry,
          onCancel,
        }}
      />,
    )
    fireEvent.click(screen.getByText('Retry'))
    fireEvent.click(screen.getByText('Cancel'))
    expect(onRetry).toHaveBeenCalledOnce()
    expect(onCancel).toHaveBeenCalledOnce()
  })

  it('omits retry / cancel buttons when callbacks are not supplied', () => {
    render(<LoadingCard view={{ state: 'loading', title: 'Loading' }} />)
    expect(screen.queryByText('Retry')).toBeNull()
    expect(screen.queryByText('Cancel')).toBeNull()
  })

  it('renders the last-activity footer when provided', () => {
    render(
      <LoadingCard
        view={{
          state: 'synced',
          title: 'Synced',
          lastActivity: 'Last activity 5 min ago',
        }}
      />,
    )
    expect(screen.getByText('Last activity 5 min ago')).toBeTruthy()
  })
})
