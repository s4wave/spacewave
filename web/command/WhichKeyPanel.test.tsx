import { CommandSurface } from '@s4wave/sdk/command/command.pb.js'
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
      surface: CommandSurface.WEB,
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
    surface: CommandSurface.WEB,
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

  it('shows remaining chord paths and narrows them as a prefix is typed', async () => {
    mockCommands = [
      commandState(
        command('spacewave.file.open', {
          label: 'Open File',
          defaultBindings: [
            sequenceBinding('leader-open', ['Leader', 'F', 'O']),
          ],
        }),
      ),
      commandState(
        command('spacewave.file.save', {
          label: 'Save File',
          defaultBindings: [
            sequenceBinding('leader-save', ['Leader', 'F', 'S']),
          ],
        }),
      ),
      commandState(
        command('spacewave.terminal.open', {
          label: 'Open Terminal',
          defaultBindings: [
            sequenceBinding('leader-terminal', ['Leader', 'T']),
          ],
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
    expect(
      view.getByRole('option', { name: /Open File/ }).textContent,
    ).toContain('FO')
    expect(
      view.getByRole('option', { name: /Save File/ }).textContent,
    ).toContain('FS')
    expect(panel.textContent).toContain('Open Terminal')

    dispatchKeydown({ key: 'f' })

    await waitFor(() => {
      expect(
        view.getByRole('option', { name: /Open File/ }).textContent,
      ).toContain('O')
      expect(
        view.getByRole('option', { name: /Save File/ }).textContent,
      ).toContain('S')
      expect(panel.textContent).not.toContain('Open Terminal')
    })

    dispatchKeydown({ key: 'o' })

    expect(mockInvokeCommand).toHaveBeenCalledWith('spacewave.file.open')
    await waitFor(() => {
      expect(
        view.queryByRole('region', { name: 'Key sequence continuations' }),
      ).toBeNull()
    })
  })

  it('falls through printable non-chord keys to fuzzy command filtering', async () => {
    mockCommands = [
      commandState(
        command('spacewave.layout.zoom', {
          label: 'Alpha Zoom',
          defaultBindings: [sequenceBinding('leader-zoom', ['Leader', 'A'])],
        }),
      ),
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
    const panel = await view.findByRole('region', {
      name: 'Key sequence continuations',
    })

    dispatchKeydown({ key: 'z' })
    dispatchKeydown({ key: 'm' })

    await waitFor(() => {
      expect(panel.textContent).toContain('Filter: zm')
      expect(panel.textContent).toContain('Alpha Zoom')
      expect(panel.textContent).not.toContain('Open File')
    })
  })

  it('navigates filtered commands with arrows and runs the selection with Enter', async () => {
    mockCommands = [
      commandState(
        command('spacewave.first.zoom', {
          label: 'First Zoom',
          defaultBindings: [sequenceBinding('leader-first', ['Leader', 'A'])],
        }),
      ),
      commandState(
        command('spacewave.second.zoom', {
          label: 'Second Zoom',
          defaultBindings: [sequenceBinding('leader-second', ['Leader', 'B'])],
        }),
      ),
    ]

    const view = render(
      <KeyDispatcher>
        <WhichKeyPanel />
      </KeyDispatcher>,
    )

    dispatchKeydown({ key: ' ', ctrlKey: true })
    await view.findByRole('region', { name: 'Key sequence continuations' })
    dispatchKeydown({ key: 'z' })

    const first = view.getByRole('option', { name: /First Zoom/ })
    const second = view.getByRole('option', { name: /Second Zoom/ })
    expect(first.getAttribute('aria-selected')).toBe('true')
    expect(second.getAttribute('aria-selected')).toBe('false')

    dispatchKeydown({ key: 'ArrowDown' })

    await waitFor(() => {
      expect(first.getAttribute('aria-selected')).toBe('false')
      expect(second.getAttribute('aria-selected')).toBe('true')
    })

    dispatchKeydown({ key: 'Enter' })

    expect(mockInvokeCommand).toHaveBeenCalledWith('spacewave.second.zoom')
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
