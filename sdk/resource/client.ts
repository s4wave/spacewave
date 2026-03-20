import { createAbortController, retryWithAbort } from '@aptre/bldr'
import type { ResourceService } from './resource_srpc.pb.js'
import type {
  ResourceAttachRequest,
  ResourceAttachResponse,
} from './resource.pb.js'
import {
  buildRpcStreamOpenStream,
  Client as SRPCClient,
  Server,
  StreamConn,
  combineUint8ArrayListTransform,
} from 'starpc'
import type { LookupMethod } from 'starpc'
import { pushable } from 'it-pushable'
import type { Pushable } from 'it-pushable'
import { pipe } from 'it-pipe'

// ReleasedResourceClient is returned from the client getter when a resource has been released.
// It allows DevTools serialization to work without throwing, but throws on actual usage.
export type ReleasedResourceClient = SRPCClient & { readonly released: true }

// releasedResourceClient is a singleton proxy returned for released resources.
// Returns undefined for inspection properties (React internals, symbols, etc.)
// but throws when actually trying to use the client for RPC calls.
const releasedResourceClient: ReleasedResourceClient = new Proxy(
  { released: true } as ReleasedResourceClient,
  {
    get(target, prop) {
      if (prop === 'released') return true
      if (prop === 'toJSON') return () => ({ released: true })
      // Return undefined for inspection properties used by React/DevTools/vitest/pretty-format
      // This includes:
      // - Symbols (Symbol.toStringTag, Symbol.iterator, etc.)
      // - React internals ($$typeof, etc.)
      // - JS internals (constructor, prototype, __proto__)
      // - Immutable.js checks (@@__IMMUTABLE_*)
      // - Node/CommonJS internals (then, asymmetricMatch, nodeType, etc.)
      const propStr = String(prop)
      if (
        typeof prop === 'symbol' ||
        propStr.startsWith('$$') ||
        propStr.startsWith('@@__') ||
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
      throw new ResourceClientError(
        `Cannot access "${propStr}" on released resource`,
        'INVALID_RESOURCE',
      )
    },
  },
)

/**
 * A reference to a remote resource that can be used to communicate with it.
 * Each resource has a unique ID and must be explicitly released when no longer needed.
 */
export interface ClientResourceRef {
  readonly resourceId: number
  readonly released: boolean
  readonly client: SRPCClient | ReleasedResourceClient
  createRef(id: number): ClientResourceRef
  createResource<T, Args extends unknown[]>(
    id: number,
    ResourceClass: new (ref: ClientResourceRef, ...args: Args) => T,
    ...args: Args
  ): T
  release(): void
  [Symbol.dispose]: () => void
}

/**
 * Event fired when a resource is released.
 */
export interface ResourceReleasedEvent {
  readonly resourceId: number
  readonly reason: ResourceReleaseReason
}

/**
 * Reasons why a resource might be released.
 */
export type ResourceReleaseReason =
  | 'client-released' // Client called release()
  | 'server-released' // Server notified us of release
  | 'connection-lost' // Connection was lost
  | 'client-disposed' // Client was disposed/cancelled

/**
 * Errors that can occur during client operations.
 */
export class ResourceClientError extends Error {
  constructor(
    message: string,
    public readonly code: ResourceClientErrorCode,
    public readonly cause?: Error,
  ) {
    super(message)
    this.name = 'ResourceClientError'
  }
}

export type ResourceClientErrorCode =
  | 'CLIENT_CANCELLED'
  | 'CLIENT_DISPOSED'
  | 'CONNECTION_FAILED'
  | 'SERVER_ERROR'
  | 'INVALID_RESOURCE'

/**
 * Simple event emitter for resource lifecycle events.
 */
class EventEmitter<T> {
  private listeners: ((event: T) => void)[] = []

  on(listener: (event: T) => void): () => void {
    this.listeners.push(listener)
    return () => {
      const index = this.listeners.indexOf(listener)
      if (index >= 0) this.listeners.splice(index, 1)
    }
  }

  emit(event: T): void {
    // Copy listeners to avoid issues if listeners are modified during emit
    const currentListeners = [...this.listeners]
    currentListeners.forEach((listener) => {
      try {
        listener(event)
      } catch (error) {
        // Don't let listener errors break the emit
        console.error('Error in event listener:', error)
      }
    })
  }

  clear(): void {
    this.listeners.length = 0
  }
}

/**
 * Internal interface that extends the public interface with mutable state.
 */
interface InternalResourceRef extends ClientResourceRef {
  _markReleased(): void
}

/**
 * Client initialization state.
 */
interface ClientInitState {
  clientHandleId: number
  rootResourceId: number
}

/**
 * Creates a reference to a remote resource.
 */
function createResourceRef(
  id: number,
  client: Client,
  onRelease: (id: number, ref: InternalResourceRef) => void,
): InternalResourceRef {
  let released = false

  const release = () => {
    if (released) return
    released = true
    onRelease(id, ref)
  }

  // Create SRPC client lazily to avoid creating connections for unused refs
  let srpcClient: SRPCClient | null = null
  const getSrpcClient = (): SRPCClient => {
    if (!srpcClient) {
      const stream = buildRpcStreamOpenStream(
        id.toString(),
        client.service.ResourceRpc.bind(client.service),
      )
      srpcClient = new SRPCClient(stream)
    }
    return srpcClient
  }

  const ref: InternalResourceRef = {
    get resourceId() {
      return id
    },

    get released() {
      return released
    },

    get client(): SRPCClient | ReleasedResourceClient {
      if (released) {
        return releasedResourceClient
      }
      return getSrpcClient()
    },

    createRef(newId: number): ClientResourceRef {
      if (released) {
        throw new ResourceClientError(
          `Cannot create ref from released resource ${id}`,
          'INVALID_RESOURCE',
        )
      }
      return client.createResourceReference(newId)
    },

    createResource<T, Args extends unknown[]>(
      newId: number,
      ResourceClass: new (ref: ClientResourceRef, ...args: Args) => T,
      ...args: Args
    ): T {
      const ref = this.createRef(newId)
      return new ResourceClass(ref, ...args)
    },

    release,
    [Symbol.dispose]: release,

    _markReleased() {
      released = true
    },
  }

  return ref
}

// AttachSession manages the single ResourceAttach stream + yamux session.
interface AttachSession {
  outgoing: Pushable<ResourceAttachRequest>
  attachIdCtr: number
  muxes: Map<number, LookupMethod>
  pending: Map<number, (resourceId: number) => void>
}

/**
 * Manages connections to remote resources via RPC.
 * Handles resource lifecycle, reference counting, and cleanup.
 *
 * Note: Server-side handlers may send the same resource ID to the client multiple times
 * (out-of-band from this Client). Additionally, client code may create multiple references
 * to the same resource ID. We use reference counting to track when all client-side
 * references to a resource have been released before notifying the server.
 */
export class Client {
  private initState: ClientInitState | null = null
  private connectionController: AbortController | null = null
  private resources = new Map<number, Set<InternalResourceRef>>()
  private events = new EventEmitter<ResourceReleasedEvent>()
  private connectionLostEvents = new EventEmitter<void>()
  private initPromise: Promise<ClientInitState> | null = null
  private disposed = false
  private _connectionGeneration = 0
  private _reconnectResolve: ((state: ClientInitState) => void) | null = null
  private attachSession: AttachSession | null = null

  constructor(
    public readonly service: ResourceService,
    private readonly signal: AbortSignal,
  ) {
    // Clean up when the main signal is aborted
    signal.addEventListener(
      'abort',
      () => {
        this.dispose('CLIENT_CANCELLED')
      },
      { once: true },
    )
  }

  /**
   * The connection generation counter. Increments each time the connection
   * is lost and resources are released. React hooks can use this to detect
   * when resources need to be re-created.
   */
  get connectionGeneration(): number {
    return this._connectionGeneration
  }

  /**
   * Register a callback for when resources are released.
   * Returns an unsubscribe function.
   */
  onResourceReleased(
    callback: (event: ResourceReleasedEvent) => void,
  ): () => void {
    this.throwIfDisposed()
    return this.events.on(callback)
  }

  /**
   * Register a callback for when the connection is lost and all resources
   * are released. This fires when the server connection drops and reconnects.
   * Returns an unsubscribe function.
   */
  onConnectionLost(callback: () => void): () => void {
    this.throwIfDisposed()
    return this.connectionLostEvents.on(callback)
  }

  /**
   * Get a reference to the root resource.
   * This starts the client connection if not already active.
   */
  async accessRootResource(): Promise<ClientResourceRef> {
    const state = await this.ensureInitialized()
    return this.createResourceReference(state.rootResourceId)
  }

  // attachResource provides a mux that server-side handlers can
  // invoke. The mux is served over a yamux session inside the
  // ResourceAttach bidi stream. Multiple resources share one session.
  // Returns the server-assigned resource ID and a cleanup function.
  async attachResource(
    label: string,
    mux: LookupMethod,
    signal?: AbortSignal,
  ): Promise<{ resourceId: number; cleanup: () => void }> {
    const sess = await this.ensureAttachSession(signal)

    // Allocate attach correlation ID.
    const attachId = ++sess.attachIdCtr
    const resultPromise = new Promise<number>((resolve, reject) => {
      sess.pending.set(attachId, resolve)
      signal?.addEventListener('abort', () => {
        sess.pending.delete(attachId)
        reject(new Error('aborted'))
      }, { once: true })
    })

    // Send Add.
    sess.outgoing.push({
      body: {
        case: 'add' as const,
        value: { attachId, label },
      },
    })

    // Wait for AddAck.
    const resourceId = await resultPromise

    // Register the mux for routed dispatch.
    sess.muxes.set(resourceId, mux)

    const cleanup = () => {
      sess.muxes.delete(resourceId)
      // Send Detach (best-effort).
      sess.outgoing.push({
        body: {
          case: 'detach' as const,
          value: { resourceId },
        },
      })
    }

    return { resourceId, cleanup }
  }

  // ensureAttachSession opens the ResourceAttach bidi stream if needed.
  private async ensureAttachSession(
    signal?: AbortSignal,
  ): Promise<AttachSession> {
    if (this.attachSession) return this.attachSession
    const state = await this.ensureInitialized()

    // Create outgoing packet pushable.
    const outgoing = pushable<ResourceAttachRequest>({ objectMode: true })

    // Open the ResourceAttach bidi stream.
    const incoming = this.service.ResourceAttach(
      (async function* () {
        yield* outgoing
      })(),
      signal,
    )
    const incomingIt = incoming[Symbol.asyncIterator]()

    // Send session-only Init.
    outgoing.push({
      body: {
        case: 'init' as const,
        value: { clientHandleId: state.clientHandleId },
      },
    })

    // Read session Ack.
    const ackResult = await incomingIt.next()
    if (ackResult.done) {
      outgoing.end()
      throw new Error('stream closed before ack')
    }
    const ackBody = ackResult.value?.body
    if (ackBody?.case !== 'ack') {
      outgoing.end()
      throw new Error('expected ack packet')
    }
    if (ackBody.value.error) {
      outgoing.end()
      throw new Error(ackBody.value.error)
    }

    const sess: AttachSession = {
      outgoing,
      attachIdCtr: 0,
      muxes: new Map(),
      pending: new Map(),
    }

    // Create yamux StreamConn.
    // CLIENT side is yamux server (inbound) -- accepts streams
    // and routes to the correct mux via service ID prefix routing.
    const routedLookup: LookupMethod = async (serviceId: string, methodId: string) => {
      const slashIdx = serviceId.indexOf('/')
      if (slashIdx < 0) return null
      const resourceId = parseInt(serviceId.substring(0, slashIdx), 10)
      if (isNaN(resourceId)) return null
      const mux = sess.muxes.get(resourceId)
      if (!mux) return null
      return mux(serviceId.substring(slashIdx + 1), methodId)
    }
    const server = new Server(routedLookup)
    const conn = new StreamConn(server, {
      direction: 'inbound',
      yamuxParams: { enableKeepAlive: false },
    })

    // Pipe mux_data between ResourceAttach stream and yamux.
    // Incoming packets -> dispatch control or extract mux_data bytes.
    const incomingBytes = (async function* () {
      for (;;) {
        const result = await incomingIt.next()
        if (result.done) break
        const body = result.value?.body
        if (body?.case === 'muxData') {
          yield body.value
        } else if (body?.case === 'addAck') {
          const addAck = body.value
          if (!addAck.error) {
            const resolve = sess.pending.get(addAck.attachId ?? 0)
            sess.pending.delete(addAck.attachId ?? 0)
            resolve?.(addAck.resourceId ?? 0)
          }
        }
        // detachAck: no action needed.
      }
    })()

    // conn.source -> wrap as mux_data -> push to outgoing.
    pipe(
      incomingBytes,
      conn,
      combineUint8ArrayListTransform(),
      async (source: AsyncIterable<Uint8Array>) => {
        for await (const chunk of source) {
          outgoing.push({
            body: {
              case: 'muxData' as const,
              value: chunk,
            },
          })
        }
        outgoing.end()
      },
    ).catch(() => {
      outgoing.end()
    })

    this.attachSession = sess
    return sess
  }

  /**
   * Create a reference to a specific resource by ID.
   * The resource should already exist on the server.
   */
  createResourceReference(id: number): ClientResourceRef {
    this.throwIfDisposed()
    return this.createRef(id)
  }

  /**
   * Dispose the client and clean up all resources.
   */
  dispose(reason: ResourceClientErrorCode = 'CLIENT_DISPOSED'): void {
    if (this.disposed) return
    this.disposed = true

    // Cancel the connection
    if (this.connectionController) {
      this.connectionController.abort()
      this.connectionController = null
    }

    // Mark all resources as released and emit events
    const releaseReason: ResourceReleaseReason =
      reason === 'CLIENT_CANCELLED' ? 'client-disposed' : 'client-disposed'

    for (const [resourceId, refs] of this.resources.entries()) {
      refs.forEach((ref) => ref._markReleased())
      this.events.emit({ resourceId, reason: releaseReason })
    }

    // Clean up state
    this.resources.clear()
    this.events.clear()
    this.connectionLostEvents.clear()
    this.initState = null
    this.initPromise = null
  }

  /**
   * Ensure the client is initialized and return the init state.
   */
  private async ensureInitialized(): Promise<ClientInitState> {
    this.throwIfDisposed()

    // Return existing state if available
    if (this.initState) {
      return this.initState
    }

    // Return existing promise if already initializing
    if (this.initPromise) {
      return this.initPromise
    }

    // Start initialization
    this.initPromise = this.performInitialization()
    return this.initPromise
  }

  /**
   * Perform the actual client initialization.
   */
  private async performInitialization(): Promise<ClientInitState> {
    this.throwIfDisposed()

    // Create connection controller
    this.connectionController = createAbortController(this.signal)

    return new Promise<ClientInitState>((resolve, reject) => {
      let initialized = false

      const cleanup = () => {
        if (!initialized) {
          this.connectionController = null
          this.initPromise = null
        }
      }

      // Handle cancellation
      const handleCancel = () => {
        if (!initialized) {
          cleanup()
          reject(
            new ResourceClientError(
              'Client initialization was cancelled',
              'CLIENT_CANCELLED',
            ),
          )
        }
      }

      if (this.signal.aborted) {
        handleCancel()
        return
      }

      this.signal.addEventListener('abort', handleCancel, { once: true })

      // Start the connection with retry
      this.startConnection(resolve, reject, () => {
        initialized = true
        this.signal.removeEventListener('abort', handleCancel)
      }).catch((error) => {
        cleanup()
        if (!initialized) {
          const cause =
            error instanceof Error ? error : new Error(String(error))
          reject(
            new ResourceClientError(
              'Failed to initialize client connection',
              'CONNECTION_FAILED',
              cause,
            ),
          )
        }
      })
    })
  }

  /**
   * Start the connection and handle the message stream.
   */
  private async startConnection(
    onInitialized: (state: ClientInitState) => void,
    onError: (error: Error) => void,
    markInitialized: () => void,
  ): Promise<void> {
    if (!this.connectionController) {
      throw new ResourceClientError(
        'No connection controller',
        'CONNECTION_FAILED',
      )
    }

    await retryWithAbort(this.connectionController.signal, async (signal) => {
      const stream = this.service.ResourceClient({}, signal)

      try {
        for await (const msg of stream) {
          if (signal.aborted) return

          // Handle initialization message
          if (msg.body?.case === 'init') {
            const clientHandleId = msg.body.value.clientHandleId ?? 0
            const rootResourceId = msg.body.value.rootResourceId ?? 0

            const state: ClientInitState = { clientHandleId, rootResourceId }

            if (this._reconnectResolve) {
              // Reconnected: resolve the reconnect promise so that
              // accessRootResource() unblocks with the new state.
              this.initState = state
              this._reconnectResolve(state)
              this._reconnectResolve = null
            } else if (!this.initState) {
              // First init: resolve the initialization promise
              this.initState = state
              markInitialized()
              onInitialized(state)
            } else {
              // Duplicate init (shouldn't happen): update state
              this.initState = state
            }
            continue
          }

          // Handle resource release notifications
          if (msg.body?.case === 'resourceReleased') {
            const resourceId = msg.body.value.resourceId ?? 0
            this.handleServerResourceRelease(resourceId)
            continue
          }

          // Handle errors
          if (msg.body?.case === 'clientError') {
            const error = new ResourceClientError(
              `Server error: ${msg.body.value}`,
              'SERVER_ERROR',
            )

            if (!this.initState) {
              onError(error)
            }
            throw error
          }
        }
      } finally {
        // Release all resources when connection ends (disconnect/error/reconnect).
        if (this.resources.size > 0) {
          this.releaseAllResources('connection-lost')
        }
      }
    })
  }

  /**
   * Throw an error if the client has been disposed.
   */
  private throwIfDisposed(): void {
    if (this.disposed) {
      throw new ResourceClientError(
        'Client has been disposed',
        'CLIENT_DISPOSED',
      )
    }
  }

  /**
   * Creates a new reference to a resource.
   */
  private createRef(id: number): ClientResourceRef {
    this.throwIfDisposed()

    // Initialize the resource set if it doesn't exist
    if (!this.resources.has(id)) {
      this.resources.set(id, new Set())
    }

    // Create the reference
    const ref = createResourceRef(id, this, this.releaseRef.bind(this))

    // Track this reference
    this.resources.get(id)!.add(ref)
    return ref
  }

  /**
   * Called when a reference is released.
   * Cleans up the resource if no more references exist.
   *
   * Since server handlers may send the same resource ID multiple times and
   * client code may create multiple references to the same resource ID,
   * we only notify the server when the last client-side reference is released.
   */
  private releaseRef(id: number, ref: InternalResourceRef): void {
    const refs = this.resources.get(id)
    if (!refs) return

    // Remove this reference
    refs.delete(ref)

    // If no more references to this resource ID, clean up completely
    if (refs.size === 0) {
      this.resources.delete(id)

      // Notify server (ignore errors during cleanup)
      this.notifyServerResourceRelease(id).catch(() => {})

      // Emit release event
      this.events.emit({
        resourceId: id,
        reason: 'client-released',
      })
    }
  }

  /**
   * Notify the server that a resource should be released.
   */
  private async notifyServerResourceRelease(resourceId: number): Promise<void> {
    if (!this.initState || this.disposed) {
      return
    }

    try {
      await this.service.ResourceRefRelease(
        {
          clientHandleId: this.initState.clientHandleId,
          resourceId,
        },
        this.signal,
      )
    } catch (error) {
      // Silently ignore RPC abort errors - these are expected during cleanup
      if (error instanceof Error && error.message.includes('ERR_RPC_ABORT')) {
        return
      }
      if (error instanceof Error) {
        console.warn(
          `Failed to notify server of resource ${resourceId} release:`,
          error,
        )
      } else {
        console.warn(
          `Failed to notify server of resource ${resourceId} release:`,
          String(error),
        )
      }
    }
  }

  /**
   * Called when the server notifies us that a resource was released.
   */
  private handleServerResourceRelease(resourceId: number): void {
    const refs = this.resources.get(resourceId)
    if (!refs) return

    // Mark all references as released
    refs.forEach((ref) => ref._markReleased())

    // Remove from tracking
    this.resources.delete(resourceId)

    // Emit event
    this.events.emit({
      resourceId,
      reason: 'server-released',
    })
  }

  /**
   * Release all resources with the given reason.
   * Used when connection is lost and resources are no longer valid.
   */
  private releaseAllResources(reason: ResourceReleaseReason): void {
    for (const [resourceId, refs] of this.resources.entries()) {
      refs.forEach((ref) => ref._markReleased())
      this.events.emit({ resourceId, reason })
    }
    this.resources.clear()

    // Clear init state and create a new promise so that accessRootResource()
    // waits for the retry to establish a new connection before returning.
    // Without this, React re-creates resources immediately using the old
    // initState, racing against the retry and hitting "resource not found"
    // errors because the new Go client doesn't exist yet.
    this.initState = null
    this.initPromise = new Promise<ClientInitState>((resolve) => {
      this._reconnectResolve = resolve
    })

    // Increment generation and notify listeners so React can re-create resources
    this._connectionGeneration++
    this.connectionLostEvents.emit(undefined)
  }
}
