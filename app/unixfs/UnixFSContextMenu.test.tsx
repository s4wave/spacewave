import React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'

import { UnixFSContextMenu } from './UnixFSContextMenu.js'

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
    disabled,
    onClick,
  }: {
    children: React.ReactNode
    disabled?: boolean
    onClick?: () => void
  }) => (
    <button disabled={disabled} onClick={onClick} type="button">
      {children}
    </button>
  ),
  DropdownMenuSeparator: () => <hr />,
  DropdownMenuShortcut: ({ children }: { children: React.ReactNode }) => (
    <span>{children}</span>
  ),
}))

vi.mock('@s4wave/web/ui/DropdownMenuGhostAnchor.js', () => ({
  DropdownMenuGhostAnchor: () => <span data-testid="ghost-anchor" />,
}))

describe('UnixFSContextMenu', () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('shows New folder for item context menus', () => {
    const onNewFolder = vi.fn()

    render(
      <UnixFSContextMenu
        state={{
          position: { x: 4, y: 5 },
          entry: { id: 'docs', name: 'docs', isDir: true },
          actionEntries: [{ id: 'docs', name: 'docs', isDir: true }],
          moveItems: [],
        }}
        onClose={vi.fn()}
        onNewFolder={onNewFolder}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: /new folder/i }))

    expect(onNewFolder).toHaveBeenCalledOnce()
  })
})
