import { describe, expect, it, vi } from 'vitest'
import { ERR_RPC_ABORT, Packet } from 'starpc'
import type { RpcStreamPacket } from 'starpc'

import type {
  ResourceAttachResponse,
  ResourceClientRequest,
  ResourceClientResponse,
} from './resource.pb.js'
import type { ResourceService } from './resource_srpc.pb.js'
import { Client } from './client.js'

function setInitializedResourceSession(client: Client): void {
  Reflect.set(client, 'initState', { clientHandleId: 7, rootResourceId: 1 })
  Reflect.set(client, 'resourceSession', {
    controller: new AbortController(),
    outgoing: { push: vi.fn(), end: vi.fn() },
    generation: 1,
    initialized: true,
    closed: false,
    nextControlId: 0,
    acknowledgedControlId: Number.MAX_SAFE_INTEGER,
    controlWaiters: new Set(),
  })
}

describe('ResourceClient', () => {
  it('clears stale attachSession state on reconnect cleanup', () => {
    const client = new Client(
      buildUnusedService(),
      new AbortController().signal,
    )
    const controller = new AbortController()
    const end = vi.fn()
    const reject = vi.fn()

    Reflect.set(client, 'attachSession', {
      controller,
      outgoing: { end, push: vi.fn() },
      attachIdCtr: 1,
      closed: false,
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
    const client = new Client(
      buildUnusedService(),
      new AbortController().signal,
    )
    const controller = new AbortController()
    const end = vi.fn()

    Reflect.set(client, 'attachSession', {
      controller,
      outgoing: { end, push: vi.fn() },
      attachIdCtr: 1,
      closed: false,
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
    const client = new Client(
      buildUnusedService(),
      new AbortController().signal,
    )
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
    const client = new Client(
      buildUnusedService(),
      new AbortController().signal,
    )
    const sess = buildAttachSession()
    Reflect.set(client, 'ensureAttachSession', vi.fn().mockResolvedValue(sess))

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

  it('ResourceAttach stream close runs release callback', async () => {
    const closeAttach = deferredVoid()
    const service = buildUnusedService()
    service.ResourceAttach = async function* (request) {
      const incoming = request[Symbol.asyncIterator]()

      const init = await incoming.next()
      expect(init.done).toBe(false)
      expect(init.value?.body).toEqual({
        case: 'init',
        value: { clientHandleId: 7 },
      })
      const ack: ResourceAttachResponse = {
        body: {
          case: 'ack',
          value: {},
        },
      }
      yield ack

      const add = await incoming.next()
      expect(add.done).toBe(false)
      expect(add.value?.body).toEqual({
        case: 'add',
        value: { attachId: 1, label: 'tree-handler' },
      })
      const addAck: ResourceAttachResponse = {
        body: {
          case: 'addAck',
          value: { attachId: 1, resourceId: 73 },
        },
      }
      yield addAck

      await closeAttach.promise
    }

    const client = new Client(service, new AbortController().signal)
    setInitializedResourceSession(client)

    const release = vi.fn()
    await client.attachResourceTree('tree-handler', vi.fn(), undefined, release)
    const sess = Reflect.get(client, 'attachSession')

    expect(sess).toBeTruthy()
    expect(release).not.toHaveBeenCalled()
    expect(Reflect.get(sess, 'muxes').has(73)).toBe(true)
    expect(Reflect.get(sess, 'releaseFns').has(73)).toBe(true)

    closeAttach.resolve()

    await waitForCondition(() => release.mock.calls.length === 1)
    await waitForCondition(() => Reflect.get(client, 'attachSession') === null)

    Reflect.get(client, 'clearAttachSession').call(client, sess)
    expect(release).toHaveBeenCalledOnce()
    expect(Reflect.get(sess, 'muxes').has(73)).toBe(false)
    expect(Reflect.get(sess, 'releaseFns').has(73)).toBe(false)
    expect(Reflect.get(sess, 'controller').signal.aborted).toBe(true)
    expect(Reflect.get(sess, 'closed')).toBe(true)
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
    setInitializedResourceSession(client)
    Reflect.set(client, 'connectionController', connectionController)
    const ref = client.createResourceReference(1)

    await expect(
      client.attachResource('test-handler', vi.fn()),
    ).rejects.toEqual(
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

  it('waits for prior lifecycle controls before opening ResourceRpc', async () => {
    const service = buildUnusedService()
    const controller = new AbortController()
    let allowAck: (() => void) | undefined
    const ackAllowed = new Promise<void>((resolve) => {
      allowAck = resolve
    })
    let controlSeen: (() => void) | undefined
    const sawControl = new Promise<void>((resolve) => {
      controlSeen = resolve
    })
    const rpcError = new Error('ResourceRpc started')
    const resourceRpc = vi.fn(() => {
      throw rpcError
    })
    service.ResourceRpc = resourceRpc
    service.ResourceClient = async function* (request, abortSignal) {
      const incoming = request[Symbol.asyncIterator]()
      const init = await incoming.next()
      expect(init.value?.body?.case).toBe('init')
      yield {
        body: {
          case: 'init' as const,
          value: { clientHandleId: 7, rootResourceId: 1 },
        },
      }
      const control = await incoming.next()
      expect(control.value?.body?.case).toBe('adopt')
      controlSeen?.()
      await ackAllowed
      yield {
        body: {
          case: 'controlAck' as const,
          value: { controlId: control.value?.controlId ?? 0 },
        },
      }
      await new Promise<void>((resolve) => {
        abortSignal?.addEventListener('abort', () => resolve(), { once: true })
      })
    }

    const client = new Client(service, controller.signal)
    const root = await client.accessRootResource()
    const callController = new AbortController()
    const canceledCall = root.client.request(
      'test.Service',
      'Call',
      new Uint8Array(),
      callController.signal,
    )
    await sawControl
    expect(resourceRpc).not.toHaveBeenCalled()
    callController.abort()
    await expect(canceledCall).rejects.toThrow(ERR_RPC_ABORT)
    expect(resourceRpc).not.toHaveBeenCalled()

    const call = root.client.request('test.Service', 'Call', new Uint8Array())
    allowAck?.()
    await expect(call).rejects.toBe(rpcError)
    expect(resourceRpc).toHaveBeenCalledOnce()
    controller.abort()
  })

  it('delivers ResourceRpc call data after stream ack', async () => {
    const service = buildUnusedService()
    let resourceRpcCalls = 0
    service.ResourceRpc = async function* (request) {
      resourceRpcCalls++
      const incoming = request[Symbol.asyncIterator]()
      const initPacket = await readResourceRpcPacket(incoming)

      expect(initPacket.body).toEqual({
        case: 'init',
        value: { componentId: '51' },
      })

      yield {
        body: {
          case: 'ack' as const,
          value: {},
        },
      }

      const dataPacket = await readResourceRpcPacket(incoming)
      expect(dataPacket.body?.case).toBe('data')
      if (dataPacket.body?.case !== 'data') {
        throw new Error('expected ResourceRpc data packet')
      }

      const callStart = Packet.fromBinary(dataPacket.body.value)
      expect(callStart.body?.case).toBe('callStart')
      if (callStart.body?.case !== 'callStart') {
        throw new Error('expected nested SRPC call start')
      }
      expect(callStart.body.value.rpcService).toBe(
        's4wave.space.SpaceResourceService',
      )
      expect(callStart.body.value.rpcMethod).toBe('MountSpaceContents')
      expect(callStart.body.value.data?.length ?? 0).toBe(0)
      expect(callStart.body.value.dataIsZero).toBe(true)

      yield {
        body: {
          case: 'data' as const,
          value: Packet.toBinary({
            body: {
              case: 'callData',
              value: {
                data: new Uint8Array([7]),
                complete: true,
              },
            },
          }),
        },
      }
    }

    const client = new Client(service, new AbortController().signal)
    setInitializedResourceSession(client)

    const result = await client
      .createResourceReference(51)
      .client.request(
        's4wave.space.SpaceResourceService',
        'MountSpaceContents',
        new Uint8Array(0),
      )

    expect([...result]).toEqual([7])
    expect(resourceRpcCalls).toBe(1)
  })

  it('marks stale ResourceRpc refs server-released before ack error rejects the request', async () => {
    const service = buildUnusedService()
    const staleMessage = 'resource or client was released'
    service.ResourceRpc = async function* (request) {
      const incoming = request[Symbol.asyncIterator]()
      const initPacket = await readResourceRpcPacket(incoming)

      expect(initPacket.body).toEqual({
        case: 'init',
        value: { componentId: '51' },
      })

      yield {
        body: {
          case: 'ack' as const,
          value: {
            error: staleMessage,
          },
        },
      }

      throw new Error(staleMessage)
    }

    const client = new Client(service, new AbortController().signal)
    setInitializedResourceSession(client)
    const onResourceReleased = vi.fn()
    client.onResourceReleased(onResourceReleased)
    const ref = client.createResourceReference(51)

    await expect(
      ref.client
        .request(
          's4wave.space.SpaceResourceService',
          'MountSpaceContents',
          new Uint8Array(0),
        )
        .catch((error: unknown) => {
          expect(ref.released).toBe(true)
          expect(onResourceReleased).toHaveBeenCalledOnce()
          expect(onResourceReleased).toHaveBeenCalledWith({
            resourceId: 51,
            reason: 'server-released',
          })
          throw error
        }),
    ).rejects.toThrow(`rpcstream: remote: ${staleMessage}`)

    expect(onResourceReleased).toHaveBeenCalledOnce()
  })

  it.each([
    {
      name: 'resource not found',
      message: 'resource not found: resource 51',
    },
    {
      name: 'invalid resource id',
      message: 'invalid resource id: 51',
    },
  ])(
    'marks stale ResourceRpc refs server-released on $name open failure',
    async ({ message }) => {
      const service = buildUnusedService()
      const openError = new Error(message)
      service.ResourceRpc = () => {
        throw openError
      }

      const client = new Client(service, new AbortController().signal)
      setInitializedResourceSession(client)
      const onResourceReleased = vi.fn()
      client.onResourceReleased(onResourceReleased)
      const ref = client.createResourceReference(51)

      await expect(
        ref.client.request(
          's4wave.space.SpaceResourceService',
          'MountSpaceContents',
          new Uint8Array(0),
        ),
      ).rejects.toBe(openError)

      expect(ref.released).toBe(true)
      expect(onResourceReleased).toHaveBeenCalledOnce()
      expect(onResourceReleased).toHaveBeenCalledWith({
        resourceId: 51,
        reason: 'server-released',
      })
    },
  )

  it('queues one adopt and one final release in FIFO order', async () => {
    const controls: ResourceClientRequest[] = []
    const service = buildUnusedService()
    service.ResourceClient = async function* (request) {
      let initialized = false
      for await (const control of request) {
        controls.push(control)
        if (!initialized) {
          initialized = true
          yield buildResourceClientInit(1)
        }
      }
    }

    const client = new Client(service, new AbortController().signal)
    const first = await client.accessRootResource()
    await waitForCondition(() => controls.length === 2)
    const second = first.createRef(first.resourceId)
    await new Promise((resolve) => setTimeout(resolve, 0))
    first.release()
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(controls).toHaveLength(2)

    second.release()
    await waitForCondition(() => controls.length === 3)
    expect(
      controls.map((control) => [
        control.body?.case,
        control.body?.case === 'adopt' || control.body?.case === 'release'
          ? control.body.value.resourceId
          : undefined,
      ]),
    ).toEqual([
      ['init', undefined],
      ['adopt', 1],
      ['release', 1],
    ])

    client.dispose()
  })

  it('drains final releases before a normal ResourceClient close', async () => {
    const controls: ResourceClientRequest[] = []
    const streamDone = Promise.withResolvers<void>()
    const service = buildUnusedService()
    service.ResourceClient = async function* (request, signal) {
      try {
        let initialized = false
        for await (const control of request) {
          controls.push(control)
          if (!initialized) {
            initialized = true
            yield buildResourceClientInit(1)
          }
          if (control.body?.case === 'release') {
            expect(signal?.aborted).toBe(false)
          }
        }
      } finally {
        streamDone.resolve()
      }
    }

    const client = new Client(service, new AbortController().signal)
    await client.accessRootResource()
    await waitForCondition(() => controls.length === 2)

    client.dispose()
    await streamDone.promise

    expect(
      controls.map((control) => [
        control.body?.case,
        control.body?.case === 'adopt' || control.body?.case === 'release'
          ? control.body.value.resourceId
          : undefined,
      ]),
    ).toEqual([
      ['init', undefined],
      ['adopt', 1],
      ['release', 1],
    ])
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
    const generations: number[] = []
    client.onConnectionLost(() => generations.push(client.connectionGeneration))
    const first = await client.accessRootResource()

    expect(first.resourceId).toBe(1)
    expect(client.connectionGeneration).toBe(0)

    if (!firstStream.close) {
      throw new Error('expected first ResourceClient stream close callback')
    }
    firstStream.close()
    await vi.advanceTimersByTimeAsync(0)

    expect(first.released).toBe(true)
    expect(generations).toEqual([1])
    expect(Reflect.get(client, 'initState')).toBe(null)
    expect(Reflect.get(client, 'initPromise')).toBeInstanceOf(Promise)

    const secondPromise = client.accessRootResource()
    await vi.advanceTimersByTimeAsync(500)
    const second = await secondPromise

    expect(second.resourceId).toBe(2)
    expect(client.connectionGeneration).toBe(1)
    expect(calls).toBe(2)

    client.dispose()
    vi.useRealTimers()
  })

  it('retries ResourceClient stream resets without warning spam', async () => {
    vi.useFakeTimers()
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const service = buildUnusedService()
    const calls = { count: 0 }
    service.ResourceClient = vi.fn(async function* (_request, signal) {
      calls.count++
      yield buildResourceClientInit(calls.count)
      await new Promise<void>((resolve) => {
        setTimeout(resolve, 1)
        signal?.addEventListener('abort', resolve, { once: true })
      })
      if (!signal?.aborted && calls.count === 1) {
        throw { name: 'StreamResetError', message: 'stream reset' }
      }
      await new Promise<void>((resolve) => {
        signal?.addEventListener('abort', resolve, { once: true })
      })
    })

    try {
      const client = new Client(service, new AbortController().signal)
      const first = await client.accessRootResource()

      expect(first.resourceId).toBe(1)

      await vi.advanceTimersByTimeAsync(501)

      const second = await client.accessRootResource()

      expect(second.resourceId).toBe(2)
      expect(calls.count).toBe(2)
      expect(warn).not.toHaveBeenCalled()

      client.dispose()
    } finally {
      warn.mockRestore()
      vi.useRealTimers()
    }
  })

  it('retries stringified ResourceClient stream resets without warning spam', async () => {
    vi.useFakeTimers()
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const service = buildUnusedService()
    const calls = { count: 0 }
    service.ResourceClient = vi.fn(async function* (_request, signal) {
      calls.count++
      yield buildResourceClientInit(calls.count)
      await new Promise<void>((resolve) => {
        setTimeout(resolve, 1)
        signal?.addEventListener('abort', resolve, { once: true })
      })
      if (!signal?.aborted && calls.count === 1) {
        throw { toString: () => 'StreamResetError: stream reset' }
      }
      await new Promise<void>((resolve) => {
        signal?.addEventListener('abort', resolve, { once: true })
      })
    })

    try {
      const client = new Client(service, new AbortController().signal)
      const first = await client.accessRootResource()

      expect(first.resourceId).toBe(1)

      await vi.advanceTimersByTimeAsync(501)

      const second = await client.accessRootResource()

      expect(second.resourceId).toBe(2)
      expect(calls.count).toBe(2)
      expect(warn).not.toHaveBeenCalled()

      client.dispose()
    } finally {
      warn.mockRestore()
      vi.useRealTimers()
    }
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

  it('retries ResourceClient streams that hang before init', async () => {
    vi.useFakeTimers()
    const service = buildUnusedService()
    let calls = 0
    service.ResourceClient = vi.fn(async function* (_request, signal) {
      calls++
      if (calls === 1) {
        await new Promise<void>((resolve) => {
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
    const rootPromise = client.accessRootResource()

    await vi.advanceTimersByTimeAsync(30500)
    const root = await rootPromise

    expect(root.resourceId).toBe(2)
    expect(calls).toBe(2)

    client.dispose()
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
    ResourceAttach() {
      throw new Error('unused')
    },
  }
}

function buildResourceClientInit(resourceId: number): ResourceClientResponse {
  return {
    body: {
      case: 'init',
      value: { clientHandleId: resourceId, rootResourceId: resourceId },
    },
  }
}

async function readResourceRpcPacket(
  incoming: AsyncIterator<RpcStreamPacket>,
): Promise<RpcStreamPacket> {
  const next = await Promise.race([
    incoming.next(),
    new Promise<IteratorResult<RpcStreamPacket>>((_, reject) => {
      setTimeout(() => reject(new Error('timed out waiting for packet')), 1000)
    }),
  ])
  if (next.done) {
    throw new Error('ResourceRpc stream closed before packet')
  }
  return next.value
}

async function waitForCondition(condition: () => boolean): Promise<void> {
  const deadline = Date.now() + 1000
  while (Date.now() < deadline) {
    if (condition()) {
      return
    }
    await new Promise((resolve) => setTimeout(resolve, 5))
  }
  throw new Error('timed out waiting for condition')
}

function deferredVoid(): { promise: Promise<void>; resolve: () => void } {
  const holder: { resolve?: () => void } = {}
  const promise = new Promise<void>((resolve) => {
    holder.resolve = resolve
  })
  return {
    promise,
    resolve() {
      holder.resolve?.()
    },
  }
}

function buildAttachSession() {
  return {
    controller: new AbortController(),
    outgoing: { end: vi.fn(), push: vi.fn() },
    attachIdCtr: 0,
    closed: false,
    muxes: new Map<number, unknown>(),
    pending: new Map<
      number,
      { resolve: (resourceId: number) => void; reject: (err: Error) => void }
    >(),
    releaseFns: new Map<number, () => void>(),
  }
}
