import React from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, waitFor } from '@testing-library/react'

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

interface BuiltinCommandMocks {
  isDesktop: boolean
  commands: RegisteredCommand[]
  quitDesktopRuntime: ReturnType<typeof vi.fn<() => Promise<void>>>
  addRootAlias: ReturnType<typeof vi.fn>
  openPathInNewTab: ReturnType<typeof vi.fn>
  appPath: string
  setAppPath: ReturnType<typeof vi.fn>
}

const mocks = vi.hoisted<BuiltinCommandMocks>(() => ({
  isDesktop: true,
  commands: [],
  quitDesktopRuntime: vi.fn(() => Promise.resolve()),
  addRootAlias: vi.fn(),
  openPathInNewTab: vi.fn(),
  appPath: '/',
  setAppPath: vi.fn(),
}))

vi.mock('@aptre/bldr', () => ({
  get isDesktop() {
    return mocks.isDesktop
  },
  quitDesktopRuntime: mocks.quitDesktopRuntime,
}))

vi.mock('@s4wave/web/command/useCommand.js', () => ({
  useCommand: (opts: RegisteredCommand) => {
    mocks.commands.push(opts)
  },
}))

vi.mock('@s4wave/web/command/KeyboardShortcutsDialog.js', () => ({
  KeyboardShortcutsDialog: () => null,
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
    add: mocks.addRootAlias,
    canAdd: true,
  }),
}))

vi.mock('@s4wave/app/ShellTabContext.js', () => ({
  useShellTabs: () => ({
    activeTabId: 'home',
    openPathInNewTab: mocks.openPathInNewTab,
  }),
}))

vi.mock('@s4wave/web/router/app-path.js', () => ({
  getAppPath: () => mocks.appPath,
  setAppPath: mocks.setAppPath,
}))

function findCommand(commandId: string): RegisteredCommand | undefined {
  return mocks.commands.find((cmd) => cmd.commandId === commandId)
}

describe('BuiltinCommands', () => {
  beforeEach(() => {
    mocks.isDesktop = true
    mocks.appPath = '/'
    mocks.commands.length = 0
    mocks.quitDesktopRuntime.mockResolvedValue(undefined)
  })

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('registers Close Window and Quit only for desktop mode', () => {
    render(<BuiltinCommands />)

    expect(findCommand('spacewave.file.close-window')).toMatchObject({
      label: 'Close Window',
      keybinding: 'CmdOrCtrl+W',
      menuPath: 'File/Close Window',
      menuGroup: 90,
      menuOrder: 1,
    })
    expect(findCommand('spacewave.file.quit')).toMatchObject({
      label: 'Quit',
      keybinding: 'CmdOrCtrl+Q',
      menuPath: 'File/Quit',
      menuGroup: 90,
      menuOrder: 2,
    })
  })

  it('omits desktop commands outside desktop mode', () => {
    mocks.isDesktop = false

    render(<BuiltinCommands />)

    expect(findCommand('spacewave.file.close-window')).toBeUndefined()
    expect(findCommand('spacewave.file.quit')).toBeUndefined()
  })

  it('deactivates Add State Root outside desktop mode', () => {
    mocks.isDesktop = false

    render(<BuiltinCommands />)

    expect(findCommand('spacewave.root.add')).toMatchObject({
      active: false,
    })
  })

  it('routes File Quit through the desktop runtime quit bridge', async () => {
    render(<BuiltinCommands />)

    findCommand('spacewave.file.quit')?.handler({})

    await waitFor(() => {
      expect(mocks.quitDesktopRuntime).toHaveBeenCalledTimes(1)
    })
  })

  it('opens Documentation through provider Shell Tab semantics inside a session', () => {
    mocks.appPath = '/u/1'
    render(<BuiltinCommands />)

    findCommand('spacewave.help.docs')?.handler({})

    expect(mocks.openPathInNewTab).toHaveBeenCalledWith('/docs', {
      afterTabId: 'home',
      focusExisting: true,
    })
    expect(mocks.setAppPath).not.toHaveBeenCalled()
  })

  it('opens Documentation through provider Shell Tab semantics from home', () => {
    mocks.appPath = '/'
    render(<BuiltinCommands />)

    findCommand('spacewave.help.docs')?.handler({})

    expect(mocks.openPathInNewTab).toHaveBeenCalledWith('/docs', {
      afterTabId: 'home',
      focusExisting: true,
    })
    expect(mocks.setAppPath).not.toHaveBeenCalled()
  })
})
