import '../../dispose-symbol.js'

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

interface PendingWarning {
  clientHandleId: number
  parentResourceID: number
  resourceID: number
  ageMS: number
  serviceID?: string
  methodID?: string
}

interface RemoteResourceClientOptions {
  now?: () => number
  onPendingWarning?: (warning: PendingWarning) => void
}

class RemoteResourceClient {
  readonly clientID: number
  readonly controller: AbortController
  released = false
  resources = new Map<number, TrackedResource>()
  attachedResources = new Map<number, AttachedResource>()
  // Tombstones retain every released ID until this immutable generation ends
  // so a late Adopt cannot revive or miss the matching release notification.
  readonly tombstones = new Set<number>()
  private retainedRootResourceID: number | undefined

  private txQueue: ResourceClientResponse[] = []
  private notifyCallbacks = new Set<() => void>()
  private nextResourceID: () => number
  private pendingChildren = new Map<number, Set<number>>()
  private warnedPending = new Set<number>()
  private warningTimer: ReturnType<typeof setTimeout> | undefined
  private readonly now: () => number
  private readonly onPendingWarning: (warning: PendingWarning) => void

  constructor(
    nextResourceID: () => number,
    clientID: number,
    parentSignal?: AbortSignal,
    options: RemoteResourceClientOptions = {},
  ) {
    this.nextResourceID = nextResourceID
    this.clientID = clientID
    this.controller = new AbortController()
    this.now = options.now ?? Date.now
    this.onPendingWarning =
      options.onPendingWarning ??
      ((warning) => console.warn('Resource remains pending adoption', warning))
    parentSignal?.addEventListener('abort', () => this.controller.abort(), {
      once: true,
    })
  }

  get signal(): AbortSignal {
    return this.controller.signal
  }
  setRetainedRootResourceID(resourceID: number): void {
    this.retainedRootResourceID = resourceID
  }

  addResource(
    mux: Mux,
    releaseFn?: () => void,
    parentResourceID?: number,
    serviceID?: string,
    methodID?: string,
  ): number {
    if (this.released) throw new Error('client was released')
    const resourceID = this.nextResourceID()
    this.resources.set(resourceID, {
      mux,
      ownerClientID: this.clientID,
      releaseFn,
      parentResourceID,
      pendingSince: parentResourceID === undefined ? undefined : this.now(),
      serviceID,
      methodID,
      adopted: false,
    })
    if (parentResourceID !== undefined) {
      let children = this.pendingChildren.get(parentResourceID)
      if (!children) {
        children = new Set()
        this.pendingChildren.set(parentResourceID, children)
      }
      children.add(resourceID)
      this.schedulePendingWarning()
    }
    return resourceID
  }

  adoptResource(resourceID: number): 'adopted' | 'released' | 'invalid' {
    if (this.tombstones.has(resourceID)) {
      this.pushReleased(resourceID)
      return 'released'
    }
    const resource = this.resources.get(resourceID)
    if (!resource) {
      return this.attachedResources.has(resourceID) ? 'adopted' : 'invalid'
    }
    resource.adopted = true
    resource.pendingSince = undefined
    this.warnedPending.delete(resourceID)
    this.schedulePendingWarning()
    return 'adopted'
  }

  getAttachedRef(id: number): ClientResourceRef {
    if (!this.attachedResources.has(id)) {
      throw new Error(`attached resource ${id} not found`)
    }
    return createAttachedResourceRef(id, this)
  }

  getRawAttachedRef(id: number): ClientResourceRef {
    const attached = this.attachedResources.get(id)
    if (!attached) throw new Error(`attached resource ${id} not found`)
    return createRawAttachedResourceRef(id, attached)
  }

  getRawAttachedClient(id: number): SRPCClient {
    const attached = this.attachedResources.get(id)
    if (!attached) throw new Error(`attached resource ${id} not found`)
    return attached.client
  }

  releaseResource(resourceID: number, notify = true): boolean {
    if (this.released) return false
    const resource = this.resources.get(resourceID)
    if (!resource) {
      const attached = this.attachedResources.get(resourceID)
      if (!attached) return this.tombstones.has(resourceID)
      this.attachedResources.delete(resourceID)
      this.tombstones.add(resourceID)
      attached.controller.abort()
      if (notify) this.pushReleased(resourceID)
      attached.release?.()
      return true
    }
    if (resourceID === this.retainedRootResourceID) {
      this.releasePendingChildren(resourceID)
      return true
    }
    this.releasePendingChildren(resourceID)
    if (resource.parentResourceID !== undefined) {
      this.pendingChildren.get(resource.parentResourceID)?.delete(resourceID)
    }
    this.resources.delete(resourceID)
    this.tombstones.add(resourceID)
    if (notify) this.pushReleased(resourceID)
    resource.releaseFn?.()
    this.warnedPending.delete(resourceID)
    this.schedulePendingWarning()
    return true
  }

  releaseAll(): void {
    if (this.warningTimer !== undefined) {
      clearTimeout(this.warningTimer)
      this.warningTimer = undefined
    }
    for (const [resourceID, resource] of this.resources) {
      if (resource.parentResourceID === undefined) {
        this.releaseResourceTree(resourceID)
      }
    }
    // A released parent can leave an adopted child with provenance that no
    // longer reaches a root. Generation teardown still releases that orphan.
    while (this.resources.size > 0) {
      const resourceID = this.resources.keys().next().value
      if (resourceID === undefined) break
      this.releaseResourceTree(resourceID)
    }
    for (const resourceID of [...this.attachedResources.keys()]) {
      this.releaseResource(resourceID, false)
    }
    this.released = true
    this.resources.clear()
    this.pendingChildren.clear()
    this.controller.abort()
    this.notify()
  }

  pushMessage(msg: ResourceClientResponse): void {
    this.txQueue.push(msg)
    this.notify()
  }

  drainQueue(): ResourceClientResponse[] {
    const msgs = this.txQueue
    this.txQueue = []
    return msgs
  }

  waitForNotify(signal?: AbortSignal): Promise<void> {
    if (this.txQueue.length > 0 || this.released) return Promise.resolve()
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

  private rootResourceID(): number {
    for (const [id, resource] of this.resources) {
      if (resource.parentResourceID === undefined) return id
    }
    return 0
  }

  private releaseResourceTree(resourceID: number): void {
    for (const childID of this.pendingChildren.get(resourceID) ?? []) {
      this.releaseResourceTree(childID)
    }
    const resource = this.resources.get(resourceID)
    if (!resource) return
    this.resources.delete(resourceID)
    this.tombstones.add(resourceID)
    resource.releaseFn?.()
  }
  private releasePendingChildren(parentID: number): void {
    const children = [...(this.pendingChildren.get(parentID) ?? [])]
    for (const childID of children) {
      const child = this.resources.get(childID)
      if (child?.adopted) continue
      this.releaseResource(childID)
    }
  }

  private pushReleased(resourceID: number): void {
    this.pushMessage({
      body: {
        case: 'resourceReleased' as const,
        value: { resourceId: resourceID },
      },
    })
  }

  private scanPending(): void {
    if (this.released) return
    const now = this.now()
    for (const [resourceID, resource] of this.resources) {
      if (
        !resource.adopted &&
        resource.pendingSince !== undefined &&
        now - resource.pendingSince >= 10000 &&
        !this.warnedPending.has(resourceID)
      ) {
        this.warnedPending.add(resourceID)
        this.onPendingWarning({
          clientHandleId: this.clientID,
          parentResourceID: resource.parentResourceID ?? 0,
          resourceID,
          ageMS: now - resource.pendingSince,
          serviceID: resource.serviceID,
          methodID: resource.methodID,
        })
      }
    }
    this.schedulePendingWarning()
  }

  private schedulePendingWarning(): void {
    if (this.warningTimer !== undefined) {
      clearTimeout(this.warningTimer)
      this.warningTimer = undefined
    }
    if (this.released) return

    const now = this.now()
    let delay: number | undefined
    for (const [resourceID, resource] of this.resources) {
      if (
        resource.adopted ||
        resource.pendingSince === undefined ||
        this.warnedPending.has(resourceID)
      ) {
        continue
      }
      const remaining = Math.max(0, resource.pendingSince + 10000 - now)
      delay = delay === undefined ? remaining : Math.min(delay, remaining)
    }
    if (delay === undefined) return
    this.warningTimer = setTimeout(() => {
      this.warningTimer = undefined
      this.scanPending()
    }, delay)
  }

  private notify(): void {
    for (const callback of this.notifyCallbacks) {
      this.notifyCallbacks.delete(callback)
      callback()
    }
  }
}

export { RemoteResourceClient }
