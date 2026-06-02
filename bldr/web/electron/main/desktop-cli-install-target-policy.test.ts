import { describe, expect, it } from 'vitest'

import { DesktopCLIInstallTargetPathState } from '../desktop-runtime/desktop-runtime.pb.js'
import {
  blockedTargetReason,
  buildDesktopCLIInstallTargets,
  targetPathState,
} from './desktop-cli-install-target-policy.js'

describe('desktop CLI install target policy', () => {
  it('builds conservative macOS and Linux user-level candidates from process PATH evidence', async () => {
    await expect(
      buildDesktopCLIInstallTargets({
        homeDir: '/Users/test',
        platformId: 'desktop/darwin/arm64',
        pathEntries: ['/Users/test/bin'],
        canWrite: async () => true,
      }),
    ).resolves.toMatchObject([
      {
        id: 'home-bin',
        path: '/Users/test/bin/spacewave',
        pathState:
          DesktopCLIInstallTargetPathState.DESKTOP_CLI_INSTALL_TARGET_PATH_STATE_ON_PATH,
        detail: 'Detected on PATH',
        selected: true,
      },
      {
        id: 'home-local-bin',
        path: '/Users/test/.local/bin/spacewave',
        pathState:
          DesktopCLIInstallTargetPathState.DESKTOP_CLI_INSTALL_TARGET_PATH_STATE_OFF_PATH,
        detail: 'Manual PATH update needed',
      },
    ])

    await expect(
      buildDesktopCLIInstallTargets({
        homeDir: '/home/test',
        platformId: 'desktop/linux/amd64',
        pathEntries: [],
        canWrite: async () => true,
      }),
    ).resolves.toMatchObject([
      {
        id: 'home-local-bin',
        path: '/home/test/.local/bin/spacewave',
        pathState:
          DesktopCLIInstallTargetPathState.DESKTOP_CLI_INSTALL_TARGET_PATH_STATE_UNKNOWN,
        detail: 'PATH evidence unavailable',
      },
      {
        id: 'home-bin',
        path: '/home/test/bin/spacewave',
      },
    ])
  })

  it('builds Windows user-level exe targets and rejects system/package-manager prefixes', async () => {
    await expect(
      buildDesktopCLIInstallTargets({
        homeDir: 'C:\\Users\\test',
        platformId: 'desktop/windows/amd64',
        pathEntries: ['C:\\Users\\test\\AppData\\Local\\Programs\\Spacewave'],
        canWrite: async () => true,
      }),
    ).resolves.toMatchObject([
      {
        id: 'local-app-data',
        path: 'C:\\Users\\test\\AppData\\Local\\Programs\\Spacewave\\spacewave.exe',
        pathState:
          DesktopCLIInstallTargetPathState.DESKTOP_CLI_INSTALL_TARGET_PATH_STATE_ON_PATH,
      },
    ])

    expect(
      blockedTargetReason('/usr/local/bin/spacewave', 'desktop/darwin/arm64'),
    ).toBe('System prefix')
    expect(
      blockedTargetReason(
        '/opt/homebrew/bin/spacewave',
        'desktop/darwin/arm64',
      ),
    ).toBe('Package-manager prefix')
    expect(
      blockedTargetReason(
        'C:/Users/test/bin/spacewave',
        'desktop/windows/amd64',
      ),
    ).toBe('Windows CLI target must end in .exe')
  })

  it('classifies PATH evidence without shell probes', () => {
    expect(
      targetPathState('/Users/test/bin/spacewave', ['/Users/test/bin']),
    ).toBe(
      DesktopCLIInstallTargetPathState.DESKTOP_CLI_INSTALL_TARGET_PATH_STATE_ON_PATH,
    )
    expect(
      targetPathState('/Users/test/bin/spacewave', ['/Users/test/.local/bin']),
    ).toBe(
      DesktopCLIInstallTargetPathState.DESKTOP_CLI_INSTALL_TARGET_PATH_STATE_OFF_PATH,
    )
    expect(targetPathState('/Users/test/bin/spacewave', [])).toBe(
      DesktopCLIInstallTargetPathState.DESKTOP_CLI_INSTALL_TARGET_PATH_STATE_UNKNOWN,
    )
  })
})
