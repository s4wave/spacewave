import { describe, it, expect, afterEach } from 'vitest'
import { render, cleanup } from '@testing-library/react'
import { TopBar } from '@s4wave/app/window/TopBar.js'

// Reference implementation: TopBar tab styling
// This test documents the exact styling that ShellTabStrip tabs should match

describe('TopBar Reference Styling', () => {
  const mockWorkspaces = [
    { id: 'home', name: 'Home' },
    { id: 'space', name: 'My Space' },
  ]

  afterEach(() => {
    cleanup()
  })

  it('documents TopBar tab structure and styling', () => {
    const { container } = render(
      <TopBar
        activeWorkspace="home"
        workspaces={mockWorkspaces}
        onWorkspaceChange={() => {}}
        onWorkspaceClose={() => {}}
        onWorkspaceAdd={() => {}}
      />,
    )

    // Top bar container height set via CSS variable
    const topBarContainer = container.firstChild as HTMLElement
    expect(topBarContainer.className).toContain(
      'h-[var(--spacing-shell-header)]',
    )
    expect(topBarContainer.className).toContain('bg-topbar-back')

    // Tab container should align items to bottom
    const tabContainer = container.querySelector(
      '.flex.h-full.min-w-0.flex-1.items-end',
    )
    expect(tabContainer).toBeTruthy()
    expect(tabContainer?.className).toContain('items-end')
    expect(tabContainer?.className).toContain('px-2')
    expect(tabContainer?.className).toContain('pb-px')

    // Individual tabs should have correct styling
    const tabs = container.querySelectorAll('.group.relative')
    expect(tabs.length).toBe(2)

    const firstTab = tabs[0] as HTMLElement
    // Height
    expect(firstTab.className).toContain('h-5')
    // Border styling
    expect(firstTab.className).toContain('border-foreground/8')
    expect(firstTab.className).toContain('border')
    expect(firstTab.className).toContain('border-b-0')
    // Rounded top corners
    expect(firstTab.className).toContain('rounded-t-lg')
    // Size constraints
    expect(firstTab.className).toContain('max-w-[120px]')
    expect(firstTab.className).toContain('min-w-[30px]')
    // Shrink behavior
    expect(firstTab.className).toContain('shrink-0')

    // Active tab styling
    expect(firstTab.className).toContain('bg-shell-tab-active')
    expect(firstTab.className).toContain('text-shell-tab-text-active')

    // Inactive tab styling
    const secondTab = tabs[1] as HTMLElement
    expect(secondTab.className).toContain('bg-shell-tab-inactive')
    expect(secondTab.className).toContain('text-shell-tab-text')
    expect(secondTab.className).toContain('hover:bg-shell-tab-active/50')
  })

  it('documents TopBar close button styling', () => {
    const { container } = render(
      <TopBar
        activeWorkspace="home"
        workspaces={mockWorkspaces}
        onWorkspaceChange={() => {}}
        onWorkspaceClose={() => {}}
        onWorkspaceAdd={() => {}}
      />,
    )

    // Close button should be hidden by default, visible on hover
    const closeButtons = container.querySelectorAll('[title="Close tab"]')
    expect(closeButtons.length).toBe(2)

    const closeButton = closeButtons[0] as HTMLElement
    // Hidden by default
    expect(closeButton.className).toContain('opacity-0')
    // Visible on hover (via group-hover)
    expect(closeButton.className).toContain('group-hover:opacity-100')
    // Size
    expect(closeButton.className).toContain('h-3.5')
    expect(closeButton.className).toContain('w-3.5')
    // Margin
    expect(closeButton.className).toContain('mr-1')
    // Flex centering
    expect(closeButton.className).toContain('flex')
    expect(closeButton.className).toContain('items-center')
    expect(closeButton.className).toContain('justify-center')
  })

  it('documents TopBar add button styling', () => {
    const { container } = render(
      <TopBar
        activeWorkspace="home"
        workspaces={mockWorkspaces}
        onWorkspaceChange={() => {}}
        onWorkspaceClose={() => {}}
        onWorkspaceAdd={() => {}}
      />,
    )

    const addButton = container.querySelector(
      '[title="Add workspace"]',
    ) as HTMLElement
    expect(addButton).toBeTruthy()

    // Same height as tabs
    expect(addButton.className).toContain('h-5')
    // Border styling matching tabs
    expect(addButton.className).toContain('border-foreground/8')
    expect(addButton.className).toContain('border')
    expect(addButton.className).toContain('border-b-0')
    // Rounded top
    expect(addButton.className).toContain('rounded-t-lg')
    // Background
    expect(addButton.className).toContain('bg-shell-tab-inactive')
    expect(addButton.className).toContain('text-shell-tab-text')
    expect(addButton.className).toContain('hover:bg-shell-tab-active/50')
    // Padding
    expect(addButton.className).toContain('px-2')
  })

  it('documents TopBar tab text button styling', () => {
    const { container } = render(
      <TopBar
        activeWorkspace="home"
        workspaces={mockWorkspaces}
        onWorkspaceChange={() => {}}
        onWorkspaceClose={() => {}}
        onWorkspaceAdd={() => {}}
      />,
    )

    // The inner button that contains the tab name
    const tabButton = container.querySelector(
      '.group.relative button',
    ) as HTMLElement
    expect(tabButton).toBeTruthy()

    // Full height
    expect(tabButton.className).toContain('h-full')
    // Padding
    expect(tabButton.className).toContain('px-2')
    expect(tabButton.className).toContain('pt-0')
    expect(tabButton.className).toContain('pb-0.5')
    // Text handling
    expect(tabButton.className).toContain('overflow-hidden')
    expect(tabButton.className).toContain('text-ellipsis')
    expect(tabButton.className).toContain('whitespace-nowrap')
    // Letter spacing
    expect(tabButton.className).toContain('tracking-tight')
  })
})

// CSS Values that ShellTabStrip (FlexLayout) tabs should match:
//
// Tab container (.flexlayout__tabset_tabbar_outer_top):
// - height: 30px
// - background: bg-topbar-back
// - align-items: flex-end
// - padding-bottom: 1px
//
// Tab button (.flexlayout__tab_button):
// - height: 20px (h-5 = 1.25rem = 20px with default spacing)
// - min-width: 30px
// - max-width: 120px
// - border: 1px solid var(--color-tab-outline)
// - border-bottom: none
// - border-radius: var(--radius-editor) var(--radius-editor) 0 0
// - background (inactive): bg-tab-inactive
// - background (active): bg-tab-active
// - color (inactive): text-tab-text
// - color (active): text-tab-text-active
// - box-shadow (active): inset 0 -1px 0 var(--color-widget-emboss)
//
// Close button (.flexlayout__tab_button_trailing):
// - opacity: 0 (default), 1 (on tab hover)
// - width: 14px (w-3.5)
// - height: 14px (h-3.5)
// - margin-right: 4px (mr-1)
