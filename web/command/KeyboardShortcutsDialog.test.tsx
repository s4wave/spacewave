import { Window } from 'happy-dom'
import React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render } from '@testing-library/react'

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

  it('shows legacy keybinding display through the resolver migration path', () => {
    mockUseCommands.mockReturnValue([
      {
        command: {
          commandId: 'spacewave.help.shortcuts',
          label: 'Keyboard Shortcuts',
          keybinding: 'Ctrl+/',
          menuPath: 'Help/Keyboard Shortcuts',
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
    ])

    const view = render(
      <KeyboardShortcutsDialog open={true} onOpenChange={mockOnOpenChange} />,
    )

    expect(view.getByText('Open File')).toBeTruthy()
    expect(view.getByText('Ctrl+O / Leader F O')).toBeTruthy()
  })
})
