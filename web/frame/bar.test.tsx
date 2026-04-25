import { describe, it, expect, afterEach } from 'vitest'
import { render, screen, cleanup } from '@testing-library/react'
import { Bar } from './bar.js'

describe('Bar', () => {
  afterEach(() => {
    cleanup()
  })

  it('returns null when hidden is true', () => {
    const { container } = render(<Bar hidden />)
    expect(container.innerHTML).toBe('')
  })

  it('renders left content', () => {
    render(<Bar left={<span>Left side</span>} />)
    expect(screen.getByText('Left side')).toBeDefined()
  })

  it('renders right content', () => {
    render(<Bar right={<span>Right side</span>} />)
    expect(screen.getByText('Right side')).toBeDefined()
  })

  it('renders both left and right content', () => {
    render(
      <Bar
        left={<span>Left content</span>}
        right={<span>Right content</span>}
      />,
    )
    expect(screen.getByText('Left content')).toBeDefined()
    expect(screen.getByText('Right content')).toBeDefined()
  })

  it('applies custom className', () => {
    const { container } = render(<Bar className="my-bar" />)
    const div = container.firstElementChild as HTMLElement
    expect(div.className).toContain('my-bar')
  })

  it('applies custom style', () => {
    const { container } = render(<Bar style={{ height: '50px' }} />)
    const div = container.firstElementChild as HTMLElement
    expect(div.style.height).toBe('50px')
  })

  it('shows top border by default (span element present)', () => {
    const { container } = render(<Bar />)
    const span = container.querySelector('span')
    expect(span).toBeTruthy()
  })

  it('hides top border when hideTopBorder is true (span element absent)', () => {
    const { container } = render(<Bar hideTopBorder />)
    const span = container.querySelector('span')
    expect(span).toBeFalsy()
  })
})
