import React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'

import { ShellMenuBar } from './ShellMenuBar.js'

const mockUseCommands = vi.fn()
const mockInvokeCommand = vi.fn()
const mockOpenCommand = vi.fn()

vi.mock('@s4wave/web/style/utils.js', () => ({
  cn: (...classes: Array<string | false | null | undefined>) =>
    classes.filter(Boolean).join(' '),
}))

vi.mock('@s4wave/web/images/AppLogo.js', () => ({
  AppLogo: ({ className }: { className?: string }) => (
    <div className={className}>logo</div>
  ),
}))

vi.mock('@s4wave/web/command/index.js', () => ({
  useCommands: () => mockUseCommands(),
  useInvokeCommand: () => mockInvokeCommand,
  useOpenCommand: () => mockOpenCommand,
}))

vi.mock('@s4wave/web/command/CommandPalette.js', () => ({
  formatKeybinding: (binding: string) => binding,
}))

vi.mock('@s4wave/web/ui/Menubar.js', () => ({
  Menubar: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  MenubarContent: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  MenubarItem: ({
    children,
    onSelect,
    disabled,
  }: {
    children: React.ReactNode
    onSelect?: () => void
    disabled?: boolean
  }) => (
    <button disabled={disabled} onClick={onSelect} type="button">
      {children}
    </button>
  ),
  MenubarMenu: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  MenubarSeparator: () => <hr />,
  MenubarShortcut: ({ children }: { children: React.ReactNode }) => (
    <span>{children}</span>
  ),
  MenubarSub: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  MenubarSubContent: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  MenubarSubTrigger: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  MenubarTrigger: ({
    children,
  }: {
    children: React.ReactNode
    asChild?: boolean
  }) => <div>{children}</div>,
}))

describe('ShellMenuBar', () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('opens the palette for menu commands with sub-items', () => {
    mockUseCommands.mockReturnValue([
      {
        command: {
          commandId: 'spacewave.nav.go-to-space',
          label: 'Go to Space',
          menuPath: 'View/Go to Space',
          hasSubItems: true,
        },
        active: true,
        enabled: true,
      },
    ])

    render(<ShellMenuBar />)

    fireEvent.click(screen.getByRole('button', { name: 'Go to Space' }))

    expect(mockOpenCommand).toHaveBeenCalledWith('spacewave.nav.go-to-space')
    expect(mockInvokeCommand).not.toHaveBeenCalled()
  })

  it('invokes regular menu commands directly', () => {
    mockUseCommands.mockReturnValue([
      {
        command: {
          commandId: 'spacewave.nav.home',
          label: 'Go Home',
          menuPath: 'View/Go Home',
        },
        active: true,
        enabled: true,
      },
    ])

    render(<ShellMenuBar />)

    fireEvent.click(screen.getByRole('button', { name: 'Go Home' }))

    expect(mockInvokeCommand).toHaveBeenCalledWith('spacewave.nav.home')
    expect(mockOpenCommand).not.toHaveBeenCalled()
  })
})
