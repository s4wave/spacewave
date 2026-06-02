import { describe, expect, it } from 'vitest'

import { DesktopTrayEntryKind } from '@go/github.com/s4wave/spacewave/bldr/desktop/tray/tray.pb.js'
import { DesktopCLIInstallStatus } from '../desktop-runtime/desktop-runtime.pb.js'
import {
  buildDesktopTrayCLIInstallEntries,
  desktopRuntimeCLISettingsRoute,
} from './desktop-tray-runtime-projection.js'

describe('desktop tray runtime projection', () => {
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
