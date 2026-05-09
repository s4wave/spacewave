import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { BackButton } from './BackButton.js'

describe('BackButton', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders the label', () => {
    render(<BackButton>Back</BackButton>)
    expect(screen.getByText('Back')).toBeDefined()
  })

  it('fires onClick handler', async () => {
    const user = userEvent.setup()
    const handleBackSelect = vi.fn()
    render(<BackButton onClick={handleBackSelect}>Home</BackButton>)
    await user.click(screen.getByRole('button'))
    expect(handleBackSelect).toHaveBeenCalledOnce()
  })

  it('adds floating positioning classes', () => {
    render(<BackButton floating>Sessions</BackButton>)
    const button = screen.getByRole('button')
    expect(button.classList.contains('absolute')).toBe(true)
    expect(button.classList.contains('top-4')).toBe(true)
    expect(button.classList.contains('left-4')).toBe(true)
  })
})
