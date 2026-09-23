import { ItState } from '../../bldr/web/bldr/it-state.js'
import { Engine } from '../world/engine.js'
import { ObjectRootRef } from '../world/world.pb.js'
import type { ObjectRef } from '../../db/bucket/bucket.pb.js'
import type { WorldStateResource } from '../world/world-state.js'
import { getObjectType } from '../world/types/types.js'

import { SyncError, publicError } from './errors.js'
import {
  canonicalJSON,
  decodeJSON,
  encodeJSON,
  type JsonValue,
} from './json.js'
import { collectionKey } from './keys.js'
import {
  type LiveQuerySnapshot,
  evaluateQuery,
  QueryFilter,
  type CollectionDependencies,
  type QueryContext,
  type QueryDefinition,
  type QueryEvaluation,
} from './query.js'
import {
  validate,
  type Input,
  type Output,
  type Principal,
  type Schema,
  type Validator,
} from './schema.js'
import {
  ApplicationTransaction,
  type ApplicationTransactionOptions,
} from './transaction.js'

/** AppQueriesOptions binds query producers to one application and authenticated principal. */
export interface AppQueriesOptions<
  S extends Schema,
  P extends Principal,
> extends Omit<ApplicationTransactionOptions<S, P>, 'write' | 'signal'> {
  readonly engine: Engine
  readonly principal: P
  readonly signal?: AbortSignal
  checkState?(state: WorldStateResource, signal: AbortSignal): Promise<void>
}

/** QueryDelivery binds a materialized result to the permissions required to disclose it. */
interface QueryDelivery<R> {
  readonly snapshot: LiveQuerySnapshot<R>
  readonly collections: readonly string[]
}

/** QueryProducer shares a single snapshot reader among equivalent subscriptions. */
interface QueryProducer {
  readonly controller: AbortController
  readonly state: ItState<QueryDelivery<unknown>>
  readonly done: Promise<void>
  users: number
}

/** QueryReads retains exactly the keys and ranges read during one evaluation. */
type QueryReads = Map<string, Map<string, JsonValue | undefined>>

/**
 * AppQueries owns live function queries for one authenticated app attachment.
 * Subscribers with identical definition and arguments share a producer. A World
 * sequence wake checks object roots first; unrelated objects do no record reads
 * or callback work. Immutable tree comparison filters changed roots before record
 * reads. Missing prior roots and retyped objects trigger full reevaluation.
 */
export class AppQueries<S extends Schema, P extends Principal> {
  private readonly engine: Engine
  private readonly controller = new AbortController()
  private readonly producers = new Map<object, Map<string, QueryProducer>>()
  private readonly active = new Set<Promise<void>>()
  private closing?: Promise<void>
  private readonly counts = {
    snapshots: 0,
    processedSeqno: 0n,
    evaluations: 0,
    keyReads: 0,
    rangeScans: 0,
  }

  /** diagnostics reports source work for profiling without exposing application data. */
  diagnostics() {
    return { ...this.counts }
  }

  /** measureReads counts real source reads, including filter work and callback evaluation. */
  private measureReads(source: QueryContext<S>): QueryContext<S> {
    return {
      collection: (name) => {
        const collection = source.collection(name)
        return {
          get: async (key) => {
            this.counts.keyReads++
            return collection.get(key)
          },
          scan: async (query) => {
            this.counts.rangeScans++
            return collection.scan(query)
          },
        }
      },
    }
  }

  constructor(private readonly options: AppQueriesOptions<S, P>) {
    this.engine = new Engine(
      options.engine.resourceRef.createRef(options.engine.id),
    )
  }

  /** watch streams materialized results and releases its producer after the last subscriber. */
  async *watch<A extends Validator, R>(
    definition: QueryDefinition<S, A, R>,
    args: Input<A>,
    signal?: AbortSignal,
  ): AsyncIterable<LiveQuerySnapshot<R>> {
    // Keep producers inside this principal's authority and lifetime.
    const lifetime = AbortSignal.any([
      this.controller.signal,
      ...[signal, this.options.signal, this.options.principal.signal].filter(
        (value): value is AbortSignal => value !== undefined,
      ),
    ])
    lifetime.throwIfAborted()
    const input = await validate(
      definition.input,
      decodeJSON(encodeJSON(args)),
      'Query input',
    )
    lifetime.throwIfAborted()
    const key = canonicalJSON(input)
    let group = this.producers.get(definition)
    if (!group) {
      group = new Map()
      this.producers.set(definition, group)
    }
    let producer = group.get(key)
    if (!producer) {
      producer = this.start(definition, input, () => {
        if (group.get(key) === producer) group.delete(key)
        if (!group.size && this.producers.get(definition) === group)
          this.producers.delete(definition)
      })
      group.set(key, producer)
    }
    producer.users++
    const iterator = producer.state.getIterable()[Symbol.asyncIterator]()
    const stop = () => {
      void iterator.return?.()
    }
    lifetime.addEventListener('abort', stop, { once: true })

    // Recheck the principal before every delivery, including a shared snapshot.
    try {
      for (;;) {
        const next = await iterator.next()
        if (next.done || lifetime.aborted) break
        this.checkPrincipal()
        for (const collection of next.value.collections) {
          // eslint-disable-next-line react-doctor/async-await-in-loop -- Stop permission checks immediately after a denial.
          await this.options.authorize(
            this.options.principal,
            collection,
            'read',
          )
        }
        lifetime.throwIfAborted()
        const snapshot = next.value.snapshot
        if (snapshot.status === 'error') {
          yield snapshot
          break
        }
        // A subscriber cannot mutate another subscriber's cached result.
        yield {
          ...snapshot,
          value: decodeJSON(encodeJSON(snapshot.value)) as R,
        }
      }
    } finally {
      lifetime.removeEventListener('abort', stop)
      await iterator.return?.()
      if (--producer.users === 0) {
        if (group.get(key) === producer) group.delete(key)
        if (!group.size && this.producers.get(definition) === group)
          this.producers.delete(definition)
        producer.controller.abort()
        await producer.done
      }
    }
  }

  /** close cancels all producers before releasing the independently held Engine ref. */
  close(): Promise<void> {
    return (this.closing ??= (async () => {
      this.controller.abort()
      await Promise.allSettled(this.active)
      this.engine.release()
    })())
  }

  /** start runs one producer without retaining a World transaction between updates. */
  private start<A extends Validator, R>(
    definition: QueryDefinition<S, A, R>,
    input: Output<A> & JsonValue,
    remove: () => void,
  ): QueryProducer {
    let current: QueryDelivery<unknown> | undefined
    const state = new ItState(async () => current, { mostRecentOnly: true })
    const controller = new AbortController()
    const signal = AbortSignal.any([
      this.controller.signal,
      controller.signal,
      ...[this.options.signal, this.options.principal.signal].filter(
        (value): value is AbortSignal => value !== undefined,
      ),
    ])
    const done = (async () => {
      try {
        for await (const snapshot of this.evaluate(definition, input, signal)) {
          current = snapshot
          state.pushChangeEvent(snapshot)
        }
      } catch (error) {
        if (!signal.aborted) {
          current = {
            snapshot: { status: 'error', error: publicError(error) },
            collections: [],
          }
          state.pushChangeEvent(current)
        }
      } finally {
        remove()
      }
    })()
    this.active.add(done)
    void done.finally(() => this.active.delete(done))
    return { controller, state, done, users: 0 }
  }

  /** evaluate filters changed snapshots before rerunning the pure callback. */
  private async *evaluate<A extends Validator, R>(
    definition: QueryDefinition<S, A, R>,
    input: Output<A> & JsonValue,
    signal: AbortSignal,
  ): AsyncIterable<QueryDelivery<R>> {
    let evaluation: QueryEvaluation<R> | undefined
    let filter: QueryFilter | undefined
    let reads: QueryReads = new Map()
    let roots: { signature: string; type: string; ref?: ObjectRef }[] = []
    let seqno = 0n
    for (;;) {
      signal.throwIfAborted()
      this.checkPrincipal()
      const tx = await this.engine.newTransaction(false, signal)
      const unit = new ApplicationTransaction(tx, {
        ...this.options,
        write: false,
        signal,
      })
      let snapshot: QueryDelivery<R> | undefined
      try {
        await this.options.checkState?.(tx, signal)
        seqno = (await tx.getSeqno(signal)).seqno
        this.counts.snapshots++
        const source = this.measureReads(
          unit.collections(this.options.principal),
        )
        let affected = !evaluation
        if (evaluation) {
          const nextRoots = await this.roots(
            tx,
            evaluation.dependencies,
            signal,
          )
          const changed = evaluation.dependencies.filter(
            (_, index) => nextRoots[index].signature !== roots[index].signature,
          )
          if (changed.length) {
            affected = nextRoots.some(
              (root, index) => root.type !== roots[index].type,
            )
            const changes = affected
              ? {}
              : await tx.compareObjectRecords(
                  changed.map(({ collection }) => {
                    const index = evaluation!.dependencies.findIndex(
                      (entry) => entry.collection === collection,
                    )
                    return {
                      objectKey: collectionKey(
                        this.options.instance,
                        this.options.principal.scope,
                        collection,
                      ),
                      rootRef: roots[index].ref,
                    }
                  }),
                  signal,
                )
            for (let i = 0; !affected && i < changed.length; i++) {
              const dependency = changed[i]
              const change = changes.changes?.[i]
              if (!change || change.unknown) {
                affected = true
                break
              }
              const collection = source.collection(dependency.collection)
              const before = reads.get(dependency.collection)!
              const decoder = new TextDecoder('utf-8', {
                fatal: true,
                ignoreBOM: true,
              })
              const keys = new Set(
                change.keys?.map((key) => decoder.decode(key)),
              )
              for (const key of keys) {
                if (!filter!.watchesKey(dependency.collection, key)) continue
                const after = (await collection.get(key)) as
                  | JsonValue
                  | undefined
                if (
                  filter!.affected({
                    collection: dependency.collection,
                    key,
                    before: before.get(key),
                    after,
                  })
                ) {
                  affected = true
                  break
                }
                before.set(key, after)
              }
            }
          }
          roots = nextRoots
        }

        // Read every collection from this same transaction and materialize before release.
        if (affected) {
          reads = new Map()
          this.counts.evaluations++
          evaluation = await evaluateQuery(
            recordReads(
              source,
              reads,
              this.options.limits.maxSnapshotBytes,
              this.options.limits.maxRecords,
            ),
            (context) =>
              definition.evaluate(
                context,
                decodeJSON(encodeJSON(input)) as Output<A>,
              ),
          )
          filter = new QueryFilter(evaluation.dependencies)
          roots = await this.roots(tx, evaluation.dependencies, signal)
          if (
            encodeJSON(evaluation.value).length >
            this.options.limits.maxSnapshotBytes
          ) {
            throw new SyncError(
              'QUERY_LIMIT',
              'Query result exceeds the configured byte limit',
            )
          }
          snapshot = {
            snapshot: { status: 'current', value: evaluation.value, seqno },
            collections: evaluation.dependencies.map(
              ({ collection }) => collection,
            ),
          }
        }
      } finally {
        await unit.release()
        try {
          await tx.discard()
        } finally {
          tx.release()
        }
      }
      this.counts.processedSeqno = seqno
      if (snapshot) yield snapshot

      // The sequence belongs to the read snapshot, so commits during reads cannot be missed.
      await this.engine.waitSeqno(seqno + 1n, signal)
    }
  }

  /** roots includes graph type as well as root identity to catch external type replacement. */
  private async roots(
    state: WorldStateResource,
    dependencies: readonly CollectionDependencies[],
    signal: AbortSignal,
  ): Promise<{ signature: string; type: string; ref?: ObjectRef }[]> {
    const keys = dependencies.map(({ collection }) =>
      collectionKey(
        this.options.instance,
        this.options.principal.scope,
        collection,
      ),
    )
    for (const dependency of dependencies) {
      // eslint-disable-next-line react-doctor/async-await-in-loop -- Stop permission checks immediately after a denial.
      await this.options.authorize(
        this.options.principal,
        dependency.collection,
        'read',
      )
    }
    const refs = await state.getObjectRootRefs(keys, signal)
    return Promise.all(
      refs.map(async (ref, index) => {
        const type = ref.exists
          ? await getObjectType(state, keys[index], signal)
          : ''
        return {
          type,
          ref: ref.rootRef,
          signature: `${type}:${Array.from(ObjectRootRef.toBinary(ref)).join(',')}`,
        }
      }),
    )
  }

  /** checkPrincipal prevents expired or revoked authentication from delivering cached data. */
  private checkPrincipal(): void {
    const principal = this.options.principal
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
  }
}

/** recordReads retains source values separately from the dependency proxies. */
function recordReads<S extends Schema>(
  source: QueryContext<S>,
  reads: QueryReads,
  maxBytes: number,
  maxRecords: number,
): QueryContext<S> {
  let bytes = 0
  let count = 0
  const save = (
    collection: string,
    key: string,
    value: JsonValue | undefined,
  ) => {
    let records = reads.get(collection)
    if (!records) {
      records = new Map()
      reads.set(collection, records)
    }
    if (!records.has(key)) {
      bytes += encodeJSON([collection, key, value ?? null]).length
      if (++count > maxRecords || bytes > maxBytes)
        throw new SyncError(
          'QUERY_LIMIT',
          'Query reads exceed the configured byte limit',
        )
    }
    records.set(key, value)
  }
  return {
    collection(name) {
      const collection = source.collection(name)
      if (!reads.has(name)) reads.set(name, new Map())
      return {
        async get(key) {
          const value = await collection.get(key)
          save(name, key, value as JsonValue | undefined)
          return value
        },
        async scan(query) {
          const entries = await collection.scan(query)
          for (const entry of entries)
            save(name, entry.key, entry.value as JsonValue)
          return entries
        },
      }
    },
  }
}
