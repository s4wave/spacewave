import { describe, it, expect, afterEach } from 'vitest'
import { render, screen, cleanup } from '@testing-library/react'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'

describe('InfoCard', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders title text in an h3 when title is provided', () => {
    render(
      <InfoCard icon={<span>ic</span>} title="Card Title">
        <p>content</p>
      </InfoCard>,
    )
    const heading = screen.getByRole('heading', { level: 3 })
    expect(heading.textContent).toContain('Card Title')
  })

  it('renders icon alongside title', () => {
    render(
      <InfoCard icon={<span data-testid="test-icon">ic</span>} title="Title">
        <p>body</p>
      </InfoCard>,
    )
    expect(screen.getByTestId('test-icon')).toBeDefined()
    // Icon should be inside the h3
    const heading = screen.getByRole('heading', { level: 3 })
    expect(heading.querySelector('[data-testid="test-icon"]')).toBeTruthy()
  })

  it('renders children content', () => {
    render(
      <InfoCard icon={<span>ic</span>} title="Title">
        <p>Child paragraph</p>
      </InfoCard>,
    )
    expect(screen.getByText('Child paragraph')).toBeDefined()
  })

  it('renders icon without h3 when no title but icon is provided', () => {
    render(
      <InfoCard icon={<span data-testid="solo-icon">ic</span>} title="">
        <p>body</p>
      </InfoCard>,
    )
    expect(screen.getByTestId('solo-icon')).toBeDefined()
    expect(screen.queryByRole('heading')).toBeNull()
  })

  it('renders children without heading when no icon or title', () => {
    render(
      <InfoCard>
        <p>bare content</p>
      </InfoCard>,
    )
    expect(screen.getByText('bare content')).toBeDefined()
    expect(screen.queryByRole('heading')).toBeNull()
  })

  it('renders custom content inside the card', () => {
    render(
      <InfoCard icon={<span>ic</span>} title="Custom">
        <div data-testid="custom-content">
          <span>Line 1</span>
          <span>Line 2</span>
        </div>
      </InfoCard>,
    )
    expect(screen.getByTestId('custom-content')).toBeDefined()
    expect(screen.getByText('Line 1')).toBeDefined()
    expect(screen.getByText('Line 2')).toBeDefined()
  })
})
