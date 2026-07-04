import '@s4wave/web/test/happy-dom.js'

import React from 'react'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render } from '@testing-library/react'
import { CommandFocusContext } from '@s4wave/sdk/command/command.pb.js'
import type { Command, CommandBinding } from '@s4wave/sdk/command/command.pb.js'
import type { CommandState } from '@s4wave/sdk/command/registry/registry.pb.js'
import { KeyDispatcher } from '../../../web/command/KeyDispatcher.js'
import { FocusContextProvider } from '../../../web/command/FocusContext.js'


interface CapturedCommandOptions {
  commandId: string
  label: string
  menuPath?: string
  keybinding?: string
  defaultBindings?: CommandBinding[]
  active?: boolean
  enabled?: boolean
}

let mockCommands: CommandState[] = []
const mockInvokeCommand = vi.fn()
const capturedCommands = new Map<string, CapturedCommandOptions>()

vi.mock('../../../web/command/CommandContext.js', () => ({
  useCommands: () => mockCommands,
  useInvokeCommand: () => mockInvokeCommand,
}))

vi.mock('@s4wave/web/command/useCommand.js', () => ({
  useCommand: (opts: CapturedCommandOptions) => {
    capturedCommands.set(opts.commandId, opts)
  },
}))

vi.mock('@lexical/react/LexicalComposerContext', () => ({
  useLexicalComposerContext: () => [
    {
      update: (fn: () => void) => fn(),
      dispatchCommand: vi.fn(),
    },
  ],
}))

vi.mock('lexical', () => ({
  $getSelection: vi.fn(),
  $isRangeSelection: vi.fn(() => false),
}))

vi.mock('@lexical/selection', () => ({
  $setBlocksType: vi.fn(),
}))

vi.mock('@lexical/rich-text', () => ({
  $createHeadingNode: vi.fn(),
  $createQuoteNode: vi.fn(),
}))

vi.mock('@lexical/code', () => ({
  $createCodeNode: vi.fn(),
}))

vi.mock('@lexical/list', () => ({
  INSERT_UNORDERED_LIST_COMMAND: 'insert-unordered-list',
  INSERT_ORDERED_LIST_COMMAND: 'insert-ordered-list',
  INSERT_CHECK_LIST_COMMAND: 'insert-check-list',
}))

vi.mock('@lexical/link', () => ({
  TOGGLE_LINK_COMMAND: 'toggle-link',
}))

vi.mock('@lexical/table', () => ({
  INSERT_TABLE_COMMAND: 'insert-table',
}))

vi.mock('@lexical/react/LexicalHorizontalRuleNode', () => ({
  INSERT_HORIZONTAL_RULE_COMMAND: 'insert-horizontal-rule',
}))

let EditorCommandsPlugin: React.ComponentType

beforeAll(async () => {
  // Dynamic import keeps the Lexical mocks installed before this plugin loads.
  EditorCommandsPlugin = (await import('./EditorCommandsPlugin.js')).default
})

function commandState(command: Command): CommandState {
  return {
    active: true,
    enabled: true,
    command,
  }
}

function commandFromCaptured(opts: CapturedCommandOptions): Command {
  return {
    commandId: opts.commandId,
    label: opts.label,
    keybinding: opts.keybinding,
    menuPath: opts.menuPath,
    defaultBindings: opts.defaultBindings ?? [],
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

function PaletteAndEditorCollisionHarness() {
  return (
    <KeyDispatcher>
      <FocusContextProvider focusContext={CommandFocusContext.EDITOR}>
        <div
          aria-label="Lexical editor"
          contentEditable={true}
          suppressContentEditableWarning={true}
        />
      </FocusContextProvider>
      <button type="button">Outside editor</button>
    </KeyDispatcher>
  )
}

describe('EditorCommandsPlugin', () => {
  afterEach(() => {
    cleanup()
    mockCommands = []
    mockInvokeCommand.mockClear()
    capturedCommands.clear()
  })

  it('routes CmdOrCtrl+K to insert-link in Lexical focus and palette outside it', () => {
    const registrationView = render(<EditorCommandsPlugin />)
    registrationView.unmount()

    const insertLink = capturedCommands.get('notes.insert.link')
    if (!insertLink) throw new Error('notes.insert.link was not registered')
    expect(insertLink.defaultBindings).toEqual([
      {
        id: 'editor-insert-link',
        binding: { case: 'combo', value: { combo: 'CmdOrCtrl+K' } },
        when: CommandFocusContext.EDITOR,
      },
    ])

    mockCommands = [
      commandState({
        commandId: 'spacewave.view.palette',
        label: 'Command Palette',
        defaultBindings: [
          {
            id: 'global-palette',
            binding: { case: 'combo', value: { combo: 'CmdOrCtrl+K' } },
            when: CommandFocusContext.GLOBAL,
          },
        ],
      }),
      commandState(commandFromCaptured(insertLink)),
    ]
    const cmdOrCtrlKey = navigator.platform.includes('Mac')
      ? { metaKey: true }
      : { ctrlKey: true }

    const view = render(<PaletteAndEditorCollisionHarness />)
    const editor = view.getByLabelText('Lexical editor')
    editor.focus()

    const editorEvent = dispatchKeydown(editor, { key: 'k', ...cmdOrCtrlKey })

    expect(editorEvent.defaultPrevented).toBe(true)
    expect(mockInvokeCommand).toHaveBeenCalledWith('notes.insert.link')

    mockInvokeCommand.mockClear()
    const outsideEvent = dispatchKeydown(view.getByRole('button'), {
      key: 'k',
      ...cmdOrCtrlKey,
    })

    expect(outsideEvent.defaultPrevented).toBe(true)
    expect(mockInvokeCommand).toHaveBeenCalledWith('spacewave.view.palette')
  })
})
