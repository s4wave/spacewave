import { describe, it, expect, afterEach } from 'vitest'
import { render, screen, cleanup } from '@testing-library/react'
import { MenuButtonGroup } from './MenuButtonGroup.js'

describe('MenuButtonGroup', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders children', () => {
    render(
      <MenuButtonGroup>
        <span>Child content</span>
      </MenuButtonGroup>,
    )
    expect(screen.getByText('Child content')).toBeTruthy()
  })

  it('applies custom className', () => {
    render(
      <MenuButtonGroup className="custom-class">
        <span>Content</span>
      </MenuButtonGroup>,
    )
    const container = screen.getByText('Content').parentElement
    expect(container?.className).toContain('custom-class')
  })

  it('renders multiple children', () => {
    render(
      <MenuButtonGroup>
        <span>First</span>
        <span>Second</span>
        <span>Third</span>
      </MenuButtonGroup>,
    )
    expect(screen.getByText('First')).toBeTruthy()
    expect(screen.getByText('Second')).toBeTruthy()
    expect(screen.getByText('Third')).toBeTruthy()
  })
})
