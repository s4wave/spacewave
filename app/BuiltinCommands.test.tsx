import React from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  act,
  cleanup,
  fireEvent,
  render,
  waitFor,
} from '@testing-library/react'

import type { KeybindingEditorProps } from '@s4wave/web/command/KeybindingEditor.js'
import type { KeyboardShortcutsDialogProps } from '@s4wave/web/command/KeyboardShortcutsDialog.js'
import { BuiltinCommands } from './BuiltinCommands.js'

interface RegisteredCommand {
  commandId: string
  label: string
  keybinding?: string
  menuPath?: string
  menuGroup?: number
  menuOrder?: number
  active?: boolean
  enabled?: boolean
  handler: (args: Record<string, string>) => void
}

interface ShellTabOpenOptions {
  afterTabId: string
  focusExisting: boolean
}

interface BuiltinCommandMocks {
  isDesktop: boolean
  commands: RegisteredCommand[]
  quitDesktopRuntime: () => Promise<void>
  addRootAlias: () => void
  resetShellTabs: () => void
  openPathInActiveTabset: (path: string, opts: ShellTabOpenOptions) => void
  appPath: string
  setAppPath: (path: string) => void
  keyboardShortcutsDialogs: KeyboardShortcutsDialogProps[]
  keybindingEditors: KeybindingEditorProps[]
}

let builtinCommandMocks: BuiltinCommandMocks

vi.mock('@aptre/bldr', () => ({
  get isDesktop() {
    return builtinCommandMocks.isDesktop
  },
  quitDesktopRuntime: () => builtinCommandMocks.quitDesktopRuntime(),
}))

vi.mock('@s4wave/web/command/useCommand.js', () => ({
  useCommand: (opts: RegisteredCommand) => {
    builtinCommandMocks.commands.push(opts)
  },
}))

vi.mock('@s4wave/web/command/KeyboardShortcutsDialog.js', () => ({
  KeyboardShortcutsDialog: (props: KeyboardShortcutsDialogProps) => {
    builtinCommandMocks.keyboardShortcutsDialogs.push(props)
    return props.open ? (
      <button
        type="button"
        onClick={() => props.onEditCommand?.('spacewave.file.open')}
      >
        Edit Open File
      </button>
    ) : null
  },
}))

vi.mock('@s4wave/web/command/KeybindingEditor.js', () => ({
  KeybindingEditor: (props: KeybindingEditorProps) => {
    builtinCommandMocks.keybindingEditors.push(props)
    return props.open ? (
      <div data-testid="keybinding-editor">
        {props.initialScope}:{props.initialCommandId}
      </div>
    ) : null
  },
}))

vi.mock('@s4wave/web/ui/toaster.js', () => ({
  toast: {
    error: vi.fn(),
    success: vi.fn(),
  },
}))

vi.mock('@s4wave/app/AboutDialog.js', () => ({
  AboutDialog: () => null,
}))

vi.mock('@s4wave/app/EmailSupportDialog.js', () => ({
  EmailSupportDialog: () => null,
}))

vi.mock('@s4wave/app/github.js', () => ({
  DISCORD_INVITE_URL: 'https://discord.example',
  GITHUB_ISSUES_URL: 'https://issues.example',
}))

vi.mock('@s4wave/app/urls.js', () => ({
  SPACEWAVE_PUBLIC_BASE_URL: 'https://spacewave.example',
}))

vi.mock('@s4wave/app/hooks/useAddSpaceRootAlias.js', () => ({
  useAddSpaceRootAlias: () => ({
    add: builtinCommandMocks.addRootAlias,
    canAdd: true,
  }),
}))

vi.mock('@s4wave/app/session/SelectAccountCommand.js', () => ({
  SelectAccountCommand: () => null,
}))
vi.mock('@s4wave/app/ShellTabContext.js', () => ({
  useShellTabs: () => ({
    activeTabId: 'home',
    openPathInActiveTabset: builtinCommandMocks.openPathInActiveTabset,
    resetShellTabs: builtinCommandMocks.resetShellTabs,
  }),
}))

vi.mock('@s4wave/web/router/app-path.js', () => ({
  getAppPath: () => builtinCommandMocks.appPath,
  setAppPath: (path: string) => builtinCommandMocks.setAppPath(path),
}))

function findCommand(commandId: string): RegisteredCommand | undefined {
  return builtinCommandMocks.commands.find((cmd) => cmd.commandId === commandId)
}

describe('BuiltinCommands', () => {
  beforeEach(() => {
    builtinCommandMocks = {
      isDesktop: true,
      commands: [],
      quitDesktopRuntime: vi.fn(() => Promise.resolve()),
      addRootAlias: vi.fn(),
      resetShellTabs: vi.fn(),
      openPathInActiveTabset: vi.fn(),
      appPath: '/',
      setAppPath: vi.fn(),
      keyboardShortcutsDialogs: [],
      keybindingEditors: [],
    }
  })

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('registers Close Window and Quit only for desktop mode', () => {
    render(<BuiltinCommands />)

    expect(findCommand('spacewave.file.close-window')).toMatchObject({
      label: 'Close Window',
    })
    expect(findCommand('spacewave.file.close-window')).not.toHaveProperty(
      'menuPath',
    )
    expect(findCommand('spacewave.file.quit')).toMatchObject({
      label: 'Quit',
    })
    expect(findCommand('spacewave.file.quit')).not.toHaveProperty('menuPath')
  })

  it('omits desktop commands outside desktop mode', () => {
    builtinCommandMocks.isDesktop = false

    render(<BuiltinCommands />)

    expect(findCommand('spacewave.file.close-window')).toBeUndefined()
    expect(findCommand('spacewave.file.quit')).toBeUndefined()
  })

  it('deactivates Add State Root outside desktop mode', () => {
    builtinCommandMocks.isDesktop = false

    render(<BuiltinCommands />)

    expect(findCommand('spacewave.root.add')).toMatchObject({
      active: false,
    })
    expect(findCommand('spacewave.root.add')).not.toHaveProperty('menuPath')
  })

  it('routes File Quit through the desktop runtime quit bridge', async () => {
    render(<BuiltinCommands />)

    findCommand('spacewave.file.quit')?.handler({})

    await waitFor(() => {
      expect(builtinCommandMocks.quitDesktopRuntime).toHaveBeenCalledTimes(1)
    })
  })

  it('opens Documentation through provider Shell Tab semantics inside a session', () => {
    builtinCommandMocks.appPath = '/u/1'
    render(<BuiltinCommands />)

    findCommand('spacewave.help.docs')?.handler({})

    expect(builtinCommandMocks.openPathInActiveTabset).toHaveBeenCalledWith(
      '/docs',
      { afterTabId: 'home', focusExisting: true },
    )
    expect(builtinCommandMocks.setAppPath).not.toHaveBeenCalled()
  })

  it('opens Documentation through provider Shell Tab semantics from home', () => {
    builtinCommandMocks.appPath = '/'
    render(<BuiltinCommands />)

    findCommand('spacewave.help.docs')?.handler({})

    expect(builtinCommandMocks.openPathInActiveTabset).toHaveBeenCalledWith(
      '/docs',
      { afterTabId: 'home', focusExisting: true },
    )
    expect(builtinCommandMocks.setAppPath).not.toHaveBeenCalled()
  })
  it('exposes a visible Shell reset command through the menu owner', () => {
    render(<BuiltinCommands />)

    const command = findCommand('spacewave.shell.reset-tabs')
    expect(command).toMatchObject({
      label: 'Reset Shell Tabs',
      menuPath: 'View/Reset Shell Tabs',
    })

    command?.handler({})
    expect(builtinCommandMocks.resetShellTabs).toHaveBeenCalledTimes(1)
  })

  it('registers the keyboard shortcut preference command and opens the keybinding editor for its target command', () => {
    render(<BuiltinCommands />)

    expect(
      findCommand('spacewave.preferences.keyboard-shortcuts'),
    ).toMatchObject({
      label: 'Edit Keyboard Shortcuts',
      menuPath: 'Tools/Keyboard Shortcuts',
      menuGroup: 10,
      menuOrder: 1,
    })

    act(() => {
      findCommand('spacewave.preferences.keyboard-shortcuts')?.handler({
        commandId: 'spacewave.file.open',
        scope: 'space',
      })
    })

    expect(builtinCommandMocks.keybindingEditors.at(-1)).toMatchObject({
      open: true,
      initialScope: 'space',
      initialCommandId: 'spacewave.file.open',
    })
  })

  it('opens the local keybinding editor from the shortcuts dialog row edit affordance', () => {
    render(<BuiltinCommands />)

    act(() => {
      findCommand('spacewave.help.shortcuts')?.handler({})
    })
    expect(
      builtinCommandMocks.keyboardShortcutsDialogs.some((props) => props.open),
    ).toBe(true)
    fireEvent.click(document.querySelector('button') as HTMLButtonElement)

    expect(builtinCommandMocks.keyboardShortcutsDialogs.at(-1)).toMatchObject({
      open: false,
    })
    expect(builtinCommandMocks.keybindingEditors.at(-1)).toMatchObject({
      open: true,
      initialScope: 'local',
      initialCommandId: 'spacewave.file.open',
    })
  })
})
