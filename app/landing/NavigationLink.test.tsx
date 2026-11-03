import * as stylex from '@stylexjs/stylex'
import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, cleanup, fireEvent } from '@testing-library/react'
import { NavigationLink } from './NavigationLink.js'

const overrides = stylex.create({
  button: { marginTop: 4 },
  text: { color: 'rgb(150, 30, 60)' },
})

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

  it('applies native styles to the button', () => {
    render(<NavigationLink text="Styled" sx={overrides.button} />)
    const button = screen.getByRole('button')
    expect(
      stylex
        .props(overrides.button)
        .className!.split(' ')
        .every((name) => button.classList.contains(name)),
    ).toBe(true)
  })

  it('applies native text styles to the text span', () => {
    render(<NavigationLink text="Colored" textSx={overrides.text} />)
    const textSpan = screen.getByText('Colored')
    expect(
      stylex
        .props(overrides.text)
        .className!.split(' ')
        .every((name) => textSpan.classList.contains(name)),
    ).toBe(true)
  })

  it('does not fire onClick on unrelated keydown', () => {
    const handleNavigationSelect = vi.fn()
    render(<NavigationLink text="Nav" onClick={handleNavigationSelect} />)
    fireEvent.keyDown(screen.getByRole('button'), { code: 'Tab' })
    expect(handleNavigationSelect).not.toHaveBeenCalled()
  })
})
