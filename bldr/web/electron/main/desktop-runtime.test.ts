import { describe, expect, it, vi } from 'vitest'

import {
  DesktopCLIInstallActionKind,
  DesktopCLIInstallStatus,
  DesktopRuntimeActivityState,
  DesktopRuntimeActionKind,
  DesktopRuntimeAttentionKind,
  DesktopRuntimeHealth,
  DesktopRuntimeLifecycle,
  DesktopRuntimeReachability,
  DesktopRuntimeSeverity,
} from '../desktop-runtime/desktop-runtime.pb.js'
import { DesktopRuntimeResource } from './desktop-runtime.js'
import { detectDesktopCLIInstallState } from './desktop-cli-install-detector.js'

describe('DesktopRuntimeResource', () => {
  it('streams initial state and window presence changes', async () => {
    const resource = new DesktopRuntimeResource({
      openOrFocusMainWindow: vi.fn(),
      quitDesktopRuntime: vi.fn(),
    })
    const iter = resource.WatchDesktopState({})[Symbol.asyncIterator]()

    await expect(iter.next()).resolves.toMatchObject({
      value: {
        state: {
          mainWindowOpen: false,
          quitting: false,
          statusText: 'Running',
          health: DesktopRuntimeHealth.HEALTHY,
          lifecycle: DesktopRuntimeLifecycle.RUNNING,
          listener: {
            reachability: DesktopRuntimeReachability.UNSPECIFIED,
          },
          sessions: [],
          spaces: [],
          activity: [],
          update: {},
          attentionItems: [],
          actions: [],
        },
      },
      done: false,
    })

    resource.setMainWindowOpen(true)

    await expect(iter.next()).resolves.toMatchObject({
      value: {
        state: {
          mainWindowOpen: true,
          quitting: false,
          statusText: 'Running',
          health: DesktopRuntimeHealth.HEALTHY,
          lifecycle: DesktopRuntimeLifecycle.RUNNING,
        },
      },
      done: false,
    })
    await iter.return?.()
  })

  it('streams an expanded daemon-console status projection', async () => {
    const resource = new DesktopRuntimeResource({
      openOrFocusMainWindow: vi.fn(),
      quitDesktopRuntime: vi.fn(),
    })
    const iter = resource.WatchDesktopState({})[Symbol.asyncIterator]()
    await iter.next()

    resource.setDesktopState({
      mainWindowOpen: true,
      quitting: false,
      statusText: 'Syncing',
      health: DesktopRuntimeHealth.ACTIVE,
      lifecycle: DesktopRuntimeLifecycle.RUNNING,
      listener: {
        reachability: DesktopRuntimeReachability.REACHABLE,
        label: 'CLI reachable',
        socketPath: '/tmp/spacewave.sock',
        connectedClients: 2,
      },
      sessions: [
        {
          id: 'session-1',
          label: 'coolguy@spacewave.app',
          detail: 'Pro',
          route: '/sessions/session-1',
          active: true,
          statusText: 'Signed in',
        },
      ],
      spaces: [
        {
          id: 'space-1',
          label: 'Company',
          detail: 'Open',
          route: '/spaces/space-1',
          active: true,
        },
      ],
      activity: [
        {
          id: 'sync-1',
          label: 'Sync',
          detail: 'Pulling updates',
          state: DesktopRuntimeActivityState.RUNNING,
          updatedAtUnixMs: 1n,
        },
      ],
      update: {
        ready: true,
        version: '1.2.3',
        label: 'Update ready',
      },
      attentionItems: [
        {
          kind: DesktopRuntimeAttentionKind.AUTH_REQUIRED,
          severity: DesktopRuntimeSeverity.WARNING,
          label: 'Sign in required',
          route: '/sessions/session-1',
        },
      ],
      actions: [
        {
          id: 'open-settings',
          kind: DesktopRuntimeActionKind.OPEN_ROUTE,
          label: 'Settings',
          route: '/settings',
          enabled: true,
        },
      ],
    })

    await expect(iter.next()).resolves.toMatchObject({
      value: {
        state: {
          statusText: 'Syncing',
          health: DesktopRuntimeHealth.ACTIVE,
          listener: {
            reachability: DesktopRuntimeReachability.REACHABLE,
            socketPath: '/tmp/spacewave.sock',
            connectedClients: 2,
          },
          sessions: [{ id: 'session-1' }],
          spaces: [{ id: 'space-1' }],
          activity: [
            {
              id: 'sync-1',
              state: DesktopRuntimeActivityState.RUNNING,
              updatedAtUnixMs: 1n,
            },
          ],
          update: { ready: true, version: '1.2.3' },
          attentionItems: [
            {
              kind: DesktopRuntimeAttentionKind.AUTH_REQUIRED,
              severity: DesktopRuntimeSeverity.WARNING,
            },
          ],
          actions: [
            {
              id: 'open-settings',
              kind: DesktopRuntimeActionKind.OPEN_ROUTE,
              route: '/settings',
            },
          ],
        },
      },
      done: false,
    })

    const state = resource.getState()
    state.sessions?.push({ id: 'mutated' })
    state.actions?.push({ id: 'mutated' })
    expect(resource.getState().sessions).toHaveLength(1)
    expect(resource.getState().actions).toHaveLength(1)
    await iter.return?.()
  })

  it('publishes projected status without clobbering Electron-owned fields', async () => {
    const resource = new DesktopRuntimeResource({
      openOrFocusMainWindow: vi.fn(),
      quitDesktopRuntime: vi.fn(),
    })
    resource.setMainWindowOpen(true)

    await resource.SetDesktopState({
      state: {
        mainWindowOpen: false,
        quitting: false,
        statusText: 'CLI reachable',
        health: DesktopRuntimeHealth.HEALTHY,
        lifecycle: DesktopRuntimeLifecycle.RUNNING,
        listener: {
          reachability: DesktopRuntimeReachability.REACHABLE,
          label: 'CLI reachable',
          socketPath: '/tmp/spacewave.sock',
          connectedClients: 1,
        },
      },
    })

    expect(resource.getState()).toMatchObject({
      mainWindowOpen: true,
      quitting: false,
      statusText: 'CLI reachable',
      health: DesktopRuntimeHealth.HEALTHY,
      lifecycle: DesktopRuntimeLifecycle.RUNNING,
      listener: {
        reachability: DesktopRuntimeReachability.REACHABLE,
        connectedClients: 1,
      },
      sessions: [],
      spaces: [],
      activity: [],
      update: {},
      attentionItems: [],
      actions: [],
    })

    await resource.QuitDesktopRuntime({})
    await resource.SetDesktopState({
      state: {
        statusText: 'Running',
        health: DesktopRuntimeHealth.HEALTHY,
        lifecycle: DesktopRuntimeLifecycle.RUNNING,
      },
    })

    expect(resource.getState()).toMatchObject({
      mainWindowOpen: true,
      quitting: true,
      statusText: 'Quitting',
      health: DesktopRuntimeHealth.QUITTING,
      lifecycle: DesktopRuntimeLifecycle.QUITTING,
    })
  })

  it('suppresses redundant desktop state updates', async () => {
    const resource = new DesktopRuntimeResource({
      openOrFocusMainWindow: vi.fn(),
      quitDesktopRuntime: vi.fn(),
    })
    const iter = resource.WatchDesktopState({})[Symbol.asyncIterator]()
    await iter.next()

    const next = iter.next()
    resource.setDesktopState(resource.getState())
    resource.setMainWindowOpen(true)

    await expect(next).resolves.toMatchObject({
      value: {
        state: {
          mainWindowOpen: true,
        },
      },
      done: false,
    })
    await iter.return?.()
  })

  it('routes open/focus and explicit quit commands', async () => {
    const openOrFocusMainWindow = vi.fn()
    const quitDesktopRuntime = vi.fn()
    const resource = new DesktopRuntimeResource({
      openOrFocusMainWindow,
      quitDesktopRuntime,
    })

    await resource.OpenOrFocusMainWindow({ route: '/settings' })
    expect(openOrFocusMainWindow).toHaveBeenCalledTimes(1)
    expect(openOrFocusMainWindow).toHaveBeenCalledWith({ route: '/settings' })

    await resource.QuitDesktopRuntime({})
    expect(quitDesktopRuntime).toHaveBeenCalledTimes(1)
    expect(resource.getState()).toMatchObject({
      quitting: true,
      statusText: 'Quitting',
      health: DesktopRuntimeHealth.QUITTING,
      lifecycle: DesktopRuntimeLifecycle.QUITTING,
    })
  })

  it('owns a ResourceServer for the desktop runtime tree', () => {
    const resource = new DesktopRuntimeResource({
      openOrFocusMainWindow: vi.fn(),
      quitDesktopRuntime: vi.fn(),
    })

    expect(resource.resourceServer).toBeDefined()
  })

  it('owns desktop CLI install detection state as a child resource', async () => {
    const resource = new DesktopRuntimeResource({
      openOrFocusMainWindow: vi.fn(),
      quitDesktopRuntime: vi.fn(),
    })
    const iter = resource.desktopCLIInstallResource
      .WatchCLIInstallState({})
      [Symbol.asyncIterator]()

    await expect(iter.next()).resolves.toMatchObject({
      value: {
        state: {
          status: DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_UNKNOWN,
          label: 'Checking command line tool',
          generation: 1n,
          installed: {},
          available: {},
          targets: [],
          actions: [
            {
              id: 'recheck',
              kind: DesktopCLIInstallActionKind.DESKTOP_CLI_INSTALL_ACTION_KIND_RECHECK,
              generation: 1n,
            },
            {
              id: 'open-settings',
              kind: DesktopCLIInstallActionKind.DESKTOP_CLI_INSTALL_ACTION_KIND_OPEN_SETTINGS,
              generation: 1n,
            },
          ],
        },
      },
      done: false,
    })

    resource.desktopCLIInstallResource.setDetectedState({
      status: DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_INSTALLED,
      label: 'Command line tool installed',
      installed: {
        path: '/Users/test/bin/spacewave',
        projectId: 'spacewave',
        entrypointRole: 'cli',
        channelKey: 'stable',
        manifestId: 'spacewave-cli',
        manifestRev: 8n,
        platformId: 'desktop/darwin/arm64',
      },
      available: {
        projectId: 'spacewave',
        entrypointRole: 'cli',
        channelKey: 'stable',
        manifestId: 'spacewave-cli',
        manifestRev: 8n,
        platformId: 'desktop/darwin/arm64',
      },
      targets: [
        {
          id: 'user-bin',
          label: 'User bin',
          path: '/Users/test/bin/spacewave',
          writable: true,
          selected: true,
        },
      ],
      selectedTargetId: 'user-bin',
    })

    await expect(iter.next()).resolves.toMatchObject({
      value: {
        state: {
          status: DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_INSTALLED,
          generation: 2n,
          installed: {
            entrypointRole: 'cli',
            manifestId: 'spacewave-cli',
            manifestRev: 8n,
          },
          targets: [{ id: 'user-bin', selected: true, generation: 2n }],
          selectedTargetId: 'user-bin',
          actions: [
            { id: 'recheck', generation: 2n },
            { id: 'open-settings', generation: 2n },
          ],
        },
      },
      done: false,
    })

    const state = resource.desktopCLIInstallResource.getState()
    state.targets?.push({ id: 'mutated' })
    state.actions?.push({ id: 'mutated' })
    expect(resource.desktopCLIInstallResource.getState().targets).toHaveLength(
      1,
    )
    expect(resource.desktopCLIInstallResource.getState().actions).toHaveLength(
      2,
    )
    await expect(
      resource.desktopCLIInstallResource.InvokeCLIInstallAction({
        actionId: 'recheck',
        generation: 1n,
      }),
    ).rejects.toThrow('desktop CLI install action generation is stale')
    await iter.return?.()
  })

  it('refreshes CLI tray settings route when sessions arrive after CLI state', async () => {
    const resource = new DesktopRuntimeResource({
      openOrFocusMainWindow: vi.fn(),
      quitDesktopRuntime: vi.fn(),
    })
    resource.desktopCLIInstallResource.setDetectedState({
      status: DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_MISSING,
      label: 'Command line tool not installed',
      targets: [],
    })

    await waitForCLITraySettingsRoute(resource, '/')
    await resource.SetDesktopState({
      state: {
        statusText: 'Running',
        health: DesktopRuntimeHealth.HEALTHY,
        lifecycle: DesktopRuntimeLifecycle.RUNNING,
        listener: {
          reachability: DesktopRuntimeReachability.UNSPECIFIED,
        },
        sessions: [
          {
            id: 'session-2',
            label: 'coolguy@spacewave.app',
            route: '/u/2',
            active: true,
          },
        ],
      },
    })

    await waitForCLITraySettingsRoute(resource, '/u/2/settings/cli')
  })

  it('invokes only current-generation desktop CLI install actions', async () => {
    const openCLISettings = vi.fn()
    const detectCLIInstallState = vi.fn(
      async (selectedTargetId = 'home-bin') => ({
        status: DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_MISSING,
        label: 'Command line tool not installed',
        targets: [
          { id: 'home-bin', selected: selectedTargetId === 'home-bin' },
          {
            id: 'home-local-bin',
            selected: selectedTargetId === 'home-local-bin',
          },
        ],
      }),
    )
    const resource = new DesktopRuntimeResource({
      openOrFocusMainWindow: vi.fn(),
      quitDesktopRuntime: vi.fn(),
      desktopCLIInstall: {
        detectCLIInstallState,
        openCLISettings,
      },
    })

    await resource.desktopCLIInstallResource.InvokeCLIInstallAction({
      actionId: 'recheck',
      generation: 1n,
    })
    expect(detectCLIInstallState).toHaveBeenCalledTimes(1)
    expect(resource.desktopCLIInstallResource.getState()).toMatchObject({
      status: DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_MISSING,
      generation: 2n,
      selectedTargetId: 'home-bin',
      targets: [
        { id: 'home-bin', generation: 2n, selected: true },
        { id: 'home-local-bin', generation: 2n, selected: false },
      ],
      actions: [
        { id: 'recheck', generation: 2n },
        { id: 'open-settings', generation: 2n },
        {
          id: 'select-target:home-local-bin',
          kind: DesktopCLIInstallActionKind.DESKTOP_CLI_INSTALL_ACTION_KIND_SELECT_TARGET,
          targetId: 'home-local-bin',
          generation: 2n,
        },
      ],
    })

    await resource.desktopCLIInstallResource.InvokeCLIInstallAction({
      actionId: 'select-target:home-local-bin',
      generation: 2n,
    })
    expect(detectCLIInstallState).toHaveBeenLastCalledWith('home-local-bin')
    expect(resource.desktopCLIInstallResource.getState()).toMatchObject({
      generation: 3n,
      selectedTargetId: 'home-local-bin',
      targets: [
        { id: 'home-bin', generation: 3n, selected: false },
        { id: 'home-local-bin', generation: 3n, selected: true },
      ],
      actions: [
        { id: 'recheck', generation: 3n },
        { id: 'open-settings', generation: 3n },
        {
          id: 'select-target:home-bin',
          kind: DesktopCLIInstallActionKind.DESKTOP_CLI_INSTALL_ACTION_KIND_SELECT_TARGET,
          targetId: 'home-bin',
          generation: 3n,
        },
      ],
    })

    await resource.desktopCLIInstallResource.InvokeCLIInstallAction({
      actionId: 'open-settings',
      generation: 3n,
    })
    expect(openCLISettings).toHaveBeenCalledTimes(1)
  })

  it('runs user-level CLI install actions through the desktop resource owner', async () => {
    const targetPath = '/Users/test/bin/spacewave'
    const fs = new TestInstallFilesystem()
    const identity = {
      projectId: 'spacewave',
      entrypointRole: 'cli',
      channelKey: 'stable',
      manifestId: 'spacewave-cli',
      manifestRev: 9n,
      platformId: 'desktop/darwin/arm64',
    }
    const detectCLIInstallState = vi.fn(async () => {
      const installed = fs.exists(targetPath)
      return {
        status: installed
          ? DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_INSTALLED
          : DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_MISSING,
        label: installed
          ? 'Command line tool installed'
          : 'Command line tool not installed',
        installed: installed ? { ...identity, path: targetPath } : {},
        available: identity,
        targets: [
          {
            id: 'home-bin',
            path: targetPath,
            writable: true,
            selected: true,
          },
        ],
      }
    })
    const resource = new DesktopRuntimeResource({
      openOrFocusMainWindow: vi.fn(),
      quitDesktopRuntime: vi.fn(),
      desktopCLIInstall: {
        detectCLIInstallState,
        readReleaseBinary: async () => new TextEncoder().encode('managed-cli'),
        probe: {
          fileExists: async (path) => fs.exists(path),
          readEntrypointIdentity: async (path) => {
            if (!fs.exists(path)) return undefined
            return { ...identity, path }
          },
        },
        filesystem: fs,
        now: () => 11,
      },
    })

    await resource.desktopCLIInstallResource.InvokeCLIInstallAction({
      actionId: 'recheck',
      generation: 1n,
    })
    const installAction = resource.desktopCLIInstallResource
      .getState()
      .actions?.find((action) => action.id === 'install')
    expect(installAction).toMatchObject({
      kind: DesktopCLIInstallActionKind.DESKTOP_CLI_INSTALL_ACTION_KIND_INSTALL,
      targetId: 'home-bin',
    })

    await resource.desktopCLIInstallResource.InvokeCLIInstallAction({
      actionId: 'install',
      generation: installAction?.generation,
    })

    expect(new TextDecoder().decode(fs.files.get(targetPath))).toBe(
      'managed-cli',
    )
    expect(resource.desktopCLIInstallResource.getState()).toMatchObject({
      status: DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_INSTALLED,
      installed: {
        manifestId: 'spacewave-cli',
        manifestRev: 9n,
      },
    })
  })

  it('detects desktop CLI install state from injected native probes', async () => {
    const homeDir = '/Users/test'
    const targetPath = '/Users/test/bin/spacewave'
    const unmanagedPath = '/opt/homebrew/bin/spacewave'
    const available = {
      projectId: 'spacewave',
      entrypointRole: 'cli',
      channelKey: 'stable',
      manifestId: 'spacewave-cli',
      manifestRev: 9n,
      platformId: 'desktop/darwin/arm64',
    }
    const probe = {
      fileExists: vi.fn(async (candidate: string) => {
        return candidate === targetPath || candidate === unmanagedPath
      }),
      targetWritable: vi.fn(async (candidate: string) => {
        return candidate === targetPath
      }),
      readEntrypointIdentity: vi.fn(async (candidate: string) => {
        if (candidate === targetPath) {
          return {
            path: candidate,
            projectId: 'spacewave',
            entrypointRole: 'cli',
            channelKey: 'stable',
            manifestId: 'spacewave-cli',
            manifestRev: 8n,
            platformId: 'desktop/darwin/arm64',
          }
        }
        return {
          path: candidate,
          entrypointRole: 'standalone',
          platformId: 'desktop/darwin/arm64',
        }
      }),
    }

    await expect(
      detectDesktopCLIInstallState({
        homeDir,
        pathEntries: ['/opt/homebrew/bin', '/Users/test/bin'],
        platformId: 'desktop/darwin/arm64',
        available,
        probe,
      }),
    ).resolves.toMatchObject({
      status: DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_CONFLICT,
      conflictPath: unmanagedPath,
      selectedTargetId: 'home-bin',
      installed: {
        manifestId: 'spacewave-cli',
        manifestRev: 8n,
      },
      available: {
        manifestRev: 9n,
      },
      targets: [
        { id: 'home-bin', writable: true, selected: true },
        { id: 'home-local-bin', writable: false, selected: false },
      ],
    })

    probe.fileExists = vi.fn(async (candidate: string) => {
      return candidate === targetPath
    })
    await expect(
      detectDesktopCLIInstallState({
        homeDir,
        pathEntries: ['/Users/test/bin'],
        platformId: 'desktop/darwin/arm64',
        available,
        probe,
      }),
    ).resolves.toMatchObject({
      status:
        DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_UPDATE_AVAILABLE,
      selectedTargetId: 'home-bin',
      actions: [
        {
          id: 'recheck',
          kind: DesktopCLIInstallActionKind.DESKTOP_CLI_INSTALL_ACTION_KIND_RECHECK,
        },
        {
          id: 'open-settings',
          kind: DesktopCLIInstallActionKind.DESKTOP_CLI_INSTALL_ACTION_KIND_OPEN_SETTINGS,
        },
        {
          id: 'select-target:home-local-bin',
          kind: DesktopCLIInstallActionKind.DESKTOP_CLI_INSTALL_ACTION_KIND_SELECT_TARGET,
          targetId: 'home-local-bin',
        },
      ],
    })
  })

  it('detects unmanaged Windows spacewave.exe before the selected managed target', async () => {
    const homeDir = 'C:\\Users\\test'
    const unmanagedPath = 'C:\\Tools\\spacewave.exe'
    const targetPath =
      'C:\\Users\\test\\AppData\\Local\\Programs\\Spacewave\\spacewave.exe'
    const platformId = 'desktop/windows/amd64'
    const probe = {
      fileExists: vi.fn(async (candidate: string) => {
        return candidate === unmanagedPath || candidate === targetPath
      }),
      targetWritable: vi.fn(async (candidate: string) => {
        return candidate === targetPath
      }),
      readEntrypointIdentity: vi.fn(async (candidate: string) => {
        if (candidate === targetPath) {
          return {
            path: candidate,
            projectId: 'spacewave',
            entrypointRole: 'cli',
            channelKey: 'stable',
            manifestId: 'spacewave-cli',
            manifestRev: 9n,
            platformId,
          }
        }
        return {
          path: candidate,
          entrypointRole: 'standalone',
          platformId,
        }
      }),
    }

    await expect(
      detectDesktopCLIInstallState({
        homeDir,
        pathEntries: [
          'C:\\Tools',
          'C:\\Users\\test\\AppData\\Local\\Programs\\Spacewave',
        ],
        platformId,
        probe,
      }),
    ).resolves.toMatchObject({
      status: DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_CONFLICT,
      conflictPath: unmanagedPath,
      selectedTargetId: 'local-app-data',
      installed: {
        manifestId: 'spacewave-cli',
        manifestRev: 9n,
      },
      targets: [
        {
          id: 'local-app-data',
          path: targetPath,
          writable: true,
          selected: true,
        },
      ],
    })
    expect(probe.fileExists).toHaveBeenNthCalledWith(1, unmanagedPath)
  })

  it('releases ResourceServer client sessions without changing desktop state', async () => {
    const resource = new DesktopRuntimeResource({
      openOrFocusMainWindow: vi.fn(),
      quitDesktopRuntime: vi.fn(),
    })
    const controller = new AbortController()
    const iter = resource.resourceServer
      .ResourceClient(
        (async function* () {
          yield { body: { case: 'init' as const, value: {} } }
        })(),
        controller.signal,
      )
      [Symbol.asyncIterator]()

    await expect(iter.next()).resolves.toMatchObject({
      value: {
        body: {
          case: 'init',
          value: {
            clientHandleId: 1,
            rootResourceId: 1,
          },
        },
      },
      done: false,
    })

    controller.abort()

    await expect(iter.next()).resolves.toMatchObject({ done: true })
    expect(resource.getState()).toMatchObject({
      mainWindowOpen: false,
      quitting: false,
    })
  })
})

class TestInstallFilesystem {
  public readonly files = new Map<string, Uint8Array>()
  public readonly modes = new Map<string, number>()

  public exists(path: string): boolean {
    return this.files.has(path)
  }

  public async readFile(path: string): Promise<Uint8Array> {
    const data = this.files.get(path)
    if (!data) throw new Error('not found')
    return data
  }

  public async writeFileExclusive(
    path: string,
    data: Uint8Array,
  ): Promise<void> {
    if (this.files.has(path)) throw new Error('exists')
    this.files.set(path, new Uint8Array(data))
  }

  public async rename(oldPath: string, newPath: string): Promise<void> {
    const data = this.files.get(oldPath)
    if (!data) throw new Error('not found')
    this.files.set(newPath, data)
    this.files.delete(oldPath)
  }

  public async mkdir(_path: string): Promise<void> {}

  public async chmod(path: string, mode: number): Promise<void> {
    this.modes.set(path, mode)
  }

  public async remove(path: string): Promise<void> {
    this.files.delete(path)
    this.modes.delete(path)
  }

  public async pathKind(
    path: string,
  ): Promise<'missing' | 'file' | 'symlink' | 'other'> {
    return this.files.has(path) ? 'file' : 'missing'
  }
}

async function waitForCLITraySettingsRoute(
  resource: DesktopRuntimeResource,
  want: string,
  attempts = 20,
): Promise<void> {
  const route =
    resource.desktopTrayResource
      .getState()
      .entries?.find((entry) => entry.id === 'cli-install-settings')?.action
      ?.route ?? ''
  if (route === want) {
    return
  }
  if (attempts <= 0) {
    expect(route).toBe(want)
    return
  }
  await new Promise((resolve) => setTimeout(resolve, 0))
  await waitForCLITraySettingsRoute(resource, want, attempts - 1)
}
