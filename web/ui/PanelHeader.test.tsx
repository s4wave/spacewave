import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, cleanup } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { PanelHeader, PanelHeaderButton } from './PanelHeader.js'
import {
  ObjectViewerProvider,
  type ObjectViewerContextValue,
} from '@s4wave/web/object/ObjectViewerContext.js'
import type { ObjectViewerComponent } from '@s4wave/web/object/object.js'

function makeComponent(name: string): ObjectViewerComponent {
  return {
    typeID: 'test-type',
    name,
    component: () => null,
  }
}

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
    expect(container?.style.height).toBe('25px')
  })

  it('sets custom height', () => {
    render(
      <PanelHeader height={40}>
        <span>Content</span>
      </PanelHeader>,
    )
    const container = screen.getByText('Content').parentElement
    expect(container?.style.height).toBe('40px')
  })

  it('applies custom className', () => {
    render(
      <PanelHeader className="my-custom-class">
        <span>Content</span>
      </PanelHeader>,
    )
    const container = screen.getByText('Content').parentElement
    expect(container?.className).toContain('my-custom-class')
  })

  it('renders without context and shows no selector', () => {
    render(
      <PanelHeader>
        <span>Content</span>
      </PanelHeader>,
    )
    expect(screen.getByText('Content')).toBeTruthy()
    expect(screen.queryByText('Properties')).toBeFalsy()
  })

  it('renders with context and multiple components showing selector', () => {
    const components = [makeComponent('Properties'), makeComponent('Raw Data')]
    const ctx: ObjectViewerContextValue = {
      visibleComponents: components,
      selectedComponent: components[0],
      onSelectComponent: vi.fn(),
    }
    render(
      <ObjectViewerProvider value={ctx}>
        <PanelHeader>
          <span>Content</span>
        </PanelHeader>
      </ObjectViewerProvider>,
    )
    expect(screen.getByText('Properties')).toBeTruthy()
    expect(screen.getByText('Content')).toBeTruthy()
  })

  it('renders with context and single component showing static name', () => {
    const components = [makeComponent('Properties')]
    const ctx: ObjectViewerContextValue = {
      visibleComponents: components,
      selectedComponent: components[0],
      onSelectComponent: vi.fn(),
    }
    render(
      <ObjectViewerProvider value={ctx}>
        <PanelHeader>
          <span>Content</span>
        </PanelHeader>
      </ObjectViewerProvider>,
    )
    expect(screen.getByText('Properties')).toBeTruthy()
    expect(screen.getByText('Content')).toBeTruthy()
  })
})

describe('PanelHeaderButton', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders children', () => {
    render(
      <PanelHeaderButton>
        <span>Button text</span>
      </PanelHeaderButton>,
    )
    expect(screen.getByText('Button text')).toBeTruthy()
  })

  it('calls onClick when clicked', async () => {
    const user = userEvent.setup()
    const onClick = vi.fn()
    render(<PanelHeaderButton onClick={onClick}>Click</PanelHeaderButton>)

    const button = screen.getByRole('button', { name: 'Click' })
    await user.click(button)

    expect(onClick).toHaveBeenCalledOnce()
  })

  it('shows title attribute', () => {
    render(<PanelHeaderButton title="Close panel">X</PanelHeaderButton>)
    const button = screen.getByRole('button', { name: 'X' })
    expect(button.getAttribute('title')).toBe('Close panel')
  })

  it('applies custom className', () => {
    render(<PanelHeaderButton className="extra-class">Btn</PanelHeaderButton>)
    const button = screen.getByRole('button', { name: 'Btn' })
    expect(button.className).toContain('extra-class')
  })
})
