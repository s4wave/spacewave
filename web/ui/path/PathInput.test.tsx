import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, waitFor, cleanup } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { PathInput } from './PathInput.js'

describe('PathInput', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders root path with home icon', () => {
    render(<PathInput path="/" />)
    const rootButton = screen.getByLabelText('Navigate to root')
    expect(rootButton).toBeTruthy()
  })

  it('renders path segments as buttons', () => {
    render(<PathInput path="/Users/testuser/Documents" />)
    expect(screen.getByText('Users')).toBeTruthy()
    expect(screen.getByText('testuser')).toBeTruthy()
    expect(screen.getByText('Documents')).toBeTruthy()
  })

  it('calls onNavigate when root button is clicked', async () => {
    const user = userEvent.setup()
    const onNavigate = vi.fn()
    render(<PathInput path="/Users/testuser" onNavigate={onNavigate} />)

    const rootButton = screen.getByLabelText('Navigate to root')
    await user.click(rootButton)

    expect(onNavigate).toHaveBeenCalledWith('/')
  })

  it('calls onNavigate with correct path when segment is clicked', async () => {
    const user = userEvent.setup()
    const onNavigate = vi.fn()
    render(
      <PathInput path="/Users/testuser/Documents" onNavigate={onNavigate} />,
    )

    const usersButton = screen.getByText('Users')
    await user.click(usersButton)

    expect(onNavigate).toHaveBeenCalledWith('/Users')
  })

  it('calls onNavigate with full path when last segment is clicked', async () => {
    const user = userEvent.setup()
    const onNavigate = vi.fn()
    render(
      <PathInput path="/Users/testuser/Documents" onNavigate={onNavigate} />,
    )

    const documentsButton = screen.getByText('Documents')
    await user.click(documentsButton)

    expect(onNavigate).toHaveBeenCalledWith('/Users/testuser/Documents')
  })

  it('enters edit mode when container is clicked', async () => {
    const user = userEvent.setup()
    render(<PathInput path="/Users/testuser" />)

    const container = screen.getByRole('button', { name: 'File path' })
    await user.click(container)

    const input = screen.getByRole('textbox')
    if (!(input instanceof HTMLInputElement)) {
      throw new Error('Expected textbox to be an HTMLInputElement')
    }
    expect(input).toBeTruthy()
    expect(input.value).toBe('/Users/testuser')
  })

  it('enters edit mode when Enter key is pressed on container', async () => {
    const user = userEvent.setup()
    render(<PathInput path="/Users/testuser" />)

    const container = screen.getByRole('button', { name: 'File path' })
    container.focus()
    await user.keyboard('{Enter}')

    await waitFor(() => {
      const input = screen.getByRole('textbox')
      expect(input).toBeTruthy()
    })
  })

  it('calls onPathChange when Enter is pressed in edit mode', async () => {
    const user = userEvent.setup()
    const onPathChange = vi.fn()
    render(<PathInput path="/Users/testuser" onPathChange={onPathChange} />)

    const container = screen.getByRole('button', { name: 'File path' })
    await user.click(container)

    const input = screen.getByRole('textbox')
    if (!(input instanceof HTMLInputElement)) {
      throw new Error('Expected textbox to be an HTMLInputElement')
    }
    await user.clear(input)
    await user.type(input, '/Users/testuser/Documents{Enter}')

    expect(onPathChange).toHaveBeenCalledWith('/Users/testuser/Documents')
  })

  it('calls onPathChange when input is blurred with changes', async () => {
    const user = userEvent.setup()
    const onPathChange = vi.fn()
    render(<PathInput path="/Users/testuser" onPathChange={onPathChange} />)

    const container = screen.getByRole('button', { name: 'File path' })
    await user.click(container)

    const input = screen.getByRole('textbox')
    if (!(input instanceof HTMLInputElement)) {
      throw new Error('Expected textbox to be an HTMLInputElement')
    }
    await user.clear(input)
    await user.type(input, '/Users/testuser/Documents')
    input.blur()

    await waitFor(() => {
      expect(onPathChange).toHaveBeenCalledWith('/Users/testuser/Documents')
    })
  })

  it('does not call onPathChange when input is blurred without changes', async () => {
    const user = userEvent.setup()
    const onPathChange = vi.fn()
    render(<PathInput path="/Users/testuser" onPathChange={onPathChange} />)

    const container = screen.getByRole('button', { name: 'File path' })
    await user.click(container)

    const input = screen.getByRole('textbox')
    if (!(input instanceof HTMLInputElement)) {
      throw new Error('Expected textbox to be an HTMLInputElement')
    }
    input.blur()

    await waitFor(() => {
      expect(onPathChange).not.toHaveBeenCalled()
    })
  })

  it('reverts changes when Escape is pressed in edit mode', async () => {
    const user = userEvent.setup()
    const onPathChange = vi.fn()
    render(<PathInput path="/Users/testuser" onPathChange={onPathChange} />)

    const container = screen.getByRole('button', { name: 'File path' })
    await user.click(container)

    const input = screen.getByRole('textbox')
    if (!(input instanceof HTMLInputElement)) {
      throw new Error('Expected textbox to be an HTMLInputElement')
    }
    await user.clear(input)
    await user.type(input, '/Users/testuser/Documents')
    await user.keyboard('{Escape}')

    await waitFor(() => {
      expect(screen.getByText('Users')).toBeTruthy()
    })

    expect(onPathChange).not.toHaveBeenCalled()
  })

  it('highlights last segment', () => {
    render(<PathInput path="/Users/testuser/Documents" />)

    const documentsButton = screen.getByText('Documents').closest('button')
    expect(documentsButton?.className).toContain('text-text-highlight')
  })

  it('highlights root when path is root', () => {
    render(<PathInput path="/" />)

    const rootButton = screen.getByLabelText('Navigate to root')
    expect(rootButton.className).toContain('text-text-highlight')
  })

  it('applies custom className', () => {
    render(<PathInput path="/Users/testuser" className="custom-class" />)

    const container = screen.getByRole('button', { name: 'File path' })
    expect(container.className).toContain('custom-class')
  })

  it('stops propagation when segment button is clicked', async () => {
    const user = userEvent.setup()
    const onNavigate = vi.fn()
    const onContainerClick = vi.fn()

    render(
      <div onClick={onContainerClick}>
        <PathInput path="/Users/testuser" onNavigate={onNavigate} />
      </div>,
    )

    const usersButton = screen.getByText('Users')
    await user.click(usersButton)

    expect(onNavigate).toHaveBeenCalledWith('/Users')
    expect(onContainerClick).not.toHaveBeenCalled()
  })

  it('filters empty path segments', () => {
    render(<PathInput path="/Users//cjs///Documents" />)

    expect(screen.getByText('Users')).toBeTruthy()
    expect(screen.getByText('cjs')).toBeTruthy()
    expect(screen.getByText('Documents')).toBeTruthy()
  })
})
