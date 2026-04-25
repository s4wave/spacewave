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
    const handleClick = vi.fn()
    render(<NavigationLink text="About" onClick={handleClick} />)
    fireEvent.click(screen.getByRole('link'))
    expect(handleClick).toHaveBeenCalledOnce()
  })

  it('fires onClick on Space keydown', () => {
    const handleClick = vi.fn()
    render(<NavigationLink text="Nav" onClick={handleClick} />)
    fireEvent.keyDown(screen.getByRole('link'), { code: 'Space' })
    expect(handleClick).toHaveBeenCalledOnce()
  })

  it('fires onClick on Enter keydown', () => {
    const handleClick = vi.fn()
    render(<NavigationLink text="Nav" onClick={handleClick} />)
    fireEvent.keyDown(screen.getByRole('link'), { code: 'Enter' })
    expect(handleClick).toHaveBeenCalledOnce()
  })

  it('prevents default on click', () => {
    const handleClick = vi.fn()
    render(<NavigationLink text="Link" onClick={handleClick} />)
    const link = screen.getByRole('link')
    const event = new MouseEvent('click', { bubbles: true, cancelable: true })
    const preventDefaultSpy = vi.spyOn(event, 'preventDefault')
    link.dispatchEvent(event)
    expect(preventDefaultSpy).toHaveBeenCalled()
  })

  it('applies custom className to the anchor', () => {
    render(<NavigationLink text="Styled" className="nav-custom" />)
    const link = screen.getByRole('link')
    expect(link.classList.contains('nav-custom')).toBe(true)
  })

  it('applies custom textClassName to the text span', () => {
    render(<NavigationLink text="Colored" textClassName="text-red" />)
    const textSpan = screen.getByText('Colored')
    expect(textSpan.classList.contains('text-red')).toBe(true)
  })

  it('does not fire onClick on unrelated keydown', () => {
    const handleClick = vi.fn()
    render(<NavigationLink text="Nav" onClick={handleClick} />)
    fireEvent.keyDown(screen.getByRole('link'), { code: 'Tab' })
    expect(handleClick).not.toHaveBeenCalled()
  })
})
