import { SyncError } from './errors.js'
import {
  canonicalJSON,
  decodeJSON,
  encodeJSON,
  type JsonValue,
} from './json.js'
import type {
  Output,
  Query,
  Schema,
  TransactionCollection,
  Validator,
} from './schema.js'

/** LiveQuerySnapshot carries a successful value or a terminal query error. */
export type LiveQuerySnapshot<R> =
  | { readonly status: 'current'; readonly value: R; readonly seqno: bigint }
  | { readonly status: 'error'; readonly error: SyncError }

/** QueryCollection exposes only snapshot reads to a query callback. */
export type QueryCollection<V extends Validator> = Pick<
  TransactionCollection<V>,
  'get' | 'scan'
>

/** QueryContext reads every collection from the same snapshot. */
export interface QueryContext<S extends Schema> {
  collection<K extends keyof S['collections'] & string>(
    name: K,
  ): QueryCollection<S['collections'][K]>
}

/** QueryDefinition binds validated arguments to a pure snapshot computation. */
export interface QueryDefinition<S extends Schema, A extends Validator, R> {
  readonly name: string
  readonly input: A
  evaluate(context: QueryContext<S>, args: Output<A>): R | Promise<R>
}

/** defineQuery preserves inferred arguments and results without evaluating a query. */
export function defineQuery<S extends Schema, A extends Validator, R>(
  _schema: S,
  definition: QueryDefinition<S, A, R>,
): QueryDefinition<S, A, R> {
  return Object.freeze({ ...definition })
}

/** RecordDependency tracks presence and the JSON paths actually inspected in one record. */
export interface RecordDependency {
  readonly key: string
  readonly paths: readonly (readonly string[])[]
  readonly shapes: readonly (readonly string[])[]
}

/** CollectionDependencies retains scan membership and inspected records, including absent keys. */
export interface CollectionDependencies {
  readonly collection: string
  readonly prefixes: readonly string[]
  readonly records: readonly RecordDependency[]
}

/** QueryEvaluation materializes the result before freezing its complete read set. */
export interface QueryEvaluation<R> {
  readonly value: R
  readonly dependencies: readonly CollectionDependencies[]
}

/** RecordChange carries accepted before/after values; undefined represents absence. */
export interface RecordChange {
  readonly collection: string
  readonly key: string
  readonly before?: JsonValue
  readonly after?: JsonValue
}

/**
 * evaluateQuery traces ordinary property reads, predicates, sorting, and branches.
 * Returning a complete record materializes every returned field. Queries may
 * await snapshot reads; external effects and mutation of read values are forbidden.
 */
export async function evaluateQuery<S extends Schema, R>(
  source: QueryContext<S>,
  evaluate: (context: QueryContext<S>) => R | Promise<R>,
): Promise<QueryEvaluation<R>> {
  // Retain every membership and record read, even when a predicate excludes it.
  const collections = new Map<
    string,
    {
      prefixes: Set<string>
      records: Map<
        string,
        { paths: Map<string, string[]>; shapes: Map<string, string[]> }
      >
    }
  >()
  const pending = new Set<Promise<unknown>>()
  let active = true
  const record = (collection: string, key: string, value: unknown) => {
    const tracked = collections.get(collection)!
    let dependencies = tracked.records.get(key)
    if (!dependencies) {
      dependencies = { paths: new Map(), shapes: new Map() }
      tracked.records.set(key, dependencies)
    }
    const read = (path: string[], whole = true) => {
      const target = whole ? dependencies.paths : dependencies.shapes
      target.set(JSON.stringify(path), path)
    }
    if (value === undefined) return undefined
    return traceValue(decodeJSON(encodeJSON(value)), [], read)
  }
  const context: QueryContext<S> = {
    collection: (name) => {
      if (!active) throw new SyncError('CLOSED', 'Query evaluation has ended')
      const original = source.collection(name)
      if (!collections.has(name))
        collections.set(name, { prefixes: new Set(), records: new Map() })
      const track = <T>(read: () => Promise<T>): Promise<T> => {
        if (!active)
          return Promise.reject(
            new SyncError('CLOSED', 'Query evaluation has ended'),
          )
        const promise = read()
        pending.add(promise)
        void promise.finally(() => pending.delete(promise)).catch(() => {})
        return promise
      }
      return {
        get: (key: string) =>
          track(async () => record(name, key, await original.get(key))),
        scan: (query: Query = {}) =>
          track(async () => {
            collections.get(name)!.prefixes.add(query.prefix ?? '')
            return (await original.scan(query)).map((entry) => ({
              key: entry.key,
              value: record(name, entry.key, entry.value),
            }))
          }),
      } as QueryCollection<S['collections'][typeof name]>
    },
  }

  // Materializing output completes the read set for records returned as values.
  try {
    // eslint-disable-next-line react-doctor/async-defer-await -- Pending reads are measured after the callback has returned.
    const result = await evaluate(context)
    if (pending.size) {
      throw new SyncError('VALIDATION', 'Query must await every snapshot read')
    }
    const value = decodeJSON(encodeJSON(result)) as R
    const dependencies = [...collections].map(([collection, tracked]) => ({
      collection,
      prefixes: [...tracked.prefixes],
      records: [...tracked.records].map(([key, { paths, shapes }]) => ({
        key,
        paths: [...paths.values()],
        shapes: [...shapes.values()],
      })),
    }))
    return { value, dependencies }
  } finally {
    active = false
    await Promise.allSettled(pending)
  }
}

/**
 * QueryFilter indexes dependencies once and rejects irrelevant accepted changes.
 * Unknown collection changes invalidate conservatively. Rebuild the filter only
 * after a successful reevaluation so dynamic branches replace their read sets.
 */
export class QueryFilter {
  private readonly collections: ReadonlyMap<
    string,
    {
      prefixes: readonly string[]
      records: ReadonlyMap<string, RecordDependency>
    }
  >

  constructor(dependencies: readonly CollectionDependencies[]) {
    this.collections = new Map(
      dependencies.map((dependency) => [
        dependency.collection,
        {
          prefixes: dependency.prefixes,
          records: new Map(
            dependency.records.map((record) => [record.key, record]),
          ),
        },
      ]),
    )
  }

  /** watchesKey filters unrelated record addresses before any value is read. */
  watchesKey(collectionName: string, key: string): boolean {
    const collection = this.collections.get(collectionName)
    return (
      !!collection &&
      (collection.records.has(key) ||
        collection.prefixes.some((prefix) => key.startsWith(prefix)))
    )
  }

  /** affected reports whether one accepted change can alter the query result. */
  affected(change: RecordChange): boolean {
    // A scan depends on insertions and deletions anywhere within its prefix.
    const collection = this.collections.get(change.collection)
    if (!collection) return false
    const presenceChanged =
      (change.before === undefined) !== (change.after === undefined)
    if (
      presenceChanged &&
      collection.prefixes.some((prefix) => change.key.startsWith(prefix))
    )
      return true

    // Existing records depend only on presence and fields the query inspected.
    const record = collection.records.get(change.key)
    if (!record) return false
    if (presenceChanged) return true
    return (
      record.paths.some(
        (path) =>
          !sameValue(valueAt(change.before, path), valueAt(change.after, path)),
      ) ||
      record.shapes.some(
        (path) =>
          valueShape(valueAt(change.before, path)) !==
          valueShape(valueAt(change.after, path)),
      )
    )
  }

  /** unknown invalidates a watched collection when no complete change description exists. */
  unknown(collection: string): boolean {
    return this.collections.has(collection)
  }
}

/** traceValue recursively wraps JSON values and rejects attempted mutation. */
function traceValue(
  value: unknown,
  path: string[],
  read: (path: string[], whole?: boolean) => void,
): unknown {
  if (value === null || typeof value !== 'object') {
    read(path)
    return value
  }
  read(path, false)
  return new Proxy(value, {
    get(target, key, receiver) {
      if (typeof key !== 'string') return Reflect.get(target, key, receiver)
      const child = [...path, key]
      const result = Reflect.get(target, key, receiver)
      if (typeof result === 'function') return result
      return traceValue(result, child, read)
    },
    has(target, key) {
      read(path)
      return Reflect.has(target, key)
    },
    ownKeys(target) {
      read(path)
      return Reflect.ownKeys(target)
    },
    getOwnPropertyDescriptor(target, key) {
      if (typeof key === 'string') read([...path, key])
      const descriptor = Reflect.getOwnPropertyDescriptor(target, key)
      if (descriptor && 'value' in descriptor && typeof key === 'string') {
        return {
          ...descriptor,
          value: traceValue(descriptor.value, [...path, key], read),
        }
      }
      return descriptor
    },
    set() {
      throw new SyncError('VALIDATION', 'Queries cannot mutate snapshot values')
    },
    deleteProperty() {
      throw new SyncError('VALIDATION', 'Queries cannot mutate snapshot values')
    },
    defineProperty() {
      throw new SyncError('VALIDATION', 'Queries cannot mutate snapshot values')
    },
    setPrototypeOf() {
      throw new SyncError('VALIDATION', 'Queries cannot mutate snapshot values')
    },
    preventExtensions() {
      throw new SyncError('VALIDATION', 'Queries cannot mutate snapshot values')
    },
  })
}

/** valueAt reads a tracked JSON path while preserving missing fields. */
function valueAt(value: unknown, path: readonly string[]): unknown {
  let current = value
  for (const key of path) {
    if (current === null || typeof current !== 'object') return undefined
    current = (current as Record<string, unknown>)[key]
  }
  return current
}

/** sameValue compares tracked JSON fields without confusing null and absence. */
function sameValue(a: unknown, b: unknown): boolean {
  if (a === undefined || b === undefined) return a === b
  return canonicalJSON(a) === canonicalJSON(b)
}

/** valueShape preserves object truthiness and array identity without inspecting siblings. */
function valueShape(value: unknown): string {
  if (value === null) return 'null'
  if (Array.isArray(value)) return 'array'
  return typeof value
}
