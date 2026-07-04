import { Window } from 'happy-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, cleanup, render, waitFor } from '@testing-library/react'
import {
  CommandFocusContext,
  type Command,
  type CommandBinding,
} from '@s4wave/sdk/command/command.pb.js'
import type { CommandState } from '@s4wave/sdk/command/registry/registry.pb.js'

import { KeyDispatcher } from './KeyDispatcher.js'
import { WhichKeyPanel } from './WhichKeyPanel.js'
import { resolveKeybindings } from './KeybindingResolver.js'

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
const mockInvokeCommand = vi.fn()
let localKeybindingSettings: {
  leaderCombo?: string
  whichKeyDelayMs?: number
} = {}

vi.mock('./CommandContext.js', () => ({
  useCommands: () => mockCommands,
  useInvokeCommand: () => mockInvokeCommand,
}))

vi.mock('./useKeybindingGraph.js', () => ({
  useKeybindingGraph: (commands: CommandState[]) =>
    resolveKeybindings(commands, {
      overrideLayers: [
        {
          scope: 'local',
          label: 'Local',
          overrideSet: {
            version: 1,
            overrides: {},
            settings: localKeybindingSettings,
          },
        },
      ],
    }),
}))

function commandState(command: Command): CommandState {
  return {
    active: true,
    enabled: true,
    command,
  }
}

function command(
  commandId: string,
  overrides: Omit<Command, 'commandId'> = {},
): Command {
  return {
    commandId,
    label: commandId,
    ...overrides,
  }
}

function sequenceBinding(id: string, steps: string[]): CommandBinding {
  return {
    id,
    binding: { case: 'sequence', value: { steps } },
    when: CommandFocusContext.GLOBAL,
  }
}

function dispatchKeydown(init: KeyboardEventInit): KeyboardEvent {
  const event = new KeyboardEvent('keydown', {
    bubbles: true,
    cancelable: true,
    ...init,
  })
  act(() => {
    document.dispatchEvent(event)
  })
  return event
}

describe('WhichKeyPanel', () => {
  beforeEach(() => {
    mockCommands = []
    localKeybindingSettings = {}
  })

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
    vi.useRealTimers()
  })

  it('renders prefix continuations from dispatcher state only', async () => {
    mockCommands = [
      commandState(
        command('spacewave.file.open', {
          label: 'Open File',
          defaultBindings: [sequenceBinding('leader-open', ['Leader', 'O'])],
        }),
      ),
    ]

    const view = render(
      <KeyDispatcher>
        <WhichKeyPanel />
      </KeyDispatcher>,
    )

    expect(
      view.queryByRole('region', { name: 'Key sequence continuations' }),
    ).toBeNull()

    dispatchKeydown({ key: ' ', ctrlKey: true })

    const panel = await view.findByRole('region', {
      name: 'Key sequence continuations',
    })
    expect(panel.textContent).toContain('Ctrl+Space')
    expect(panel.textContent).toContain('O')
    expect(panel.textContent).toContain('Open File')
    expect(panel.textContent).toContain('spacewave.file.open')

    dispatchKeydown({ key: 'o' })

    await waitFor(() => {
      expect(
        view.queryByRole('region', { name: 'Key sequence continuations' }),
      ).toBeNull()
    })
  })

  it('renders conflict hints without dispatching duplicate sequences', async () => {
    mockCommands = [
      commandState(
        command('spacewave.first', {
          defaultBindings: [sequenceBinding('first-open', ['Leader', 'O'])],
        }),
      ),
      commandState(
        command('spacewave.second', {
          defaultBindings: [sequenceBinding('second-open', ['Leader', 'O'])],
        }),
      ),
    ]

    const view = render(
      <KeyDispatcher>
        <WhichKeyPanel />
      </KeyDispatcher>,
    )

    dispatchKeydown({ key: ' ', ctrlKey: true })

    const panel = await view.findByRole('region', {
      name: 'Key sequence continuations',
    })
    expect(panel.textContent).toContain('O')
    expect(panel.textContent).toContain('Conflict')

    dispatchKeydown({ key: 'o' })

    await waitFor(() => {
      expect(panel.textContent).toContain('Conflict')
    })
    expect(mockInvokeCommand).not.toHaveBeenCalled()
  })

  it('delays only panel visibility while prefix dispatch stays immediate', () => {
    vi.useFakeTimers()
    localKeybindingSettings = { whichKeyDelayMs: 100 }
    mockCommands = [
      commandState(
        command('spacewave.file.open', {
          label: 'Open File',
          defaultBindings: [sequenceBinding('leader-open', ['Leader', 'O'])],
        }),
      ),
    ]

    const view = render(
      <KeyDispatcher>
        <WhichKeyPanel />
      </KeyDispatcher>,
    )

    dispatchKeydown({ key: ' ', ctrlKey: true })

    expect(
      view.queryByRole('region', { name: 'Key sequence continuations' }),
    ).toBeNull()

    dispatchKeydown({ key: 'o' })
    expect(mockInvokeCommand).toHaveBeenCalledWith('spacewave.file.open')

    act(() => {
      vi.advanceTimersByTime(100)
    })
    expect(
      view.queryByRole('region', { name: 'Key sequence continuations' }),
    ).toBeNull()
    vi.useRealTimers()
  })
})
