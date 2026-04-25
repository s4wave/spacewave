import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { EmailSupportDialog, SUPPORT_EMAIL } from './EmailSupportDialog.js'

describe('EmailSupportDialog', () => {
  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
  })

  it('renders the support copy and mailto link', () => {
    render(<EmailSupportDialog open={true} onOpenChange={() => {}} />)

    expect(screen.getByText('Email Support')).toBeTruthy()
    expect(screen.getByText(/we'll get back to you\./i)).toBeTruthy()

    const link = screen.getByRole('link', { name: SUPPORT_EMAIL })
    expect(link.getAttribute('href')).toBe(`mailto:${SUPPORT_EMAIL}`)
  })

  it('opens email and closes the dialog', () => {
    const onOpenChange = vi.fn()
    const openSpy = vi.spyOn(window, 'open').mockImplementation(() => null)

    render(<EmailSupportDialog open={true} onOpenChange={onOpenChange} />)

    fireEvent.click(screen.getByRole('button', { name: 'Open Email' }))

    expect(openSpy).toHaveBeenCalledWith(`mailto:${SUPPORT_EMAIL}`)
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })
})
