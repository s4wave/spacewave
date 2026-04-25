import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, waitFor, cleanup } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { TopBar } from './TopBar.js'

describe('TopBar', () => {
  const mockWorkspaces = [
    { id: 'layout', name: 'Layout' },
    { id: 'modeling', name: 'Modeling' },
    { id: 'sculpting', name: 'Sculpting' },
  ]

  afterEach(() => {
    cleanup()
  })

  it('should render workspace tabs', () => {
    const onWorkspaceChange = vi.fn()
    render(
      <TopBar
        activeWorkspace="layout"
        workspaces={mockWorkspaces}
        onWorkspaceChange={onWorkspaceChange}
      />,
    )

    expect(screen.getByText('Layout')).toBeTruthy()
    expect(screen.getByText('Modeling')).toBeTruthy()
    expect(screen.getByText('Sculpting')).toBeTruthy()
  })

  it('should call onWorkspaceChange when tab is clicked', async () => {
    const user = userEvent.setup()
    const onWorkspaceChange = vi.fn()
    render(
      <TopBar
        activeWorkspace="layout"
        workspaces={mockWorkspaces}
        onWorkspaceChange={onWorkspaceChange}
      />,
    )

    await user.click(screen.getByText('Modeling'))

    expect(onWorkspaceChange).toHaveBeenCalledWith('modeling')
  })

  it('should visually distinguish the active workspace tab', () => {
    const onWorkspaceChange = vi.fn()
    render(
      <TopBar
        activeWorkspace="modeling"
        workspaces={mockWorkspaces}
        onWorkspaceChange={onWorkspaceChange}
      />,
    )

    // The active tab wrapper gets an inline boxShadow style; inactive tabs do not
    const modelingButton = screen.getByText('Modeling').closest('button')
    const layoutButton = screen.getByText('Layout').closest('button')

    const activeTabWrapper = modelingButton?.parentElement
    const inactiveTabWrapper = layoutButton?.parentElement

    // Active tab has a boxShadow style applied
    expect(activeTabWrapper?.style.boxShadow).toBeTruthy()
    // Inactive tab does not
    expect(inactiveTabWrapper?.style.boxShadow).toBeFalsy()
  })

  it('should render menu buttons', () => {
    const onWorkspaceChange = vi.fn()
    render(
      <TopBar
        activeWorkspace="layout"
        workspaces={mockWorkspaces}
        onWorkspaceChange={onWorkspaceChange}
      />,
    )

    expect(screen.getByText('File')).toBeTruthy()
    expect(screen.getByText('Edit')).toBeTruthy()
    expect(screen.getByText('View')).toBeTruthy()
    expect(screen.getByText('Tools')).toBeTruthy()
    expect(screen.getByText('Help')).toBeTruthy()
  })

  it('should render + button when onWorkspaceAdd is provided', () => {
    const onWorkspaceChange = vi.fn()
    const onWorkspaceAdd = vi.fn()
    render(
      <TopBar
        activeWorkspace="layout"
        workspaces={mockWorkspaces}
        onWorkspaceChange={onWorkspaceChange}
        onWorkspaceAdd={onWorkspaceAdd}
      />,
    )

    expect(screen.getByTitle('Add workspace')).toBeTruthy()
  })

  it('should show scroll buttons when content overflows', async () => {
    const onWorkspaceChange = vi.fn()
    const { container } = render(
      <TopBar
        activeWorkspace="layout"
        workspaces={mockWorkspaces}
        onWorkspaceChange={onWorkspaceChange}
      />,
    )

    const scrollContainer = container.querySelector('.hide-scrollbar')
    expect(scrollContainer).toBeTruthy()

    Object.defineProperty(scrollContainer, 'scrollWidth', {
      configurable: true,
      value: 500,
    })
    Object.defineProperty(scrollContainer, 'clientWidth', {
      configurable: true,
      value: 300,
    })
    Object.defineProperty(scrollContainer, 'scrollLeft', {
      configurable: true,
      value: 0,
    })

    scrollContainer?.dispatchEvent(new Event('scroll'))

    await waitFor(() => {
      const rightArrow = screen.queryByTitle('Scroll right')
      expect(rightArrow).toBeTruthy()
    })
  })

  it('should update arrow visibility immediately when scroll button is clicked', async () => {
    const user = userEvent.setup()
    const onWorkspaceChange = vi.fn()
    const { container } = render(
      <TopBar
        activeWorkspace="layout"
        workspaces={mockWorkspaces}
        onWorkspaceChange={onWorkspaceChange}
      />,
    )

    const scrollContainer = container.querySelector('.hide-scrollbar')
    expect(scrollContainer).toBeTruthy()

    Object.defineProperty(scrollContainer, 'scrollWidth', {
      configurable: true,
      value: 500,
    })
    Object.defineProperty(scrollContainer, 'clientWidth', {
      configurable: true,
      value: 300,
    })
    Object.defineProperty(scrollContainer, 'scrollLeft', {
      configurable: true,
      writable: true,
      value: 150,
    })

    const mockScrollTo = vi.fn()
    scrollContainer!.scrollTo = mockScrollTo

    scrollContainer?.dispatchEvent(new Event('scroll'))

    await waitFor(() => {
      expect(screen.queryByTitle('Scroll left')).toBeTruthy()
    })

    const leftArrow = screen.getByTitle('Scroll left')
    await user.click(leftArrow)

    expect(mockScrollTo).toHaveBeenCalledWith({
      left: 0,
      behavior: 'smooth',
    })

    await waitFor(() => {
      expect(screen.queryByTitle('Scroll left')).toBeFalsy()
    })
  })

  it('should place + button outside scroll container when arrows are visible', async () => {
    const onWorkspaceChange = vi.fn()
    const onWorkspaceAdd = vi.fn()
    const { container } = render(
      <TopBar
        activeWorkspace="layout"
        workspaces={mockWorkspaces}
        onWorkspaceChange={onWorkspaceChange}
        onWorkspaceAdd={onWorkspaceAdd}
      />,
    )

    const scrollContainer = container.querySelector('.hide-scrollbar')
    expect(scrollContainer).toBeTruthy()

    Object.defineProperty(scrollContainer, 'scrollWidth', {
      configurable: true,
      value: 500,
    })
    Object.defineProperty(scrollContainer, 'clientWidth', {
      configurable: true,
      value: 300,
    })
    Object.defineProperty(scrollContainer, 'scrollLeft', {
      configurable: true,
      value: 150,
    })

    scrollContainer?.dispatchEvent(new Event('scroll'))

    await waitFor(() => {
      const plusButton = screen.getByTitle('Add workspace')
      // When arrows are visible, the + button is rendered as a sibling
      // of the scroll container, not inside it
      expect(plusButton.parentElement).not.toBe(scrollContainer)
    })
  })

  it('should place + button inside scroll container when no arrows are visible', () => {
    const onWorkspaceChange = vi.fn()
    const onWorkspaceAdd = vi.fn()
    const { container } = render(
      <TopBar
        activeWorkspace="layout"
        workspaces={mockWorkspaces}
        onWorkspaceChange={onWorkspaceChange}
        onWorkspaceAdd={onWorkspaceAdd}
      />,
    )

    const plusButton = screen.getByTitle('Add workspace')
    const scrollContainer = container.querySelector('.hide-scrollbar')

    expect(plusButton.parentElement).toBe(scrollContainer)
  })
})
