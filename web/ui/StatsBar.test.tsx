import { describe, it, expect, afterEach } from 'vitest'
import { render, screen, cleanup } from '@testing-library/react'
import { StatsBar, type StatItem } from './StatsBar.js'

describe('StatsBar', () => {
  afterEach(() => {
    cleanup()
  })

  it('returns null when stats is empty', () => {
    const { container } = render(<StatsBar stats={[]} />)
    expect(container.innerHTML).toBe('')
  })

  it('renders stat labels and values', () => {
    const stats: StatItem[] = [
      { label: 'Total', value: '42' },
      { label: 'Passed', value: '38' },
    ]
    render(<StatsBar stats={stats} />)
    expect(screen.getByText('42')).toBeDefined()
    expect(screen.getByText('38')).toBeDefined()
  })

  it('applies custom valueClassName', () => {
    const stats: StatItem[] = [
      { label: 'Errors', value: '5', valueClassName: 'text-error' },
    ]
    render(<StatsBar stats={stats} />)
    const valueEl = screen.getByText('5')
    expect(valueEl.classList.contains('text-error')).toBe(true)
  })

  it('applies default text-text-primary when no valueClassName', () => {
    const stats: StatItem[] = [{ label: 'Count', value: '10' }]
    render(<StatsBar stats={stats} />)
    const valueEl = screen.getByText('10')
    expect(valueEl.classList.contains('text-foreground')).toBe(true)
  })

  it('applies custom container className', () => {
    const stats: StatItem[] = [{ label: 'Items', value: '7' }]
    const { container } = render(
      <StatsBar stats={stats} className="my-stats-bar" />,
    )
    expect(
      container.firstElementChild?.classList.contains('my-stats-bar'),
    ).toBe(true)
  })
})
