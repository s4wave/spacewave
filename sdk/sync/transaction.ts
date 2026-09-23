import { KvStore, type KvTransaction } from '../kv/kv.js'
import { openWorldKvStore } from '../kv/world/store.js'
import type { WorldStateResource } from '../world/world-state.js'

import { SyncError } from './errors.js'
import {
  canonicalJSON,
  decodeJSON,
  encodeJSON,
  type JsonValue,
} from './json.js'
import {
  collectionKey,
  encodeRecordKey,
  metadataKey,
  receiptKey,
} from './keys.js'
import type { Operation } from './operation.js'
import type { AppInstance } from './instance.js'
import {
  validate,
  type MutationHandlers,
  type MutationContext,
  type Principal,
  type Schema,
  type Transaction,
  type TransactionCollection,
} from './schema.js'

/** CollectionLimits bounds materialized records and complete snapshots. */
export interface CollectionLimits {
  readonly maxRecords: number
  readonly maxSnapshotBytes: number
  readonly maxRecordBytes: number
}

/** CollectionAccess records each permission used by an accepted operation. */
interface CollectionAccess {
  collection: string
  action: 'read' | 'write'
}

/** Receipt preserves input identity and the result of an accepted request. */
interface Receipt {
  fingerprint: string
  result: JsonValue
  accesses: CollectionAccess[]
}

/** ApplicationTransactionOptions binds collection work to its host's authority. */
export interface ApplicationTransactionOptions<
  S extends Schema,
  P extends Principal,
> {
  readonly schema: S
  readonly instance: string
  readonly revision?: string
  readonly mutations: MutationHandlers<S, P>
  readonly limits: CollectionLimits
  readonly write: boolean
  readonly signal: AbortSignal
  authorize(
    principal: P,
    collection: string,
    action: 'read' | 'write',
  ): Promise<void>
}

/** UpgradeIntent binds a migration receipt to the exact source instance and revision. */
export interface UpgradeIntent {
  readonly kind: 'upgrade'
  readonly from: AppInstance
}

const encoder = new TextEncoder()
const decoder = new TextDecoder('utf-8', { fatal: true, ignoreBOM: true })

/**
 * ApplicationTransaction validates collections and stages receipts in a supplied
 * World snapshot or writer. The caller owns that WorldState and its acceptance.
 * Flush publishes KV roots into the supplied state; release frees only the KV
 * Resources this instance acquired. Neither method commits or releases the World.
 */
export class ApplicationTransaction<S extends Schema, P extends Principal> {
  private readonly stores = new Map<
    string,
    Promise<{ store: KvStore; tx: KvTransaction } | undefined>
  >()
  private released = false
  private writeCount = 0
  private writeBytes = 0

  constructor(
    private readonly state: WorldStateResource,
    private readonly options: ApplicationTransactionOptions<S, P>,
  ) {}

  /** store opens one KV transaction in the supplied World state, once per key. */
  async store(key: string): Promise<KvTransaction | undefined> {
    // Keep one typed Resource and transaction for each touched World object.
    this.checkOpen()
    let pending = this.stores.get(key)
    if (!pending) {
      pending = (async () => {
        const store = await openWorldKvStore(
          this.state,
          key,
          this.options.write,
          this.options.signal,
        )
        if (!store) return undefined
        try {
          return {
            store,
            tx: await store.openTransaction(
              this.options.write,
              this.options.signal,
            ),
          }
        } catch (error) {
          store.release()
          throw error
        }
      })()
      this.stores.set(key, pending)
    }
    return (await pending)?.tx
  }

  /** collections supplies validated record access with permission checks per call. */
  collections(principal: P, accesses: CollectionAccess[] = []): Transaction<S> {
    return {
      collection: <K extends keyof S['collections'] & string>(name: K) => {
        // Resolve only declared validators before returning the collection capability.
        const { schema, instance, limits, signal } = this.options
        if (!Object.hasOwn(schema.collections, name)) {
          throw new SyncError(
            'MISSING_COLLECTION',
            'Collection is not declared',
          )
        }
        const validator = schema.collections[name]
        const access = async (action: 'read' | 'write') => {
          this.checkOpen()
          await this.options.authorize(principal, name, action)
          if (
            !accesses.some(
              (item) => item.collection === name && item.action === action,
            )
          ) {
            accesses.push({ collection: name, action })
          }
          return this.store(collectionKey(instance, principal.scope, name))
        }

        // All reads and writes use the same snapshot and subordinate KV transaction.
        return {
          get: async (key) => {
            const bytes = recordKey(key)
            const store = await access('read')
            const entry = await store?.get(bytes, signal)
            if (entry?.found && entry.data.length > limits.maxRecordBytes) {
              throw new SyncError(
                'QUERY_LIMIT',
                'Record exceeds the configured byte limit',
              )
            }
            return entry?.found ? decodeJSON(entry.data) : undefined
          },
          scan: async (query = {}) => {
            const prefix = query.prefix ?? ''
            const bytes = prefix ? recordKey(prefix) : new Uint8Array()
            const store = await access('read')
            if (!store) return []
            const entries = await store.scanRecords(
              bytes,
              {
                maxRecords: limits.maxRecords,
                maxBytes: limits.maxSnapshotBytes,
              },
              signal,
            )
            return entries.map((entry) => ({
              key: decoder.decode(entry.key),
              value: decodeJSON(entry.value),
            }))
          },
          put: async (key, value) => {
            // Validate before changing the supplied transaction.
            const bytes = recordKey(key)
            const store = await access('write')
            if (!this.options.write || !store) {
              throw new SyncError('DENIED', 'Transaction is read-only')
            }
            const encoded = encodeJSON(
              await validate(validator, value, 'Record'),
            )
            if (encoded.length > limits.maxRecordBytes) {
              throw new SyncError(
                'QUERY_LIMIT',
                'Record exceeds the configured byte limit',
              )
            }

            // Stage the record; only the caller can accept the World transaction.
            this.trackWrite(bytes.length + encoded.length)
            await store.set(bytes, encoded, signal)
          },
          delete: async (key) => {
            const bytes = recordKey(key)
            const store = await access('write')
            if (!this.options.write || !store) {
              throw new SyncError('DENIED', 'Transaction is read-only')
            }
            this.trackWrite(bytes.length)
            await store.delete(bytes, signal)
          },
        } as TransactionCollection<S['collections'][K]>
      },
    }
  }

  /** execute stages an operation and its receipt, or returns an authorized duplicate. */
  async execute(
    principal: P,
    operation: Operation,
    requestId: string,
  ): Promise<{ value: JsonValue; duplicate: boolean }> {
    // Identify the exact intent before inspecting an earlier acceptance.
    this.checkOpen()
    if (!requestId || requestId.length > 128) {
      throw new SyncError(
        'VALIDATION',
        'Request ID must contain 1 to 128 characters',
      )
    }
    const { schema, instance, signal } = this.options
    const write = operation.kind !== 'get' && operation.kind !== 'scan'
    if (write && !this.options.write) {
      throw new SyncError('DENIED', 'Transaction is read-only')
    }
    const fingerprint = this.fingerprint(operation)
    if (
      encoder.encode(fingerprint).length > this.options.limits.maxRecordBytes
    ) {
      throw new SyncError(
        'QUERY_LIMIT',
        'Operation exceeds the configured byte limit',
      )
    }
    const accesses: CollectionAccess[] = []
    const collections = this.collections(principal, accesses)
    if ('collection' in operation) {
      await this.options.authorize(
        principal,
        operation.collection,
        write ? 'write' : 'read',
      )
    }

    // Recheck every permission of a duplicate before returning its retained result.
    const accepted = write
      ? await this.readAcceptance(principal, operation, requestId)
      : undefined
    if (accepted) return { value: accepted.value, duplicate: true }
    const receipts = write
      ? await this.store(receiptKey(instance, principal.scope))
      : undefined

    // Validate named handlers through the same contract on every host.
    let result: JsonValue
    if (operation.kind === 'mutate') {
      const definition = Object.hasOwn(schema.mutations, operation.name)
        ? schema.mutations[operation.name]
        : undefined
      const handler = Object.hasOwn(this.options.mutations, operation.name)
        ? this.options.mutations[operation.name]
        : undefined
      if (!definition || !handler) {
        throw new SyncError(
          'VALIDATION',
          'Mutation is not declared and implemented',
        )
      }
      const input = await validate(
        definition.input,
        operation.input,
        'Mutation input',
      )
      const output = await runMutation(
        { ...collections, principal, scope: principal.scope, signal },
        (context) => handler(context, input),
      )
      result = await validate(definition.output, output, 'Mutation output')
      if (encodeJSON(result).length > this.options.limits.maxRecordBytes) {
        throw new SyncError(
          'QUERY_LIMIT',
          'Mutation result exceeds the configured byte limit',
        )
      }
    } else {
      const collection = collections.collection(operation.collection)
      switch (operation.kind) {
        case 'get': {
          const value = await collection.get(operation.key)
          result =
            value === undefined
              ? { found: false }
              : { found: true, value: value as JsonValue }
          break
        }
        case 'scan':
          result = (await collection.scan({
            prefix: operation.prefix,
          })) as unknown as JsonValue
          break
        case 'put':
          await collection.put(operation.key, operation.value)
          result = null
          break
        case 'delete':
          await collection.delete(operation.key)
          result = null
          break
      }
    }

    // Record the result in the same state as every record changed by this call.
    if (receipts) {
      await this.recordAcceptance(
        principal,
        operation,
        requestId,
        result,
        accesses,
      )
    }
    return { value: result, duplicate: false }
  }

  /** upgrade stages a migration and receipt under the same identity rules as named mutations. */
  async upgrade(
    principal: P,
    intent: UpgradeIntent,
    requestId: string,
    apply: (collections: Transaction<S>) => Promise<void>,
  ): Promise<void> {
    this.checkOpen()
    if (!requestId || requestId.length > 128) {
      throw new SyncError(
        'VALIDATION',
        'Request ID must contain 1 to 128 characters',
      )
    }
    if (
      encoder.encode(this.fingerprint(intent)).length >
      this.options.limits.maxRecordBytes
    ) {
      throw new SyncError('QUERY_LIMIT', 'Upgrade exceeds the input byte limit')
    }
    if (!this.options.write)
      throw new SyncError('DENIED', 'Transaction is read-only')
    if (await this.readAcceptance(principal, intent, requestId)) return

    // The caller changes data and binding before this receipt becomes visible.
    const accesses: CollectionAccess[] = []
    await apply(this.collections(principal, accesses))
    await this.recordAcceptance(principal, intent, requestId, null, accesses)
  }

  /** recordAcceptance stages the shared receipt format after all operation work succeeds. */
  private async recordAcceptance(
    principal: P,
    operation: Operation | UpgradeIntent,
    requestId: string,
    result: JsonValue,
    accesses: CollectionAccess[],
  ): Promise<void> {
    const { instance, signal } = this.options
    const metadata = await this.store(metadataKey(instance))
    await metadata!.set(
      encoder.encode(`scope/${canonicalJSON(principal.scope)}`),
      encodeJSON(principal.scope),
      signal,
    )
    const receipts = await this.store(receiptKey(instance, principal.scope))
    await receipts!.set(
      encoder.encode(canonicalJSON([principal.subject, requestId])),
      encodeJSON({
        fingerprint: this.fingerprint(operation),
        result,
        accesses,
      }),
      signal,
    )
  }

  /** readAcceptance reads an accepted result without executing or staging an operation. */
  async readAcceptance(
    principal: P,
    operation: Operation | UpgradeIntent,
    requestId: string,
  ): Promise<{ value: JsonValue } | undefined> {
    this.checkOpen()
    const receipts = await this.store(
      receiptKey(this.options.instance, principal.scope),
    )
    const key = encoder.encode(canonicalJSON([principal.subject, requestId]))
    const stored = await receipts?.get(key, this.options.signal)
    if (!stored?.found) return undefined
    const receipt = decodeJSON(stored.data) as unknown as Receipt
    if (receipt.fingerprint !== this.fingerprint(operation)) {
      throw new SyncError(
        'CONFLICT',
        'Request ID was already used for different input',
        requestId,
      )
    }
    for (const access of receipt.accesses) {
      // eslint-disable-next-line react-doctor/async-await-in-loop -- Stop permission checks immediately after a denial.
      await this.options.authorize(principal, access.collection, access.action)
    }
    return { value: receipt.result }
  }

  /** fingerprint freezes the application revision and complete operation intent. */
  private fingerprint(operation: Operation | UpgradeIntent): string {
    return canonicalJSON({
      application: this.options.schema.id,
      version: this.options.schema.version,
      revision: this.options.revision ?? '',
      operation,
    })
  }

  /** flush publishes staged KV roots without committing the caller's World writer. */
  async flush(): Promise<void> {
    this.checkOpen()
    const stores = await Promise.all(this.stores.values())
    for (const store of stores) {
      // eslint-disable-next-line react-doctor/async-await-in-loop -- Each KV commit updates the same supplied World writer.
      await store?.tx.commit(this.options.signal)
    }
  }

  /** release discards remaining KV work and releases independently acquired handles. */
  async release(): Promise<void> {
    // Prevent new acquisitions while joining every acquisition already in progress.
    if (this.released) return
    this.released = true
    const stores = await Promise.allSettled(this.stores.values())

    // A transport failure may prevent discard, but local references still release.
    await Promise.all(
      stores.map(async (result) => {
        if (result.status !== 'fulfilled' || !result.value) return
        try {
          await result.value.tx.discard()
        } catch {
          // Disconnection already fails the surrounding operation.
        } finally {
          result.value.store.release()
        }
      }),
    )
  }

  /** trackWrite bounds the total work staged by a trusted mutation before acceptance. */
  private trackWrite(bytes: number): void {
    this.writeCount++
    this.writeBytes += bytes
    if (
      this.writeCount > this.options.limits.maxRecords ||
      this.writeBytes > this.options.limits.maxSnapshotBytes
    ) {
      throw new SyncError(
        'QUERY_LIMIT',
        'Mutation exceeds the transaction data limit',
      )
    }
  }

  /** checkOpen rejects work after cancellation or collection lifetime release. */
  private checkOpen(): void {
    if (this.released) throw new SyncError('CLOSED', 'Transaction is closed')
    this.options.signal.throwIfAborted()
  }
}

/** runMutation joins started calls and revokes collection access when a callback returns. */
async function runMutation<S extends Schema, P extends Principal, R>(
  context: MutationContext<S, P>,
  handler: (context: MutationContext<S, P>) => Promise<R>,
): Promise<R> {
  return withTransactionLifetime((wrap) =>
    handler({ ...context, ...wrap(context) }),
  )
}

/** withTransactionLifetime joins started calls and revokes every wrapped facade after the callback. */
export async function withTransactionLifetime<R>(
  handler: (
    wrap: <S extends Schema>(source: Transaction<S>) => Transaction<S>,
  ) => Promise<R>,
): Promise<R> {
  let active = true
  const pending = new Set<Promise<unknown>>()
  const call = <T>(operation: () => Promise<T>): Promise<T> => {
    if (!active)
      return Promise.reject(new SyncError('CLOSED', 'Mutation has ended'))
    const promise = operation()
    pending.add(promise)
    void promise.then(
      () => pending.delete(promise),
      () => pending.delete(promise),
    )
    return promise
  }
  try {
    // eslint-disable-next-line react-doctor/async-defer-await -- Only the completed callback determines whether collection calls remain pending.
    const result = await handler((source) => ({
      collection: (name) => {
        const collection = source.collection(name)
        return {
          get: (key) => call(() => collection.get(key)),
          scan: (query) => call(() => collection.scan(query)),
          put: (key, value) => call(() => collection.put(key, value)),
          delete: (key) => call(() => collection.delete(key)),
        }
      },
    }))
    if (pending.size) {
      throw new SyncError(
        'VALIDATION',
        'Await every collection operation before returning',
      )
    }
    return result
  } finally {
    active = false
    await Promise.allSettled(pending)
  }
}

/** recordKey reports invalid keys through the public sync error contract. */
function recordKey(key: string): Uint8Array {
  try {
    return encodeRecordKey(key)
  } catch {
    throw new SyncError(
      'VALIDATION',
      'Record key must be nonempty UTF-8 within 1024 bytes',
    )
  }
}
