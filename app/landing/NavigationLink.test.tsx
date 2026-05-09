import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, cleanup, fireEvent } from '@testing-library/react'
import { NavigationLink } from './NavigationLink.js'

describe('NavigationLink', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders text between brackets', () => {
    render(<NavigationLink text="Home" />)
    expect(screen.getByText('[')).toBeDefined()
    expect(screen.getByText('Home')).toBeDefined()
    expect(screen.getByText(']')).toBeDefined()
  })

  it('fires onClick on click', () => {
    const handleNavigationSelect = vi.fn()
    render(<NavigationLink text="About" onClick={handleNavigationSelect} />)
    fireEvent.click(screen.getByRole('button'))
    expect(handleNavigationSelect).toHaveBeenCalledOnce()
  })

  it('applies custom className to the button', () => {
    render(<NavigationLink text="Styled" className="nav-custom" />)
    const button = screen.getByRole('button')
    expect(button.classList.contains('nav-custom')).toBe(true)
  })

  it('applies custom textClassName to the text span', () => {
    render(<NavigationLink text="Colored" textClassName="text-red" />)
    const textSpan = screen.getByText('Colored')
    expect(textSpan.classList.contains('text-red')).toBe(true)
  })

  it('does not fire onClick on unrelated keydown', () => {
    const handleNavigationSelect = vi.fn()
    render(<NavigationLink text="Nav" onClick={handleNavigationSelect} />)
    fireEvent.keyDown(screen.getByRole('button'), { code: 'Tab' })
    expect(handleNavigationSelect).not.toHaveBeenCalled()
  })
})
