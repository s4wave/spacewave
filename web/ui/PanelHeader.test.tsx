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
    componentID: `test.${name}`,
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
    // Mount the header with two available viewers and its current selection.
    const components = [makeComponent('Properties'), makeComponent('Raw Data')]
    const ctx: ObjectViewerContextValue = {
      visibleComponents: components,
      selectedComponent: components[0],
      onSelectComponent: vi.fn(),
      setObjectName: vi.fn(),
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

  it('opens the viewer menu below the header', async () => {
    // Mount the controlled viewer menu with its current selection.
    const user = userEvent.setup()
    const components = [makeComponent('Properties'), makeComponent('Raw Data')]
    const ctx: ObjectViewerContextValue = {
      visibleComponents: components,
      selectedComponent: components[0],
      onSelectComponent: vi.fn(),
      setObjectName: vi.fn(),
    }
    render(
      <ObjectViewerProvider value={ctx}>
        <PanelHeader />
      </ObjectViewerProvider>,
    )

    // Open the live menu from the selected viewer's trigger.
    await user.click(screen.getByRole('button', { name: 'Properties' }))

    // The menu opens below its header and offers the other viewer.
    const menu = screen.getByRole('menu')
    expect(menu.getAttribute('data-side')).toBe('bottom')
    expect(screen.getByRole('menuitem', { name: 'Raw Data' })).toBeTruthy()
  })

  it('renders with context and single component showing static name', () => {
    // A single available viewer renders its static name alongside the header content.
    const components = [makeComponent('Properties')]
    const ctx: ObjectViewerContextValue = {
      visibleComponents: components,
      selectedComponent: components[0],
      onSelectComponent: vi.fn(),
      setObjectName: vi.fn(),
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
    // Mount an action whose callback belongs to its caller.
    const user = userEvent.setup()
    const onClick = vi.fn()
    render(<PanelHeaderButton onClick={onClick}>Click</PanelHeaderButton>)

    // Activate the real header button through its ordinary click event.
    const button = screen.getByRole('button', { name: 'Click' })
    await user.click(button)

    // One activation reaches the supplied action once.
    expect(onClick).toHaveBeenCalledOnce()
  })

  it('shows title attribute', () => {
    render(<PanelHeaderButton title="Close panel">X</PanelHeaderButton>)
    const button = screen.getByRole('button', { name: 'X' })
    expect(button.getAttribute('title')).toBe('Close panel')
  })
})
