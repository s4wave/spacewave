import { describe, expect, it, vi } from 'vitest'

import {
  DesktopRuntimeActivityState,
  DesktopRuntimeAttentionKind,
  DesktopRuntimeHealth,
  DesktopRuntimeLifecycle,
  DesktopRuntimeReachability,
  DesktopRuntimeSeverity,
} from '../desktop-runtime/desktop-runtime.pb.js'
import { DesktopRuntimeResource } from './desktop-runtime.js'

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
          label: 'christian@aperture.us',
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
        },
      },
      done: false,
    })

    const state = resource.getState()
    state.sessions?.push({ id: 'mutated' })
    expect(resource.getState().sessions).toHaveLength(1)
    await iter.return?.()
  })

  it('routes open/focus and explicit quit commands', async () => {
    const openOrFocusMainWindow = vi.fn()
    const quitDesktopRuntime = vi.fn()
    const resource = new DesktopRuntimeResource({
      openOrFocusMainWindow,
      quitDesktopRuntime,
    })

    await resource.OpenOrFocusMainWindow({})
    expect(openOrFocusMainWindow).toHaveBeenCalledTimes(1)

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

  it('releases ResourceServer client sessions without changing desktop state', async () => {
    const resource = new DesktopRuntimeResource({
      openOrFocusMainWindow: vi.fn(),
      quitDesktopRuntime: vi.fn(),
    })
    const controller = new AbortController()
    const iter = resource.resourceServer
      .ResourceClient({}, controller.signal)
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
