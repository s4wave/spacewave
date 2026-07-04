import { Window } from 'happy-dom'
import React from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, waitFor } from '@testing-library/react'
import {
  CommandFocusContext,
  type Command,
  type CommandBinding,
} from '@s4wave/sdk/command/command.pb.js'
import type { CommandState } from '@s4wave/sdk/command/registry/registry.pb.js'

import { KeyDispatcher, useKeyDispatcherState } from './KeyDispatcher.js'

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

vi.mock('./CommandContext.js', () => ({
  useCommands: () => mockCommands,
  useInvokeCommand: () => mockInvokeCommand,
}))

function commandState(
  command: Command,
  state: Pick<CommandState, 'active' | 'enabled'> = {},
): CommandState {
  return {
    active: state.active ?? true,
    enabled: state.enabled ?? true,
    command,
  }
}

function command(
  commandId: string,
  overrides: Omit<Command, 'commandId' | 'label'> = {},
): Command {
  return {
    commandId,
    label: commandId,
    ...overrides,
  }
}

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

function sequenceBinding(
  id: string,
  steps: string[],
  when = CommandFocusContext.GLOBAL,
): CommandBinding {
  return {
    id,
    binding: { case: 'sequence', value: { steps } },
    when,
  }
}

function dispatchKeydown(
  target: Document | Element,
  init: KeyboardEventInit,
): KeyboardEvent {
  const event = new KeyboardEvent('keydown', {
    bubbles: true,
    cancelable: true,
    ...init,
  })
  fireEvent(target, event)
  return event
}

function PrefixStateObserver() {
  const state = useKeyDispatcherState()
  const continuations = state.continuations
    .map((continuation) =>
      [
        continuation.key,
        continuation.commandId ?? '',
        continuation.conflict ? 'conflict' : 'ok',
      ].join(':'),
    )
    .join(',')

  return (
    <output data-testid="prefix-state">
      {[state.mode, state.activePath.join(' '), continuations].join('|')}
    </output>
  )
}

function StopPropagationTarget() {
  return (
    <button type="button" onKeyDown={(event) => event.stopPropagation()}>
      Focus target
    </button>
  )
}

function renderDispatcher(children?: React.ReactNode) {
  return render(<KeyDispatcher>{children}</KeyDispatcher>)
}

describe('KeyDispatcher', () => {
  beforeEach(() => {
    mockCommands = []
  })

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('invokes a legacy combo from the document capture listener', () => {
    mockCommands = [
      commandState(
        command('spacewave.legacy.open', {
          keybinding: 'Ctrl+K',
        }),
      ),
    ]

    const { getByRole } = renderDispatcher(<StopPropagationTarget />)

    const event = dispatchKeydown(getByRole('button'), {
      key: 'k',
      ctrlKey: true,
    })

    expect(event.defaultPrevented).toBe(true)
    expect(mockInvokeCommand).toHaveBeenCalledTimes(1)
    expect(mockInvokeCommand).toHaveBeenCalledWith('spacewave.legacy.open')
  })

  it('does not invoke either command when duplicate combos conflict', () => {
    mockCommands = [
      commandState(
        command('spacewave.first', {
          defaultBindings: [comboBinding('first-open', 'Ctrl+K')],
        }),
      ),
      commandState(
        command('spacewave.second', {
          defaultBindings: [comboBinding('second-open', 'Ctrl+K')],
        }),
      ),
    ]

    renderDispatcher()

    const event = dispatchKeydown(document, { key: 'k', ctrlKey: true })

    expect(event.defaultPrevented).toBe(true)
    expect(mockInvokeCommand).not.toHaveBeenCalled()
  })

  it('cancels prefix state after an invalid sequence step', async () => {
    mockCommands = [
      commandState(
        command('spacewave.sequence.open', {
          defaultBindings: [sequenceBinding('leader-open', ['Leader', 'O'])],
        }),
      ),
    ]

    const { getByTestId } = renderDispatcher(<PrefixStateObserver />)

    dispatchKeydown(document, { key: ' ', ctrlKey: true })

    await waitFor(() => {
      expect(getByTestId('prefix-state').textContent).toBe(
        'prefix|ctrl+space|o:spacewave.sequence.open:ok',
      )
    })

    const event = dispatchKeydown(document, { key: 'x' })

    expect(event.defaultPrevented).toBe(true)
    await waitFor(() => {
      expect(getByTestId('prefix-state').textContent).toBe('idle||')
    })
    expect(mockInvokeCommand).not.toHaveBeenCalled()
  })

  it('keeps prefix state active when a modifier key precedes a shifted sequence step', async () => {
    mockCommands = [
      commandState(
        command('spacewave.sequence.shiftOpen', {
          defaultBindings: [
            sequenceBinding('leader-shift-open', ['Leader', 'Shift+O']),
          ],
        }),
      ),
    ]

    const { getByTestId } = renderDispatcher(<PrefixStateObserver />)

    dispatchKeydown(document, { key: ' ', ctrlKey: true })

    await waitFor(() => {
      expect(getByTestId('prefix-state').textContent).toBe(
        'prefix|ctrl+space|shift+o:spacewave.sequence.shiftOpen:ok',
      )
    })

    dispatchKeydown(document, { key: 'Shift', shiftKey: true })

    await waitFor(() => {
      expect(getByTestId('prefix-state').textContent).toBe(
        'prefix|ctrl+space|shift+o:spacewave.sequence.shiftOpen:ok',
      )
    })

    const event = dispatchKeydown(document, { key: 'O', shiftKey: true })

    expect(event.defaultPrevented).toBe(true)
    expect(mockInvokeCommand).toHaveBeenCalledTimes(1)
    expect(mockInvokeCommand).toHaveBeenCalledWith(
      'spacewave.sequence.shiftOpen',
    )
    await waitFor(() => {
      expect(getByTestId('prefix-state').textContent).toBe('idle||')
    })
  })

  it('cancels prefix state when Escape follows Ctrl+Space', async () => {
    mockCommands = [
      commandState(
        command('spacewave.sequence.open', {
          defaultBindings: [sequenceBinding('leader-open', ['Leader', 'O'])],
        }),
      ),
    ]

    const { getByTestId } = renderDispatcher(<PrefixStateObserver />)

    dispatchKeydown(document, { key: ' ', ctrlKey: true })

    await waitFor(() => {
      expect(getByTestId('prefix-state').textContent).toBe(
        'prefix|ctrl+space|o:spacewave.sequence.open:ok',
      )
    })

    const event = dispatchKeydown(document, { key: 'Escape' })

    expect(event.defaultPrevented).toBe(true)
    await waitFor(() => {
      expect(getByTestId('prefix-state').textContent).toBe('idle||')
    })
    expect(mockInvokeCommand).not.toHaveBeenCalled()
  })

  it('cancels prefix state when the window blurs after Ctrl+Space', async () => {
    mockCommands = [
      commandState(
        command('spacewave.sequence.open', {
          defaultBindings: [sequenceBinding('leader-open', ['Leader', 'O'])],
        }),
      ),
    ]

    const { getByTestId } = renderDispatcher(<PrefixStateObserver />)

    dispatchKeydown(document, { key: ' ', ctrlKey: true })

    await waitFor(() => {
      expect(getByTestId('prefix-state').textContent).toBe(
        'prefix|ctrl+space|o:spacewave.sequence.open:ok',
      )
    })

    fireEvent(window, new Event('blur'))

    await waitFor(() => {
      expect(getByTestId('prefix-state').textContent).toBe('idle||')
    })
    expect(mockInvokeCommand).not.toHaveBeenCalled()
  })

  it('dispatches the command for Ctrl+Space followed by a registered sequence step', async () => {
    mockCommands = [
      commandState(
        command('spacewave.sequence.open', {
          defaultBindings: [sequenceBinding('leader-open', ['Leader', 'O'])],
        }),
      ),
    ]

    const { getByTestId } = renderDispatcher(<PrefixStateObserver />)

    dispatchKeydown(document, { key: ' ', ctrlKey: true })

    await waitFor(() => {
      expect(getByTestId('prefix-state').textContent).toBe(
        'prefix|ctrl+space|o:spacewave.sequence.open:ok',
      )
    })

    const event = dispatchKeydown(document, { key: 'o' })

    expect(event.defaultPrevented).toBe(true)
    expect(mockInvokeCommand).toHaveBeenCalledTimes(1)
    expect(mockInvokeCommand).toHaveBeenCalledWith('spacewave.sequence.open')
    await waitFor(() => {
      expect(getByTestId('prefix-state').textContent).toBe('idle||')
    })
  })
})
