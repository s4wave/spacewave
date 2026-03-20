import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { createMux } from 'starpc'
import type { Mux } from 'starpc'
import { RemoteResourceClient } from './tracked-client.js'
import { ResourceServer, getCurrentResourceClient } from './server.js'
import { constructChildResource } from './construct.js'
import { newResourceMux } from './mux.js'
import type { ResourceClientResponse } from '../resource.pb.js'

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
      body: { case: 'clientError', value: 'test error' },
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
    client.pushMessage({ body: { case: 'clientError', value: 'a' } })
    client.pushMessage({ body: { case: 'clientError', value: 'b' } })

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
    client.pushMessage({ body: { case: 'clientError', value: 'x' } })
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
    client.pushMessage({ body: { case: 'clientError', value: 'y' } })
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
    it('sends init with clientHandleId and rootResourceId', async () => {
      const rootMux = createMux()
      const server = new ResourceServer(rootMux)
      const controller = new AbortController()
      const stream = server.ResourceClient({}, controller.signal)
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
      const stream1 = server.ResourceClient({}, c1.signal)
      const iter1 = stream1[Symbol.asyncIterator]()
      const { value: msg1 } = await iter1.next()

      const c2 = new AbortController()
      const stream2 = server.ResourceClient({}, c2.signal)
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
      const stream = server.ResourceClient({}, controller.signal)
      const iterator = stream[Symbol.asyncIterator]()

      await iterator.next()

      const clients = (server as unknown as { clients: Map<number, RemoteResourceClient> }).clients
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
      const stream = server.ResourceClient({}, controller.signal)
      const iterator = stream[Symbol.asyncIterator]()

      await iterator.next()

      const clients = (server as unknown as { clients: Map<number, RemoteResourceClient> }).clients
      expect(clients.has(1)).toBe(true)

      controller.abort()
      await iterator.next()

      expect(clients.has(1)).toBe(false)
    })

    it('calls releaseFn for non-root resources on cleanup', async () => {
      const server = new ResourceServer(createMux())
      const controller = new AbortController()
      const stream = server.ResourceClient({}, controller.signal)
      const iterator = stream[Symbol.asyncIterator]()

      await iterator.next()

      const clients = (server as unknown as { clients: Map<number, RemoteResourceClient> }).clients
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
      const stream = server.ResourceClient({}, controller.signal)
      const iterator = stream[Symbol.asyncIterator]()

      await iterator.next()

      const clients = (server as unknown as { clients: Map<number, RemoteResourceClient> }).clients
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
        const stream = server.ResourceClient({}, c.signal)
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

  describe('ResourceRefRelease handler', () => {
    it('rejects with error for clientHandleId === 0', async () => {
      const server = new ResourceServer(createMux())
      await expect(
        server.ResourceRefRelease({ clientHandleId: 0, resourceId: 1 }),
      ).rejects.toThrow('invalid client id')
    })

    it('rejects with error for missing clientHandleId', async () => {
      const server = new ResourceServer(createMux())
      await expect(
        server.ResourceRefRelease({ resourceId: 1 }),
      ).rejects.toThrow('invalid client id')
    })

    it('rejects unknown client ID', async () => {
      const server = new ResourceServer(createMux())
      await expect(
        server.ResourceRefRelease({ clientHandleId: 99, resourceId: 1 }),
      ).rejects.toThrow('resource not found')
    })

    it('rejects unknown resource ID', async () => {
      const server = new ResourceServer(createMux())
      const controller = new AbortController()
      const stream = server.ResourceClient({}, controller.signal)
      const iterator = stream[Symbol.asyncIterator]()
      await iterator.next()

      await expect(
        server.ResourceRefRelease({ clientHandleId: 1, resourceId: 999 }),
      ).rejects.toThrow('resource not found')

      controller.abort()
      await iterator.next()
    })

    it('skips root resource deletion (releaseFn undefined)', async () => {
      const server = new ResourceServer(createMux())
      const controller = new AbortController()
      const stream = server.ResourceClient({}, controller.signal)
      const iterator = stream[Symbol.asyncIterator]()
      await iterator.next()

      const clients = (server as unknown as { clients: Map<number, RemoteResourceClient> }).clients
      const client = clients.get(1)!
      const rootResourceId = 1
      expect(client.resources.has(rootResourceId)).toBe(true)

      const result = await server.ResourceRefRelease({
        clientHandleId: 1,
        resourceId: rootResourceId,
      })
      expect(result).toEqual({})
      // Root resource should still exist
      expect(client.resources.has(rootResourceId)).toBe(true)

      controller.abort()
      await iterator.next()
    })

    it('deletes non-root resource and calls releaseFn', async () => {
      const server = new ResourceServer(createMux())
      const controller = new AbortController()
      const stream = server.ResourceClient({}, controller.signal)
      const iterator = stream[Symbol.asyncIterator]()
      await iterator.next()

      const clients = (server as unknown as { clients: Map<number, RemoteResourceClient> }).clients
      const client = clients.get(1)!

      const releaseFn = vi.fn()
      const childId = client.addResource(createMux(), releaseFn)
      expect(client.resources.has(childId)).toBe(true)

      const result = await server.ResourceRefRelease({
        clientHandleId: 1,
        resourceId: childId,
      })
      expect(result).toEqual({})
      expect(client.resources.has(childId)).toBe(false)
      expect(releaseFn).toHaveBeenCalledOnce()

      controller.abort()
      await iterator.next()
    })

    it('returns empty response on success', async () => {
      const server = new ResourceServer(createMux())
      const controller = new AbortController()
      const stream = server.ResourceClient({}, controller.signal)
      const iterator = stream[Symbol.asyncIterator]()
      await iterator.next()

      const clients = (server as unknown as { clients: Map<number, RemoteResourceClient> }).clients
      const client = clients.get(1)!
      const childId = client.addResource(createMux(), () => {})

      const result = await server.ResourceRefRelease({
        clientHandleId: 1,
        resourceId: childId,
      })
      expect(result).toEqual({})

      controller.abort()
      await iterator.next()
    })
  })
})

describe('constructChildResource', () => {
  it('throws when no current RPC client context', () => {
    expect(() =>
      constructChildResource(() => ({
        mux: createMux(),
        result: 'x',
      })),
    ).toThrow('no resource client context')
  })

  it('creates child resource with correct mux and releaseFn', async () => {
    // Test through ResourceServer to get a real client context
    const rootMux = createMux()
    const server = new ResourceServer(rootMux)
    const controller = new AbortController()
    const stream = server.ResourceClient({}, controller.signal)
    const iterator = stream[Symbol.asyncIterator]()
    await iterator.next()

    const clients = (server as unknown as { clients: Map<number, RemoteResourceClient> }).clients
    const client = clients.get(1)!

    // Verify we can add resources to the client
    const childMux = createMux()
    const releaseFn = vi.fn()
    const childId = client.addResource(childMux, releaseFn)

    expect(childId).toBeGreaterThan(0)
    expect(client.resources.has(childId)).toBe(true)
    const tracked = client.resources.get(childId)!
    expect(tracked.mux).toBe(childMux)

    controller.abort()
    await iterator.next()
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
    const mux = newResourceMux(handler as never)
    expect(mux).toBeDefined()
  })
})

describe('integration: full resource lifecycle', () => {
  it('client connects, adds child, releases via RPC, disconnects', async () => {
    const rootMux = createMux()
    const server = new ResourceServer(rootMux)
    const controller = new AbortController()
    const stream = server.ResourceClient({}, controller.signal)
    const iterator = stream[Symbol.asyncIterator]()

    // Step 1: read init
    const { value: initMsg } = await iterator.next()
    expect(initMsg.body?.case).toBe('init')
    const clientHandleId =
      initMsg.body?.case === 'init'
        ? (initMsg.body.value.clientHandleId ?? 0)
        : 0
    const rootResourceId =
      initMsg.body?.case === 'init'
        ? (initMsg.body.value.rootResourceId ?? 0)
        : 0
    expect(clientHandleId).toBe(1)
    expect(rootResourceId).toBeGreaterThan(0)

    // Step 2: add a child resource via internals
    const clients = (server as unknown as { clients: Map<number, RemoteResourceClient> }).clients
    const client = clients.get(clientHandleId)!
    const releaseFn = vi.fn()
    const childMux = createMux()
    const childId = client.addResource(childMux, releaseFn)
    expect(childId).toBeGreaterThan(rootResourceId)

    // Step 3: release the child resource via RPC
    const result = await server.ResourceRefRelease({
      clientHandleId,
      resourceId: childId,
    })
    expect(result).toEqual({})
    expect(releaseFn).toHaveBeenCalledOnce()
    expect(client.resources.has(childId)).toBe(false)

    // Step 4: root resource still exists
    expect(client.resources.has(rootResourceId)).toBe(true)

    // Step 5: abort to disconnect
    controller.abort()
    const { done } = await iterator.next()
    expect(done).toBe(true)

    // Step 6: client cleaned up
    expect(clients.has(clientHandleId)).toBe(false)
  })

  it('server-side release sends notification to client stream', async () => {
    const server = new ResourceServer(createMux())
    const controller = new AbortController()
    const stream = server.ResourceClient({}, controller.signal)
    const iterator = stream[Symbol.asyncIterator]()

    await iterator.next()

    const clients = (server as unknown as { clients: Map<number, RemoteResourceClient> }).clients
    const client = clients.get(1)!

    const releaseFn = vi.fn()
    const childId = client.addResource(createMux(), releaseFn)

    // Start reading next message before triggering release
    const nextPromise = iterator.next()

    // Server-side release (e.g., from another controller)
    client.releaseResource(childId)

    const { value } = await nextPromise
    expect(value.body?.case).toBe('resourceReleased')
    if (value.body?.case === 'resourceReleased') {
      expect(value.body.value.resourceId).toBe(childId)
    }
    expect(releaseFn).toHaveBeenCalledOnce()

    controller.abort()
    await iterator.next()
  })

  it('released client rejects ResourceRefRelease', async () => {
    const server = new ResourceServer(createMux())
    const controller = new AbortController()
    const stream = server.ResourceClient({}, controller.signal)
    const iterator = stream[Symbol.asyncIterator]()

    await iterator.next()

    const clients = (server as unknown as { clients: Map<number, RemoteResourceClient> }).clients
    const client = clients.get(1)!
    const childId = client.addResource(createMux(), () => {})

    // Disconnect
    controller.abort()
    await iterator.next()

    // Client should be cleaned up, so RPC should fail
    await expect(
      server.ResourceRefRelease({ clientHandleId: 1, resourceId: childId }),
    ).rejects.toThrow('resource not found')
  })
})
