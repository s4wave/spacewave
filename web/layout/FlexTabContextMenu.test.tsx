import React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'

import { FlexTabContextMenu } from './FlexTabContextMenu.js'

vi.mock('@s4wave/web/ui/DropdownMenu.js', () => ({
  DropdownMenu: ({
    children,
    open,
    onOpenChange,
  }: {
    children: React.ReactNode
    open?: boolean
    onOpenChange?: (open: boolean) => void
  }) =>
    open === false ? null : (
      <div data-testid="context-menu" data-open={String(open)}>
        {children}
        <button type="button" onClick={() => onOpenChange?.(false)}>
          dismiss
        </button>
      </div>
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
    variant,
  }: {
    children: React.ReactNode
    onClick?: () => void
    disabled?: boolean
    variant?: string
  }) => (
    <button
      type="button"
      disabled={disabled}
      data-variant={variant ?? ''}
      onClick={onClick}
    >
      {children}
    </button>
  ),
  DropdownMenuSeparator: () => <hr role="separator" />,
}))

vi.mock('@s4wave/web/ui/DropdownMenuGhostAnchor.js', () => ({
  DropdownMenuGhostAnchor: ({ x, y }: { x: number; y: number }) => (
    <span data-testid="ghost-anchor" data-x={x} data-y={y} />
  ),
}))

describe('FlexTabContextMenu', () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('renders positioned tab actions and routes the clicked tab id', () => {
    const rename = vi.fn()
    const close = vi.fn()

    render(
      <FlexTabContextMenu
        state={{ tabId: 'tab-7', x: 12, y: 34 }}
        onClose={vi.fn()}
        items={[
          {
            id: 'rename',
            label: 'Rename Tab',
            icon: <span aria-hidden="true">r</span>,
            onSelect: rename,
          },
          { id: 'sep', type: 'separator' },
          {
            id: 'close',
            label: 'Close Tab',
            variant: 'destructive',
            onSelect: close,
          },
        ]}
      />,
    )

    expect(screen.getByTestId('ghost-anchor').getAttribute('data-x')).toBe('12')
    expect(screen.getByTestId('ghost-anchor').getAttribute('data-y')).toBe('34')
    expect(screen.getAllByRole('separator')).toHaveLength(1)

    fireEvent.click(screen.getByRole('button', { name: /rename tab/i }))
    fireEvent.click(screen.getByRole('button', { name: /close tab/i }))

    expect(rename).toHaveBeenCalledWith('tab-7')
    expect(close).toHaveBeenCalledWith('tab-7')
    expect(
      screen
        .getByRole('button', { name: /close tab/i })
        .getAttribute('data-variant'),
    ).toBe('destructive')
  })

  it('closes on dropdown dismissal and respects disabled items', () => {
    const onClose = vi.fn()
    const disabled = vi.fn()

    render(
      <FlexTabContextMenu
        state={{ tabId: 'tab-1', x: 0, y: 0 }}
        onClose={onClose}
        items={[
          {
            id: 'disabled',
            label: 'Close Tab',
            disabled: true,
            onSelect: disabled,
          },
        ]}
      />,
    )

    const closeButton = screen.getByRole('button', { name: /close tab/i })
    if (!(closeButton instanceof HTMLButtonElement)) {
      throw new Error('expected close button')
    }

    expect(closeButton.disabled).toBe(true)
    fireEvent.click(closeButton)
    fireEvent.click(screen.getByRole('button', { name: /dismiss/i }))

    expect(disabled).not.toHaveBeenCalled()
    expect(onClose).toHaveBeenCalledOnce()
  })
})
