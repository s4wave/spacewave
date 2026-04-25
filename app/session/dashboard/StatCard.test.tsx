import { describe, it, expect, afterEach } from 'vitest'
import { render, screen, cleanup } from '@testing-library/react'
import { StatCard } from './StatCard.js'

describe('StatCard', () => {
  afterEach(() => {
    cleanup()
  })

  // A simple test icon component that accepts className like the real icons do
  function TestIcon({ className }: { className?: string }) {
    return <svg data-testid="test-icon" className={className} />
  }

  it('renders label text', () => {
    render(<StatCard icon={TestIcon} label="Connections" value="42" />)
    expect(screen.getByText('Connections')).toBeDefined()
  })

  it('renders a string value', () => {
    render(<StatCard icon={TestIcon} label="Status" value="Active" />)
    expect(screen.getByText('Active')).toBeDefined()
  })

  it('renders a numeric value', () => {
    render(<StatCard icon={TestIcon} label="Count" value={128} />)
    expect(screen.getByText('128')).toBeDefined()
  })

  it('renders the icon component', () => {
    render(<StatCard icon={TestIcon} label="Metric" value="10" />)
    expect(screen.getByTestId('test-icon')).toBeDefined()
  })

  it('passes className to the icon component', () => {
    render(<StatCard icon={TestIcon} label="Metric" value="5" />)
    const icon = screen.getByTestId('test-icon')
    expect(icon.getAttribute('class')).toContain('text-brand')
  })
})
