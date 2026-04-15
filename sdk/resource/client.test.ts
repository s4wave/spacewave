import { describe, expect, it, vi } from 'vitest'

import type { ResourceService } from './resource_srpc.pb.js'
import { Client } from './client.js'

describe('ResourceClient', () => {
  it('clears stale attachSession state on reconnect cleanup', () => {
    const client = new Client(buildUnusedService(), new AbortController().signal)
    const controller = new AbortController()
    const end = vi.fn()
    const reject = vi.fn()

    Reflect.set(client, 'attachSession', {
      controller,
      outgoing: { end, push: vi.fn() },
      attachIdCtr: 1,
      muxes: new Map([[1, vi.fn()]]),
      pending: new Map([[1, { resolve: vi.fn(), reject }]]),
    })

    Reflect.get(client, 'releaseAllResources').call(client, 'connection-lost')

    expect(Reflect.get(client, 'attachSession')).toBe(null)
    expect(controller.signal.aborted).toBe(true)
    expect(end).toHaveBeenCalledOnce()
    expect(reject).toHaveBeenCalledWith(expect.any(Error))
  })

  it('clears stale attachSession state on dispose', () => {
    const client = new Client(buildUnusedService(), new AbortController().signal)
    const controller = new AbortController()
    const end = vi.fn()

    Reflect.set(client, 'attachSession', {
      controller,
      outgoing: { end, push: vi.fn() },
      attachIdCtr: 1,
      muxes: new Map(),
      pending: new Map(),
    })

    client.dispose()

    expect(Reflect.get(client, 'attachSession')).toBe(null)
    expect(controller.signal.aborted).toBe(true)
    expect(end).toHaveBeenCalledOnce()
  })

  it('retries queued resource releases after runtime ack timeouts', async () => {
    vi.useFakeTimers()
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const service: ResourceService = {
      ResourceRefRelease: vi
        .fn()
        .mockRejectedValueOnce(
          new Error(
            'WebRuntimeClient: client: timeout waiting for runtime connected ack',
          ),
        )
        .mockResolvedValue({}),
      ResourceClient() {
        throw new Error('unused')
      },
      ResourceRpc() {
        throw new Error('unused')
      },
      ResourceAttach() {
        throw new Error('unused')
      },
    }
    const client = new Client(service, new AbortController().signal)
    Reflect.set(client, 'initState', { clientHandleId: 7, rootResourceId: 1 })

    const ref = client.createResourceReference(49)
    ref.release()

    await vi.advanceTimersByTimeAsync(0)
    expect(service.ResourceRefRelease).toHaveBeenCalledTimes(1)
    expect(getPendingResourceReleases(client).size).toBe(1)
    expect(warn).not.toHaveBeenCalled()

    await vi.advanceTimersByTimeAsync(100)

    expect(service.ResourceRefRelease).toHaveBeenCalledTimes(2)
    expect(service.ResourceRefRelease).toHaveBeenLastCalledWith(
      { clientHandleId: 7, resourceId: 49 },
      expect.any(AbortSignal),
    )
    expect(getPendingResourceReleases(client).size).toBe(0)
    expect(warn).not.toHaveBeenCalled()

    warn.mockRestore()
    vi.useRealTimers()
  })
})

function buildUnusedService(): ResourceService {
  return {
    ResourceClient() {
      throw new Error('unused')
    },
    ResourceRpc() {
      throw new Error('unused')
    },
    ResourceRefRelease() {
      throw new Error('unused')
    },
    ResourceAttach() {
      throw new Error('unused')
    },
  }
}

function getPendingResourceReleases(client: Client) {
  const pending = Reflect.get(client, 'pendingResourceReleases')
  if (!(pending instanceof Map)) {
    throw new Error('expected pendingResourceReleases map')
  }
  return pending
}
