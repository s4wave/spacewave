import { cleanup, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

interface RegisteredCommand {
  commandId: string
  label: string
  description?: string
  menuPath?: string
  menuGroup?: number
  menuOrder?: number
  active?: boolean
  handler: (args: Record<string, string>) => void
}

interface ShellTabOpenOptions {
  afterTabId?: string
  focusExisting: boolean
  select?: boolean
}

const h = vi.hoisted(() => ({
  registeredCommands: [] as RegisteredCommand[],
  navigate: vi.fn(),
  navigateSession: vi.fn(),
  setOpenMenu: vi.fn(),
  openPathInNewTab: vi.fn<(path: string, opts: ShellTabOpenOptions) => void>(),
  openPathInActiveTabset:
    vi.fn<(path: string, opts: ShellTabOpenOptions) => void>(),
  sessionResource: { value: {} },
  resourcesList: { spacesList: [] },
}))

vi.mock('@aptre/bldr-react', () => ({
  useWatchStateRpc: () => h.resourcesList,
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResourceValue: (resource: { value?: unknown } | undefined) =>
    resource?.value,
}))

vi.mock('@s4wave/web/command/useCommand.js', () => ({
  useCommand: (opts: RegisteredCommand) => {
    h.registeredCommands.push(opts)
  },
}))

vi.mock('@s4wave/web/contexts/TabActiveContext.js', () => ({
  useIsTabActive: () => true,
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  SessionContext: {
    useContext: () => h.sessionResource,
  },
  useSessionIndex: () => 7,
  useSessionNavigate: () => h.navigateSession,
}))

vi.mock('@s4wave/web/frame/bottom-bar-context.js', () => ({
  useBottomBarSetOpenMenu: () => h.setOpenMenu,
}))

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => h.navigate,
}))

vi.mock('@s4wave/app/ShellTabContext.js', () => ({
  useShellTabs: () => ({
    activeTabId: 'tab-session',
    openPathInNewTab: h.openPathInNewTab,
    openPathInActiveTabset: h.openPathInActiveTabset,
  }),
}))

import { SessionCommands } from './SessionCommands.js'

describe('SessionCommands', () => {
  beforeEach(() => {
    h.registeredCommands.length = 0
    h.navigate.mockReset()
    h.navigateSession.mockReset()
    h.setOpenMenu.mockReset()
    h.openPathInNewTab.mockReset()
    h.openPathInActiveTabset.mockReset()
    h.sessionResource = { value: {} }
    h.resourcesList = { spacesList: [] }
  })

  afterEach(() => cleanup())

  it('opens File/Open CLI Terminal in the active shell tabset for the current session', () => {
    render(<SessionCommands />)

    const command = h.registeredCommands.find(
      (item) => item.commandId === 'spacewave.session.open-cli-terminal',
    )
    expect(command).toMatchObject({
      label: 'Open CLI terminal',
      menuPath: 'File/Open CLI Terminal',
      active: true,
    })

    command?.handler({})

    expect(h.openPathInActiveTabset).toHaveBeenCalledWith(
      '/u/7/settings/cli/terminal',
      {
        afterTabId: 'tab-session',
        focusExisting: true,
        select: true,
      },
    )
    expect(h.openPathInNewTab).not.toHaveBeenCalled()
  })
})
