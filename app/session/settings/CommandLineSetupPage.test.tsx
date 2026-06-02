import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  DesktopCLIInstallActionKind,
  DesktopCLIInstallStatus,
  type DesktopCLIInstallState,
} from '@go/github.com/s4wave/spacewave/bldr/web/electron/desktop-runtime/desktop-runtime.pb.js'

import {
  DesktopCLIInstallCard,
  WalkthroughSection,
} from './CommandLineSetupPage.js'

describe('DesktopCLIInstallCard', () => {
  afterEach(() => cleanup())

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
