import { Window } from 'happy-dom'
import React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { act, cleanup, render } from '@testing-library/react'
import type { CommandState } from '@s4wave/sdk/command/registry/registry.pb.js'

import { CommandPalette } from './CommandPalette.js'

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

let mockCommands: CommandState[] = []
let paletteHandler: (() => void) | undefined
const mockInvokeCommand = vi.fn()
const mockGetSubItems = vi.fn()
const mockRegisterOpenCommand = vi.fn(() => () => {})

vi.mock('./CommandContext.js', () => ({
  useCommands: () => mockCommands,
  useInvokeCommand: () => mockInvokeCommand,
  useCommandContext: () => ({
    getSubItems: mockGetSubItems,
    registerOpenCommand: mockRegisterOpenCommand,
  }),
}))

vi.mock('./useCommand.js', () => ({
  useCommand: (opts: { commandId: string; handler: () => void }) => {
    if (opts.commandId === 'spacewave.view.palette')
      paletteHandler = opts.handler
  },
}))

vi.mock('@s4wave/web/ui/command.js', () => ({
  CommandDialog: ({
    open,
    children,
  }: {
    open: boolean
    children: React.ReactNode
  }) => (open ? <div role="dialog">{children}</div> : null),
  CommandEmpty: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  CommandFooter: () => <div />,
  CommandGroup: ({
    heading,
    children,
  }: {
    heading: string
    children: React.ReactNode
  }) => (
    <section>
      <h3>{heading}</h3>
      {children}
    </section>
  ),
  CommandInput: () => <input aria-label="Command search" />,
  CommandItem: ({
    children,
    disabled,
    onSelect,
  }: {
    children: React.ReactNode
    disabled?: boolean
    onSelect?: () => void
  }) => (
    <button disabled={disabled} onClick={onSelect} type="button">
      {children}
    </button>
  ),
  CommandList: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  CommandShortcut: ({ children }: { children: React.ReactNode }) => (
    <span>{children}</span>
  ),
}))

describe('CommandPalette', () => {
  afterEach(() => {
    cleanup()
    mockCommands = []
    paletteHandler = undefined
    vi.clearAllMocks()
  })

  it('shows legacy keybinding display through the resolver migration path', () => {
    mockCommands = [
      {
        command: {
          commandId: 'spacewave.view.help',
          label: 'Open Help',
          keybinding: 'Ctrl+H',
          menuPath: 'Help/Open Help',
        },
        active: true,
        enabled: true,
      },
    ]

    const view = render(<CommandPalette />)
    act(() => paletteHandler?.())

    expect(view.getByRole('dialog')).toBeTruthy()
    expect(view.getByText('Open Help')).toBeTruthy()
    expect(view.getByText('Ctrl+H')).toBeTruthy()
  })

  it('shows plural typed default bindings on command rows', () => {
    mockCommands = [
      {
        command: {
          commandId: 'spacewave.file.open',
          label: 'Open File',
          menuPath: 'File/Open File',
          defaultBindings: [
            {
              id: 'open-combo',
              binding: { case: 'combo', value: { combo: 'Ctrl+O' } },
            },
            {
              id: 'open-sequence',
              binding: {
                case: 'sequence',
                value: { steps: ['Leader', 'F', 'O'] },
              },
            },
          ],
        },
        active: true,
        enabled: true,
      },
    ]

    const view = render(<CommandPalette />)
    act(() => paletteHandler?.())

    expect(view.getByText('Open File')).toBeTruthy()
    expect(view.getByText('Ctrl+O / Leader F O')).toBeTruthy()
  })
})
