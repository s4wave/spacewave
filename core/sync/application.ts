import { randomUUID } from 'node:crypto'

import { ItState } from '../../bldr/web/bldr/it-state.js'
import { KvScanLimitError, KvStore } from '../../sdk/kv/kv.js'
import { KvObjectTypeError } from '../../sdk/kv/world/store.js'
import { Engine } from '../../sdk/world/engine.js'
import type { Tx } from '../../sdk/world/world-state.js'
import { getObjectType } from '../../sdk/world/types/types.js'
import { SyncError, publicError } from '../../sdk/sync/errors.js'
import type { Operation } from '../../sdk/sync/operation.js'
import {
  canonicalJSON,
  decodeJSON,
  encodeJSON,
  type JsonValue,
} from '../../sdk/sync/json.js'
import {
  collectionKey,
  encodeRecordKey,
  metadataKey,
  receiptKey,
} from '../../sdk/sync/keys.js'
import {
  defineSchema,
  type CallOptions,
  type Principal,
  type Schema,
  type RecordEntry,
} from '../../sdk/sync/schema.js'

import { ApplicationTransaction } from '../../sdk/sync/transaction.js'
import { AppQueries } from '../../sdk/sync/live-query.js'
import type { AppSource } from '../../sdk/sync/app.js'
import { createAccess, type DatabaseAccess } from '../../sdk/sync/access.js'

import type { ApplicationConfig, Migration } from './config.js'
export type { Access, Migration } from './config.js'

export interface ApplicationOptions<
  S extends Schema,
  P extends Principal,
> extends ApplicationConfig<S, P> {
  engine: Engine
}

interface StoredVersion {
  application: string
  version: number
}

type QuerySnapshot =
  | { entries: readonly RecordEntry<JsonValue>[] }
  | { error: SyncError }

interface SharedQuery {
  controller: AbortController
  state: ItState<QuerySnapshot>
  users: number
}

const encoder = new TextEncoder()
const decoder = new TextDecoder('utf-8', { fatal: true, ignoreBOM: true })
const versionKey = encoder.encode('version')

// Application owns admission, policy, transactions, receipts, and its engine ref.
// All application writes, including metadata and receipts, commit through one Tx.
export class Application<S extends Schema, P extends Principal> {
  readonly schema: S
  readonly instance: string
  readonly signal: AbortSignal
  readonly limits: {
    maxRecords: number
    maxSnapshotBytes: number
    maxRecordBytes: number
  }
  private readonly engine: Engine
  private readonly controller = new AbortController()
  private readonly active = new Set<Promise<unknown>>()
  private readonly queries = new Map<string, SharedQuery>()
  private readonly functionQueries = new Set<AppQueries<S, P>>()
  private closing?: Promise<void>

  constructor(private readonly options: ApplicationOptions<S, P>) {
    this.schema = defineSchema(options.schema)
    this.instance = options.instance ?? this.schema.id
    try {
      receiptKey(this.instance, 'schema')
      for (const [name, validator] of Object.entries(this.schema.collections)) {
        collectionKey(this.instance, 'schema', name)
        if (validator['~standard']?.version !== 1)
          throw new Error('invalid validator')
      }
      for (const [name, mutation] of Object.entries(this.schema.mutations)) {
        if (
          !Object.hasOwn(options.mutations, name) ||
          mutation.input['~standard']?.version !== 1 ||
          mutation.output['~standard']?.version !== 1
        )
          throw new Error('invalid mutation')
      }
    } catch {
      throw new SyncError(
        'VALIDATION',
        'Schema requires valid names, Standard Schema v1 validators, and every mutation handler',
      )
    }
    this.signal = this.controller.signal
    this.limits = {
      maxRecords: options.limits?.maxRecords ?? 10_000,
      maxSnapshotBytes: options.limits?.maxSnapshotBytes ?? 8 * 1024 * 1024,
      maxRecordBytes: options.limits?.maxRecordBytes ?? 256 * 1024,
    }
    for (const value of Object.values(this.limits)) {
      if (!Number.isSafeInteger(value) || value < 1)
        throw new SyncError('VALIDATION', 'Limits must be positive integers')
    }
    this.engine = new Engine(
      options.engine.resourceRef.createRef(options.engine.id),
    )
  }

  // initialize runs explicit maintenance before any application caller is admitted.
  async initialize(migration?: Migration<S>): Promise<void> {
    const tx = await this.engine.newTransaction(true, this.signal)
    const unit = this.transaction(tx, true, this.signal, true)
    try {
      const metadata = await unit.store(metadataKey(this.instance))
      if (!metadata) throw new Error('missing metadata store')
      const stored = await metadata.get(versionKey, this.signal)
      const previous = stored.found
        ? (decodeJSON(stored.data) as unknown as StoredVersion)
        : undefined
      if (previous && previous.application !== this.schema.id) {
        throw new SyncError(
          'SCHEMA_MISMATCH',
          'This dataset belongs to a different application',
        )
      }
      if (previous && previous.version !== this.schema.version) {
        if (!migration || migration.from !== previous.version) {
          throw new SyncError(
            'SCHEMA_MISMATCH',
            'Stored version differs; supply an explicit maintenance migration',
          )
        }
        await migration.run({
          signal: this.signal,
          scopes: async () => {
            const entries = await metadata.scanRecords(
              encoder.encode('scope/'),
              {
                maxRecords: this.limits.maxRecords,
                maxBytes: this.limits.maxSnapshotBytes,
              },
              this.signal,
            )
            return entries.map((entry) => decodeJSON(entry.value) as string)
          },
          scope: (scope) => unit.collections({ subject: 'admin', scope } as P),
        })
      }
      await metadata.set(
        versionKey,
        encodeJSON({
          application: this.schema.id,
          version: this.schema.version,
        }),
        this.signal,
      )
      await unit.flush()
      await tx.commit()
      await this.engine.sync()
    } finally {
      await unit.release()
      try {
        await tx.discard()
      } finally {
        tx.release()
      }
    }
  }

  async authorize(
    principal: P,
    collection: string,
    action: 'read' | 'write',
    admin = false,
  ): Promise<void> {
    this.checkPrincipal(principal)
    if (!Object.hasOwn(this.schema.collections, collection))
      throw new SyncError('MISSING_COLLECTION', 'Collection is not declared')
    if (
      !admin &&
      !(await this.options.authorize({
        principal,
        scope: principal.scope,
        collection,
        action,
      }))
    ) {
      throw new SyncError('DENIED', `Collection ${action} is not authorized`)
    }
  }

  // Opening a capability grants no operation; every call checks its own policy.
  async openCollection(principal: P, collection: string): Promise<void> {
    try {
      await this.authorize(principal, collection, 'read')
    } catch (error) {
      if (!(error instanceof SyncError) || error.code !== 'DENIED') throw error
      await this.authorize(principal, collection, 'write')
    }
  }

  execute(
    principal: P,
    operation: Operation,
    options: CallOptions = {},
    admin = false,
  ): Promise<JsonValue> {
    if (this.closing)
      return Promise.reject(new SyncError('CLOSED', 'Server is closed'))
    const work = this.run(principal, operation, options, admin)
    this.active.add(work)
    void work.finally(() => this.active.delete(work)).catch(() => {})
    return work
  }

  /** access binds local calls and function queries to one authenticated identity. */
  access(principal: P): DatabaseAccess<S> & AppSource<S> {
    const identity = { ...principal }
    // eslint-disable-next-line @typescript-eslint/no-this-alias -- The access object's generator has its own receiver; retain the application owner.
    const application = this
    let queries: AppQueries<S, P> | undefined
    let users = 0
    return {
      ...createAccess<S>((operation, options) =>
        this.execute(identity, operation, options),
      ),
      async *watch(definition, args, signal) {
        if (application.closing)
          throw new SyncError('CLOSED', 'Server is closed')
        const source = (queries ??= new AppQueries({
          schema: application.schema,
          instance: application.instance,
          mutations: application.options.mutations,
          engine: application.engine,
          limits: application.limits,
          principal: identity,
          signal: application.signal,
          authorize: (caller, collection, action) =>
            application.authorize(caller, collection, action),
        }))
        application.functionQueries.add(source)
        users++
        try {
          yield* source.watch(definition, args, signal)
        } finally {
          if (--users === 0) {
            queries = undefined
            application.functionQueries.delete(source)
            await source.close()
          }
        }
      },
    }
  }

  close(): Promise<void> {
    this.closing ??= (async () => {
      this.controller.abort(new SyncError('CLOSED', 'Server is closing'))
      await Promise.all(
        Array.from(this.functionQueries, (queries) => queries.close()),
      )
      await Promise.allSettled(this.active)
      this.engine.release()
    })()
    return this.closing
  }

  // Identical queries share a bounded producer; authority remains per delivery.
  async *watch(
    principal: P,
    collection: string,
    prefix: string,
    signal: AbortSignal,
  ): AsyncIterable<readonly RecordEntry<JsonValue>[]> {
    if (this.closing) throw new SyncError('CLOSED', 'Server is closed')
    const lifetime = AbortSignal.any([
      this.signal,
      signal,
      ...(principal.signal ? [principal.signal] : []),
    ])
    const done = Promise.withResolvers<void>()
    this.active.add(done.promise)
    let query: SharedQuery | undefined
    let iterator: AsyncIterator<QuerySnapshot> | undefined
    const stop = () => {
      void iterator?.return?.()
    }
    const queryId = canonicalJSON([principal.scope, collection, prefix])
    try {
      await this.authorize(principal, collection, 'read')
      lifetime.throwIfAborted()
      query =
        this.queries.get(queryId) ??
        this.startQuery(queryId, principal.scope, collection, prefix)
      query.users++
      iterator = query.state.getIterable()[Symbol.asyncIterator]()
      lifetime.addEventListener('abort', stop, { once: true })
      for (;;) {
        // Delivery stays ordered while the shared producer coalesces snapshots.
        // eslint-disable-next-line react-doctor/async-await-in-loop
        const next = await iterator.next()
        if (next.done || lifetime.aborted) break
        await this.authorize(principal, collection, 'read')
        lifetime.throwIfAborted()
        if ('error' in next.value) throw next.value.error
        yield next.value.entries
      }
    } finally {
      lifetime.removeEventListener('abort', stop)
      await iterator?.return?.()
      if (query && --query.users === 0) {
        if (this.queries.get(queryId) === query) this.queries.delete(queryId)
        query.controller.abort()
      }
      done.resolve()
      this.active.delete(done.promise)
    }
  }

  private startQuery(
    id: string,
    scope: string,
    collection: string,
    prefix: string,
  ): SharedQuery {
    let snapshot: QuerySnapshot | undefined
    const query: SharedQuery = {
      controller: new AbortController(),
      state: new ItState(async () => snapshot, { mostRecentOnly: true }),
      users: 0,
    }
    this.queries.set(id, query)
    const signal = AbortSignal.any([this.signal, query.controller.signal])
    const publish = (value: QuerySnapshot) => {
      snapshot = value
      query.state.pushChangeEvent(value)
    }
    const pump = (async () => {
      try {
        for await (const entries of this.watchQuery(
          scope,
          collection,
          prefix,
          signal,
        ))
          publish({ entries })
      } catch (error) {
        if (!signal.aborted) publish({ error: publicError(error) })
      } finally {
        if (this.queries.get(id) === query) this.queries.delete(id)
      }
    })()
    this.active.add(pump)
    void pump.finally(() => this.active.delete(pump)).catch(() => {})
    return query
  }

  private async *watchQuery(
    scope: string,
    collection: string,
    prefix: string,
    lifetime: AbortSignal,
  ): AsyncIterable<readonly RecordEntry<JsonValue>[]> {
    let store: KvStore | undefined
    try {
      const key = collectionKey(this.instance, scope, collection)
      const bytes = prefix ? this.recordKey(prefix) : new Uint8Array()
      let emptyDelivered = false
      for (;;) {
        lifetime.throwIfAborted()
        // Wait for collection creation before opening its owning KV producer.
        // eslint-disable-next-line react-doctor/async-await-in-loop
        const sequence = await this.engine.getSeqno(lifetime)
        const read = await this.engine.newTransaction(false, lifetime)
        let exists = false
        try {
          const object = await read.getObject(key, lifetime)
          exists = object !== null
          object?.release()
          if (
            exists &&
            (await getObjectType(read, key, lifetime)) !== 'kv/store'
          )
            throw new KvObjectTypeError()
        } finally {
          try {
            await read.discard()
          } finally {
            read.release()
          }
        }
        if (exists) break
        if (!emptyDelivered) {
          yield []
          emptyDelivered = true
        }
        await this.engine.waitSeqno((sequence.seqno ?? 0n) + 1n, lifetime)
      }
      const access = await this.engine.accessTypedObject(key, lifetime)
      store = this.engine.resourceRef.createResource(access.resourceId, KvStore)
      for await (const entries of store.watchRecords(
        bytes,
        {
          maxRecords: this.limits.maxRecords,
          maxBytes: this.limits.maxSnapshotBytes,
        },
        lifetime,
      )) {
        yield entries.map((entry) => ({
          key: decoder.decode(entry.key),
          value: decodeJSON(entry.value),
        }))
      }
    } catch (error) {
      if (error instanceof KvScanLimitError)
        throw new SyncError('QUERY_LIMIT', error.message)
      if (!lifetime.aborted) throw publicError(error)
    } finally {
      store?.release()
    }
  }

  checkPrincipal(principal: P): void {
    if (
      !principal.subject ||
      !principal.scope ||
      principal.signal?.aborted ||
      (principal.expiresAt !== undefined &&
        (!Number.isFinite(principal.expiresAt) ||
          principal.expiresAt <= Date.now()))
    ) {
      throw new SyncError('AUTHENTICATION', 'Authentication is no longer valid')
    }
    try {
      receiptKey(this.instance, principal.scope)
      if (encodeRecordKey(principal.subject).length > 256)
        throw new Error('invalid subject')
    } catch {
      throw new SyncError(
        'AUTHENTICATION',
        'Principal subject and scope must be valid identities within 256 UTF-8 bytes',
      )
    }
  }

  private async run(
    principal: P,
    operation: Operation,
    options: CallOptions,
    admin: boolean,
  ): Promise<JsonValue> {
    // One World transaction owns records and the stable request receipt.
    let tx: Tx | undefined
    let unit: ApplicationTransaction<S, P> | undefined
    let committing = false
    const write = operation.kind !== 'get' && operation.kind !== 'scan'
    const requestId = options.requestId ?? randomUUID()
    const signal = AbortSignal.any([
      this.signal,
      ...[principal.signal, options.signal].filter((s): s is AbortSignal =>
        Boolean(s),
      ),
    ])
    try {
      this.checkPrincipal(principal)
      signal.throwIfAborted()
      tx = await this.engine.newTransaction(write, signal)
      unit = this.transaction(tx, write, signal, admin)
      const result = await unit.execute(principal, operation, requestId)

      // Commit fresh work exactly once; duplicate acceptance still needs its fence.
      this.checkPrincipal(principal)
      signal.throwIfAborted()
      if (write && !result.duplicate) {
        await unit.flush()
        signal.throwIfAborted()
        committing = true
        await tx.commit()
      }
      if (write) {
        // Release the writer before fencing an already accepted duplicate.
        await unit.release()
        unit = undefined
        await tx.discard()
        tx.release()
        tx = undefined
        await this.engine.sync()
      }
      return result.value
    } catch (error) {
      if (error instanceof KvObjectTypeError) {
        throw new SyncError('SCHEMA_MISMATCH', error.message)
      }
      if (committing) {
        throw new SyncError(
          'UNCERTAIN',
          'Acceptance could not be confirmed; retry the same request ID and input',
          requestId,
        )
      }
      if (error instanceof KvScanLimitError) {
        throw new SyncError('QUERY_LIMIT', error.message)
      }
      if (signal.aborted) {
        this.checkPrincipal(principal)
        throw new SyncError(
          this.signal.aborted ? 'CLOSED' : 'UNAVAILABLE',
          'Operation was canceled',
        )
      }
      throw publicError(error)
    } finally {
      // Subordinate KV Resources release before the caller-owned World writer.
      await unit?.release()
      if (tx) {
        try {
          await tx.discard()
        } catch {
          // Transport failure already rejects the operation.
        } finally {
          tx.release()
        }
      }
    }
  }

  /** transaction lends collection access without transferring World ownership. */
  private transaction(
    tx: Tx,
    write: boolean,
    signal: AbortSignal,
    admin: boolean,
  ): ApplicationTransaction<S, P> {
    return new ApplicationTransaction(tx, {
      schema: this.schema,
      instance: this.instance,
      mutations: this.options.mutations,
      limits: this.limits,
      write,
      signal,
      authorize: (principal, collection, action) =>
        this.authorize(principal, collection, action, admin),
    })
  }

  private recordKey(key: string): Uint8Array {
    try {
      return encodeRecordKey(key)
    } catch {
      throw new SyncError(
        'VALIDATION',
        'Record key must be nonempty UTF-8 within 1024 bytes',
      )
    }
  }
}
