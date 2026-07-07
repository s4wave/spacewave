import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  DesktopCLIInstallActionKind,
  DesktopCLIInstallStatus,
  type DesktopCLIInstallState,
} from '@go/github.com/s4wave/spacewave/bldr/web/electron/desktop-runtime/desktop-runtime.pb.js'

const commandLinePageMocks = vi.hoisted(() => ({
  isDesktop: false,
  activeTabId: 'tab-settings',
  openPathInActiveTabset: vi.fn(),
  openPathInNewTab: vi.fn(),
  navigate: vi.fn(),
  sessionIndex: 7,
  listenerStatus: {
    listening: true,
    socketPath: '/run/spacewave-session-7.sock',
    connectedClients: 2,
  },
  rootResource: { value: null },
  cliInstallResource: {
    value: undefined as { state?: DesktopCLIInstallState } | undefined,
    loading: false,
    error: null as Error | null,
  },
  runtimeHandoff: { active: false, requesterName: '' },
  stateAtomSetter: vi.fn(),
}))

vi.mock('@aptre/bldr', () => ({
  get isDesktop() {
    return commandLinePageMocks.isDesktop
  },
}))

vi.mock('@aptre/bldr-sdk/hooks/useStreamingResource.js', () => ({
  useStreamingResource: () => commandLinePageMocks.cliInstallResource,
}))

vi.mock('@s4wave/app/hooks/useListenerStatus.js', () => ({
  useListenerStatus: () => commandLinePageMocks.listenerStatus,
}))

vi.mock('@s4wave/app/listener/RuntimeHandoffContext.js', () => ({
  useRuntimeHandoff: () => commandLinePageMocks.runtimeHandoff,
}))

vi.mock('@s4wave/app/prerender/StaticContext.js', () => ({
  useStaticHref: (path: string) => path,
}))

vi.mock('@s4wave/app/ShellTabContext.js', () => ({
  useShellTabs: () => ({
    activeTabId: commandLinePageMocks.activeTabId,
    openPathInActiveTabset: commandLinePageMocks.openPathInActiveTabset,
    openPathInNewTab: commandLinePageMocks.openPathInNewTab,
  }),
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  useSessionIndex: () => commandLinePageMocks.sessionIndex,
}))

vi.mock('@s4wave/web/hooks/useRootResource.js', () => ({
  useRootResource: () => commandLinePageMocks.rootResource,
  useRootResourceWithClient: () => commandLinePageMocks.rootResource,
}))

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => commandLinePageMocks.navigate,
}))

vi.mock('@s4wave/web/state/persist.js', () => ({
  useStateNamespace: (parts: string[]) => parts.join('/'),
  useStateAtom: () => [false, commandLinePageMocks.stateAtomSetter],
}))

import {
  CommandLineSetupPage,
  DesktopCLIInstallCard,
  WalkthroughSection,
} from './CommandLineSetupPage.js'

describe('DesktopCLIInstallCard', () => {
  afterEach(() => cleanup())
  beforeEach(() => {
    commandLinePageMocks.isDesktop = false
    commandLinePageMocks.activeTabId = 'tab-settings'
    commandLinePageMocks.openPathInActiveTabset.mockReset()
    commandLinePageMocks.openPathInNewTab.mockReset()
    commandLinePageMocks.navigate.mockReset()
    commandLinePageMocks.sessionIndex = 7
    commandLinePageMocks.listenerStatus = {
      listening: true,
      socketPath: '/run/spacewave-session-7.sock',
      connectedClients: 2,
    }
    commandLinePageMocks.rootResource = { value: null }
    commandLinePageMocks.cliInstallResource = {
      value: { state: state({}) },
      loading: false,
      error: null,
    }
    commandLinePageMocks.runtimeHandoff = { active: false, requesterName: '' }
    commandLinePageMocks.stateAtomSetter.mockReset()
  })

  it('renders missing CLI target and release identity from resource state', () => {
    render(
      <DesktopCLIInstallCard
        state={state({
          status: DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_MISSING,
          label: 'Command line tool not installed',
        })}
      />,
    )

    expect(screen.getByText('Desktop CLI install')).toBeDefined()
    expect(screen.getByText('Command line tool not installed')).toBeDefined()
    expect(screen.getByText('/Users/test/bin/spacewave')).toBeDefined()
    expect(
      screen.getByText('spacewave-cli rev 9 (desktop/darwin/arm64)'),
    ).toBeDefined()
  })

  it('maps unimplemented desktop install resource errors to unavailable copy', () => {
    render(<DesktopCLIInstallCard error={new Error('unimplemented')} />)

    expect(screen.getByText('Desktop CLI install unavailable')).toBeDefined()
    expect(
      screen.getByText(
        'This desktop build has not exposed managed CLI install yet. You can still use the in-app terminal below.',
      ),
    ).toBeDefined()
    expect(screen.queryByText('unimplemented')).toBeNull()
  })

  it('renders update, conflict, failure, and progress states without local PATH reconstruction', () => {
    const { rerender } = render(
      <DesktopCLIInstallCard
        state={state({
          status:
            DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_UPDATE_AVAILABLE,
          label: 'Command line tool update available',
          installedRev: 8n,
        })}
      />,
    )
    expect(screen.getByText('Command line tool update available')).toBeDefined()
    expect(
      screen.getByText('Installed spacewave-cli rev 8 (desktop/darwin/arm64)'),
    ).toBeDefined()

    rerender(
      <DesktopCLIInstallCard
        state={state({
          status: DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_CONFLICT,
          label: 'Command line tool conflict',
          conflictPath: '/opt/homebrew/bin/spacewave',
        })}
      />,
    )
    expect(screen.getByText('PATH conflict')).toBeDefined()
    expect(screen.getByText('/opt/homebrew/bin/spacewave')).toBeDefined()

    rerender(
      <DesktopCLIInstallCard
        state={state({
          status: DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_ERROR,
          label: 'Command line tool check failed',
          errorMessage: 'version probe failed',
        })}
      />,
    )
    expect(screen.getByText('version probe failed')).toBeDefined()

    rerender(
      <DesktopCLIInstallCard
        state={state({
          status: DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_UPDATING,
          label: 'Updating command line tool',
          detail: 'Replacing managed binary.',
        })}
      />,
    )
    expect(screen.getByText('Updating command line tool')).toBeDefined()
    expect(screen.getByText('Replacing managed binary.')).toBeDefined()
  })

  it('invokes only resource-owned action rows', () => {
    const onInvokeAction = vi.fn()
    render(
      <DesktopCLIInstallCard
        state={state({})}
        onInvokeAction={onInvokeAction}
      />,
    )

    fireEvent.click(screen.getByLabelText('Check again'))
    fireEvent.click(screen.getByLabelText('Settings'))

    expect(onInvokeAction).toHaveBeenCalledTimes(2)
    expect(onInvokeAction.mock.calls[0][0]).toMatchObject({
      id: 'recheck',
      generation: 3n,
    })
    expect(onInvokeAction.mock.calls[1][0]).toMatchObject({
      id: 'open-settings',
      generation: 3n,
    })
  })

  it('invokes resource-owned target selection actions', () => {
    const onInvokeAction = vi.fn()
    const cliState = state({})
    cliState.targets?.push({
      id: 'home-local-bin',
      label: 'Local user bin',
      path: '/Users/test/.local/bin/spacewave',
      writable: true,
      selected: false,
      generation: 3n,
      detail: 'Manual PATH update needed',
    })
    cliState.actions?.push({
      id: 'select-target:home-local-bin',
      kind: DesktopCLIInstallActionKind.DESKTOP_CLI_INSTALL_ACTION_KIND_SELECT_TARGET,
      label: 'Use Local user bin',
      enabled: true,
      targetId: 'home-local-bin',
      generation: 3n,
    })
    render(
      <DesktopCLIInstallCard
        state={cliState}
        onInvokeAction={onInvokeAction}
      />,
    )

    expect(screen.getByText('Install target')).toBeDefined()
    fireEvent.click(screen.getByLabelText('Use Local user bin'))

    expect(onInvokeAction).toHaveBeenCalledTimes(1)
    expect(onInvokeAction.mock.calls[0][0]).toMatchObject({
      id: 'select-target:home-local-bin',
      kind: DesktopCLIInstallActionKind.DESKTOP_CLI_INSTALL_ACTION_KIND_SELECT_TARGET,
      targetId: 'home-local-bin',
      generation: 3n,
    })
  })

  it('keeps the session-local walkthrough bound to selected session socket options', () => {
    render(
      <WalkthroughSection
        opts={{
          sessionIndex: 4,
          socketPath: '/run/spacewave-session-4.sock',
        }}
      />,
    )

    expect(screen.getByText('Try it out')).toBeDefined()
    expect(
      screen.getByText(
        "spacewave --socket-path '/run/spacewave-session-4.sock' --session-index 4 status",
      ),
    ).toBeDefined()
    expect(
      screen.getByText(
        "spacewave --socket-path '/run/spacewave-session-4.sock' --session-index 4 whoami",
      ),
    ).toBeDefined()
    expect(
      screen.getByText(
        "spacewave --socket-path '/run/spacewave-session-4.sock' --session-index 4 space list",
      ),
    ).toBeDefined()
  })

  it('opens the session-local in-app terminal in browser composition without desktop install state', () => {
    commandLinePageMocks.isDesktop = false
    commandLinePageMocks.cliInstallResource = {
      value: undefined,
      loading: false,
      error: new Error('desktop install state should not be required'),
    }

    render(<CommandLineSetupPage />)

    expect(screen.getByRole('heading', { name: 'Command Line' })).toBeDefined()
    expect(
      screen.getByText(
        'Run the Spacewave CLI in this browser tab without installing a desktop command first.',
      ),
    ).toBeDefined()
    expect(screen.queryByText('Desktop CLI install')).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: 'Open CLI terminal' }))

    expect(commandLinePageMocks.openPathInActiveTabset).toHaveBeenCalledWith(
      '/u/7/settings/cli/terminal',
      {
        afterTabId: 'tab-settings',
        focusExisting: true,
        select: true,
      },
    )
    expect(commandLinePageMocks.openPathInNewTab).not.toHaveBeenCalled()
  })

  it('keeps desktop install affordances while launching the same in-app terminal path', () => {
    commandLinePageMocks.isDesktop = true

    render(<CommandLineSetupPage />)

    expect(screen.getByText('Desktop CLI install')).toBeDefined()
    expect(screen.getByText('Ready')).toBeDefined()
    expect(screen.getByText('/run/spacewave-session-7.sock')).toBeDefined()
    expect(screen.getByText('Try it out')).toBeDefined()

    fireEvent.click(screen.getByRole('button', { name: 'Open CLI terminal' }))

    expect(commandLinePageMocks.openPathInActiveTabset).toHaveBeenCalledWith(
      '/u/7/settings/cli/terminal',
      {
        afterTabId: 'tab-settings',
        focusExisting: true,
        select: true,
      },
    )
    expect(commandLinePageMocks.openPathInNewTab).not.toHaveBeenCalled()
  })
})

function state(opts: {
  status?: DesktopCLIInstallStatus
  label?: string
  detail?: string
  installedRev?: bigint
  conflictPath?: string
  errorMessage?: string
}): DesktopCLIInstallState {
  return {
    status:
      opts.status ??
      DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_INSTALLED,
    label: opts.label ?? 'Command line tool installed',
    detail: opts.detail ?? '',
    generation: 3n,
    selectedTargetId: 'home-bin',
    installed: opts.installedRev
      ? {
          projectId: 'spacewave',
          entrypointRole: 'cli',
          channelKey: 'stable',
          manifestId: 'spacewave-cli',
          manifestRev: opts.installedRev,
          platformId: 'desktop/darwin/arm64',
          path: '/Users/test/bin/spacewave',
        }
      : {},
    available: {
      projectId: 'spacewave',
      entrypointRole: 'cli',
      channelKey: 'stable',
      manifestId: 'spacewave-cli',
      manifestRev: 9n,
      platformId: 'desktop/darwin/arm64',
    },
    targets: [
      {
        id: 'home-bin',
        label: 'User bin',
        path: '/Users/test/bin/spacewave',
        writable: true,
        selected: true,
        generation: 3n,
      },
    ],
    conflictPath: opts.conflictPath ?? '',
    errorMessage: opts.errorMessage ?? '',
    actions: [
      {
        id: 'recheck',
        kind: DesktopCLIInstallActionKind.DESKTOP_CLI_INSTALL_ACTION_KIND_RECHECK,
        label: 'Check again',
        enabled: true,
        generation: 3n,
      },
      {
        id: 'open-settings',
        kind: DesktopCLIInstallActionKind.DESKTOP_CLI_INSTALL_ACTION_KIND_OPEN_SETTINGS,
        label: 'Settings',
        enabled: true,
        generation: 3n,
      },
    ],
  }
}
