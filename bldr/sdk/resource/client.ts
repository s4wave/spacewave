import '../dispose-symbol.js'

import { createAbortController } from '../../web/bldr/abort.js'
import { exponentialBackoff, retryWithAbort } from '../../web/bldr/retry.js'
import type { ResourceService } from './resource_srpc.pb.js'
import type {
  ResourceAttachRequest,
  ResourceClientRequest,
} from './resource.pb.js'
import {
  Client as SRPCClient,
  ERR_RPC_ABORT,
  Server,
  StreamConn,
  combineUint8ArrayListTransform,
} from 'starpc'
import type { LookupMethod } from 'starpc'
import { pushable } from 'it-pushable'
import type { Pushable } from 'it-pushable'
import { pipe } from 'it-pipe'
import { ResourceFailureCode } from './resource.pb.js'
import { openResourceRpcStream, ResourceFailureError } from './rpc-stream.js'
import type { ResourceRpcStream } from './rpc-stream.js'

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
      // Answer the released marker and serialization directly.
      if (prop === 'released') return true
      if (prop === 'toJSON') return () => ({ released: true })

      // Return undefined for inspection properties used by
      // React/DevTools/vitest/pretty-format. These properties include:
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

const resourceClientInitTimeoutMS = 30000
const resourceClientInitTimeoutMessage =
  'ResourceClient stream did not initialize before timeout'

function withResourceClientInitTimeout<T>(promise: Promise<T>): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    let settled = false
    const timeout = setTimeout(() => {
      if (settled) return
      settled = true
      reject(
        new ResourceClientError(
          resourceClientInitTimeoutMessage,
          'CONNECTION_FAILED',
        ),
      )
    }, resourceClientInitTimeoutMS)

    promise
      .then((value) => {
        if (settled) return
        settled = true
        clearTimeout(timeout)
        resolve(value)
      })
      .catch((error) => {
        if (settled) return
        settled = true
        clearTimeout(timeout)
        reject(error)
      })
  })
}

function isServerMissingResourceError(error: unknown): boolean {
  if (!(error instanceof ResourceFailureError)) return false
  return [
    ResourceFailureCode.RESOURCE_NOT_FOUND,
    ResourceFailureCode.INVALID_RESOURCE_ID,
    ResourceFailureCode.RESOURCE_OR_CLIENT_RELEASED,
  ].includes(error.failure.code ?? ResourceFailureCode.UNKNOWN)
}

/**
 * ClientResourceRef is a reference to a remote resource that can be used to
 * communicate with that resource. Each resource has a unique ID and must be
 * explicitly released when no longer needed.
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
 * ResourceReleasedEvent is fired when a resource is released.
 */
export interface ResourceReleasedEvent {
  readonly resourceId: number
  readonly reason: ResourceReleaseReason
}

/**
 * ResourceReleaseReason describes why a resource might be released.
 */
export type ResourceReleaseReason =
  | 'client-released' // Client called release()
  | 'server-released' // Server notified the client of release
  | 'connection-lost' // Connection was lost
  | 'client-disposed' // Client was disposed/cancelled

// clientErrorBrand marks ResourceClientError instances from any bundle copy of
// the SDK. The registered symbol is shared across bundles.
const clientErrorBrand = Symbol.for('bldr.ResourceClientError')

/** ResourceClientError reports a client lifecycle failure across bundle boundaries. */
export class ResourceClientError extends Error {
  override readonly name = 'ResourceClientError'
  readonly [clientErrorBrand] = true

  /** [Symbol.hasInstance] accepts any error carrying the brand, so instanceof holds across bundles. */
  static [Symbol.hasInstance](value: unknown): boolean {
    return (
      value instanceof Error &&
      clientErrorBrand in value &&
      value[clientErrorBrand] === true
    )
  }

  constructor(
    message: string,
    public readonly code: ResourceClientErrorCode,
    public readonly cause?: Error,
  ) {
    super(message)
  }
}

export type ResourceClientErrorCode =
  | 'CLIENT_CANCELLED'
  | 'CLIENT_DISPOSED'
  | 'CONNECTION_FAILED'
  | 'INVALID_RESOURCE'
  | 'ATTACH_SESSION_CLOSED'

/**
 * EventEmitter is a simple event emitter for resource lifecycle events.
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
 * InternalResourceRef extends the public interface with mutable state.
 */
interface InternalResourceRef extends ClientResourceRef {
  _markReleased(): void
}

/**
 * ClientInitState contains the client initialization state.
 */
interface ClientInitState {
  clientHandleId: number
  rootResourceId: number
}

/** OrderedSRPCClient applies lifecycle controls before opening a resource route. */
class OrderedSRPCClient extends SRPCClient {
  constructor(
    private readonly resourceClient: Client,
    private readonly openRoute: (
      retain?: boolean,
    ) => Promise<ResourceRpcStream>,
  ) {
    super(() => openRoute())
  }

  /** request abandons children when a unary result never reaches its caller. */
  override async request(
    service: string,
    method: string,
    data: Uint8Array,
    abortSignal?: AbortSignal,
  ): Promise<Uint8Array> {
    // Keep this call's route separate from concurrent unary and streaming calls.
    await this.resourceClient.waitForControls(abortSignal)
    let route: ResourceRpcStream | undefined
    const call = new SRPCClient(async () => {
      route = await this.openRoute(true)
      return route
    })

    // Settle the result before closing the outer ResourceRpc stream.
    try {
      const result = await call.request(service, method, data, abortSignal)
      route?.finish(false)
      return result
    } catch (error) {
      route?.finish(true)
      throw error
    }
  }

  override async clientStreamingRequest(
    service: string,
    method: string,
    data: AsyncIterable<Uint8Array>,
    abortSignal?: AbortSignal,
  ): Promise<Uint8Array> {
    await this.resourceClient.waitForControls(abortSignal)
    return super.clientStreamingRequest(service, method, data, abortSignal)
  }

  override serverStreamingRequest(
    service: string,
    method: string,
    data: Uint8Array,
    abortSignal?: AbortSignal,
  ): AsyncIterable<Uint8Array> {
    const outgoing = pushable<Uint8Array>({ objectMode: true })
    void (async () => {
      await this.resourceClient.waitForControls(abortSignal)
      for await (const value of super.serverStreamingRequest(
        service,
        method,
        data,
        abortSignal,
      )) {
        outgoing.push(value)
      }
      outgoing.end()
    })().catch((error) => outgoing.end(error))
    return outgoing
  }

  override bidirectionalStreamingRequest(
    service: string,
    method: string,
    data: AsyncIterable<Uint8Array>,
    abortSignal?: AbortSignal,
  ): AsyncIterable<Uint8Array> {
    const outgoing = pushable<Uint8Array>({ objectMode: true })
    void (async () => {
      await this.resourceClient.waitForControls(abortSignal)
      for await (const value of super.bidirectionalStreamingRequest(
        service,
        method,
        data,
        abortSignal,
      )) {
        outgoing.push(value)
      }
      outgoing.end()
    })().catch((error) => outgoing.end(error))
    return outgoing
  }
}

function createResourceRef(
  id: number,
  client: Client,
  onRelease: (id: number, ref: InternalResourceRef) => void,
  onServerRelease: (id: number) => void,
): InternalResourceRef {
  // Release the reference once, reporting it to the client.
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
      srpcClient = new OrderedSRPCClient(client, async (retain) => {
        return withResourceClientInitTimeout(
          openResourceRpcStream(
            id,
            client.service.ResourceRpc.bind(client.service),
            retain,
          ),
        ).catch((error) => {
          if (isServerMissingResourceError(error)) {
            onServerRelease(id)
          }
          throw error
        })
      })
    }
    return srpcClient
  }

  // Expose the reference state and its lazily created client.
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
  controller: AbortController
  outgoing: Pushable<ResourceAttachRequest>
  attachIdCtr: number
  closed: boolean
  muxes: Map<number, LookupMethod>
  releaseFns: Map<number, () => void>
  pending: Map<
    number,
    {
      resolve: (resourceId: number) => void
      reject: (err: Error) => void
      canceled?: boolean
    }
  >
}

interface ResourceClientSession {
  controller: AbortController
  outgoing: Pushable<ResourceClientRequest>
  generation: number
  initialized: boolean
  closed: boolean
  nextControlId: number
  acknowledgedControlId: number
  controlWaiters: Set<{
    target: number
    resolve: () => void
    reject: (error: Error) => void
  }>
}

/**
 * Client manages connections to remote resources via RPC.
 * Handles resource lifecycle, reference counting, and cleanup.
 *
 * Note: Server-side handlers may send the same resource ID to the client
 * multiple times (out-of-band from this Client). Additionally, client code may
 * create multiple references to the same resource ID. Client uses reference
 * counting to track when all client-side references to a resource have been
 * released before notifying the server.
 */
export class Client {
  private initState: ClientInitState | null = null
  private connectionController: AbortController | null = null
  private resourceSession: ResourceClientSession | null = null
  private resources = new Map<number, Set<InternalResourceRef>>()
  private events = new EventEmitter<ResourceReleasedEvent>()
  private connectionLostEvents = new EventEmitter<void>()
  private initPromise: Promise<ClientInitState> | null = null
  private disposed = false
  private _connectionGeneration = 0
  private _reconnectResolve: ((state: ClientInitState) => void) | null = null
  private _reconnectReject: ((error: Error) => void) | null = null
  private attachSession: AttachSession | null = null
  private attachSessionInitPromise: Promise<AttachSession> | null = null

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
   * connectionGeneration counts the connection generations. The counter
   * increments each time the connection is lost and resources are released.
   * React hooks can use the counter to detect when resources need to be
   * re-created.
   */
  get connectionGeneration(): number {
    return this._connectionGeneration
  }

  /**
   * onResourceReleased registers a callback for when resources are released.
   * Returns an unsubscribe function.
   */
  onResourceReleased(
    callback: (event: ResourceReleasedEvent) => void,
  ): () => void {
    this.throwIfDisposed()
    return this.events.on(callback)
  }

  /**
   * onConnectionLost registers a callback for when the connection is lost and
   * all resources are released. The callback fires when the server connection
   * drops and reconnects.
   * Returns an unsubscribe function.
   */
  onConnectionLost(callback: () => void): () => void {
    this.throwIfDisposed()
    return this.connectionLostEvents.on(callback)
  }

  /**
   * accessRootResource returns a reference to the root resource. It starts the
   * connection if needed and rejects on an initial stream failure. A later call
   * opens a fresh connection after that failure.
   */
  async accessRootResource(): Promise<ClientResourceRef> {
    const state = await this.ensureInitialized()
    return this.createResourceReference(state.rootResourceId)
  }

  // attachRawInvoker provides a leaf callback mux. Raw attached invokers are
  // not resource trees and server handlers must not create child refs from
  // them.
  async attachRawInvoker(
    label: string,
    mux: LookupMethod,
    signal?: AbortSignal,
  ): Promise<{ resourceId: number; cleanup: () => void }> {
    return this.attachResource(label, mux, signal)
  }

  // attachResourceTree provides a Resource SDK tree root. Server handlers may
  // wrap the returned id as an attached resource tree and call child resources
  // returned from that tree.
  async attachResourceTree(
    label: string,
    mux: LookupMethod,
    signal?: AbortSignal,
    releaseFn?: () => void,
  ): Promise<{ resourceId: number; cleanup: () => void }> {
    return this.attachResource(label, mux, signal, releaseFn)
  }

  // attachResource provides a mux that server-side handlers can
  // invoke. The mux is served over a yamux session inside the
  // ResourceAttach bidi stream. Multiple resources share one session.
  // Returns the server-assigned resource ID and a cleanup function.
  async attachResource(
    label: string,
    mux: LookupMethod,
    signal?: AbortSignal,
    releaseFn?: () => void,
  ): Promise<{ resourceId: number; cleanup: () => void }> {
    let attempt = 0

    for (;;) {
      const sess = await this.ensureAttachSession()

      try {
        // Allocate an attach correlation ID.
        const attachId = ++sess.attachIdCtr
        const resultPromise = new Promise<number>((resolve, reject) => {
          sess.pending.set(attachId, { resolve, reject })
          signal?.addEventListener(
            'abort',
            () => {
              const pending = sess.pending.get(attachId)
              if (pending) {
                pending.canceled = true
              }
              reject(new Error('aborted'))
            },
            { once: true },
          )
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
        if (releaseFn) {
          sess.releaseFns.set(resourceId, releaseFn)
        }

        const cleanup = () => {
          this.releaseAttachedResource(sess, resourceId)
          // Send Detach (best-effort).
          sess.outgoing.push({
            body: {
              case: 'detach' as const,
              value: { resourceId },
            },
          })
        }

        return { resourceId, cleanup }
      } catch (err) {
        if (
          signal?.aborted ||
          attempt >= 3 ||
          !this.shouldRetryAttachResource(err, sess)
        ) {
          throw err
        }
        attempt++
      }
    }
  }

  // shouldRetryAttachResource reports whether attachResource should retry.
  private shouldRetryAttachResource(
    err: unknown,
    sess: AttachSession,
  ): boolean {
    return (
      err instanceof ResourceClientError &&
      err.code === 'ATTACH_SESSION_CLOSED' &&
      this.attachSession !== sess
    )
  }

  // ensureAttachSession opens the ResourceAttach bidi stream if needed.
  private async ensureAttachSession(): Promise<AttachSession> {
    if (this.attachSession) return this.attachSession
    if (this.attachSessionInitPromise) return this.attachSessionInitPromise

    this.attachSessionInitPromise = this.openAttachSession().finally(() => {
      this.attachSessionInitPromise = null
    })
    return this.attachSessionInitPromise
  }

  // openAttachSession opens the ResourceAttach bidi stream.
  private async openAttachSession(): Promise<AttachSession> {
    // Attach under the initialized client handle.
    const state = await this.ensureInitialized()
    const controller = createAbortController(this.signal)

    // Create an outgoing packet pushable.
    const outgoing = pushable<ResourceAttachRequest>({ objectMode: true })

    // Open the ResourceAttach bidi stream.
    const incoming = this.service.ResourceAttach(
      (async function* () {
        yield* outgoing
      })(),
      controller.signal,
    )
    const incomingIt = incoming[Symbol.asyncIterator]()

    // Send the session-only Init.
    outgoing.push({
      body: {
        case: 'init' as const,
        value: { clientHandleId: state.clientHandleId },
      },
    })

    // Read the session Ack.
    const ackResult = await incomingIt.next()
    if (ackResult.done) {
      outgoing.end()
      controller.abort()
      throw new Error('stream closed before ack')
    }
    const ackBody = ackResult.value?.body
    if (ackBody?.case !== 'ack') {
      outgoing.end()
      controller.abort()
      throw new Error('expected ack packet')
    }
    if (ackBody.value.failure) {
      outgoing.end()
      controller.abort()
      const err = new ResourceFailureError(ackBody.value.failure)
      if (ackBody.value.failure.code === ResourceFailureCode.CLIENT_RELEASED) {
        this.resetConnection()
        throw new ResourceClientError(
          'Resource attach client was released',
          'CONNECTION_FAILED',
          err,
        )
      }
      throw err
    }

    // Track the session's attached resources and pending adds.
    const sess: AttachSession = {
      controller,
      outgoing,
      attachIdCtr: 0,
      closed: false,
      muxes: new Map(),
      releaseFns: new Map(),
      pending: new Map(),
    }

    // Create a yamux StreamConn.
    // CLIENT side is yamux server (inbound). The server accepts streams
    // and routes to the correct mux via service ID prefix routing.
    const routedLookup: LookupMethod = async (
      serviceId: string,
      methodId: string,
    ) => {
      // Split the resource ID prefix from the service ID.
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

    // Release an attached resource when the server detaches it.
    const releaseAttachedResource = (resourceId: number) =>
      this.releaseAttachedResource(sess, resourceId)

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
          const attachId = addAck.attachId ?? 0
          const pending = sess.pending.get(attachId)
          sess.pending.delete(attachId)
          if (!pending) {
            continue
          }
          if (pending.canceled) {
            if (!addAck.failure) {
              outgoing.push({
                body: {
                  case: 'detach' as const,
                  value: { resourceId: addAck.resourceId ?? 0 },
                },
              })
            }
            continue
          }
          if (!addAck.failure) {
            pending.resolve(addAck.resourceId ?? 0)
          } else {
            pending.reject(new ResourceFailureError(addAck.failure))
          }
        }
        if (body?.case === 'detachAck') {
          releaseAttachedResource(body.value.resourceId ?? 0)
        }
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
    )
      .catch(() => {
        outgoing.end()
      })
      .finally(() => {
        this.clearAttachSession(sess)
      })

    // Cache the session for later attaches.
    this.attachSession = sess
    return sess
  }

  /**
   * createResourceReference creates a reference to a specific resource by ID.
   * The resource should already exist on the server.
   */
  createResourceReference(id: number): ClientResourceRef {
    this.throwIfDisposed()
    return this.createRef(id)
  }

  /**
   * dispose disposes the client and cleans up all resources.
   */
  dispose(reason: ResourceClientErrorCode = 'CLIENT_DISPOSED'): void {
    // Dispose once and fail any pending reconnect.
    if (this.disposed) return
    this.disposed = true
    this._reconnectReject?.(
      new ResourceClientError('Resource client is closed', reason),
    )
    this._reconnectResolve = null
    this._reconnectReject = null

    // A normal close ends the request stream after its final controls. A
    // failed transport retires the generation without replaying releases.
    const graceful = reason !== 'CONNECTION_FAILED'
    if (graceful) {
      for (const resourceId of this.resources.keys()) {
        this.queueControl({ case: 'release', value: { resourceId } })
      }
    }
    this.clearAttachSession()
    if (graceful) {
      this.finishResourceSession()
      this.connectionController = null
    } else {
      this.retireResourceSession()
      this.connectionController?.abort()
      this.connectionController = null
    }

    // Release every reference and clear the client state.
    const releaseReason: ResourceReleaseReason = 'client-disposed'
    for (const [resourceId, refs] of this.resources.entries()) {
      refs.forEach((ref) => ref._markReleased())
      this.events.emit({ resourceId, reason: releaseReason })
    }
    this.resources.clear()
    this.events.clear()
    this.connectionLostEvents.clear()
    this.initState = null
    this.initPromise = null
  }

  /**
   * ensureInitialized ensures the client is initialized and returns the init
   * state.
   */
  private async ensureInitialized(): Promise<ClientInitState> {
    // Refuse a disposed client.
    this.throwIfDisposed()

    // Return the existing state if available
    if (this.initState) {
      return this.initState
    }

    // Return the existing promise if already initializing
    if (this.initPromise) {
      return this.initPromise
    }

    // Start initialization
    this.initPromise = this.performInitialization()
    return this.initPromise
  }

  /**
   * performInitialization initializes the client.
   */
  private async performInitialization(): Promise<ClientInitState> {
    this.throwIfDisposed()

    // Create the connection controller
    this.connectionController = createAbortController(this.signal)

    return new Promise<ClientInitState>((resolve, reject) => {
      // Abort the connection when initialization does not complete.
      let initialized = false
      const cleanup = () => {
        if (!initialized) {
          this.connectionController?.abort()
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

      // Cancel at once or when the client signal aborts.
      if (this.signal.aborted) {
        handleCancel()
        return
      }
      this.signal.addEventListener('abort', handleCancel, { once: true })

      // Start the connection with retry
      this.startConnection(resolve, () => {
        initialized = true
        this.signal.removeEventListener('abort', handleCancel)
      }).catch((error) => {
        cleanup()
        if (!initialized) {
          const cause =
            error instanceof Error ? error : new Error(String(error))
          reject(
            new ResourceClientError(
              `Failed to initialize client connection: ${cause.message}`,
              'CONNECTION_FAILED',
              cause,
            ),
          )
        }
      })
    })
  }

  /**
   * startConnection starts the connection and handles the message stream.
   */
  private async startConnection(
    onInitialized: (state: ClientInitState) => void,
    markInitialized: () => void,
  ): Promise<void> {
    // Require the controller performInitialization created.
    const controller = this.connectionController
    if (!controller) {
      throw new ResourceClientError(
        'No connection controller',
        'CONNECTION_FAILED',
      )
    }

    // Reject the first failed stream. Once initialized, the same connection
    // loop retains its backoff and reconnect behavior, and each initialized
    // stream resets the backoff so a long healthy session reconnects quickly.
    let initializedOnce = false
    const backoffFn = exponentialBackoff()
    await retryWithAbort(
      controller.signal,
      async (signal) => {
        // Abort the attempt when init does not arrive in time.
        const attemptController = createAbortController(signal)
        let initialized = false
        let initTimedOut = false
        let initTimeout: ReturnType<typeof setTimeout> | undefined = setTimeout(
          () => {
            if (initialized || attemptController.signal.aborted) return
            initTimedOut = true
            attemptController.abort()
          },
          resourceClientInitTimeoutMS,
        )

        // Publish the attempt's control session.
        const outgoing = pushable<ResourceClientRequest>({ objectMode: true })
        const session: ResourceClientSession = {
          controller: attemptController,
          outgoing,
          generation: this._connectionGeneration,
          initialized: false,
          closed: false,
          nextControlId: 0,
          acknowledgedControlId: 0,
          controlWaiters: new Set(),
        }
        this.resourceSession = session

        // Close the session and fail its control waiters.
        const retire = () => {
          // Close the session once.
          if (session.closed) return
          session.closed = true
          outgoing.end()

          // Fail the waiters and drop the published session.
          const error = new ResourceClientError(
            'ResourceClient control stream closed',
            'CONNECTION_FAILED',
          )
          for (const waiter of session.controlWaiters) {
            waiter.reject(error)
          }
          session.controlWaiters.clear()
          if (this.resourceSession === session) this.resourceSession = null
        }

        // Report an init timeout as a connection failure.
        const throwInitTimeout = (): never => {
          throw new ResourceClientError(
            resourceClientInitTimeoutMessage,
            'CONNECTION_FAILED',
          )
        }
        try {
          const stream = this.service.ResourceClient(
            (async function* () {
              yield* outgoing
            })(),
            attemptController.signal,
          )
          // Init is the first control on every generation.
          outgoing.push({ body: { case: 'init' as const, value: {} } })
          for await (const msg of stream) {
            if (signal.aborted) return
            if (attemptController.signal.aborted) {
              if (initTimedOut) throwInitTimeout()
              return
            }
            const body = msg.body
            if (body?.case === 'init') {
              initialized = true
              session.initialized = true
              backoffFn.reset()
              if (initTimeout) clearTimeout(initTimeout)
              initTimeout = undefined
              const state: ClientInitState = {
                clientHandleId: body.value.clientHandleId ?? 0,
                rootResourceId: body.value.rootResourceId ?? 0,
              }
              if (this._reconnectResolve) {
                this.initState = state
                this._reconnectResolve(state)
                this._reconnectResolve = null
                this._reconnectReject = null
              } else if (!this.initState) {
                this.initState = state
                initializedOnce = true
                markInitialized()
                onInitialized(state)
              } else {
                this.initState = state
              }
              continue
            }
            if (body?.case === 'resourceReleased') {
              this.handleServerResourceRelease(body.value.resourceId ?? 0)
              continue
            }
            if (body?.case === 'controlAck') {
              this.acknowledgeControl(session, body.value.controlId ?? 0)
            }
          }
        } catch (err) {
          if (!signal.aborted && initTimedOut) throwInitTimeout()
          throw err
        } finally {
          if (initTimeout) clearTimeout(initTimeout)
          initTimeout = undefined
          retire()
          if (
            this.connectionController === controller &&
            !this.disposed &&
            initialized
          ) {
            this.releaseAllResources('connection-lost')
          }
        }
        if (this.disposed) return
        if (!signal.aborted) {
          if (initTimedOut) throwInitTimeout()
          throw new Error(
            initialized
              ? 'ResourceClient stream closed'
              : 'ResourceClient stream closed before init',
          )
        }
      },
      {
        backoffFn,
        errorCb: (error) => {
          if (!initializedOnce) {
            controller.abort()
            return
          }
          if (this.shouldRetryResourceClientStreamSilently(error)) return
          console.warn('Retry: retrying after error', { error })
        },
      },
    )
  }

  /**
   * throwIfDisposed throws an error if the client has been disposed.
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
   * createRef creates a new reference to a resource.
   */
  private createRef(id: number): ClientResourceRef {
    // Find or start the reference set for the resource.
    this.throwIfDisposed()
    let refs = this.resources.get(id)
    const firstRef = !refs
    if (!refs) {
      refs = new Set()
      this.resources.set(id, refs)
    }

    // Add the reference, adopting the resource on its first one.
    const ref = createResourceRef(
      id,
      this,
      this.releaseRef.bind(this),
      this.handleServerResourceRelease.bind(this),
    )
    refs.add(ref)
    if (firstRef)
      this.queueControl({ case: 'adopt', value: { resourceId: id } })
    return ref
  }

  // releaseRef drops ref and releases the resource with its last reference.
  private releaseRef(id: number, ref: InternalResourceRef): void {
    // Keep the resource while other references remain.
    const refs = this.resources.get(id)
    if (!refs) return
    refs.delete(ref)
    if (refs.size !== 0) return

    // Release the resource on the server and notify listeners.
    this.resources.delete(id)
    this.queueControl({ case: 'release', value: { resourceId: id } })
    this.events.emit({ resourceId: id, reason: 'client-released' })
  }

  // queueControl sends body on the initialized control session, if any.
  private queueControl(body: NonNullable<ResourceClientRequest['body']>): void {
    const session = this.resourceSession
    if (!session || session.closed || !session.initialized) return
    const controlId = ++session.nextControlId
    session.outgoing.push({ controlId, body })
  }

  // waitForControls resolves once the server acknowledges every queued control.
  async waitForControls(abortSignal?: AbortSignal): Promise<void> {
    // Require a live control session.
    if (abortSignal?.aborted) throw new Error(ERR_RPC_ABORT)
    const session = this.resourceSession
    if (!session || session.closed || !session.initialized) {
      throw new ResourceClientError(
        'ResourceClient control stream is unavailable',
        'CONNECTION_FAILED',
      )
    }

    // Return when the latest control is already acknowledged.
    const target = session.nextControlId
    if (target <= session.acknowledgedControlId) return

    // Wait for the acknowledgment or an abort.
    await new Promise<void>((resolve, reject) => {
      // Settle the waiter once, removing its abort listeners.
      const cleanup = () => {
        session.controller.signal.removeEventListener('abort', onSessionAbort)
        abortSignal?.removeEventListener('abort', onCallAbort)
      }
      const waiter = {
        target,
        resolve: () => {
          cleanup()
          resolve()
        },
        reject: (error: Error) => {
          cleanup()
          reject(error)
        },
      }

      // Reject a waiter still registered when either signal aborts.
      const rejectWaiter = (error: Error) => {
        if (!session.controlWaiters.delete(waiter)) return
        waiter.reject(error)
      }
      const onSessionAbort = () =>
        rejectWaiter(
          new ResourceClientError(
            'ResourceClient control stream closed',
            'CONNECTION_FAILED',
          ),
        )
      const onCallAbort = () => rejectWaiter(new Error(ERR_RPC_ABORT))

      // Register the waiter, then check signals that aborted already.
      session.controlWaiters.add(waiter)
      session.controller.signal.addEventListener('abort', onSessionAbort, {
        once: true,
      })
      abortSignal?.addEventListener('abort', onCallAbort, { once: true })
      if (session.controller.signal.aborted) onSessionAbort()
      if (abortSignal?.aborted) onCallAbort()
    })
  }

  // acknowledgeControl records the server's in-order acknowledgment of
  // controlId and resolves the waiters it satisfies.
  private acknowledgeControl(
    session: ResourceClientSession,
    controlId: number,
  ): void {
    if (
      controlId === 0 ||
      controlId !== session.acknowledgedControlId + 1 ||
      controlId > session.nextControlId
    ) {
      throw new Error(
        `unexpected ResourceClient control acknowledgment ${controlId} after ${session.acknowledgedControlId}`,
      )
    }
    session.acknowledgedControlId = controlId
    for (const waiter of session.controlWaiters) {
      if (waiter.target > controlId) continue
      session.controlWaiters.delete(waiter)
      waiter.resolve()
    }
  }

  // shouldRetryResourceClientStreamSilently reports whether error is an
  // expected transport reset or init timeout that needs no warning.
  private shouldRetryResourceClientStreamSilently(error: unknown): boolean {
    // An init timeout is expected.
    if (
      error instanceof ResourceClientError &&
      error.message === resourceClientInitTimeoutMessage
    ) {
      return true
    }

    // A yamux stream reset is expected, by text or by name and message.
    const errText = String(error)
    if (errText === 'StreamResetError: stream reset') return true
    if (typeof error !== 'object' || error === null) return false
    const errName = Reflect.get(error, 'name')
    const errMessage = Reflect.get(error, 'message')
    const name = typeof errName === 'string' ? errName : ''
    const message = typeof errMessage === 'string' ? errMessage : ''
    return name === 'StreamResetError' || message === 'stream reset'
  }

  /**
   * handleServerResourceRelease handles the server notification that a
   * resource was released.
   */
  private handleServerResourceRelease(resourceId: number): void {
    // Ignore a resource with no references.
    const refs = this.resources.get(resourceId)
    if (!refs) return

    // Mark all references as released
    refs.forEach((ref) => ref._markReleased())

    // Remove the resource from tracking
    this.resources.delete(resourceId)

    // Emit the resource release event
    this.events.emit({
      resourceId,
      reason: 'server-released',
    })
  }

  /**
   * releaseAllResources releases all resources with the given reason.
   * Used when the connection is lost and resources are no longer valid.
   */
  private releaseAllResources(reason: ResourceReleaseReason): void {
    // Retire the generation and release every reference.
    this._connectionGeneration++
    this.clearAttachSession()
    for (const [resourceId, refs] of this.resources.entries()) {
      refs.forEach((ref) => ref._markReleased())
      this.events.emit({ resourceId, reason })
    }
    this.resources.clear()

    // Wait for the connection loop to initialize the next generation.
    this.initState = null
    this.initPromise = new Promise<ClientInitState>((resolve, reject) => {
      this._reconnectResolve = resolve
      this._reconnectReject = reject
    })

    // A generation may be retired before any caller requests its root.
    void this.initPromise.catch(() => {})
    this.connectionLostEvents.emit(undefined)
  }

  // resetConnection retires the current transport generation and its references.
  // Transport owners call this after their underlying connection closes.
  // The next resource acquisition opens a fresh generation through the service.
  resetConnection(): void {
    // Detach the connection and release every reference.
    if (this.disposed) return
    const controller = this.connectionController
    this.connectionController = null
    this.clearAttachSession()
    for (const [resourceId, refs] of this.resources.entries()) {
      refs.forEach((ref) => ref._markReleased())
      this.events.emit({ resourceId, reason: 'connection-lost' })
    }
    this.resources.clear()

    // Fail a pending reconnect so the next acquisition initializes afresh.
    this.initState = null
    this.initPromise = null
    this._reconnectReject?.(
      new ResourceClientError(
        'Resource generation was replaced',
        'CONNECTION_FAILED',
      ),
    )
    this._reconnectResolve = null
    this._reconnectReject = null

    // Abort the old transport and announce the new generation.
    this.retireResourceSession()
    controller?.abort()
    this._connectionGeneration++
    this.connectionLostEvents.emit(undefined)
  }

  // finishResourceSession ends the control stream after its queued controls.
  private finishResourceSession(): void {
    // Close the session and end its request stream.
    const session = this.resourceSession
    if (!session) return
    session.closed = true
    session.outgoing.end()
    this.resourceSession = null
  }

  // retireResourceSession closes the control session and aborts its stream.
  private retireResourceSession(): void {
    // Close the session and abort its transport.
    const session = this.resourceSession
    if (!session) return
    session.closed = true
    session.outgoing.end()
    session.controller.abort()
    this.resourceSession = null
  }

  // clearAttachSession tears down any cached ResourceAttach session state.
  private clearAttachSession(sess?: AttachSession): void {
    // Drop the cached session and close it once.
    const current = sess ?? this.attachSession
    if (!current) {
      return
    }
    if (this.attachSession === current) {
      this.attachSession = null
    }
    if (current.closed) {
      return
    }

    // Abort the stream and reject the pending attaches.
    current.closed = true
    current.controller.abort()
    current.outgoing.end()
    for (const [, pending] of current.pending) {
      pending.reject(
        new ResourceClientError(
          'Resource attach session closed',
          'ATTACH_SESSION_CLOSED',
        ),
      )
    }
    current.pending.clear()
    this.releaseAllAttachedResources(current)
  }

  private releaseAttachedResource(
    sess: AttachSession,
    resourceId: number,
  ): void {
    const releaseFn = sess.releaseFns.get(resourceId)
    sess.releaseFns.delete(resourceId)
    sess.muxes.delete(resourceId)
    releaseFn?.()
  }

  private releaseAllAttachedResources(sess: AttachSession): void {
    const releaseFns = [...sess.releaseFns.values()]
    sess.releaseFns.clear()
    sess.muxes.clear()
    for (const releaseFn of releaseFns) {
      releaseFn()
    }
  }
}
