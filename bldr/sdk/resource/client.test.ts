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
      releaseFns: new Map(),
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
      releaseFns: new Map(),
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

  it('attachResourceTree cleanup runs release callback', async () => {
    const client = new Client(buildUnusedService(), new AbortController().signal)
    const sess = buildAttachSession()
    vi.spyOn(
      client as unknown as { ensureAttachSession: () => Promise<unknown> },
      'ensureAttachSession',
    ).mockResolvedValue(sess)

    sess.outgoing.push.mockImplementation((pkt) => {
      if (pkt.body?.case === 'add') {
        queueMicrotask(() => {
          const pending = sess.pending.get(1)
          sess.pending.delete(1)
          pending?.resolve(73)
        })
      }
    })

    const release = vi.fn()
    const result = await client.attachResourceTree(
      'tree-handler',
      vi.fn(),
      undefined,
      release,
    )

    expect(result.resourceId).toBe(73)
    expect(release).not.toHaveBeenCalled()
    expect(sess.muxes.has(73)).toBe(true)
    expect(sess.releaseFns.has(73)).toBe(true)

    result.cleanup()

    expect(release).toHaveBeenCalledOnce()
    expect(sess.muxes.has(73)).toBe(false)
    expect(sess.releaseFns.has(73)).toBe(false)
  })

  it('invalidates cached resources when attach reports a missing client', async () => {
    const service = buildUnusedService()
    service.ResourceAttach = async function* (request) {
      await request[Symbol.asyncIterator]().next()
      yield {
        body: {
          case: 'ack' as const,
          value: { error: 'client not found' },
        },
      }
    }

    const client = new Client(service, new AbortController().signal)
    const onConnectionLost = vi.fn()
    client.onConnectionLost(onConnectionLost)
    const connectionController = new AbortController()
    Reflect.set(client, 'initState', { clientHandleId: 7, rootResourceId: 1 })
    Reflect.set(client, 'connectionController', connectionController)
    const ref = client.createResourceReference(1)

    await expect(client.attachResource('test-handler', vi.fn())).rejects.toEqual(
      expect.objectContaining({
        code: 'CONNECTION_FAILED',
        cause: expect.objectContaining({ message: 'client not found' }),
      }),
    )

    expect(ref.released).toBe(true)
    expect(client.connectionGeneration).toBe(1)
    expect(onConnectionLost).toHaveBeenCalledOnce()
    expect(connectionController.signal.aborted).toBe(true)
    expect(Reflect.get(client, 'connectionController')).toBe(null)
    expect(Reflect.get(client, 'initState')).toBe(null)
    expect(Reflect.get(client, 'initPromise')).toBe(null)
  })

  it('retries ResourceClient streams that close after init', async () => {
    vi.useFakeTimers()
    const service = buildUnusedService()
    const firstStream = { close: null as (() => void) | null }
    let calls = 0
    service.ResourceClient = vi.fn(async function* (_request, signal) {
      calls++
      if (calls === 1) {
        yield {
          body: {
            case: 'init' as const,
            value: { clientHandleId: 7, rootResourceId: 1 },
          },
        }
        await new Promise<void>((resolve) => {
          firstStream.close = resolve
          signal?.addEventListener('abort', resolve, { once: true })
        })
        return
      }
      yield {
        body: {
          case: 'init' as const,
          value: { clientHandleId: 8, rootResourceId: 2 },
        },
      }
      await new Promise<void>((resolve) => {
        signal?.addEventListener('abort', resolve, { once: true })
      })
    })

    const client = new Client(service, new AbortController().signal)
    const first = await client.accessRootResource()

    expect(first.resourceId).toBe(1)

    if (!firstStream.close) {
      throw new Error('expected first ResourceClient stream close callback')
    }
    firstStream.close()
    await vi.advanceTimersByTimeAsync(0)

    expect(first.released).toBe(true)
    expect(Reflect.get(client, 'initState')).toBe(null)
    expect(Reflect.get(client, 'initPromise')).toBeInstanceOf(Promise)

    const secondPromise = client.accessRootResource()
    await vi.advanceTimersByTimeAsync(500)
    const second = await secondPromise

    expect(second.resourceId).toBe(2)
    expect(calls).toBe(2)

    client.dispose()
    vi.useRealTimers()
  })

  it('retries ResourceClient streams that close before init', async () => {
    vi.useFakeTimers()
    const service = buildUnusedService()
    let calls = 0
    service.ResourceClient = vi.fn(async function* (_request, signal) {
      calls++
      if (calls === 1) {
        return
      }
      yield {
        body: {
          case: 'init' as const,
          value: { clientHandleId: 8, rootResourceId: 2 },
        },
      }
      await new Promise<void>((resolve) => {
        signal?.addEventListener('abort', resolve, { once: true })
      })
    })

    const client = new Client(service, new AbortController().signal)
    const rootPromise = client.accessRootResource()

    await vi.advanceTimersByTimeAsync(500)
    const root = await rootPromise

    expect(root.resourceId).toBe(2)
    expect(calls).toBe(2)

    client.dispose()
    vi.useRealTimers()
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
    releaseFns: new Map<number, () => void>(),
  }
}
