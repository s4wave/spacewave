import { describe, it, expect, afterEach } from 'vitest'
import { render, screen, cleanup, fireEvent } from '@testing-library/react'
import { Window } from './Window.js'

describe('Window', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders children', () => {
    render(<Window>Window content</Window>)
    expect(screen.getByText('Window content')).toBeDefined()
  })

  it('has border-window-border class by default', () => {
    const { container } = render(<Window>Content</Window>)
    const div = container.firstElementChild as HTMLElement
    expect(div.className).toContain('border-window-border')
    expect(div.className).not.toContain('border-ui-outline-active')
  })

  it('changes to border-ui-outline-active on mouse enter', () => {
    const { container } = render(<Window>Content</Window>)
    const div = container.firstElementChild as HTMLElement

    fireEvent.mouseEnter(div)

    expect(div.className).toContain('border-ui-outline-active')
    expect(div.className).not.toContain('border-window-border')
  })

  it('reverts to border-window-border on mouse leave', () => {
    const { container } = render(<Window>Content</Window>)
    const div = container.firstElementChild as HTMLElement

    fireEvent.mouseEnter(div)
    fireEvent.mouseLeave(div)

    expect(div.className).toContain('border-window-border')
    expect(div.className).not.toContain('border-ui-outline-active')
  })

  it('passes data-area-id attribute', () => {
    const { container } = render(
      <Window data-area-id="test-area">Content</Window>,
    )
    const div = container.firstElementChild as HTMLElement
    expect(div.getAttribute('data-area-id')).toBe('test-area')
  })

  it('applies custom className', () => {
    const { container } = render(<Window className="my-window">Content</Window>)
    const div = container.firstElementChild as HTMLElement
    expect(div.className).toContain('my-window')
  })

  it('applies custom style', () => {
    const { container } = render(
      <Window style={{ width: '400px', height: '300px' }}>Content</Window>,
    )
    const div = container.firstElementChild as HTMLElement
    expect(div.style.width).toBe('400px')
    expect(div.style.height).toBe('300px')
  })
})
