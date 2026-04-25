import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, cleanup } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MenuButton } from './MenuButton.js'

describe('MenuButton', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders children text', () => {
    render(<MenuButton>Click me</MenuButton>)
    expect(screen.getByText('Click me')).toBeTruthy()
  })

  it('calls onClick when clicked', async () => {
    const user = userEvent.setup()
    const onClick = vi.fn()
    render(<MenuButton onClick={onClick}>Click me</MenuButton>)

    const button = screen.getByRole('button', { name: 'Click me' })
    await user.click(button)

    expect(onClick).toHaveBeenCalledOnce()
  })

  it('renders without onClick (no crash)', () => {
    render(<MenuButton>No handler</MenuButton>)
    const button = screen.getByRole('button', { name: 'No handler' })
    expect(button).toBeTruthy()
  })
})
