import type { Mux, Client as SRPCClient } from 'starpc'
import type { ClientResourceRef, ReleasedResourceClient } from '../client.js'
import type { ResourceClientResponse } from '../resource.pb.js'
import type { AttachedResource } from './attached-resource.js'
import type { TrackedResource } from './tracked-resource.js'

// releasedAttachedClient is a singleton proxy for released attached refs.
const releasedAttachedClient: ReleasedResourceClient = new Proxy(
  { released: true } as ReleasedResourceClient,
  {
    get(_, prop) {
      if (prop === 'released') return true
      if (prop === 'toJSON') return () => ({ released: true })
      if (
        typeof prop === 'symbol' ||
        prop === 'constructor' ||
        prop === 'prototype' ||
        prop === '__proto__' ||
        prop === 'then' ||
        prop === 'asymmetricMatch' ||
        prop === 'nodeType' ||
        prop === 'tagName'
      ) {
        return undefined
      }
      throw new Error(
        `Cannot access "${String(prop)}" on released attached resource`,
      )
    },
  },
)

// createRawAttachedResourceRef builds a leaf ClientResourceRef backed by a
// single attached resource's srpc.Client. It cannot create child refs.
function createRawAttachedResourceRef(
  id: number,
  attached: AttachedResource,
): ClientResourceRef {
  let released = false

  const release = () => {
    released = true
  }

  const ref: ClientResourceRef = {
    get resourceId() {
      return id
    },
    get released() {
      return released || attached.signal.aborted
    },
    get client(): SRPCClient | ReleasedResourceClient {
      if (released || attached.signal.aborted) {
        return releasedAttachedClient
      }
      return attached.client
    },
    createRef(): ClientResourceRef {
      throw new Error(
        `Cannot create child ref from raw attached resource ${id}`,
      )
    },
    createResource<T>(): T {
      throw new Error(
        `Cannot create child resource from raw attached resource ${id}`,
      )
    },
    release,
    [Symbol.dispose]: release,
  }

  return ref
}

// createAttachedResourceRef builds a ClientResourceRef backed by an attached
// resource tree. Child refs resolve by child resource id through the owning
// RemoteResourceClient instead of reusing the parent routed client.
function createAttachedResourceRef(
  id: number,
  owner: RemoteResourceClient,
): ClientResourceRef {
  let released = false

  const release = () => {
    if (released) return
    released = true
    owner.releaseResource(id)
  }

  const ref: ClientResourceRef = {
    get resourceId() {
      return id
    },
    get released() {
      const attached = owner.attachedResources.get(id)
      return released || !attached || attached.signal.aborted
    },
    get client(): SRPCClient | ReleasedResourceClient {
      const attached = owner.attachedResources.get(id)
      if (released || !attached || attached.signal.aborted) {
        return releasedAttachedClient
      }
      return attached.client
    },
    createRef(newId: number): ClientResourceRef {
      const attached = owner.attachedResources.get(id)
      if (released || !attached || attached.signal.aborted) {
        throw new Error(
          `Cannot create ref from released attached resource ${id}`,
        )
      }
      if (!owner.attachedResources.has(newId)) {
        throw new Error(`attached child resource ${newId} not found`)
      }
      return createAttachedResourceRef(newId, owner)
    },
    createResource<T, Args extends unknown[]>(
      newId: number,
      ResourceClass: new (ref: ClientResourceRef, ...args: Args) => T,
      ...args: Args
    ): T {
      const childRef = this.createRef(newId)
      return new ResourceClass(childRef, ...args)
    },
    release,
    [Symbol.dispose]: release,
  }

  return ref
}

// RemoteResourceClient tracks a connected client.
class RemoteResourceClient {
  readonly clientID: number
  readonly controller: AbortController
  released = false
  resources = new Map<number, TrackedResource>()
  attachedResources = new Map<number, AttachedResource>()

  private txQueue: ResourceClientResponse[] = []
  private notifyCallbacks = new Set<() => void>()
  private nextResourceID: () => number

  constructor(
    nextResourceID: () => number,
    clientID: number,
    parentSignal?: AbortSignal,
  ) {
    this.nextResourceID = nextResourceID
    this.clientID = clientID
    this.controller = new AbortController()
    if (parentSignal) {
      parentSignal.addEventListener(
        'abort',
        () => {
          this.controller.abort()
        },
        { once: true },
      )
    }
  }

  // signal returns the client session lifetime signal.
  get signal(): AbortSignal {
    return this.controller.signal
  }

  // addResource allocates a globally unique resource ID and
  // registers the resource with this client.
  addResource(mux: Mux, releaseFn?: () => void): number {
    if (this.released) {
      throw new Error('client was released')
    }
    const resourceID = this.nextResourceID()
    this.resources.set(resourceID, {
      mux,
      ownerClientID: this.clientID,
      releaseFn,
    })
    return resourceID
  }

  // getAttachedRef returns a ClientResourceRef wrapping an
  // attached resource's srpc.Client.
  getAttachedRef(id: number): ClientResourceRef {
    const attached = this.attachedResources.get(id)
    if (!attached) {
      throw new Error(`attached resource ${id} not found`)
    }
    return createAttachedResourceRef(id, this)
  }

  // getRawAttachedRef returns a leaf ref for callback-style attached resources.
  // Raw attached refs cannot create child refs.
  getRawAttachedRef(id: number): ClientResourceRef {
    const attached = this.attachedResources.get(id)
    if (!attached) {
      throw new Error(`attached resource ${id} not found`)
    }
    return createRawAttachedResourceRef(id, attached)
  }

  // getRawAttachedClient returns the raw SRPC client for a leaf callback.
  getRawAttachedClient(id: number): SRPCClient {
    const attached = this.attachedResources.get(id)
    if (!attached) {
      throw new Error(`attached resource ${id} not found`)
    }
    return attached.client
  }

  // releaseResource releases a resource server-side and queues
  // a ResourceReleasedResponse to the client stream.
  releaseResource(resourceID: number): boolean {
    if (this.released) return false
    const resource = this.resources.get(resourceID)
    if (!resource) {
      const attached = this.attachedResources.get(resourceID)
      if (!attached) return false
      this.attachedResources.delete(resourceID)
      attached.release?.()
      attached.controller.abort()
      return true
    }
    this.resources.delete(resourceID)
    this.pushMessage({
      body: {
        case: 'resourceReleased' as const,
        value: { resourceId: resourceID },
      },
    })
    if (resource.releaseFn) {
      resource.releaseFn()
    }
    return true
  }

  // pushMessage adds a response to the txQueue and notifies
  // the ResourceClient transmit loop.
  pushMessage(msg: ResourceClientResponse): void {
    this.txQueue.push(msg)
    this.notify()
  }

  // drainQueue returns and clears all queued messages.
  drainQueue(): ResourceClientResponse[] {
    if (this.txQueue.length === 0) return []
    const msgs = this.txQueue
    this.txQueue = []
    return msgs
  }

  // waitForNotify returns a Promise that resolves when a message
  // is pushed or the signal aborts.
  waitForNotify(signal?: AbortSignal): Promise<void> {
    if (this.txQueue.length > 0 || this.released) {
      return Promise.resolve()
    }
    if (signal?.aborted) return Promise.resolve()
    return new Promise<void>((resolve) => {
      const onNotify = () => {
        signal?.removeEventListener('abort', onAbort)
        resolve()
      }
      const onAbort = () => {
        this.notifyCallbacks.delete(onNotify)
        resolve()
      }
      this.notifyCallbacks.add(onNotify)
      signal?.addEventListener('abort', onAbort, { once: true })
    })
  }

  // notify wakes the transmit loop.
  private notify(): void {
    for (const cb of this.notifyCallbacks) {
      this.notifyCallbacks.delete(cb)
      cb()
    }
  }
}

export { RemoteResourceClient }
