import { describe, expect, it, vi } from 'vitest'

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

    await resource.OpenOrFocusMainWindow({})
    expect(openOrFocusMainWindow).toHaveBeenCalledTimes(1)

    await resource.QuitDesktopRuntime({})
    expect(quitDesktopRuntime).toHaveBeenCalledTimes(1)
    expect(resource.getState()).toMatchObject({ quitting: true })
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
