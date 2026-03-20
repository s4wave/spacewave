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
      throw new Error(`Cannot access "${String(prop)}" on released attached resource`)
    },
  },
)

// createAttachedResourceRef builds a ClientResourceRef backed by an
// attached resource's srpc.Client. The ref does not need
// ResourceRefRelease -- attached resources are released when the
// attach stream closes.
function createAttachedResourceRef(
  id: number,
  client: SRPCClient,
  signal: AbortSignal,
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
      return released || signal.aborted
    },
    get client(): SRPCClient | ReleasedResourceClient {
      if (released || signal.aborted) {
        return releasedAttachedClient
      }
      return client
    },
    createRef(newId: number): ClientResourceRef {
      if (released || signal.aborted) {
        throw new Error(`Cannot create ref from released attached resource ${id}`)
      }
      return createAttachedResourceRef(newId, client, signal)
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
    return createAttachedResourceRef(id, attached.client, attached.signal)
  }

  // releaseResource releases a resource server-side and queues
  // a ResourceReleasedResponse to the client stream.
  releaseResource(resourceID: number): boolean {
    if (this.released) return false
    const resource = this.resources.get(resourceID)
    if (!resource) return false
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
