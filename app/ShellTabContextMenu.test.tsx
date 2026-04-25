import React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'

import { ShellTabContextMenu } from './ShellTabContextMenu.js'

vi.mock('@s4wave/web/ui/DropdownMenu.js', () => ({
  DropdownMenu: ({
    children,
    open,
  }: {
    children: React.ReactNode
    open?: boolean
  }) =>
    open === false ? null : (
      <div data-testid={open ? 'context-menu' : undefined}>{children}</div>
    ),
  DropdownMenuTrigger: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
  DropdownMenuContent: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  DropdownMenuItem: ({
    children,
    onClick,
    disabled,
  }: {
    children: React.ReactNode
    onClick?: () => void
    disabled?: boolean
  }) => (
    <button disabled={disabled} onClick={onClick} type="button">
      {children}
    </button>
  ),
  DropdownMenuSeparator: () => <hr />,
}))

describe('ShellTabContextMenu', () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('routes every action through the clicked tab id', () => {
    const onClose = vi.fn()
    const onNewTab = vi.fn()
    const onRenameTab = vi.fn()
    const onDuplicateTab = vi.fn()
    const onPopoutTab = vi.fn()
    const onCloseOtherTabs = vi.fn()
    const onCloseTab = vi.fn()

    render(
      <ShellTabContextMenu
        state={{ tabId: 'tab-7', x: 10, y: 20 }}
        canCloseTabs={true}
        onClose={onClose}
        onNewTab={onNewTab}
        onRenameTab={onRenameTab}
        onDuplicateTab={onDuplicateTab}
        onPopoutTab={onPopoutTab}
        onCloseOtherTabs={onCloseOtherTabs}
        onCloseTab={onCloseTab}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: /^new tab$/i }))
    fireEvent.click(screen.getByRole('button', { name: /rename tab/i }))
    fireEvent.click(screen.getByRole('button', { name: /duplicate tab/i }))
    fireEvent.click(screen.getByRole('button', { name: /open in new tab/i }))
    fireEvent.click(screen.getByRole('button', { name: /close other tabs/i }))
    fireEvent.click(screen.getByRole('button', { name: /^close tab$/i }))

    expect(onNewTab).toHaveBeenCalledWith('tab-7')
    expect(onRenameTab).toHaveBeenCalledWith('tab-7')
    expect(onDuplicateTab).toHaveBeenCalledWith('tab-7')
    expect(onPopoutTab).toHaveBeenCalledWith('tab-7')
    expect(onCloseOtherTabs).toHaveBeenCalledWith('tab-7')
    expect(onCloseTab).toHaveBeenCalledWith('tab-7')
    expect(onClose).not.toHaveBeenCalled()
  })

  it('disables destructive close actions when only one tab remains', () => {
    render(
      <ShellTabContextMenu
        state={{ tabId: 'tab-1', x: 10, y: 20 }}
        canCloseTabs={false}
        onClose={vi.fn()}
        onNewTab={vi.fn()}
        onRenameTab={vi.fn()}
        onDuplicateTab={vi.fn()}
        onPopoutTab={vi.fn()}
        onCloseOtherTabs={vi.fn()}
        onCloseTab={vi.fn()}
      />,
    )

    const closeOtherTabsButton = screen.getByRole('button', {
      name: /close other tabs/i,
    })
    const closeTabButton = screen.getByRole('button', {
      name: /^close tab$/i,
    })

    if (!(closeOtherTabsButton instanceof HTMLButtonElement)) {
      throw new Error('expected close other tabs button')
    }
    if (!(closeTabButton instanceof HTMLButtonElement)) {
      throw new Error('expected close tab button')
    }

    expect(closeOtherTabsButton.disabled).toBe(true)
    expect(closeTabButton.disabled).toBe(true)
  })
})
