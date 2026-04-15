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

  it('retries attachResource after attach session closes before addAck', async () => {
    const client = new Client(buildUnusedService(), new AbortController().signal)
    const first = buildAttachSession()
    const second = buildAttachSession()
    const ensureAttachSession = vi
      .fn()
      .mockImplementationOnce(async () => {
        Reflect.set(client, 'attachSession', first)
        return first
      })
      .mockImplementationOnce(async () => {
        Reflect.set(client, 'attachSession', second)
        return second
      })
    vi.spyOn(
      client as unknown as { ensureAttachSession: () => Promise<unknown> },
      'ensureAttachSession',
    ).mockImplementation(ensureAttachSession)

    first.outgoing.push.mockImplementation(() => {
      queueMicrotask(() => {
        Reflect.set(client, 'attachSession', null)
        const pending = first.pending.get(1)
        first.pending.delete(1)
        pending?.reject(new Error('attach session closed'))
      })
    })
    second.outgoing.push.mockImplementation((pkt) => {
      if (pkt.body?.case === 'add') {
        queueMicrotask(() => {
          const pending = second.pending.get(1)
          second.pending.delete(1)
          pending?.resolve(73)
        })
      }
    })

    const result = await client.attachResource('test-handler', vi.fn())

    expect(result.resourceId).toBe(73)
    expect(ensureAttachSession).toHaveBeenCalledTimes(2)
    expect(second.muxes.get(73)).toBeTypeOf('function')

    result.cleanup()

    expect(second.muxes.has(73)).toBe(false)
    expect(second.outgoing.push).toHaveBeenCalledTimes(2)
    expect(second.outgoing.push).toHaveBeenLastCalledWith({
      body: {
        case: 'detach',
        value: { resourceId: 73 },
      },
    })
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

function buildAttachSession() {
  return {
    controller: new AbortController(),
    outgoing: { end: vi.fn(), push: vi.fn() },
    attachIdCtr: 0,
    muxes: new Map<number, unknown>(),
    pending: new Map<
      number,
      { resolve: (resourceId: number) => void; reject: (err: Error) => void }
    >(),
  }
}
