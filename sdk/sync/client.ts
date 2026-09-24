import { Client as RpcClient, WebSocketConn } from 'starpc'
import {
  Client as ResourceClient,
  type ClientResourceRef,
} from '@aptre/bldr-sdk/resource/client.js'
import { ResourceServiceClient } from '@aptre/bldr-sdk/resource/resource_srpc.pb.js'
import { ItState } from '../../bldr/web/bldr/it-state.js'
import {
  createAccess,
  type CollectionAccess,
  type DatabaseAccess,
} from './access.js'
import { SyncError } from './errors.js'
import { decodeJSON, encodeJSON, type JsonValue } from './json.js'
import type { Operation } from './operation.js'
import type {
  CallOptions,
  Output,
  Query,
  RecordEntry,
  Schema,
  Validator,
} from './schema.js'
import {
  ApplicationClient,
  CollectionClient,
  GatewayClient,
} from './sync_srpc.pb.js'
import { checkFailure, wireVersion } from './wire.js'

export interface ConnectionState {
  readonly status: 'ready' | 'reconnecting' | 'closed'
  readonly error?: SyncError
}

export interface SubscriptionState<T> {
  readonly status: 'loading' | 'current' | 'stale' | 'error'
  readonly data: readonly RecordEntry<T>[]
  readonly error?: SyncError
}

export interface Observer<T> {
  next(state: SubscriptionState<T>): void | Promise<void>
  error?(error: SyncError): void
}

export interface Collection<V extends Validator> extends CollectionAccess<V> {
  watch(
    query?: Query,
    options?: CallOptions,
  ): AsyncIterable<SubscriptionState<Output<V>>>
  subscribe(
    query: Query,
    observer: Observer<Output<V>>,
    options?: CallOptions,
  ): () => void
}

export interface ConnectionStatus {
  readonly current: ConnectionState
  subscribe(listener: (state: ConnectionState) => void): () => void
}

export interface Database<S extends Schema>
  extends DatabaseAccess<S>, AsyncDisposable {
  readonly connection: ConnectionStatus
  collection<K extends keyof S['collections'] & string>(
    name: K,
  ): Collection<S['collections'][K]>
  close(): Promise<void>
}

export interface ConnectOptions<S extends Schema> {
  url: string
  schema: S
  getAccessToken(signal: AbortSignal): string | Promise<string>
  signal?: AbortSignal
  requestTimeoutMs?: number
}

interface Root {
  ref: ClientResourceRef
  client: ApplicationClient
}

interface RemoteCollection {
  ref: ClientResourceRef
  client: CollectionClient
}

// connect resolves after authentication, version checks, and root Resource admission.
export async function connect<S extends Schema>(
  options: ConnectOptions<S>,
): Promise<Database<S>> {
  const client = new SyncClient(options)
  try {
    await client.initialize()
    return client
  } catch (error) {
    await client.close()
    throw error
  }
}

class SyncClient<S extends Schema> implements Database<S> {
  readonly connection: ConnectionStatus
  readonly mutate: DatabaseAccess<S>['mutate']
  private readonly controller = new AbortController()
  private readonly resources: ResourceClient
  private readonly transport: Transport<S>
  private readonly access: DatabaseAccess<S>
  private readonly connections = new Set<(state: ConnectionState) => void>()
  private state: ConnectionState = { status: 'reconnecting' }
  private readonly collections = new Map<string, Promise<RemoteCollection>>()
  private readonly work = new Set<Promise<unknown>>()
  private readiness = Promise.withResolvers<Root>()
  private closing?: Promise<void>
  private failure?: SyncError
  private readonly onAbort = () => {
    void this.close()
  }

  constructor(private readonly options: ConnectOptions<S>) {
    const current = () => this.state
    this.connection = {
      get current() {
        return current()
      },
      subscribe: (listener) => {
        this.connections.add(listener)
        listener(this.state)
        return () => this.connections.delete(listener)
      },
    }
    void this.readiness.promise.catch(() => {})
    this.transport = new Transport(
      options,
      this.controller.signal,
      () => {
        if (this.closing) return
        this.retireRoot()
      },
      (error) => {
        this.failure = error
        this.readiness.reject(error)
        void this.close()
      },
    )
    const rpc = new RpcClient(async () =>
      (await this.transport.ensure()).openStream(),
    )
    this.resources = new ResourceClient(
      new ResourceServiceClient(rpc),
      this.controller.signal,
    )
    this.resources.onConnectionLost(() => {
      this.retireRoot()
      this.refreshRoot()
    })
    this.access = createAccess((operation, options) => {
      const pending = this.dispatch(operation, options)
      this.track(pending)
      return pending
    })
    this.mutate = this.access.mutate
    options.signal?.addEventListener('abort', this.onAbort, { once: true })
  }

  async initialize(): Promise<void> {
    this.options.signal?.throwIfAborted()
    // Initial authentication failures reject connect directly, before Resource retries.
    await this.transport.ensure()
    this.refreshRoot()
    await waitFor(
      this.readiness.promise,
      AbortSignal.any([
        this.controller.signal,
        AbortSignal.timeout(this.options.requestTimeoutMs ?? 15_000),
      ]),
    )
  }

  collection<K extends keyof S['collections'] & string>(
    name: K,
  ): Collection<S['collections'][K]> {
    const access = this.access.collection(name)
    const watch = (query?: Query, options?: CallOptions) =>
      this.watch(name, query ?? {}, options) as AsyncIterable<
        SubscriptionState<Output<S['collections'][K]>>
      >
    return {
      ...access,
      watch,
      subscribe: (query, observer, options) => {
        const controller = new AbortController()
        const signal = options?.signal
          ? AbortSignal.any([options.signal, controller.signal])
          : controller.signal
        const task = (async () => {
          try {
            for await (const state of watch(query, { ...options, signal })) {
              await observer.next(state)
              if (state.error) observer.error?.(state.error)
            }
          } catch (error) {
            if (!signal.aborted) observer.error?.(clientError(error))
          }
        })()
        this.track(task)
        return () => controller.abort()
      },
    }
  }

  close(): Promise<void> {
    this.closing ??= (async () => {
      const error = this.failure ?? new SyncError('CLOSED', 'Client is closed')
      this.readiness.reject(error)
      this.setConnection({ status: 'closed', error: this.failure })
      this.controller.abort(error)
      this.resources.dispose()
      await this.transport.close()
      await Promise.allSettled(this.work)
      this.collections.clear()
      this.connections.clear()
      this.options.signal?.removeEventListener('abort', this.onAbort)
    })()
    return this.closing
  }

  [Symbol.asyncDispose](): Promise<void> {
    return this.close()
  }

  private setConnection(state: ConnectionState): void {
    this.state = state
    for (const listener of this.connections) listener(state)
  }

  private retireRoot(): void {
    if (this.closing) return
    if (this.state.status === 'ready') {
      this.readiness = Promise.withResolvers<Root>()
      void this.readiness.promise.catch(() => {})
    }
    this.collections.clear()
    this.setConnection({ status: 'reconnecting' })
  }

  private track(pending: Promise<unknown>): void {
    this.work.add(pending)
    void pending.finally(() => this.work.delete(pending)).catch(() => {})
  }

  private refreshRoot(): void {
    const generation = this.resources.connectionGeneration
    const ready = this.readiness
    const pending = (async () => {
      const ref = await this.resources.accessRootResource()
      if (
        this.controller.signal.aborted ||
        ref.released ||
        generation !== this.resources.connectionGeneration ||
        !this.transport.connected
      ) {
        ref.release()
        return
      }
      ready.resolve({ ref, client: new ApplicationClient(ref.client) })
      this.setConnection({ status: 'ready' })
    })().catch((error) => {
      if (this.controller.signal.aborted)
        ready.reject(this.failure ?? clientError(error))
    })
    this.track(pending)
  }

  private async root(signal: AbortSignal): Promise<Root> {
    if (this.closing)
      throw this.failure ?? new SyncError('CLOSED', 'Client is closed')
    return waitFor(this.readiness.promise, signal)
  }

  private async remoteCollection(
    name: string,
    signal: AbortSignal,
  ): Promise<RemoteCollection> {
    await this.root(signal)
    let pending = this.collections.get(name)
    if (!pending) {
      const generation = this.resources.connectionGeneration
      pending = (async () => {
        const root = await this.root(this.controller.signal)
        const response = await root.client.OpenCollection(
          { name },
          this.controller.signal,
        )
        checkFailure(response.error)
        if (
          generation !== this.resources.connectionGeneration ||
          root.ref.released
        )
          throw new SyncError(
            'UNAVAILABLE',
            'Collection connection was replaced',
          )
        const ref = this.resources.createResourceReference(
          response.resourceId ?? 0,
        )
        return { ref, client: new CollectionClient(ref.client) }
      })()
      this.collections.set(name, pending)
      void pending.catch(() => {
        if (this.collections.get(name) === pending)
          this.collections.delete(name)
      })
    }
    return waitFor(pending, signal)
  }

  private async dispatch(
    operation: Operation,
    options: CallOptions = {},
  ): Promise<JsonValue> {
    const write = operation.kind !== 'get' && operation.kind !== 'scan'
    const requestId = options.requestId ?? crypto.randomUUID()
    const signal = AbortSignal.any([
      this.controller.signal,
      AbortSignal.timeout(this.options.requestTimeoutMs ?? 15_000),
      ...(options.signal ? [options.signal] : []),
    ])
    let transmitted = false
    for (let attempt = 0; attempt < 2; attempt++) {
      try {
        signal.throwIfAborted()
        if (operation.kind === 'mutate') {
          const root = await this.root(signal)
          transmitted = true
          const result = await root.client.Mutate(
            {
              name: operation.name,
              input: encodeJSON(operation.input),
              requestId,
            },
            signal,
          )
          checkFailure(result.error)
          return decodeJSON(result.result ?? new Uint8Array())
        }
        const { client } = await this.remoteCollection(
          operation.collection,
          signal,
        )
        switch (operation.kind) {
          case 'get': {
            const result = await client.Get({ key: operation.key }, signal)
            checkFailure(result.error)
            return result.found
              ? {
                  found: true,
                  value: decodeJSON(result.data ?? new Uint8Array()),
                }
              : { found: false }
          }
          case 'scan': {
            const result = await client.Scan(
              { prefix: operation.prefix },
              signal,
            )
            checkFailure(result.error)
            return (result.entries ?? []).map((entry) => ({
              key: entry.key ?? '',
              value: decodeJSON(entry.data ?? new Uint8Array()),
            }))
          }
          case 'put': {
            transmitted = true
            const result = await client.Put(
              {
                key: operation.key,
                data: encodeJSON(operation.value),
                requestId,
              },
              signal,
            )
            checkFailure(result.error)
            return null
          }
          case 'delete': {
            transmitted = true
            const result = await client.Delete(
              { key: operation.key, requestId },
              signal,
            )
            checkFailure(result.error)
            return null
          }
        }
      } catch (error) {
        if (
          error instanceof SyncError &&
          !['UNAVAILABLE', 'UNCERTAIN'].includes(error.code)
        )
          throw error
        if (attempt === 1 || signal.aborted) {
          if (write && transmitted)
            throw new SyncError(
              'UNCERTAIN',
              'Acceptance could not be confirmed; retain this request ID and input for recovery',
              requestId,
            )
          throw (
            this.failure ??
            new SyncError(
              this.closing ? 'CLOSED' : 'UNAVAILABLE',
              'The connection is unavailable',
            )
          )
        }
        // Resource generations retire old handles; retry retains the same request.
        await waitFor(this.readiness.promise, signal).catch(() => {})
      }
    }
    throw new SyncError('UNAVAILABLE', 'The connection is unavailable')
  }

  private async *watch(
    name: string,
    query: Query,
    options?: CallOptions,
  ): AsyncIterable<SubscriptionState<JsonValue>> {
    const controller = new AbortController()
    const signal = AbortSignal.any([
      this.controller.signal,
      controller.signal,
      ...(options?.signal ? [options.signal] : []),
    ])
    signal.throwIfAborted()
    let current: SubscriptionState<JsonValue> = { status: 'loading', data: [] }
    const state = new ItState(async () => current, { mostRecentOnly: true })
    const iterator = state.getIterable()[Symbol.asyncIterator]()
    const publish = (next: SubscriptionState<JsonValue>) => {
      current = next
      state.pushChangeEvent(next)
    }
    const stop = () => {
      void iterator.return?.()
    }
    signal.addEventListener('abort', stop, { once: true })
    const unsubscribe = this.connection.subscribe((connection) => {
      if (connection.status === 'reconnecting' && current.status !== 'loading')
        publish({ status: 'stale', data: current.data })
      if (connection.status === 'closed' && connection.error)
        publish({
          status: 'error',
          data: current.data,
          error: connection.error,
        })
    })
    const pump = (async () => {
      while (!signal.aborted) {
        const generation = this.resources.connectionGeneration
        try {
          // A new watch waits for the preceding connection generation to retire.
          // eslint-disable-next-line react-doctor/async-await-in-loop
          const { client } = await this.remoteCollection(name, signal)
          for await (const snapshot of client.Watch(
            { prefix: query.prefix ?? '' },
            signal,
          )) {
            checkFailure(snapshot.error)
            publish({
              status: 'current',
              data: (snapshot.entries ?? []).map((entry) => ({
                key: entry.key ?? '',
                value: decodeJSON(entry.data ?? new Uint8Array()),
              })),
            })
          }
          if (!signal.aborted)
            throw new SyncError('UNAVAILABLE', 'Subscription connection ended')
        } catch (error) {
          if (signal.aborted) break
          const failure = clientError(error)
          const recoverable =
            ['UNAVAILABLE', 'CLOSED'].includes(failure.code) &&
            (this.state.status === 'reconnecting' ||
              generation !== this.resources.connectionGeneration)
          if (!recoverable) {
            publish({ status: 'error', data: current.data, error: failure })
            return
          }
          publish({ status: 'stale', data: current.data })
          await waitFor(this.readiness.promise, signal).catch(() => {})
        }
      }
    })()
    this.track(pump)
    try {
      for (;;) {
        const next = await iterator.next()
        if (next.done) break
        yield next.value
        if (next.value.status === 'error') break
      }
    } finally {
      controller.abort()
      unsubscribe()
      signal.removeEventListener('abort', stop)
      await iterator.return?.()
      await pump
    }
  }
}

// Transport owns one authenticated socket at a time; Resource owns retry generations.
class Transport<S extends Schema> {
  private pending?: Promise<WebSocketConn>
  private connection?: WebSocketConn
  private socket?: WebSocket
  private closed?: Promise<void>
  private ended?: Promise<void>

  constructor(
    private readonly options: ConnectOptions<S>,
    private readonly signal: AbortSignal,
    private readonly onClose: () => void,
    private readonly onFatal: (error: SyncError) => void,
  ) {}

  get connected(): boolean {
    return (
      this.socket?.readyState === WebSocket.OPEN &&
      this.connection !== undefined
    )
  }

  ensure(): Promise<WebSocketConn> {
    if (this.signal.aborted)
      return Promise.reject(new SyncError('CLOSED', 'Client is closed'))
    this.pending ??= this.open().catch((error) => {
      this.pending = undefined
      throw error
    })
    return this.pending
  }

  close(): Promise<void> {
    this.closed ??= (async () => {
      const connection = this.connection
      this.socket?.close(1000, 'Client closed')
      await connection?.muxer.close()
      await this.ended
    })()
    return this.closed
  }

  private async open(): Promise<WebSocketConn> {
    const url = new URL(this.options.url)
    if (
      !['ws:', 'wss:'].includes(url.protocol) ||
      url.username ||
      url.password ||
      url.search ||
      url.hash
    )
      throw new SyncError(
        'VALIDATION',
        'Use a WebSocket URL without credentials, query, or fragment',
      )
    const socket = new WebSocket(url)
    const handshake = AbortSignal.any([
      this.signal,
      AbortSignal.timeout(10_000),
    ])
    this.socket = socket
    const ended = Promise.withResolvers<void>()
    this.ended = ended.promise
    const onAbort = () => socket.close()
    this.signal.addEventListener('abort', onAbort, { once: true })
    socket.addEventListener(
      'close',
      () => {
        this.signal.removeEventListener('abort', onAbort)
        if (this.socket === socket) {
          this.connection?.close(new Error('WebSocket connection ended'))
          this.connection = undefined
          this.pending = undefined
          this.onClose()
        }
        ended.resolve()
      },
      { once: true },
    )
    try {
      await waitFor(
        new Promise<void>((resolve, reject) => {
          socket.addEventListener('open', () => resolve(), { once: true })
          socket.addEventListener(
            'error',
            () =>
              reject(
                new SyncError('UNAVAILABLE', 'WebSocket connection failed'),
              ),
            { once: true },
          )
        }),
        handshake,
      )
      // The adapter uses the shared WHATWG methods; its dependency types name ws.
      const connection = new WebSocketConn(
        socket as unknown as ConstructorParameters<typeof WebSocketConn>[0],
        'outbound',
      )
      this.connection = connection
      const token = await waitFor(
        Promise.resolve().then(() => this.options.getAccessToken(handshake)),
        handshake,
      )
      const response = await new GatewayClient(
        connection.buildClient(),
      ).Authenticate(
        {
          wireVersion,
          applicationId: this.options.schema.id,
          applicationVersion: this.options.schema.version,
          token,
        },
        handshake,
      )
      checkFailure(response.error)
      return connection
    } catch (error) {
      socket.close()
      if (
        error instanceof SyncError &&
        ['AUTHENTICATION', 'SCHEMA_MISMATCH', 'VALIDATION'].includes(error.code)
      )
        this.onFatal(error)
      throw error
    }
  }
}

function clientError(error: unknown): SyncError {
  return error instanceof SyncError
    ? error
    : new SyncError('UNAVAILABLE', 'The connection is unavailable')
}

// waitFor cancels one waiter without canceling shared connection initialization.
async function waitFor<T>(
  pending: Promise<T>,
  signal: AbortSignal,
): Promise<T> {
  signal.throwIfAborted()
  let stop: (() => void) | undefined
  try {
    return await Promise.race([
      pending,
      new Promise<never>((_, reject) => {
        stop = () => reject(signal.reason)
        signal.addEventListener('abort', stop, { once: true })
        if (signal.aborted) stop()
      }),
    ])
  } finally {
    if (stop) signal.removeEventListener('abort', stop)
  }
}
