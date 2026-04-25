import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, cleanup } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { EmptyState } from './EmptyState.js'

describe('EmptyState', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders title', () => {
    render(<EmptyState title="No results" />)
    expect(screen.getByText('No results')).toBeDefined()
  })

  it('renders description when provided', () => {
    render(<EmptyState title="Empty" description="Try adding something" />)
    expect(screen.getByText('Try adding something')).toBeDefined()
  })

  it('does not render description when not provided', () => {
    render(<EmptyState title="Empty" />)
    expect(screen.queryByText('Try adding something')).toBeNull()
  })

  it('renders action button when action is provided', () => {
    const handleClick = vi.fn()
    render(
      <EmptyState
        title="Empty"
        action={{ label: 'Add Item', onClick: handleClick }}
      />,
    )
    expect(screen.getByText('Add Item')).toBeDefined()
  })

  it('calls action.onClick when button is clicked', async () => {
    const user = userEvent.setup()
    const handleClick = vi.fn()
    render(
      <EmptyState
        title="Empty"
        action={{ label: 'Add Item', onClick: handleClick }}
      />,
    )

    await user.click(screen.getByText('Add Item'))
    expect(handleClick).toHaveBeenCalledOnce()
  })

  it('does not render button when no action', () => {
    render(<EmptyState title="Empty" />)
    expect(screen.queryByRole('button')).toBeNull()
  })

  it('renders custom icon when provided', () => {
    render(
      <EmptyState
        title="Empty"
        icon={<span data-testid="custom-icon">icon</span>}
      />,
    )
    expect(screen.getByTestId('custom-icon')).toBeDefined()
  })

  it('renders default icon (folder) when no icon given', () => {
    const { container } = render(<EmptyState title="Empty" />)
    // The default icon is LuFolderOpen which renders as an SVG
    const svg = container.querySelector('svg')
    expect(svg).toBeTruthy()
  })

  it('applies custom className', () => {
    const { container } = render(
      <EmptyState title="Empty" className="my-custom-class" />,
    )
    expect(
      container.firstElementChild?.classList.contains('my-custom-class'),
    ).toBe(true)
  })

  it('renders compact variant without error', () => {
    const { container } = render(
      <EmptyState title="Compact" variant="compact" />,
    )
    expect(screen.getByText('Compact')).toBeDefined()
    // Compact variant uses p-4 instead of p-8
    expect(container.firstElementChild?.classList.contains('p-4')).toBe(true)
  })
})
