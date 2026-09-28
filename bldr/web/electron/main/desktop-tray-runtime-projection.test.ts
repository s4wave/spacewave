import { describe, expect, it } from 'vitest'

import {
  DesktopTrayActionKind,
  DesktopTrayEntryKind,
} from '@go/github.com/s4wave/spacewave/bldr/desktop/tray/tray.pb.js'
import { DesktopCLIInstallStatus } from '../desktop-runtime/desktop-runtime.pb.js'
import {
  buildDesktopTrayCLIInstallEntries,
  buildDesktopTrayEntriesFromRuntimeState,
  desktopRuntimeCLISettingsRoute,
} from './desktop-tray-runtime-projection.js'

describe('desktop tray runtime projection', () => {
  it('opens the app for installation instead of invoking a daemon update handler', () => {
    const entries = buildDesktopTrayEntriesFromRuntimeState({
      update: { ready: true, version: '1.2.3' },
    })
    expect(entries.find((entry) => entry.id === 'apply-update')).toMatchObject({
      label: 'Open app to install update',
      action: { kind: DesktopTrayActionKind.OPEN_ROUTE },
    })
  })

  it('routes CLI install summary settings to the supplied session route', () => {
    const entries = buildDesktopTrayCLIInstallEntries({
      status: DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_MISSING,
      label: 'Command line tool not installed',
      route: '/u/2/settings/cli',
    })
    const settings = entries.find(
      (entry) => entry.id === 'cli-install-settings',
    )

    expect(settings).toMatchObject({
      kind: DesktopTrayEntryKind.ACTION,
      action: {
        route: '/u/2/settings/cli',
      },
    })
  })

  it('derives command line settings from the active session route', () => {
    expect(
      desktopRuntimeCLISettingsRoute({
        sessions: [
          { label: 'first', route: '/u/1/' },
          { label: 'second', route: '/u/2/', active: true },
        ],
      }),
    ).toBe('/u/2/settings/cli')
    expect(desktopRuntimeCLISettingsRoute({ sessions: [] })).toBe('')
  })
})
