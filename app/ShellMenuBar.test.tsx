import { Window } from 'happy-dom'
import React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render } from '@testing-library/react'
import { CommandBindingKind } from '@s4wave/sdk/command/command.pb.js'

import { ShellMenuBar } from './ShellMenuBar.js'

if (typeof document === 'undefined') {
  const happyDomWindow = new Window({ url: 'http://localhost/' })

  Object.defineProperties(globalThis, {
    window: { value: happyDomWindow, configurable: true },
    document: { value: happyDomWindow.document, configurable: true },
    HTMLElement: { value: happyDomWindow.HTMLElement, configurable: true },
    Element: { value: happyDomWindow.Element, configurable: true },
    Node: { value: happyDomWindow.Node, configurable: true },
    Text: { value: happyDomWindow.Text, configurable: true },
    DocumentFragment: {
      value: happyDomWindow.DocumentFragment,
      configurable: true,
    },
    SVGElement: { value: happyDomWindow.SVGElement, configurable: true },
    Event: { value: happyDomWindow.Event, configurable: true },
    CustomEvent: { value: happyDomWindow.CustomEvent, configurable: true },
    KeyboardEvent: { value: happyDomWindow.KeyboardEvent, configurable: true },
    MouseEvent: { value: happyDomWindow.MouseEvent, configurable: true },
    FocusEvent: { value: happyDomWindow.FocusEvent, configurable: true },
    InputEvent: { value: happyDomWindow.InputEvent, configurable: true },
    MutationObserver: {
      value: happyDomWindow.MutationObserver,
      configurable: true,
    },
  })
}

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
  useCommands: () => {
    const commands: unknown = mockUseCommands()
    return commands
  },
  useInvokeCommand: () => mockInvokeCommand,
  useOpenCommand: () => mockOpenCommand,
}))

vi.mock('@s4wave/web/command/CommandPalette.js', () => ({
  formatKeybindingHint: (bindings: string[]) => bindings.join(' / '),
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

    const view = render(<ShellMenuBar />)

    fireEvent.click(view.getByRole('button', { name: 'Go to Space' }))

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

    const view = render(<ShellMenuBar />)

    fireEvent.click(view.getByRole('button', { name: 'Go Home' }))

    expect(mockInvokeCommand).toHaveBeenCalledWith('spacewave.nav.home')
    expect(mockOpenCommand).not.toHaveBeenCalled()
  })

  it('shows effective typed binding hints for menu commands', () => {
    mockUseCommands.mockReturnValue([
      {
        command: {
          commandId: 'spacewave.nav.home',
          label: 'Go Home',
          menuPath: 'View/Go Home',
          defaultBindings: [
            {
              id: 'home-combo',
              kind: CommandBindingKind.COMBO,
              combo: { combo: 'Ctrl+H' },
            },
            {
              id: 'home-sequence',
              kind: CommandBindingKind.SEQUENCE,
              sequence: { steps: ['Leader', 'H'] },
            },
          ],
        },
        active: true,
        enabled: true,
      },
    ])

    const view = render(<ShellMenuBar />)

    expect(view.getByText('Ctrl+H / Leader H')).toBeTruthy()
  })
})
