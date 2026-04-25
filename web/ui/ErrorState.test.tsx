import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, cleanup } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ErrorState } from './ErrorState.js'

describe('ErrorState', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders with default title "Error"', () => {
    render(<ErrorState message="Something went wrong" />)
    expect(screen.getByText('Error')).toBeDefined()
  })

  it('renders custom title', () => {
    render(<ErrorState title="Connection Failed" message="Cannot connect" />)
    expect(screen.getByText('Connection Failed')).toBeDefined()
  })

  it('renders message text', () => {
    render(<ErrorState message="Something went wrong" />)
    expect(screen.getByText('Something went wrong')).toBeDefined()
  })

  it('shows Retry button when onRetry is provided', () => {
    const handleRetry = vi.fn()
    render(<ErrorState message="Failed" onRetry={handleRetry} />)
    expect(screen.getByText('Retry')).toBeDefined()
  })

  it('calls onRetry when Retry button is clicked', async () => {
    const user = userEvent.setup()
    const handleRetry = vi.fn()
    render(<ErrorState message="Failed" onRetry={handleRetry} />)

    await user.click(screen.getByText('Retry'))
    expect(handleRetry).toHaveBeenCalledOnce()
  })

  it('does not show Retry button when onRetry is not provided', () => {
    render(<ErrorState message="Failed" />)
    expect(screen.queryByText('Retry')).toBeNull()
  })

  it('renders card variant by default', () => {
    const { container } = render(<ErrorState message="Failed" />)
    // Card variant uses p-6
    expect(container.firstElementChild?.classList.contains('p-6')).toBe(true)
  })

  it('renders inline variant', () => {
    const { container } = render(
      <ErrorState message="Failed" variant="inline" />,
    )
    // Inline variant uses flex-row layout with items-center and gap-3
    expect(container.firstElementChild?.classList.contains('p-3')).toBe(true)
  })

  it('renders fullscreen variant', () => {
    const { container } = render(
      <ErrorState message="Failed" variant="fullscreen" />,
    )
    expect(
      container.firstElementChild?.classList.contains('min-h-screen'),
    ).toBe(true)
  })

  it('applies custom className', () => {
    const { container } = render(
      <ErrorState message="Failed" className="my-custom-class" />,
    )
    expect(
      container.firstElementChild?.classList.contains('my-custom-class'),
    ).toBe(true)
  })
})
