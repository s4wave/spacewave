import { Window } from 'happy-dom'
import React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  act,
  cleanup,
  fireEvent,
  render,
  waitFor,
  within,
} from '@testing-library/react'
import {
  CommandFocusContext,
  type Command,
  type CommandBinding,
} from '@s4wave/sdk/command/command.pb.js'
import type { CommandState } from '@s4wave/sdk/command/registry/registry.pb.js'

import { StateNamespaceProvider, atom } from '@s4wave/web/state/index.js'
import {
  KeybindingEditor,
  type KeybindingEditorScope,
} from './KeybindingEditor.js'

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

let commandStates: CommandState[] = []

vi.mock('./CommandContext.js', () => ({
  useCommands: () => commandStates,
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

function comboBinding(
  id: string,
  combo: string,
  when = CommandFocusContext.GLOBAL,
): CommandBinding {
  return {
    id,
    binding: { case: 'combo', value: { combo } },
    when,
  }
}

function sequenceBinding(id: string, steps: string[]): CommandBinding {
  return {
    id,
    binding: { case: 'sequence', value: { steps } },
    when: CommandFocusContext.GLOBAL,
  }
}

function commandState(
  command: Command,
  state: Pick<CommandState, 'active' | 'enabled'> = {},
): CommandState {
  return {
    command,
    active: state.active ?? true,
    enabled: state.enabled ?? true,
  }
}

function command(commandId: string, overrides: Partial<Command>): Command {
  return {
    commandId,
    label: overrides.label ?? commandId,
    ...overrides,
  }
}

function renderEditor(
  initialCommandId?: string,
  initialScope: KeybindingEditorScope = 'local',
) {
  return render(
    <StateNamespaceProvider rootAtom={atom({})}>
      <KeybindingEditor
        open={true}
        onOpenChange={vi.fn()}
        initialCommandId={initialCommandId}
        initialScope={initialScope}
      />
    </StateNamespaceProvider>,
  )
}

function editorPanel(): HTMLElement {
  const panel = document.querySelector('section')
  if (!panel) throw new Error('KeybindingEditor panel was not rendered')
  return panel
}

describe('KeybindingEditor', () => {
  afterEach(() => {
    commandStates = []
    cleanup()
    vi.clearAllMocks()
  })

  it('lists commands from useCommands, filters by label, command id, menu path, and chord, and only enables Local scope', async () => {
    commandStates = [
      commandState(
        command('spacewave.paint.brush', {
          label: 'Paint Brush',
          menuPath: 'Canvas/Brush Tool',
          defaultBindings: [comboBinding('brush-default', 'Ctrl+B')],
        }),
      ),
      commandState(
        command('spacewave.terminal.open', {
          label: 'Open Terminal',
          menuPath: 'Tools/Terminal',
          defaultBindings: [
            sequenceBinding('terminal-default', ['Leader', 'T']),
          ],
        }),
      ),
      commandState(
        command('spacewave.hidden.special', {
          label: 'Special Command',
          menuPath: 'Help/Special',
          defaultBindings: [comboBinding('special-default', 'Alt+S')],
        }),
      ),
    ]

    const view = renderEditor()
    const scope = view.container.querySelector('select') as HTMLSelectElement
    const options = Array.from(scope.options).map((option) => ({
      text: option.textContent,
      value: option.value,
      disabled: option.disabled,
    }))

    expect(options).toEqual([
      { text: 'Local', value: 'local', disabled: false },
      { text: 'Account (next phase)', value: 'account', disabled: true },
      { text: 'Space (next phase)', value: 'space', disabled: true },
    ])
    expect(scope.value).toBe('local')
    expect(view.getAllByText('Paint Brush').length).toBeGreaterThan(0)
    expect(view.getAllByText('Open Terminal').length).toBeGreaterThan(0)
    expect(view.getAllByText('Special Command').length).toBeGreaterThan(0)

    const search = view.getByPlaceholderText('Search commands...')

    await act(async () =>
      fireEvent.input(search, { target: { value: 'brush' } }),
    )
    await waitFor(() => {
      expect(view.getAllByText('Paint Brush').length).toBeGreaterThan(0)
      expect(view.queryAllByText('Open Terminal')).toHaveLength(0)
    })

    await act(async () =>
      fireEvent.input(search, { target: { value: 'spacewave.terminal' } }),
    )
    await waitFor(() => {
      expect(view.getAllByText('Open Terminal').length).toBeGreaterThan(0)
      expect(view.queryByText('Paint Brush')).toBeNull()
    })

    await act(async () =>
      fireEvent.input(search, { target: { value: 'Help/Special' } }),
    )
    await waitFor(() => {
      expect(view.getAllByText('Special Command').length).toBeGreaterThan(0)
      expect(view.queryByText('Open Terminal')).toBeNull()
    })

    await act(async () =>
      fireEvent.input(search, { target: { value: 'Ctrl+B' } }),
    )
    await waitFor(() => {
      expect(view.getAllByText('Paint Brush').length).toBeGreaterThan(0)
      expect(view.queryByText('Special Command')).toBeNull()
    })
    view.unmount()
  })

  it('opens account and Space scopes as disabled views until persistence layers land', () => {
    commandStates = [
      commandState(
        command('spacewave.open', {
          label: 'Open File',
          defaultBindings: [comboBinding('open-default', 'Ctrl+O')],
        }),
      ),
    ]

    const accountView = renderEditor('spacewave.open', 'account')
    const accountScope = accountView.container.querySelector(
      'select',
    ) as HTMLSelectElement

    expect(accountScope.value).toBe('account')
    expect(
      accountView.getByText(/Account overrides are visible here/),
    ).toBeTruthy()
    expect(
      accountView.getByRole('button', { name: 'Replace with combo' }),
    ).toHaveProperty('disabled', true)
    expect(
      accountView.getByRole('button', { name: 'Save binding' }),
    ).toHaveProperty('disabled', true)
    accountView.unmount()

    const spaceView = renderEditor('spacewave.open', 'space')
    const spaceScope = spaceView.container.querySelector(
      'select',
    ) as HTMLSelectElement

    expect(spaceScope.value).toBe('space')
    expect(spaceView.getByText(/Space overrides are visible here/)).toBeTruthy()
    expect(
      spaceView.getByRole('button', { name: 'Replace with combo' }),
    ).toHaveProperty('disabled', true)
    expect(
      spaceView.getByRole('button', { name: 'Save binding' }),
    ).toHaveProperty('disabled', true)
    spaceView.unmount()
  })

  it('captures combos and Leader sequences, mutates the local layer, clears defaults, and resets overrides', () => {
    commandStates = [
      commandState(
        command('spacewave.open', {
          label: 'Open File',
          menuPath: 'File/Open',
          defaultBindings: [comboBinding('open-default', 'Ctrl+O')],
        }),
      ),
      commandState(
        command('spacewave.close', {
          label: 'Close File',
          menuPath: 'File/Close',
          defaultBindings: [comboBinding('close-default', 'Ctrl+W')],
        }),
      ),
    ]

    const view = renderEditor('spacewave.open')

    expect(within(editorPanel()).getByText('Ctrl+O')).toBeTruthy()

    fireEvent.click(view.getByRole('button', { name: 'Clear' }))
    expect(view.queryByText('Ctrl+O')).toBeNull()
    expect(view.getByText(/No keyboard binding/)).toBeTruthy()

    fireEvent.click(view.getByRole('button', { name: /Reset command/ }))
    expect(within(editorPanel()).getByText('Ctrl+O')).toBeTruthy()

    fireEvent.click(view.getByRole('button', { name: 'Replace with combo' }))
    fireEvent.keyDown(view.getByRole('button', { name: /Press one combo/ }), {
      key: 'K',
      ctrlKey: true,
    })
    expect(view.getByText(/Pending replacement:/).textContent).toContain(
      'ctrl+k',
    )
    fireEvent.click(view.getByRole('button', { name: 'Save binding' }))
    expect(view.queryByText('Ctrl+O')).toBeNull()
    expect(view.getAllByText('ctrl+k').length).toBeGreaterThan(0)
    expect(view.getByText('Local · Global')).toBeTruthy()

    fireEvent.click(view.getByRole('button', { name: 'Add Leader sequence' }))
    fireEvent.keyDown(
      view.getByRole('button', { name: /Press sequence keys/ }),
      {
        key: 'K',
      },
    )
    expect(view.getByText(/Pending addition:/).textContent).toContain(
      'Leader k',
    )
    fireEvent.click(view.getByRole('button', { name: 'Save binding' }))
    expect(view.getByText('Leader k')).toBeTruthy()

    fireEvent.click(view.getByText('Close File'))
    expect(within(editorPanel()).getByText('Ctrl+W')).toBeTruthy()
    fireEvent.click(
      view.getByRole('button', { name: /Disable command bindings/ }),
    )
    expect(within(editorPanel()).queryByText('Ctrl+W')).toBeNull()
    expect(within(editorPanel()).getByText(/No keyboard binding/)).toBeTruthy()

    fireEvent.click(view.getByRole('button', { name: /Reset local layer/ }))
    fireEvent.click(view.getByText('Open File'))
    expect(within(editorPanel()).getByText('Ctrl+O')).toBeTruthy()
    expect(within(editorPanel()).queryByText('ctrl+k')).toBeNull()
    fireEvent.click(view.getByText('Close File'))
    expect(within(editorPanel()).getByText('Ctrl+W')).toBeTruthy()
  })

  it('shows same-context conflicts before save and blocks the conflicting local binding', () => {
    commandStates = [
      commandState(
        command('spacewave.open', {
          label: 'Open File',
          defaultBindings: [comboBinding('open-default', 'Ctrl+O')],
        }),
      ),
      commandState(
        command('spacewave.close', {
          label: 'Close File',
          defaultBindings: [comboBinding('close-default', 'Ctrl+W')],
        }),
      ),
    ]

    const view = renderEditor('spacewave.open')

    fireEvent.click(view.getByRole('button', { name: 'Replace with combo' }))
    fireEvent.keyDown(view.getByRole('button', { name: /Press one combo/ }), {
      key: 'K',
      ctrlKey: true,
    })
    fireEvent.click(view.getByRole('button', { name: 'Save binding' }))
    expect(view.getAllByText('ctrl+k').length).toBeGreaterThan(0)

    fireEvent.click(view.getByText('Close File'))
    fireEvent.click(view.getByRole('button', { name: 'Replace with combo' }))
    fireEvent.keyDown(view.getByRole('button', { name: /Press one combo/ }), {
      key: 'K',
      ctrlKey: true,
    })

    expect(view.getByText('Conflict')).toBeTruthy()
    expect(
      view.getAllByText(
        (_, node) =>
          node?.textContent ===
          'Global combo ctrl+k is used by Open File, Close File.',
      ).length,
    ).toBeGreaterThan(0)
    expect(view.getByRole('button', { name: 'Save binding' })).toHaveProperty(
      'disabled',
      true,
    )
    expect(within(editorPanel()).getByText('Ctrl+W')).toBeTruthy()
  })
})
