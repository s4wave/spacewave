import { describe, it, expect, afterEach } from 'vitest'
import { render, screen, cleanup } from '@testing-library/react'
import { PanelHeader } from './PanelHeader.js'

describe('PanelHeader', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders children', () => {
    render(
      <PanelHeader>
        <span>Header content</span>
      </PanelHeader>,
    )
    expect(screen.getByText('Header content')).toBeTruthy()
  })

  it('sets default height of 25', () => {
    render(
      <PanelHeader>
        <span>Content</span>
      </PanelHeader>,
    )
    const container = screen.getByText('Content').parentElement
    expect(container?.style.getPropertyValue('--panel-header-height')).toBe(
      '25px',
    )
  })

  it('sets custom height', () => {
    render(
      <PanelHeader height={40}>
        <span>Content</span>
      </PanelHeader>,
    )
    const container = screen.getByText('Content').parentElement
    expect(container?.style.getPropertyValue('--panel-header-height')).toBe(
      '40px',
    )
  })

  it('applies custom className', () => {
    render(
      <PanelHeader className="mt-1">
        <span>Content</span>
      </PanelHeader>,
    )
    const container = screen.getByText('Content').parentElement
    expect(container?.className).toContain('mt-1')
  })
})
