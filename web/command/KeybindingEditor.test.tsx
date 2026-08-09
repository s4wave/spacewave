import { CommandSurface } from '@s4wave/sdk/command/command.pb.js'
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
import userEvent from '@testing-library/user-event'
import {
  CommandFocusContext,
  type Command,
  type CommandBinding,
} from '@s4wave/sdk/command/command.pb.js'
import type { CommandState } from '@s4wave/sdk/command/registry/registry.pb.js'

import type { KeybindingOverrideSettings } from '@s4wave/sdk/command/command.pb.js'
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

const sharedLayerHookState = {
  account: {
    overrideSet: { overrides: {}, settings: {} },
    layer: null as unknown,
    available: false,
    readOnly: true,
    loading: false,
    error: null as Error | null,
    setCommandOverride: vi.fn(),
    setSettings: vi.fn<(settings: KeybindingOverrideSettings) => void>(),
    setOverrideSet: vi.fn(),
    setCommandBindings: vi.fn(),
    addCommandBinding: vi.fn(),
    clearCommandBindings: vi.fn(),
    clearCommandBindingId: vi.fn(),
    removeCommandBinding: vi.fn(),
    resetCommand: vi.fn(),
    resetLayer: vi.fn(),
  },
  space: {
    overrideSet: { overrides: {}, settings: {} },
    layer: null as unknown,
    available: false,
    readOnly: true,
    loading: false,
    error: null as Error | null,
    setCommandOverride: vi.fn(),
    setSettings: vi.fn<(settings: KeybindingOverrideSettings) => void>(),
    setOverrideSet: vi.fn(),
    setCommandBindings: vi.fn(),
    addCommandBinding: vi.fn(),
    clearCommandBindings: vi.fn(),
    clearCommandBindingId: vi.fn(),
    removeCommandBinding: vi.fn(),
    resetCommand: vi.fn(),
    resetLayer: vi.fn(),
  },
}

vi.mock('./useAccountKeybindingOverrides.js', () => ({
  useAccountKeybindingOverrides: () => sharedLayerHookState.account,
}))

vi.mock('./useSpaceKeybindingOverrides.js', () => ({
  useSpaceKeybindingOverrides: () => sharedLayerHookState.space,
}))

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
    surface: CommandSurface.WEB,
  }
}

function sequenceBinding(id: string, steps: string[]): CommandBinding {
  return {
    id,
    binding: { case: 'sequence', value: { steps } },
    when: CommandFocusContext.GLOBAL,
    surface: CommandSurface.WEB,
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
    sharedLayerHookState.account.overrideSet = {
      overrides: {},
      settings: {},
    }
    sharedLayerHookState.account.layer = null
    sharedLayerHookState.account.available = false
    sharedLayerHookState.account.readOnly = true
    sharedLayerHookState.account.loading = false
    sharedLayerHookState.account.error = null
    sharedLayerHookState.space.overrideSet = {
      overrides: {},
      settings: {},
    }
    sharedLayerHookState.space.layer = null
    sharedLayerHookState.space.available = false
    sharedLayerHookState.space.readOnly = true
    sharedLayerHookState.space.loading = false
    sharedLayerHookState.space.error = null
    cleanup()
    vi.clearAllMocks()
  })

  it('lists commands from useCommands, filters by label, command id, menu path, and chord, and starts on Local scope', async () => {
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

    expect(options).toContainEqual({
      text: 'Local',
      value: 'local',
      disabled: false,
    })
    expect(options.some((option) => option.text?.includes('next phase'))).toBe(
      false,
    )
    expect(scope.value).toBe('local')
    expect(view.getAllByText('Paint Brush').length).toBeGreaterThan(0)
    expect(view.getAllByText('Open Terminal').length).toBeGreaterThan(0)
    expect(view.getAllByText('Special Command').length).toBeGreaterThan(0)

    const search = view.getByPlaceholderText('Search commands…')

    await act(() => fireEvent.input(search, { target: { value: 'brush' } }))
    await waitFor(() => {
      expect(view.getAllByText('Paint Brush').length).toBeGreaterThan(0)
      expect(view.queryAllByText('Open Terminal')).toHaveLength(0)
    })

    await act(() =>
      fireEvent.input(search, { target: { value: 'spacewave.terminal' } }),
    )
    await waitFor(() => {
      expect(view.getAllByText('Open Terminal').length).toBeGreaterThan(0)
      expect(view.queryByText('Paint Brush')).toBeNull()
    })

    await act(() =>
      fireEvent.input(search, { target: { value: 'Help/Special' } }),
    )
    await waitFor(() => {
      expect(view.getAllByText('Special Command').length).toBeGreaterThan(0)
      expect(view.queryByText('Open Terminal')).toBeNull()
    })

    await act(() => fireEvent.input(search, { target: { value: 'Ctrl+B' } }))
    await waitFor(() => {
      expect(view.getAllByText('Paint Brush').length).toBeGreaterThan(0)
      expect(view.queryByText('Special Command')).toBeNull()
    })
    view.unmount()
  })

  it('enables Account scope when the account hook is writable, edits account discovery settings, and saves and resets the selected account layer', async () => {
    commandStates = [
      commandState(
        command('spacewave.open', {
          label: 'Open File',
          defaultBindings: [comboBinding('open-default', 'Ctrl+O')],
        }),
      ),
    ]
    sharedLayerHookState.account.available = true
    sharedLayerHookState.account.readOnly = false
    sharedLayerHookState.account.layer = {
      scope: 'account',
      label: 'Account',
      overrideSet: { overrides: {}, settings: {} },
    }

    const view = renderEditor('spacewave.open', 'account')
    const scope = view.container.querySelector('select') as HTMLSelectElement
    const options = Array.from(scope.options).map((option) => ({
      text: option.textContent,
      value: option.value,
      disabled: option.disabled,
    }))

    expect(options).toContainEqual({
      text: 'Account',
      value: 'account',
      disabled: false,
    })
    expect(options.find((option) => option.value === 'space')?.disabled).toBe(
      true,
    )
    expect(scope.value).toBe('account')
    const leaderInput = view.getByLabelText('Leader combo') as HTMLInputElement
    const delayInput = view.getByLabelText(
      'Which-key delay',
    ) as HTMLInputElement
    expect(leaderInput.disabled).toBe(false)
    expect(delayInput.disabled).toBe(false)
    expect(leaderInput.readOnly).toBe(false)
    expect(delayInput.readOnly).toBe(false)
    expect(options.some((option) => option.text?.includes('next phase'))).toBe(
      false,
    )
    expect(view.queryByText(/account settings are read-only/i)).toBeNull()

    const user = userEvent.setup({ document })
    sharedLayerHookState.account.setSettings.mockImplementation((settings) => {
      sharedLayerHookState.account.overrideSet = {
        ...sharedLayerHookState.account.overrideSet,
        settings,
      }
      sharedLayerHookState.account.layer = {
        scope: 'account',
        label: 'Account',
        overrideSet: sharedLayerHookState.account.overrideSet,
      }
      view.rerender(
        <StateNamespaceProvider rootAtom={atom({})}>
          <KeybindingEditor
            open={true}
            onOpenChange={vi.fn()}
            initialCommandId="spacewave.open"
            initialScope="account"
          />
        </StateNamespaceProvider>,
      )
    })

    await user.type(leaderInput, 'Alt+Space')
    expect(sharedLayerHookState.account.setSettings).toHaveBeenLastCalledWith({
      leaderCombo: 'Alt+Space',
    })
    await user.type(delayInput, '125')
    expect(sharedLayerHookState.account.setSettings).toHaveBeenLastCalledWith({
      leaderCombo: 'Alt+Space',
      whichKeyDelayMs: 125,
    })

    fireEvent.click(view.getByRole('button', { name: 'Replace with combo' }))
    fireEvent.keyDown(document, {
      key: 'K',
      ctrlKey: true,
    })
    fireEvent.click(view.getByRole('button', { name: 'Save binding' }))

    expect(
      sharedLayerHookState.account.setCommandBindings,
    ).toHaveBeenCalledWith('spacewave.open', [
      expect.objectContaining({
        binding: { case: 'combo', value: { combo: 'ctrl+k' } },
      }),
    ])
    expect(sharedLayerHookState.space.setCommandBindings).not.toHaveBeenCalled()

    fireEvent.click(view.getByRole('button', { name: /Reset command/ }))
    expect(sharedLayerHookState.account.resetCommand).toHaveBeenCalledWith(
      'spacewave.open',
    )

    fireEvent.click(view.getByRole('button', { name: /Reset Account layer/ }))
    expect(sharedLayerHookState.account.resetLayer).toHaveBeenCalled()
  })

  it('offers Space scope only when the Space hook is available and targets Space save and reset operations', () => {
    commandStates = [
      commandState(
        command('spacewave.open', {
          label: 'Open File',
          defaultBindings: [comboBinding('open-default', 'Ctrl+O')],
        }),
      ),
    ]
    sharedLayerHookState.account.available = true
    sharedLayerHookState.account.readOnly = false
    sharedLayerHookState.account.layer = {
      scope: 'account',
      label: 'Account',
      overrideSet: { overrides: {}, settings: {} },
    }
    sharedLayerHookState.space.available = true
    sharedLayerHookState.space.readOnly = false
    sharedLayerHookState.space.layer = {
      scope: 'space',
      label: 'Space',
      overrideSet: { overrides: {}, settings: {} },
    }

    const view = renderEditor('spacewave.open', 'space')
    const scope = view.container.querySelector('select') as HTMLSelectElement
    const spaceOption = Array.from(scope.options).find(
      (option) => option.value === 'space',
    )

    expect(spaceOption).toBeDefined()
    expect(spaceOption?.disabled).toBe(false)
    expect(scope.value).toBe('space')

    fireEvent.click(view.getByRole('button', { name: 'Replace with combo' }))
    fireEvent.keyDown(document, {
      key: 'K',
      ctrlKey: true,
    })
    fireEvent.click(view.getByRole('button', { name: 'Save binding' }))

    expect(sharedLayerHookState.space.setCommandBindings).toHaveBeenCalledWith(
      'spacewave.open',
      [
        expect.objectContaining({
          binding: { case: 'combo', value: { combo: 'ctrl+k' } },
        }),
      ],
    )
    expect(
      sharedLayerHookState.account.setCommandBindings,
    ).not.toHaveBeenCalled()

    fireEvent.click(
      view.getByRole('button', { name: /Disable command bindings/ }),
    )
    expect(
      sharedLayerHookState.space.clearCommandBindings,
    ).toHaveBeenCalledWith('spacewave.open')

    fireEvent.click(view.getByRole('button', { name: /Reset command/ }))
    expect(sharedLayerHookState.space.resetCommand).toHaveBeenCalledWith(
      'spacewave.open',
    )

    fireEvent.click(view.getByRole('button', { name: /Reset Space layer/ }))
    expect(sharedLayerHookState.space.resetLayer).toHaveBeenCalled()
  })

  it('captures without recorder focus, suppresses the combo, and cancels with Escape or click-away', async () => {
    commandStates = [
      commandState(
        command('spacewave.open', {
          label: 'Open File',
          defaultBindings: [comboBinding('open-default', 'Ctrl+O')],
        }),
      ),
    ]

    const view = renderEditor('spacewave.open')

    fireEvent.click(view.getByRole('button', { name: 'Replace with combo' }))
    expect(document.activeElement).not.toBe(view.getByRole('status'))
    fireEvent.keyDown(document, {
      key: 'Shift',
      shiftKey: true,
    })
    expect(view.getByText('Shift+…')).toBeTruthy()
    const comboEvent = new KeyboardEvent('keydown', {
      key: 'K',
      ctrlKey: true,
      shiftKey: true,
      bubbles: true,
      cancelable: true,
    })
    await act(() => document.dispatchEvent(comboEvent))
    expect(comboEvent.defaultPrevented).toBe(true)
    expect(view.getByText(/Pending replacement:/).textContent).toContain(
      'Ctrl+Shift+K',
    )

    fireEvent.click(view.getByRole('button', { name: 'Replace with combo' }))
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(view.queryByText(/Recording… press keys/)).toBeNull()
    expect(view.queryByText(/Pending replacement:/)).toBeNull()

    fireEvent.click(view.getByRole('button', { name: 'Replace with combo' }))
    fireEvent.pointerDown(document.body)
    expect(view.queryByText(/Recording… press keys/)).toBeNull()
  })

  it('captures combos and Leader sequences, mutates the local layer, clears defaults, and resets overrides', async () => {
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
    await waitFor(() => {
      expect(view.queryByText('Ctrl+O')).toBeNull()
      expect(view.getByText(/No keyboard binding/)).toBeTruthy()
    })

    fireEvent.click(view.getByRole('button', { name: /Reset command/ }))
    await waitFor(() => {
      expect(within(editorPanel()).getByText('Ctrl+O')).toBeTruthy()
    })

    fireEvent.click(view.getByRole('button', { name: 'Replace with combo' }))
    fireEvent.keyDown(document, {
      key: 'K',
      ctrlKey: true,
    })
    expect(view.getByText(/Pending replacement:/).textContent).toContain(
      'Ctrl+K',
    )
    fireEvent.click(view.getByRole('button', { name: 'Save binding' }))
    await waitFor(() => {
      expect(view.queryByText('Ctrl+O')).toBeNull()
      expect(view.getAllByText('Ctrl+K').length).toBeGreaterThan(0)
      expect(view.getByText('Local · Global')).toBeTruthy()
    })

    fireEvent.click(view.getByRole('button', { name: 'Add Leader sequence' }))
    fireEvent.keyDown(document, {
      key: 'K',
    })
    expect(view.getByText(/Pending addition:/).textContent).toContain(
      'Leader K',
    )
    fireEvent.click(view.getByRole('button', { name: 'Save binding' }))
    await waitFor(() => {
      expect(view.getByText('Leader K')).toBeTruthy()
    })

    fireEvent.click(view.getByText('Close File'))
    expect(within(editorPanel()).getByText('Ctrl+W')).toBeTruthy()
    fireEvent.click(
      view.getByRole('button', { name: /Disable command bindings/ }),
    )
    await waitFor(() => {
      expect(within(editorPanel()).queryByText('Ctrl+W')).toBeNull()
      expect(
        within(editorPanel()).getByText(/No keyboard binding/),
      ).toBeTruthy()
    })

    fireEvent.click(view.getByRole('button', { name: /Reset Local layer/ }))
    fireEvent.click(view.getByText('Open File'))
    await waitFor(() => {
      expect(within(editorPanel()).getByText('Ctrl+O')).toBeTruthy()
      expect(within(editorPanel()).queryByText('Ctrl+K')).toBeNull()
    })
    fireEvent.click(view.getByText('Close File'))
    await waitFor(() => {
      expect(within(editorPanel()).getByText('Ctrl+W')).toBeTruthy()
    })
  })

  it('surfaces a same-context conflict and replaces its existing owner', async () => {
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
    fireEvent.keyDown(document, {
      key: 'K',
      ctrlKey: true,
    })
    fireEvent.click(view.getByRole('button', { name: 'Save binding' }))
    await waitFor(() => {
      expect(view.getAllByText('Ctrl+K').length).toBeGreaterThan(0)
    })

    fireEvent.click(view.getByText('Close File'))
    fireEvent.click(view.getByRole('button', { name: 'Replace with combo' }))
    fireEvent.keyDown(document, {
      key: 'K',
      ctrlKey: true,
    })

    await waitFor(() => {
      expect(view.getByText('Conflict')).toBeTruthy()
      expect(
        view.getAllByText(
          (_, node) =>
            node?.textContent ===
            'Global combo Ctrl+K is used by Open File, Close File.',
        ).length,
      ).toBeGreaterThan(0)
      expect(view.getByRole('button', { name: 'Save binding' })).toHaveProperty(
        'disabled',
        true,
      )
      expect(within(editorPanel()).getByText('Ctrl+W')).toBeTruthy()
    })

    expect(view.getByText('Already used by Open File')).toBeTruthy()
    fireEvent.click(
      view.getByRole('button', { name: 'Replace existing binding' }),
    )
    await waitFor(() => {
      expect(within(editorPanel()).getByText('Ctrl+K')).toBeTruthy()
      expect(view.queryByText('Already used by Open File')).toBeNull()
    })
    fireEvent.click(view.getByText('Open File'))
    await waitFor(() => {
      expect(within(editorPanel()).queryByText('Ctrl+K')).toBeNull()
    })
  })

  it('edits local leader and which-key delay settings', async () => {
    commandStates = [
      commandState(
        command('spacewave.open', {
          label: 'Open File',
          defaultBindings: [comboBinding('open-default', 'Ctrl+O')],
        }),
      ),
    ]

    const view = renderEditor('spacewave.open')
    const leaderInput = view.getByLabelText('Leader combo') as HTMLInputElement
    const delayInput = view.getByLabelText(
      'Which-key delay',
    ) as HTMLInputElement

    fireEvent.change(leaderInput, { target: { value: 'Alt+Space' } })
    fireEvent.change(delayInput, { target: { value: '125' } })

    await waitFor(() => {
      expect(leaderInput.value).toBe('Alt+Space')
      expect(delayInput.value).toBe('125')
    })
  })
})
