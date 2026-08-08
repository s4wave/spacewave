import { CommandSurface } from '@s4wave/sdk/command/command.pb.js'
import { Window } from 'happy-dom'
import React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render } from '@testing-library/react'
import { CommandFocusContext } from '@s4wave/sdk/command/command.pb.js'

import { KeyboardShortcutsDialog } from './KeyboardShortcutsDialog.js'

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
const mockOnOpenChange = vi.fn()

vi.mock('./CommandContext.js', () => ({
  useCommands: () => mockUseCommands(),
}))

vi.mock('./CommandPalette.js', () => ({
  formatKeybindingHint: (bindings: string[]) => bindings.join(' / '),
}))

vi.mock('@s4wave/web/ui/dialog.js', () => ({
  Dialog: ({ open, children }: { open: boolean; children: React.ReactNode }) =>
    open ? <div>{children}</div> : null,
  DialogContent: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  DialogHeader: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  DialogTitle: ({ children }: { children: React.ReactNode }) => (
    <h2>{children}</h2>
  ),
}))

describe('KeyboardShortcutsDialog', () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('shows typed keybinding display through the resolver', () => {
    mockUseCommands.mockReturnValue([
      {
        command: {
          commandId: 'spacewave.help.shortcuts',
          label: 'Keyboard Shortcuts',
          menuPath: 'Help/Keyboard Shortcuts',
          defaultBindings: [
            {
              id: 'shortcuts',
              surface: CommandSurface.WEB,
              binding: { case: 'combo', value: { combo: 'Ctrl+/' } },
            },
          ],
        },
        active: true,
        enabled: true,
      },
    ])

    const view = render(
      <KeyboardShortcutsDialog open={true} onOpenChange={mockOnOpenChange} />,
    )

    expect(view.getAllByText('Keyboard Shortcuts').length).toBeGreaterThan(0)
    expect(view.getByText('Ctrl+/')).toBeTruthy()
  })

  it('shows plural typed default bindings for shortcut reference rows', () => {
    mockUseCommands.mockReturnValue([
      {
        command: {
          commandId: 'spacewave.file.open',
          label: 'Open File',
          menuPath: 'File/Open File',
          defaultBindings: [
            {
              id: 'open-combo',
              surface: CommandSurface.WEB,
              binding: { case: 'combo', value: { combo: 'Ctrl+O' } },
            },
            {
              id: 'open-sequence',
              surface: CommandSurface.WEB,
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
    ])

    const view = render(
      <KeyboardShortcutsDialog open={true} onOpenChange={mockOnOpenChange} />,
    )

    expect(view.getByText('Open File')).toBeTruthy()
    expect(view.getByText('Ctrl+O / Leader F O')).toBeTruthy()
  })

  it('shows context labels when same binding text appears in multiple contexts', () => {
    mockUseCommands.mockReturnValue([
      {
        command: {
          commandId: 'spacewave.view.palette',
          label: 'Command Palette',
          menuPath: 'View/Command Palette',
          defaultBindings: [
            {
              id: 'palette',
              surface: CommandSurface.WEB,
              binding: { case: 'combo', value: { combo: 'CmdOrCtrl+K' } },
              when: CommandFocusContext.GLOBAL,
            },
          ],
        },
        active: true,
        enabled: true,
      },
      {
        command: {
          commandId: 'notes.insert.link',
          label: 'Insert Link',
          menuPath: 'Edit/Insert Link',
          defaultBindings: [
            {
              id: 'insert-link',
              surface: CommandSurface.WEB,
              binding: { case: 'combo', value: { combo: 'CmdOrCtrl+K' } },
              when: CommandFocusContext.EDITOR,
            },
          ],
        },
        active: true,
        enabled: true,
      },
    ])

    const view = render(
      <KeyboardShortcutsDialog open={true} onOpenChange={mockOnOpenChange} />,
    )

    expect(view.getByText('Command Palette')).toBeTruthy()
    expect(view.getByText('CmdOrCtrl+K (Global)')).toBeTruthy()
    expect(view.getByText('Insert Link')).toBeTruthy()
    expect(view.getByText('CmdOrCtrl+K (Editor)')).toBeTruthy()
  })

  it('renders editor entrypoints and passes the row command id to the edit callback', () => {
    mockUseCommands.mockReturnValue([
      {
        command: {
          commandId: 'spacewave.file.open',
          label: 'Open File',
          menuPath: 'File/Open File',
          defaultBindings: [
            {
              id: 'open-default',
              surface: CommandSurface.WEB,
              binding: { case: 'combo', value: { combo: 'Ctrl+O' } },
            },
          ],
        },
        active: true,
        enabled: true,
      },
    ])
    const onEditCommand = vi.fn()

    const view = render(
      <KeyboardShortcutsDialog
        open={true}
        onOpenChange={mockOnOpenChange}
        onEditCommand={onEditCommand}
      />,
    )

    fireEvent.click(view.getByText('Edit Keyboard Shortcuts'))
    expect(onEditCommand).toHaveBeenCalledWith()

    fireEvent.click(view.getByText('Edit'))
    expect(onEditCommand).toHaveBeenLastCalledWith('spacewave.file.open')
  })
})
