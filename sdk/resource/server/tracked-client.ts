import type { Mux } from 'starpc'
import type { ResourceClientResponse } from '../resource.pb.js'
import type { TrackedResource } from './tracked-resource.js'

// RemoteResourceClient tracks a connected client.
class RemoteResourceClient {
  readonly clientID: number
  readonly controller: AbortController
  released = false
  resources = new Map<number, TrackedResource>()

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
