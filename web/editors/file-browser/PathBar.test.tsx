import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, waitFor, cleanup } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { PathBar } from './PathBar.js'

describe('PathBar', () => {
  afterEach(() => {
    cleanup()
  })

  it('should render breadcrumb segments for path', () => {
    render(<PathBar path="/Users/testuser/Documents" />)

    expect(screen.getByText('Users')).toBeTruthy()
    expect(screen.getByText('testuser')).toBeTruthy()
    expect(screen.getByText('Documents')).toBeTruthy()
  })

  it('should render only root icon for root path', () => {
    render(<PathBar path="/" />)

    const rootButton = screen.getAllByLabelText('Navigate to root')[0]
    expect(rootButton).toBeTruthy()
    expect(rootButton.querySelector('svg')).toBeTruthy()
  })

  it('should switch to edit mode on click', async () => {
    const user = userEvent.setup()
    render(<PathBar path="/Users/testuser" />)

    const container = screen.getAllByRole('button', { name: 'File path' })[0]
    await user.click(container)

    await waitFor(() => {
      const input = screen.getByRole('textbox')
      expect(input).toBeTruthy()
    })
  })

  it('should switch to edit mode on Enter key', async () => {
    const user = userEvent.setup()
    render(<PathBar path="/Users/testuser" />)

    const container = screen.getAllByRole('button', { name: 'File path' })[0]
    container.focus()
    await user.keyboard('{Enter}')

    await waitFor(() => {
      const input = screen.getByRole('textbox')
      expect(input).toBeTruthy()
    })
  })

  it('should call onNavigate when clicking breadcrumb segment', async () => {
    const user = userEvent.setup()
    const onNavigate = vi.fn()
    render(<PathBar path="/Users/testuser/Documents" onNavigate={onNavigate} />)

    const usersButton = screen.getAllByRole('button', {
      name: 'Navigate to Users',
    })[0]
    await user.click(usersButton)

    expect(onNavigate).toHaveBeenCalledWith('/Users')
  })

  it('should call onNavigate when clicking root icon', async () => {
    const user = userEvent.setup()
    const onNavigate = vi.fn()
    render(<PathBar path="/Users/testuser" onNavigate={onNavigate} />)

    const rootButton = screen.getAllByRole('button', {
      name: 'Navigate to root',
    })[0]
    await user.click(rootButton)

    expect(onNavigate).toHaveBeenCalledWith('/')
  })

  it('should call onPathChange when editing and pressing Enter', async () => {
    const user = userEvent.setup()
    const onPathChange = vi.fn()
    render(<PathBar path="/Users/testuser" onPathChange={onPathChange} />)

    const container = screen.getAllByRole('button', { name: 'File path' })[0]
    await user.click(container)

    const input = await screen.findByRole('textbox')
    await user.clear(input)
    await user.type(input, '/Users/testuser/Documents')
    await user.keyboard('{Enter}')

    await waitFor(() => {
      expect(onPathChange).toHaveBeenCalledWith('/Users/testuser/Documents')
    })
  })

  it('should revert changes when pressing Escape', async () => {
    const user = userEvent.setup()
    const onPathChange = vi.fn()
    render(<PathBar path="/Users/testuser" onPathChange={onPathChange} />)

    const container = screen.getAllByRole('button', { name: 'File path' })[0]
    await user.click(container)

    const input = await screen.findByRole('textbox')
    await user.clear(input)
    await user.type(input, '/Users/testuser/Documents')
    await user.keyboard('{Escape}')

    await waitFor(() => {
      expect(screen.getByText('Users')).toBeTruthy()
    })

    expect(onPathChange).not.toHaveBeenCalled()
  })

  it('should exit edit mode on blur', async () => {
    const user = userEvent.setup()
    render(<PathBar path="/Users/testuser" />)

    const container = screen.getAllByRole('button', { name: 'File path' })[0]
    await user.click(container)

    const input = await screen.findByRole('textbox')
    input.blur()

    await waitFor(() => {
      expect(screen.getByText('Users')).toBeTruthy()
    })
  })

  it('should highlight last segment in breadcrumbs', () => {
    render(<PathBar path="/Users/testuser/Documents" />)

    const documentsButtons = screen.getAllByRole('button', {
      name: 'Navigate to Documents',
    })
    const lastButton = documentsButtons[0]

    expect(lastButton.className).toContain('text-text-highlight')
  })

  it('should select all text when entering edit mode', async () => {
    const user = userEvent.setup()
    render(<PathBar path="/Users/testuser" />)

    const container = screen.getAllByRole('button', { name: 'File path' })[0]
    await user.click(container)

    const inputEl = await screen.findByRole('textbox')
    if (!(inputEl instanceof HTMLInputElement)) {
      throw new Error('Expected textbox to be an HTMLInputElement')
    }

    await waitFor(() => {
      expect(inputEl.selectionStart).toBe(0)
      expect(inputEl.selectionEnd).toBe(inputEl.value.length)
    })

    // Sanity-check DOM APIs work in test environment
    expect(typeof inputEl.selectionStart).toBe('number')
    expect(typeof inputEl.selectionEnd).toBe('number')
  })

  it('should not call onNavigate when clicking container in breadcrumb mode', async () => {
    const user = userEvent.setup()
    const onNavigate = vi.fn()
    render(<PathBar path="/Users/testuser" onNavigate={onNavigate} />)

    const container = screen.getAllByRole('button', { name: 'File path' })[0]
    await user.click(container)

    expect(onNavigate).not.toHaveBeenCalled()
  })

  it('should update when path prop changes', () => {
    const { rerender } = render(<PathBar path="/Users/testuser" />)

    expect(screen.getByText('Users')).toBeTruthy()
    expect(screen.getByText('testuser')).toBeTruthy()

    rerender(<PathBar path="/Users/testuser/Documents" />)

    expect(screen.getByText('Documents')).toBeTruthy()
  })

  it('should maintain consistent height in both modes', async () => {
    const user = userEvent.setup()
    const { container } = render(<PathBar path="/Users/testuser" />)

    const pathBarContainer = container.firstChild as HTMLElement
    const initialHeight = pathBarContainer.className

    expect(initialHeight).toContain('h-5')

    const clickableContainer = screen.getAllByRole('button', {
      name: 'File path',
    })[0]
    await user.click(clickableContainer)

    await waitFor(() => {
      expect(pathBarContainer.className).toContain('h-5')
    })
  })

  it('should handle empty path segments', () => {
    render(<PathBar path="/Users//cjs///Documents" />)

    expect(screen.getByText('Users')).toBeTruthy()
    expect(screen.getByText('cjs')).toBeTruthy()
    expect(screen.getByText('Documents')).toBeTruthy()
  })

  it('should call onPathChange on blur if value changed', async () => {
    const user = userEvent.setup()
    const onPathChange = vi.fn()
    render(<PathBar path="/Users/testuser" onPathChange={onPathChange} />)

    const container = screen.getAllByRole('button', { name: 'File path' })[0]
    await user.click(container)

    const input = await screen.findByRole('textbox')
    await user.clear(input)
    await user.type(input, '/Users/testuser/Documents')
    input.blur()

    await waitFor(() => {
      expect(onPathChange).toHaveBeenCalledWith('/Users/testuser/Documents')
    })
  })

  it('should not call onPathChange on blur if value unchanged', async () => {
    const user = userEvent.setup()
    const onPathChange = vi.fn()
    render(<PathBar path="/Users/testuser" onPathChange={onPathChange} />)

    const container = screen.getAllByRole('button', { name: 'File path' })[0]
    await user.click(container)

    const input = await screen.findByRole('textbox')
    input.blur()

    await waitFor(() => {
      expect(onPathChange).not.toHaveBeenCalled()
    })
  })
})
