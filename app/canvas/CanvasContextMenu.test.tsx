import React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'

import { CanvasContextMenu } from './CanvasContextMenu.js'

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
    onSelect,
    disabled,
  }: {
    children: React.ReactNode
    onClick?: () => void
    onSelect?: () => void
    disabled?: boolean
  }) => (
    <button
      disabled={disabled}
      onClick={() => {
        onSelect?.()
        onClick?.()
      }}
      type="button"
    >
      {children}
    </button>
  ),
  DropdownMenuSeparator: () => <hr />,
  DropdownMenuShortcut: ({ children }: { children: React.ReactNode }) => (
    <span>{children}</span>
  ),
}))

describe('CanvasContextMenu', () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('routes each menu item to the supplied callback', () => {
    const onClose = vi.fn()
    const onPaste = vi.fn()
    const onAddText = vi.fn()
    const onAddObject = vi.fn()
    const onFitView = vi.fn()
    const onZoomReset = vi.fn()
    const onSelectAll = vi.fn()

    render(
      <CanvasContextMenu
        state={{ position: { x: 10, y: 20 } }}
        canAddObject={true}
        onClose={onClose}
        onPaste={onPaste}
        onAddText={onAddText}
        onAddObject={onAddObject}
        onFitView={onFitView}
        onZoomReset={onZoomReset}
        onSelectAll={onSelectAll}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: /paste/i }))
    fireEvent.click(screen.getByRole('button', { name: /add text node here/i }))
    fireEvent.click(
      screen.getByRole('button', { name: /add object to canvas/i }),
    )
    fireEvent.click(screen.getByRole('button', { name: /^fit view/i }))
    fireEvent.click(screen.getByRole('button', { name: /zoom to 100%/i }))
    fireEvent.click(screen.getByRole('button', { name: /^select all/i }))

    expect(onPaste).toHaveBeenCalledOnce()
    expect(onAddText).toHaveBeenCalledOnce()
    expect(onAddObject).toHaveBeenCalledOnce()
    expect(onFitView).toHaveBeenCalledOnce()
    expect(onZoomReset).toHaveBeenCalledOnce()
    expect(onSelectAll).toHaveBeenCalledOnce()
    expect(onClose).not.toHaveBeenCalled()
  })

  it('disables add object when object insertion is unavailable', () => {
    render(
      <CanvasContextMenu
        state={{ position: { x: 10, y: 20 } }}
        canAddObject={false}
        onClose={vi.fn()}
        onPaste={vi.fn()}
        onAddText={vi.fn()}
        onAddObject={vi.fn()}
        onFitView={vi.fn()}
        onZoomReset={vi.fn()}
        onSelectAll={vi.fn()}
      />,
    )

    const addObjectButton = screen.getByRole('button', {
      name: /add object to canvas/i,
    })
    if (!(addObjectButton instanceof HTMLButtonElement)) {
      throw new Error('expected add object button')
    }

    expect(addObjectButton.disabled).toBe(true)
  })
})
