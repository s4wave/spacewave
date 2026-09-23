import { act, renderHook, waitFor } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { pushable } from 'it-pushable'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { Space } from '@s4wave/sdk/space/space.js'
import type { PluginFrontend } from '@s4wave/sdk/space/plugin-frontend.js'
import { Event } from '@go/github.com/s4wave/spacewave/bldr/frontend/frontend.pb.js'

import { usePluginFrontend } from './usePluginFrontend.js'

vi.mock('@aptre/bldr', async () => ({
  FrontendResource: (await import('../../bldr/web/bldr/frontend.js'))
    .FrontendResource,
}))

const request = {
  sourceKey: 'projects/colors',
  deviceKey: 'devices/build',
  manifestId: 'colors',
}

/** attachment supplies the SDK boundary while retaining the real browser transport. */
function attachment(id: string) {
  const updates = pushable<Event>({ objectMode: true })
  const release = vi.fn()
  const grant = {
    executionKey: `builds/${id}`,
    [Symbol.dispose]: release,
    frontend: {
      async *Watch(_: unknown, signal?: AbortSignal) {
        const abort = () => updates.end()
        signal?.addEventListener('abort', abort, { once: true })
        try {
          yield Event.create({
            session: {
              id,
              routePrefix: `/b/fe/${id}/`,
              entrypoints: ['Viewer.tsx'],
            },
          })
          yield* updates
        } finally {
          signal?.removeEventListener('abort', abort)
          updates.end()
        }
      },
      Send: async () => ({}),
    },
  } as unknown as PluginFrontend
  return { grant, release, updates }
}

/** parent gives the hook a mounted Space with a controlled attachment RPC. */
function parent(open: () => Promise<PluginFrontend>): Resource<Space> {
  return {
    value: { openPluginFrontend: open } as unknown as Space,
    loading: false,
    error: null,
    retry: vi.fn(),
  }
}

afterEach(() => {
  globalThis.__bldrFrontendEnabled = undefined
})

it.each(['disconnect', 'reload'])(
  'keeps authoring views independent after %s',
  async (failure) => {
    globalThis.__bldrFrontendEnabled = true
    const first = attachment('first')
    const second = attachment('second')
    const firstParent = parent(async () => first.grant)
    const secondParent = parent(async () => second.grant)
    const left = renderHook(() => usePluginFrontend(firstParent, request))
    const right = renderHook(() => usePluginFrontend(secondParent, request))
    await waitFor(() =>
      expect(left.result.current.value?.session.id).toBe('first'),
    )
    await waitFor(() =>
      expect(right.result.current.value?.session.id).toBe('second'),
    )

    // A terminal stream failure settles the UI and revokes only that attachment.
    await act(async () => {
      if (failure === 'reload') {
        first.updates.push(
          Event.create({ sequence: 1n, payload: '{"type":"full-reload"}' }),
        )
        return
      }
      first.updates.end(new Error('device disconnected'))
    })
    await waitFor(() =>
      expect(left.result.current.error?.message).toBe(
        failure === 'reload'
          ? 'Frontend configuration changed. Restart the preview.'
          : 'device disconnected',
      ),
    )
    expect(first.release).toHaveBeenCalledOnce()
    expect(globalThis.__bldrFrontends?.has('first')).toBe(false)
    expect(globalThis.__bldrFrontends?.has('second')).toBe(true)
    expect(second.release).not.toHaveBeenCalled()

    // Closing the remaining view releases its compiler and transport together.
    left.unmount()
    right.unmount()
    await waitFor(() => expect(second.release).toHaveBeenCalledOnce())
    expect(globalThis.__bldrFrontends?.size).toBe(0)
  },
)

it('releases a late attachment after the authoring view has closed', async () => {
  globalThis.__bldrFrontendEnabled = true
  const pending = Promise.withResolvers<PluginFrontend>()
  const late = attachment('late')
  const open = vi.fn(() => pending.promise)
  const space = parent(open)
  const hook = renderHook(() => usePluginFrontend(space, request))
  await waitFor(() => expect(open).toHaveBeenCalledOnce())
  hook.unmount()
  await act(async () => pending.resolve(late.grant))
  await waitFor(() => expect(late.release).toHaveBeenCalledOnce())
  expect(globalThis.__bldrFrontends?.has('late')).toBe(false)
})

it('explains the renderer requirement before queuing a device job', async () => {
  const open = vi.fn()
  const space = parent(open)
  const hook = renderHook(() => usePluginFrontend(space, request))
  await waitFor(() =>
    expect(hook.result.current.error?.message).toContain('development client'),
  )
  expect(open).not.toHaveBeenCalled()
  hook.unmount()
})
