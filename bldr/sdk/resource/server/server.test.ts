import { describe, it, expect, vi, beforeEach } from 'vitest'
import { createMux, Packet, Server, type ServerContext } from 'starpc'
import { RemoteResourceClient } from './tracked-client.js'
import { ResourceServer } from './server.js'
import { getResourceCall, withResourceCall } from './context.js'
import { newResourceMux } from './mux.js'
import { ResourceServiceDefinition } from '../resource_srpc.pb.js'
import {
  ResourceClientResponse,
  ResourceClientRequest,
} from '../resource.pb.js'
import type {
  ResourceAttachRequest,
  ResourceAttachResponse,
} from '../resource.pb.js'

function serverContext(signal = new AbortController().signal): ServerContext {
  return { signal }
}

describe('RemoteResourceClient', () => {
  let idCtr: number
  const nextID = () => ++idCtr

  beforeEach(() => {
    idCtr = 0
  })

  it('allocates resource IDs via nextResourceID callback', () => {
    const client = new RemoteResourceClient(nextID, 1)
    const mux = createMux()
    const id1 = client.addResource(mux)
    const id2 = client.addResource(mux)
    expect(id1).toBe(1)
    expect(id2).toBe(2)
  })

  it('addResource stores resource in map', () => {
    const client = new RemoteResourceClient(nextID, 1)
    const mux = createMux()
    const id = client.addResource(mux, () => {})
    expect(client.resources.has(id)).toBe(true)
    const tracked = client.resources.get(id)!
    expect(tracked.mux).toBe(mux)
    expect(tracked.ownerClientID).toBe(1)
    expect(typeof tracked.releaseFn).toBe('function')
  })

  it('addResource throws when client is released', () => {
    const client = new RemoteResourceClient(nextID, 1)
    client.released = true
    expect(() => client.addResource(createMux())).toThrow('client was released')
  })

  it('releaseResource removes resource and queues notification', () => {
    const releaseFn = vi.fn()
    const client = new RemoteResourceClient(nextID, 1)
    const id = client.addResource(createMux(), releaseFn)
    expect(client.resources.has(id)).toBe(true)

    const result = client.releaseResource(id)
    expect(result).toBe(true)
    expect(client.resources.has(id)).toBe(false)
    expect(releaseFn).toHaveBeenCalledOnce()

    const msgs = client.drainQueue()
    expect(msgs).toHaveLength(1)
    expect(msgs[0].body?.case).toBe('resourceReleased')
    if (msgs[0].body?.case === 'resourceReleased') {
      expect(msgs[0].body.value.resourceId).toBe(id)
    }
  })

  it('aborts an attached invocation before notifying its release once', () => {
    const client = new RemoteResourceClient(nextID, 1)
    const controller = new AbortController()
    const release = vi.fn(() => {
      expect(controller.signal.aborted).toBe(true)
    })
    client.attachedResources.set(50, {
      label: 'attached',
      client: buildMockSRPCClient(),
      signal: controller.signal,
      controller,
      release,
    })

    expect(client.releaseResource(50, false)).toBe(true)
    expect(client.releaseResource(50, false)).toBe(true)
    client.releaseAll()

    expect(release).toHaveBeenCalledOnce()
  })

  it('aborts and releases each attachment once when its generation ends', () => {
    const client = new RemoteResourceClient(nextID, 1)
    const controller = new AbortController()
    const release = vi.fn(() => {
      expect(controller.signal.aborted).toBe(true)
    })
    client.attachedResources.set(50, {
      label: 'attached',
      client: buildMockSRPCClient(),
      signal: controller.signal,
      controller,
      release,
    })

    client.releaseAll()
    client.releaseAll()

    expect(release).toHaveBeenCalledOnce()
    expect(client.attachedResources.size).toBe(0)
  })

  it('releaseResource returns false for missing resource ID', () => {
    const client = new RemoteResourceClient(nextID, 1)
    expect(client.releaseResource(999)).toBe(false)
  })

  it('releaseResource returns false when client is released', () => {
    const client = new RemoteResourceClient(nextID, 1)
    const id = client.addResource(createMux())
    client.released = true
    expect(client.releaseResource(id)).toBe(false)
  })

  it('pushMessage adds to queue and notifies waiting consumer', async () => {
    const client = new RemoteResourceClient(nextID, 1)
    const msg: ResourceClientResponse = {
      body: { case: 'resourceReleased', value: { resourceId: 1 } },
    }

    const waitPromise = client.waitForNotify()
    client.pushMessage(msg)
    await waitPromise

    const msgs = client.drainQueue()
    expect(msgs).toHaveLength(1)
    expect(msgs[0]).toBe(msg)
  })

  it('drainQueue returns and clears all messages', () => {
    const client = new RemoteResourceClient(nextID, 1)
    client.pushMessage({
      body: { case: 'resourceReleased', value: { resourceId: 1 } },
    })
    client.pushMessage({
      body: { case: 'resourceReleased', value: { resourceId: 1 } },
    })

    const msgs = client.drainQueue()
    expect(msgs).toHaveLength(2)

    const empty = client.drainQueue()
    expect(empty).toHaveLength(0)
  })

  it('drainQueue returns empty array when no messages', () => {
    const client = new RemoteResourceClient(nextID, 1)
    expect(client.drainQueue()).toHaveLength(0)
  })

  it('waitForNotify resolves immediately if queue has messages', async () => {
    const client = new RemoteResourceClient(nextID, 1)
    client.pushMessage({
      body: { case: 'resourceReleased', value: { resourceId: 1 } },
    })
    await client.waitForNotify()
    expect(client.drainQueue()).toHaveLength(1)
  })

  it('waitForNotify resolves when message is pushed', async () => {
    const client = new RemoteResourceClient(nextID, 1)
    let resolved = false
    const p = client.waitForNotify().then(() => {
      resolved = true
    })
    expect(resolved).toBe(false)
    client.pushMessage({
      body: { case: 'resourceReleased', value: { resourceId: 1 } },
    })
    await p
    expect(resolved).toBe(true)
  })

  it('waitForNotify resolves when abort signal fires', async () => {
    const client = new RemoteResourceClient(nextID, 1)
    const controller = new AbortController()

    const p = client.waitForNotify(controller.signal)
    controller.abort()
    await p
    expect(client.drainQueue()).toHaveLength(0)
  })

  it('waitForNotify resolves immediately if signal already aborted', async () => {
    const client = new RemoteResourceClient(nextID, 1)
    const controller = new AbortController()
    controller.abort()
    await client.waitForNotify(controller.signal)
  })

  it('waitForNotify resolves immediately when client is released', async () => {
    const client = new RemoteResourceClient(nextID, 1)
    client.released = true
    await client.waitForNotify()
  })

  it('controller aborts when parent signal aborts', () => {
    const parent = new AbortController()
    const client = new RemoteResourceClient(nextID, 1, parent.signal)
    expect(client.signal.aborted).toBe(false)
    parent.abort()
    expect(client.signal.aborted).toBe(true)
  })

  it('signal getter returns controller signal', () => {
    const client = new RemoteResourceClient(nextID, 1)
    expect(client.signal).toBe(client.controller.signal)
  })

  it('releases an adopted orphan when the generation disconnects', () => {
    const client = new RemoteResourceClient(nextID, 1)
    const rootID = client.addResource(createMux())
    client.setRetainedRootResourceID(rootID)
    const parentRelease = vi.fn()
    const childRelease = vi.fn()
    const parentID = client.addResource(createMux(), parentRelease, rootID)
    expect(client.adoptResource(parentID)).toBe('adopted')
    const childID = client.addResource(createMux(), childRelease, parentID)
    expect(client.adoptResource(childID)).toBe('adopted')

    expect(client.releaseResource(parentID)).toBe(true)
    expect(parentRelease).toHaveBeenCalledOnce()
    expect(childRelease).not.toHaveBeenCalled()
    expect(client.resources.has(childID)).toBe(true)

    client.releaseAll()

    expect(childRelease).toHaveBeenCalledOnce()
    expect(client.resources.size).toBe(0)
  })

  it('schedules one warning at the oldest pending deadline', async () => {
    vi.useFakeTimers()
    let now = 0
    const onPendingWarning = vi.fn()
    const client = new RemoteResourceClient(nextID, 1, undefined, {
      now: () => now,
      onPendingWarning,
    })
    try {
      const rootID = client.addResource(createMux())
      const firstID = client.addResource(createMux(), undefined, rootID)
      expect(vi.getTimerCount()).toBe(1)

      now = 5000
      const secondID = client.addResource(createMux(), undefined, rootID)
      expect(vi.getTimerCount()).toBe(1)
      now = 10000
      await vi.advanceTimersByTimeAsync(5000)

      expect(onPendingWarning).toHaveBeenCalledOnce()
      expect(onPendingWarning).toHaveBeenCalledWith(
        expect.objectContaining({ resourceID: firstID, ageMS: 10000 }),
      )
      expect(
        onPendingWarning.mock.calls.some(
          ([warning]) => warning.resourceID === secondID,
        ),
      ).toBe(false)
      expect(vi.getTimerCount()).toBe(1)
    } finally {
      client.releaseAll()
      vi.useRealTimers()
    }
  })
})

describe('ResourceServer', () => {
  it('nextResourceID returns incrementing values starting at 1', () => {
    const server = new ResourceServer()
    expect(server.nextResourceID()).toBe(1)
    expect(server.nextResourceID()).toBe(2)
    expect(server.nextResourceID()).toBe(3)
  })

  it('register wires the service into a mux', () => {
    const server = new ResourceServer()
    const registered: Array<{ getServiceID(): string }> = []
    const mockMux = {
      register(handler: { getServiceID(): string }) {
        registered.push(handler)
      },
    }
    server.register(mockMux)
    expect(registered).toHaveLength(1)
  })

  describe('ResourceClient handler', () => {
    it('sends init through a detached StarPC packet stream', async () => {
      const rootMux = createMux()
      const resourceServer = new ResourceServer(rootMux)
      const outerMux = createMux()
      resourceServer.register(outerMux)
      const srpcServer = new Server(outerMux.lookupMethod)

      const firstResponse = new Promise<Packet>((resolve, reject) => {
        srpcServer.handlePacketStream({
          source: (async function* () {
            yield Packet.toBinary({
              body: {
                case: 'callStart',
                value: {
                  rpcService: ResourceServiceDefinition.typeName,
                  rpcMethod:
                    ResourceServiceDefinition.methods.ResourceClient.name,
                },
              },
            })
            yield Packet.toBinary({
              body: {
                case: 'callData',
                value: {
                  data: ResourceClientRequest.toBinary({
                    body: { case: 'init', value: {} },
                  }),
                },
              },
            })
          })(),
          sink: async (source) => {
            try {
              for await (const packetData of source) {
                const packet = Packet.fromBinary(packetData)
                if (packet.body?.case === 'callData') {
                  resolve(packet)
                  return
                }
              }
              reject(new Error('ResourceClient stream ended before init'))
            } catch (err) {
              reject(err)
            }
          },
        })
      })

      const packet = await promiseWithTimeout(
        firstResponse,
        'ResourceClient init over detached packet stream',
      )
      const body = packet.body
      expect(body?.case).toBe('callData')
      if (body?.case !== 'callData') {
        throw new Error('expected ResourceClient callData packet')
      }
      const response = ResourceClientResponse.fromBinary(body.value.data)
      expect(response.body?.case).toBe('init')
      if (response.body?.case === 'init') {
        expect(response.body.value.clientHandleId).toBe(1)
        expect(response.body.value.rootResourceId).toBe(1)
      }
    })

    it('sends init with clientHandleId and rootResourceId', async () => {
      const rootMux = createMux()
      const server = new ResourceServer(rootMux)
      const controller = new AbortController()
      const stream = server.ResourceClient(
        resourceClientInit(),
        controller.signal,
      )
      const iterator = stream[Symbol.asyncIterator]()

      const { value: initMsg, done } = await iterator.next()
      expect(done).toBeFalsy()
      expect(initMsg.body?.case).toBe('init')
      if (initMsg.body?.case === 'init') {
        expect(initMsg.body.value.clientHandleId).toBe(1)
        expect(initMsg.body.value.rootResourceId).toBe(1)
      }

      controller.abort()
      const final = await iterator.next()
      expect(final.done).toBe(true)
    })

    it('init IDs start at 1 and increment for each client', async () => {
      const server = new ResourceServer(createMux())

      const c1 = new AbortController()
      const stream1 = server.ResourceClient(resourceClientInit(), c1.signal)
      const iter1 = stream1[Symbol.asyncIterator]()
      const { value: msg1 } = await iter1.next()

      const c2 = new AbortController()
      const stream2 = server.ResourceClient(resourceClientInit(), c2.signal)
      const iter2 = stream2[Symbol.asyncIterator]()
      const { value: msg2 } = await iter2.next()

      if (msg1.body?.case === 'init') {
        expect(msg1.body.value.clientHandleId).toBe(1)
        expect(msg1.body.value.rootResourceId).toBe(1)
      }
      if (msg2.body?.case === 'init') {
        expect(msg2.body.value.clientHandleId).toBe(2)
        expect(msg2.body.value.rootResourceId).toBe(2)
      }

      c1.abort()
      c2.abort()
      await iter1.next()
      await iter2.next()
    })

    it('sends queued messages from releaseResource', async () => {
      const rootMux = createMux()
      const server = new ResourceServer(rootMux)
      const controller = new AbortController()
      const stream = server.ResourceClient(
        resourceClientInit(),
        controller.signal,
      )
      const iterator = stream[Symbol.asyncIterator]()

      await iterator.next()

      const clients = (
        server as unknown as { clients: Map<number, RemoteResourceClient> }
      ).clients
      const client = clients.get(1)!
      expect(client).toBeDefined()

      const childMux = createMux()
      const childId = client.addResource(childMux, () => {})

      const nextPromise = iterator.next()
      client.releaseResource(childId)

      const { value: releasedMsg, done } = await nextPromise
      expect(done).toBeFalsy()
      expect(releasedMsg.body?.case).toBe('resourceReleased')
      if (releasedMsg.body?.case === 'resourceReleased') {
        expect(releasedMsg.body.value.resourceId).toBe(childId)
      }

      controller.abort()
      await iterator.next()
    })

    it('cleans up client on abort', async () => {
      const server = new ResourceServer(createMux())
      const controller = new AbortController()
      const stream = server.ResourceClient(
        resourceClientInit(),
        controller.signal,
      )
      const iterator = stream[Symbol.asyncIterator]()

      await iterator.next()

      const clients = (
        server as unknown as { clients: Map<number, RemoteResourceClient> }
      ).clients
      expect(clients.has(1)).toBe(true)

      controller.abort()
      await iterator.next()

      expect(clients.has(1)).toBe(false)
    })

    it('calls releaseFn for non-root resources on cleanup', async () => {
      const server = new ResourceServer(createMux())
      const controller = new AbortController()
      const stream = server.ResourceClient(
        resourceClientInit(),
        controller.signal,
      )
      const iterator = stream[Symbol.asyncIterator]()

      await iterator.next()

      const clients = (
        server as unknown as { clients: Map<number, RemoteResourceClient> }
      ).clients
      const client = clients.get(1)!

      const releaseFn = vi.fn()
      client.addResource(createMux(), releaseFn)

      controller.abort()
      await iterator.next()

      // releaseFn is called via queueMicrotask in the finally block
      await new Promise<void>((r) => queueMicrotask(r))
      expect(releaseFn).toHaveBeenCalledOnce()
    })

    it('does NOT call releaseFn for root resource on cleanup', async () => {
      const server = new ResourceServer(createMux())
      const controller = new AbortController()
      const stream = server.ResourceClient(
        resourceClientInit(),
        controller.signal,
      )
      const iterator = stream[Symbol.asyncIterator]()

      await iterator.next()

      const clients = (
        server as unknown as { clients: Map<number, RemoteResourceClient> }
      ).clients
      const client = clients.get(1)!

      // Root resource (ID 1) has no releaseFn (added with undefined)
      const rootResource = client.resources.get(1)
      expect(rootResource).toBeDefined()
      expect(rootResource!.releaseFn).toBeUndefined()

      controller.abort()
      await iterator.next()
    })

    it('supports multiple concurrent clients with unique IDs', async () => {
      const server = new ResourceServer(createMux())

      const controllers: AbortController[] = []
      const iterators: AsyncIterator<ResourceClientResponse>[] = []
      const clientIDs: number[] = []

      for (let i = 0; i < 3; i++) {
        const c = new AbortController()
        controllers.push(c)
        const stream = server.ResourceClient(resourceClientInit(), c.signal)
        const iter = stream[Symbol.asyncIterator]()
        iterators.push(iter)
        const { value } = await iter.next()
        if (value.body?.case === 'init') {
          clientIDs.push(value.body.value.clientHandleId ?? 0)
        }
      }

      expect(clientIDs).toEqual([1, 2, 3])
      expect(new Set(clientIDs).size).toBe(3)

      for (let i = 0; i < 3; i++) {
        controllers[i].abort()
        await iterators[i].next()
      }
    })
  })

  it('transmits ResourceReleased without another inbound control', async () => {
    const server = new ResourceServer(createMux())
    const controller = new AbortController()
    const controls = createResourceClientControlStream()
    const stream = server.ResourceClient(controls.iterable, controller.signal)
    const iterator = stream[Symbol.asyncIterator]()
    await iterator.next()
    const clients = (
      server as unknown as { clients: Map<number, RemoteResourceClient> }
    ).clients
    const client = clients.get(1)!
    const resourceID = client.addResource(createMux())
    client.releaseResource(resourceID)
    await expect(iterator.next()).resolves.toMatchObject({
      value: {
        body: {
          case: 'resourceReleased',
          value: { resourceId: resourceID },
        },
      },
    })
    controller.abort()
    controls.end()
  })
})

describe('ResourceCall', () => {
  it('throws when the server context has no resource call', () => {
    const context = serverContext()
    expect(() => getResourceCall(context)).toThrow(
      'no resource call in server context',
    )
  })

  it('constructs children under the invoking resource', () => {
    let resourceId = 0
    const client = new RemoteResourceClient(
      () => ++resourceId,
      1,
      new AbortController().signal,
    )
    const parentResourceId = client.addResource(createMux())
    const context = withResourceCall(serverContext(), {
      client,
      parentResourceId,
      serviceId: 'test.Service',
      methodId: 'Construct',
    })
    const childMux = createMux()
    const releaseFn = vi.fn()

    const child = getResourceCall(context).constructChildResource(() => ({
      mux: childMux,
      result: 'child',
      releaseFn,
    }))

    expect(child.result).toBe('child')
    expect(client.resources.get(child.resourceId)).toMatchObject({
      mux: childMux,
      parentResourceID: parentResourceId,
      serviceID: 'test.Service',
      methodID: 'Construct',
    })
    client.releaseAll()
    expect(releaseFn).toHaveBeenCalledOnce()
  })

  it('isolates concurrent calls that share an AbortSignal', () => {
    let resourceId = 0
    const signal = new AbortController().signal
    const client1 = new RemoteResourceClient(() => ++resourceId, 1, signal)
    const client2 = new RemoteResourceClient(() => ++resourceId, 2, signal)
    const parent1 = client1.addResource(createMux())
    const parent2 = client2.addResource(createMux())
    const base = serverContext(signal)
    const context1 = withResourceCall(base, {
      client: client1,
      parentResourceId: parent1,
      serviceId: 'test.First',
      methodId: 'Construct',
    })
    const context2 = withResourceCall(base, {
      client: client2,
      parentResourceId: parent2,
      serviceId: 'test.Second',
      methodId: 'Construct',
    })

    const child1 = getResourceCall(context1).constructChildResource(() => ({
      mux: createMux(),
      result: 'first',
    }))
    const child2 = getResourceCall(context2).constructChildResource(() => ({
      mux: createMux(),
      result: 'second',
    }))

    expect(client1.resources.has(child1.resourceId)).toBe(true)
    expect(client1.resources.has(child2.resourceId)).toBe(false)
    expect(client2.resources.has(child1.resourceId)).toBe(false)
    expect(client2.resources.has(child2.resourceId)).toBe(true)
    expect(() => getResourceCall(base)).toThrow(
      'no resource call in server context',
    )
    client1.releaseAll()
    client2.releaseAll()
  })
})

describe('newResourceMux', () => {
  it('creates a mux', () => {
    const mux = newResourceMux()
    expect(mux).toBeDefined()
    expect(typeof mux.lookupMethod).toBe('function')
  })

  it('registers provided handlers', () => {
    const handler = {
      getServiceID: () => 'test.Service',
      getMethodIDs: () => ['TestMethod'],
      lookupMethod: async (_serviceID: string, _methodID: string) => null,
    }
    const mux = newResourceMux(handler)
    expect(mux).toBeDefined()
  })
})

// createControllableStream builds an async iterable of ResourceAttachRequest
// where packets can be pushed imperatively and the stream ended on demand.
function createControllableStream() {
  let resolve: ((value: IteratorResult<ResourceAttachRequest>) => void) | null =
    null
  const queue: ResourceAttachRequest[] = []
  let done = false

  const push = (pkt: ResourceAttachRequest) => {
    if (resolve) {
      const r = resolve
      resolve = null
      r({ value: pkt, done: false })
    } else {
      queue.push(pkt)
    }
  }

  const end = () => {
    done = true
    if (resolve) {
      const r = resolve
      resolve = null
      r({ value: undefined as never, done: true })
    }
  }

  const iterable: AsyncIterable<ResourceAttachRequest> = {
    [Symbol.asyncIterator]() {
      return {
        next(): Promise<IteratorResult<ResourceAttachRequest>> {
          if (queue.length > 0) {
            return Promise.resolve({ value: queue.shift()!, done: false })
          }
          if (done) {
            return Promise.resolve({ value: undefined as never, done: true })
          }
          return new Promise<IteratorResult<ResourceAttachRequest>>((r) => {
            resolve = r
          })
        },
      }
    },
  }

  return { push, end, iterable }
}

function createResourceClientControlStream() {
  const queue: ResourceClientRequest[] = [
    { body: { case: 'init' as const, value: {} } },
  ]
  let resolve:
    | ((result: IteratorResult<ResourceClientRequest>) => void)
    | null = null
  let done = false
  const end = () => {
    done = true
    resolve?.({ value: undefined as never, done: true })
    resolve = null
  }
  const iterable: AsyncIterable<ResourceClientRequest> = {
    [Symbol.asyncIterator]() {
      return {
        next() {
          if (queue.length) {
            return Promise.resolve({ value: queue.shift()!, done: false })
          }
          if (done)
            return Promise.resolve({ value: undefined as never, done: true })
          return new Promise<IteratorResult<ResourceClientRequest>>((r) => {
            resolve = r
          })
        },
      }
    },
  }
  return { iterable, end }
}

function resourceClientInit(): AsyncIterable<ResourceClientRequest> {
  return (async function* () {
    yield { body: { case: 'init' as const, value: {} } }
    await new Promise<void>(() => {})
  })()
}

// setupClientSession creates a ResourceClient session and returns the
// client handle ID and the internal RemoteResourceClient instance.
async function setupClientSession(server: ResourceServer) {
  const clientController = new AbortController()
  const clientStream = server.ResourceClient(
    resourceClientInit(),
    clientController.signal,
  )
  const clientIter = clientStream[Symbol.asyncIterator]()
  const { value: initMsg } = await clientIter.next()
  const clientHandleId =
    initMsg.body?.case === 'init' ? (initMsg.body.value.clientHandleId ?? 0) : 0
  const clients = (
    server as unknown as { clients: Map<number, RemoteResourceClient> }
  ).clients
  const client = clients.get(clientHandleId)!
  return { clientController, clientIter, clientHandleId, client }
}

// sendAddAndGetResourceId pushes an Add message and reads the addAck,
// returning the server-assigned resourceId.
async function sendAddAndGetResourceId(
  stream: ReturnType<typeof createControllableStream>,
  attachIter: AsyncIterator<ResourceAttachResponse>,
  label: string,
): Promise<number> {
  stream.push({
    body: {
      case: 'add' as const,
      value: { attachId: 1, label },
    },
  })
  const { value: addAckPkt } = await attachIter.next()
  expect(addAckPkt.body?.case).toBe('addAck')
  return addAckPkt.body?.case === 'addAck'
    ? (addAckPkt.body.value.resourceId ?? 0)
    : 0
}

// readNextControl reads from the attach iterator, skipping muxData
// packets, until a packet matching the expected case is found.
async function readNextControl(
  attachIter: AsyncIterator<ResourceAttachResponse>,
  expectedCase: string,
): Promise<ResourceAttachResponse> {
  for (;;) {
    const { value, done } = await attachIter.next()
    if (done) throw new Error('stream ended before control packet')
    if (value.body?.case === 'muxData') continue
    expect(value.body?.case).toBe(expectedCase)
    return value
  }
}

// readNextControlPacket sends an add message then reads the addAck,
// skipping any interleaved muxData packets.
async function readNextControlPacket(
  stream: ReturnType<typeof createControllableStream>,
  attachIter: AsyncIterator<ResourceAttachResponse>,
  label: string,
): Promise<number> {
  stream.push({
    body: {
      case: 'add' as const,
      value: { attachId: 1, label },
    },
  })
  const pkt = await readNextControl(attachIter, 'addAck')
  return pkt.body?.case === 'addAck' ? (pkt.body.value.resourceId ?? 0) : 0
}

describe('ResourceAttach handler', () => {
  describe('Init/Ack handshake', () => {
    it('sends session ack after valid init', async () => {
      const server = new ResourceServer(createMux())
      const { clientController, clientIter, clientHandleId } =
        await setupClientSession(server)

      const stream = createControllableStream()
      stream.push({
        body: {
          case: 'init' as const,
          value: { clientHandleId },
        },
      })

      const attachGen = server.ResourceAttach(stream.iterable)
      const attachIter = attachGen[Symbol.asyncIterator]()
      const { value: ackPkt } = await attachIter.next()

      expect(ackPkt.body?.case).toBe('ack')
      if (ackPkt.body?.case === 'ack') {
        expect(ackPkt.body.value.error).toBeFalsy()
      }

      stream.end()
      // Drain remaining output so the generator completes.
      for await (const _ of { [Symbol.asyncIterator]: () => attachIter }) {
        // consume
      }

      clientController.abort()
      await clientIter.next()
    })

    it('sends addAck with resourceId after add', async () => {
      const server = new ResourceServer(createMux())
      const { clientController, clientIter, clientHandleId } =
        await setupClientSession(server)

      const stream = createControllableStream()
      stream.push({
        body: {
          case: 'init' as const,
          value: { clientHandleId },
        },
      })

      const attachGen = server.ResourceAttach(stream.iterable)
      const attachIter = attachGen[Symbol.asyncIterator]()
      await attachIter.next()

      const resourceId = await sendAddAndGetResourceId(
        stream,
        attachIter,
        'test-attach',
      )
      expect(resourceId).toBeGreaterThan(0)

      stream.end()
      for await (const _ of { [Symbol.asyncIterator]: () => attachIter }) {
        // consume
      }

      clientController.abort()
      await clientIter.next()
    })

    it('sends error ack for unknown clientHandleId', async () => {
      const server = new ResourceServer(createMux())

      const stream = createControllableStream()
      stream.push({
        body: {
          case: 'init' as const,
          value: { clientHandleId: 9999 },
        },
      })
      stream.end()

      const attachGen = server.ResourceAttach(stream.iterable)
      const attachIter = attachGen[Symbol.asyncIterator]()
      const { value: ackPkt } = await attachIter.next()

      expect(ackPkt.body?.case).toBe('ack')
      if (ackPkt.body?.case === 'ack') {
        expect(ackPkt.body.value.error).toBe('client not found')
      }

      const { done } = await attachIter.next()
      expect(done).toBe(true)
    })

    it('sends error ack for released client', async () => {
      const server = new ResourceServer(createMux())
      const { clientController, clientIter, clientHandleId } =
        await setupClientSession(server)

      // Release the client session.
      clientController.abort()
      await clientIter.next()

      const stream = createControllableStream()
      stream.push({
        body: {
          case: 'init' as const,
          value: { clientHandleId },
        },
      })
      stream.end()

      const attachGen = server.ResourceAttach(stream.iterable)
      const attachIter = attachGen[Symbol.asyncIterator]()
      const { value: ackPkt } = await attachIter.next()

      expect(ackPkt.body?.case).toBe('ack')
      if (ackPkt.body?.case === 'ack') {
        expect(ackPkt.body.value.error).toBe('client not found')
      }

      const { done } = await attachIter.next()
      expect(done).toBe(true)
    })

    it('throws on stream closed before init', async () => {
      const server = new ResourceServer(createMux())

      // Empty iterable: yields nothing.
      const empty: AsyncIterable<ResourceAttachRequest> = {
        [Symbol.asyncIterator]() {
          return {
            next() {
              return Promise.resolve({
                value: undefined as never,
                done: true,
              })
            },
          }
        },
      }

      const attachGen = server.ResourceAttach(empty)
      const attachIter = attachGen[Symbol.asyncIterator]()
      await expect(attachIter.next()).rejects.toThrow(
        'stream closed before init',
      )
    })

    it('throws on non-init first packet', async () => {
      const server = new ResourceServer(createMux())

      const stream = createControllableStream()
      stream.push({
        body: {
          case: 'muxData' as const,
          value: new Uint8Array([0x00]),
        },
      })
      stream.end()

      const attachGen = server.ResourceAttach(stream.iterable)
      const attachIter = attachGen[Symbol.asyncIterator]()
      await expect(attachIter.next()).rejects.toThrow('expected init packet')
    })
  })

  describe('attached resource tracking', () => {
    it('registers attached resource on client', async () => {
      const server = new ResourceServer(createMux())
      const { clientController, clientIter, clientHandleId, client } =
        await setupClientSession(server)

      const stream = createControllableStream()
      stream.push({
        body: {
          case: 'init' as const,
          value: { clientHandleId },
        },
      })

      const attachGen = server.ResourceAttach(stream.iterable)
      const attachIter = attachGen[Symbol.asyncIterator]()
      await attachIter.next()

      const resourceId = await sendAddAndGetResourceId(
        stream,
        attachIter,
        'tracked',
      )
      expect(resourceId).toBeGreaterThan(0)
      expect(client.attachedResources.has(resourceId)).toBe(true)

      stream.end()
      for await (const _ of { [Symbol.asyncIterator]: () => attachIter }) {
        // consume
      }
      clientController.abort()
      await clientIter.next()
    })

    it('attached resource has correct label', async () => {
      const server = new ResourceServer(createMux())
      const { clientController, clientIter, clientHandleId, client } =
        await setupClientSession(server)

      const stream = createControllableStream()
      stream.push({
        body: {
          case: 'init' as const,
          value: { clientHandleId },
        },
      })

      const attachGen = server.ResourceAttach(stream.iterable)
      const attachIter = attachGen[Symbol.asyncIterator]()
      await attachIter.next()

      const resourceId = await sendAddAndGetResourceId(
        stream,
        attachIter,
        'my-special-label',
      )
      const attached = client.attachedResources.get(resourceId)!
      expect(attached.label).toBe('my-special-label')

      stream.end()
      for await (const _ of { [Symbol.asyncIterator]: () => attachIter }) {
        // consume
      }
      clientController.abort()
      await clientIter.next()
    })

    it('attached resource has srpc client', async () => {
      const server = new ResourceServer(createMux())
      const { clientController, clientIter, clientHandleId, client } =
        await setupClientSession(server)

      const stream = createControllableStream()
      stream.push({
        body: {
          case: 'init' as const,
          value: { clientHandleId },
        },
      })

      const attachGen = server.ResourceAttach(stream.iterable)
      const attachIter = attachGen[Symbol.asyncIterator]()
      await attachIter.next()

      const resourceId = await sendAddAndGetResourceId(
        stream,
        attachIter,
        'with-client',
      )
      const attached = client.attachedResources.get(resourceId)!
      expect(attached.client).toBeDefined()
      expect(typeof attached.client.request).toBe('function')

      stream.end()
      for await (const _ of { [Symbol.asyncIterator]: () => attachIter }) {
        // consume
      }
      clientController.abort()
      await clientIter.next()
    })

    it('cleans up attached resource on stream end', async () => {
      const server = new ResourceServer(createMux())
      const { clientController, clientIter, clientHandleId, client } =
        await setupClientSession(server)

      const stream = createControllableStream()
      stream.push({
        body: {
          case: 'init' as const,
          value: { clientHandleId },
        },
      })

      const attachGen = server.ResourceAttach(stream.iterable)
      const attachIter = attachGen[Symbol.asyncIterator]()
      await attachIter.next()

      const resourceId = await sendAddAndGetResourceId(
        stream,
        attachIter,
        'cleanup-test',
      )
      expect(client.attachedResources.has(resourceId)).toBe(true)

      // End the incoming stream to trigger cleanup.
      stream.end()
      for await (const _ of { [Symbol.asyncIterator]: () => attachIter }) {
        // consume
      }

      expect(client.attachedResources.has(resourceId)).toBe(false)

      clientController.abort()
      await clientIter.next()
    })
  })

  describe('getAttachedRef integration', () => {
    it('getAttachedRef returns ref for attached resource', async () => {
      const server = new ResourceServer(createMux())
      const { clientController, clientIter, clientHandleId, client } =
        await setupClientSession(server)

      const stream = createControllableStream()
      stream.push({
        body: {
          case: 'init' as const,
          value: { clientHandleId },
        },
      })

      const attachGen = server.ResourceAttach(stream.iterable)
      const attachIter = attachGen[Symbol.asyncIterator]()
      await attachIter.next()

      const resourceId = await sendAddAndGetResourceId(
        stream,
        attachIter,
        'ref-test',
      )
      const ref = client.getAttachedRef(resourceId)
      expect(ref.resourceId).toBe(resourceId)
      expect(ref.released).toBe(false)

      stream.end()
      for await (const _ of { [Symbol.asyncIterator]: () => attachIter }) {
        // consume
      }
      clientController.abort()
      await clientIter.next()
    })

    it('getAttachedRef throws for unknown id', () => {
      const client = new RemoteResourceClient(() => 1, 1)
      expect(() => client.getAttachedRef(999)).toThrow(
        'attached resource 999 not found',
      )
    })

    it('raw attached ref rejects child refs', () => {
      const client = new RemoteResourceClient(() => 1, 1)
      const controller = new AbortController()
      client.attachedResources.set(10, {
        label: 'raw',
        client: buildMockSRPCClient(),
        signal: controller.signal,
        controller,
      })

      const ref = client.getRawAttachedRef(10)
      expect(() => ref.createRef(11)).toThrow(
        'Cannot create child ref from raw attached resource 10',
      )
    })

    it('attached tree child refs resolve through child resource id', () => {
      const client = new RemoteResourceClient(() => 1, 1)
      const rootController = new AbortController()
      const childController = new AbortController()
      const rootClient = buildMockSRPCClient()
      const childClient = buildMockSRPCClient()
      client.attachedResources.set(10, {
        label: 'root',
        client: rootClient,
        signal: rootController.signal,
        controller: rootController,
      })
      client.attachedResources.set(11, {
        label: 'child',
        client: childClient,
        signal: childController.signal,
        controller: childController,
      })

      const rootRef = client.getAttachedRef(10)
      const childRef = rootRef.createRef(11)

      expect(rootRef.client).toBe(rootClient)
      expect(childRef.client).toBe(childClient)
    })

    it('attached tree ref release notifies owner and aborts attached signal', () => {
      const client = new RemoteResourceClient(() => 1, 1)
      const controller = new AbortController()
      const release = vi.fn()
      client.attachedResources.set(10, {
        label: 'tree',
        client: buildMockSRPCClient(),
        signal: controller.signal,
        controller,
        release,
      })

      const ref = client.getAttachedRef(10)
      ref.release()

      expect(release).toHaveBeenCalledOnce()
      expect(controller.signal.aborted).toBe(true)
      expect(client.attachedResources.has(10)).toBe(false)
      expect(ref.released).toBe(true)
    })

    it('attached ref becomes released when attach stream ends', async () => {
      const server = new ResourceServer(createMux())
      const { clientController, clientIter, clientHandleId, client } =
        await setupClientSession(server)

      const stream = createControllableStream()
      stream.push({
        body: {
          case: 'init' as const,
          value: { clientHandleId },
        },
      })

      const attachGen = server.ResourceAttach(stream.iterable)
      const attachIter = attachGen[Symbol.asyncIterator]()
      await attachIter.next()

      const resourceId = await sendAddAndGetResourceId(
        stream,
        attachIter,
        'release-check',
      )
      const ref = client.getAttachedRef(resourceId)
      expect(ref.released).toBe(false)

      // End the stream to trigger cleanup, which aborts the controller.
      stream.end()
      for await (const _ of { [Symbol.asyncIterator]: () => attachIter }) {
        // consume
      }

      // The ref checks signal.aborted, so after cleanup it is released.
      expect(ref.released).toBe(true)

      clientController.abort()
      await clientIter.next()
    })
  })

  describe('cleanup', () => {
    it('ResourceClient cleanup aborts all attached resources', async () => {
      const server = new ResourceServer(createMux())
      const { clientController, clientIter, clientHandleId, client } =
        await setupClientSession(server)

      const stream = createControllableStream()
      stream.push({
        body: {
          case: 'init' as const,
          value: { clientHandleId },
        },
      })

      const attachGen = server.ResourceAttach(stream.iterable)
      const attachIter = attachGen[Symbol.asyncIterator]()
      await attachIter.next()

      const resourceId = await sendAddAndGetResourceId(
        stream,
        attachIter,
        'abort-test',
      )
      const attached = client.attachedResources.get(resourceId)!
      expect(attached.signal.aborted).toBe(false)

      // Abort the ResourceClient session. The finally block in
      // ResourceClient aborts all attached resources.
      clientController.abort()
      await clientIter.next()

      expect(attached.signal.aborted).toBe(true)

      // Clean up the attach generator.
      stream.end()
      // The attach generator may have already errored from yamux teardown,
      // so catch any remaining iteration errors.
      try {
        for await (const _ of { [Symbol.asyncIterator]: () => attachIter }) {
          // consume
        }
      } catch {
        // Expected: yamux conn may error on teardown.
      }
    })

    it('detaching one resource does not abort another resource signal', async () => {
      const server = new ResourceServer(createMux())
      const { clientController, clientIter, clientHandleId, client } =
        await setupClientSession(server)

      const stream = createControllableStream()
      stream.push({
        body: {
          case: 'init' as const,
          value: { clientHandleId },
        },
      })

      const attachGen = server.ResourceAttach(stream.iterable)
      const attachIter = attachGen[Symbol.asyncIterator]()
      await attachIter.next()

      const resId1 = await sendAddAndGetResourceId(
        stream,
        attachIter,
        'resource-a',
      )

      const resId2 = await readNextControlPacket(
        stream,
        attachIter,
        'resource-b',
      )
      expect(resId2).toBeGreaterThan(0)

      const attached1 = client.attachedResources.get(resId1)!
      const attached2 = client.attachedResources.get(resId2)!
      expect(attached1.signal.aborted).toBe(false)
      expect(attached2.signal.aborted).toBe(false)

      // Detach resource 1.
      stream.push({
        body: {
          case: 'detach' as const,
          value: { resourceId: resId1 },
        },
      })
      const detachAck = await readNextControl(attachIter, 'detachAck')
      expect(detachAck).toBeDefined()

      // Resource 1 signal is aborted, resource 2 is not.
      expect(attached1.signal.aborted).toBe(true)
      expect(attached2.signal.aborted).toBe(false)
      expect(client.attachedResources.has(resId1)).toBe(false)
      expect(client.attachedResources.has(resId2)).toBe(true)

      stream.end()
      for await (const _ of { [Symbol.asyncIterator]: () => attachIter }) {
        // consume
      }
      clientController.abort()
      await clientIter.next()
    })

    it('detaching a resource aborts that resource signal', async () => {
      const server = new ResourceServer(createMux())
      const { clientController, clientIter, clientHandleId, client } =
        await setupClientSession(server)

      const stream = createControllableStream()
      stream.push({
        body: {
          case: 'init' as const,
          value: { clientHandleId },
        },
      })

      const attachGen = server.ResourceAttach(stream.iterable)
      const attachIter = attachGen[Symbol.asyncIterator]()
      await attachIter.next()

      const resourceId = await sendAddAndGetResourceId(
        stream,
        attachIter,
        'detach-signal-test',
      )
      const ref = client.getAttachedRef(resourceId)
      expect(ref.released).toBe(false)

      // Detach the resource.
      stream.push({
        body: {
          case: 'detach' as const,
          value: { resourceId },
        },
      })
      const detachAck = await readNextControl(attachIter, 'detachAck')
      expect(detachAck).toBeDefined()

      // The ref captured before detach now reports released.
      expect(ref.released).toBe(true)

      stream.end()
      for await (const _ of { [Symbol.asyncIterator]: () => attachIter }) {
        // consume
      }
      clientController.abort()
      await clientIter.next()
    })

    it('session close aborts all per-resource signals', async () => {
      const server = new ResourceServer(createMux())
      const { clientController, clientIter, clientHandleId, client } =
        await setupClientSession(server)

      const stream = createControllableStream()
      stream.push({
        body: {
          case: 'init' as const,
          value: { clientHandleId },
        },
      })

      const attachGen = server.ResourceAttach(stream.iterable)
      const attachIter = attachGen[Symbol.asyncIterator]()
      await attachIter.next()

      const resId1 = await sendAddAndGetResourceId(
        stream,
        attachIter,
        'session-close-a',
      )

      const resId2 = await readNextControlPacket(
        stream,
        attachIter,
        'session-close-b',
      )

      const attached1 = client.attachedResources.get(resId1)!
      const attached2 = client.attachedResources.get(resId2)!
      expect(attached1.signal.aborted).toBe(false)
      expect(attached2.signal.aborted).toBe(false)

      // End the attach stream to trigger session-level cleanup.
      stream.end()
      for await (const _ of { [Symbol.asyncIterator]: () => attachIter }) {
        // consume
      }

      // Session cleanup aborts the attachController, which propagates
      // to all per-resource controllers.
      expect(attached1.signal.aborted).toBe(true)
      expect(attached2.signal.aborted).toBe(true)

      clientController.abort()
      await clientIter.next()
    })
  })
})

function buildMockSRPCClient() {
  return {
    request: vi.fn(),
    clientStreamingRequest: vi.fn(),
    serverStreamingRequest: vi.fn(),
    bidirectionalStreamingRequest: vi.fn(),
  } as never
}

function promiseWithTimeout<T>(promise: Promise<T>, label: string): Promise<T> {
  return Promise.race([
    promise,
    new Promise<T>((_, reject) => {
      setTimeout(() => reject(new Error(`timed out waiting for ${label}`)), 100)
    }),
  ])
}
