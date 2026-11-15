import type { Message, MessageType } from '@aptre/protobuf-es-lite'

import type { Engine } from './engine.js'
import {
  changePattern,
  type ChangeFeedEvent,
  type ChangeFeedSubscription,
  type ChangeNames,
  type ChangePattern,
} from './change-feed.js'
import { iriToKey, keyToIRI } from './graph-utils.js'
import { buildTypeObjectKey, TypePred } from './types/types.js'
import type { Tx, WorldStateResource } from './world-state.js'
import { GraphEdgeBucketDirection, type WorldChange } from './world.pb.js'

// KeyedSet is a set of World objects and one value read for each, all read
// at or after one revision. keys lists the members at seqno in listing
// order; rows holds the value of each member that has one, such as each
// existing object's body. complete is false while a full read is still
// reading values; a member without a row then may not have been read yet.
export interface KeyedSet<T> {
  readonly seqno: bigint
  readonly keys: readonly string[]
  readonly rows: ReadonlyMap<string, T>
  readonly complete: boolean
}

// KeyedRowPage is one page of values a source read, in request order. seqno
// is the World revision the page read, when the read reports it.
export interface KeyedRowPage<T> {
  readonly seqno?: bigint
  readonly rows: readonly (readonly [string, T | undefined])[]
}

// KeyedRoute is what one change touches: whether it may change the members,
// and the members whose values it may change.
export interface KeyedRoute {
  readonly list: boolean
  readonly keys: readonly string[]
}

// KeyedSetReader is the read side of one World revision a source reads: a
// read transaction, or an immutable World snapshot.
export type KeyedSetReader = Pick<
  WorldStateResource,
  | 'getSeqno'
  | 'listGraphEdgeBuckets'
  | 'listObjectsWithType'
  | 'getObjectBodyPages'
>

// KeyedSetSource describes one keyed set: how to list its members, what to
// follow for them, and how to read their values. Every read takes the one
// read transaction of a wake, so a wake reads one revision.
export interface KeyedSetSource<T> {
  // members names the changes that may change the members.
  readonly members: ChangeNames
  // list reads the members at tx's revision, in listing order.
  list(tx: KeyedSetReader, signal: AbortSignal): Promise<string[]>
  // names names the changes that may change the values of keys.
  names(keys: readonly string[]): ChangeNames
  // route reads which members a change touches.
  route(change: WorldChange, members: ReadonlySet<string>): KeyedRoute
  // read reads the values of keys at tx's revision, page by page.
  read(
    tx: KeyedSetReader,
    keys: readonly string[],
    signal: AbortSignal,
  ): AsyncIterable<KeyedRowPage<T>>
}

// mergeNames joins change names.
function mergeNames(...names: ChangeNames[]): ChangeNames {
  return {
    keys: names.flatMap((n) => n.keys ?? []),
    patterns: names.flatMap((n) => n.patterns ?? []),
  }
}

/**
 * watchKeyedSet follows a keyed set, yielding it after its first page and
 * after each wake that changed it.
 *
 * The first read lists the members and reads every value at one revision R,
 * yielding each page as it arrives, then follows the set's names after R
 * over the Engine's change feed. Each later wake takes every queued event,
 * opens one read transaction at R', re-lists the members only when a change
 * may have moved them, reads again only the values a change after their own
 * read touched and those of new members, and yields once. A change at or
 * below the revision a value was read at is already in that value, so it is
 * skipped; a followed event past the revision a new member was read at reads
 * it again, since its earlier changes may not have been sent. An unknown
 * change, sent after a gap the stream could not fill, reads the whole set
 * again at a new revision. A failed read or stream ends the iterator with
 * its error.
 */
export async function* watchKeyedSet<T>(
  engine: Engine,
  source: KeyedSetSource<T>,
  signal: AbortSignal,
): AsyncGenerator<KeyedSet<T>> {
  // members and readAt hold the set as last read. rows is copied before the
  // first change after a yield, so a yielded set never changes.
  let members: string[] = []
  let memberSet = new Set<string>()
  let rows = new Map<string, T>()
  let yielded = false
  const readAt = new Map<string, bigint>()
  let listedAt = 0n
  let seqno = 0n
  // unfollowed holds the members added since the last followed event.
  const unfollowed = new Set<string>()
  // listUnfollowed is set from a subscription that joined an open stream
  // until its first followed event: the stream passed the commits between
  // the listing and that event without the members' pattern.
  let listUnfollowed = false
  let sub: ChangeFeedSubscription | undefined
  let events: AsyncIterator<ChangeFeedEvent> | undefined

  // edit returns rows to change, copying it if it was yielded.
  function edit(): Map<string, T> {
    if (yielded) {
      rows = new Map(rows)
      yielded = false
    }
    return rows
  }

  // read reads the values of keys at tx, updating rows page by page, and
  // returns the revision it read. emit yields after each page.
  async function* read(
    tx: Tx,
    keys: readonly string[],
    emit: boolean,
  ): AsyncGenerator<KeyedSet<T>, bigint> {
    let at: bigint | undefined
    for await (const page of source.read(tx, keys, signal)) {
      at ??= page.seqno
      for (const [key, value] of page.rows) {
        if (value === undefined) edit().delete(key)
        else edit().set(key, value)
        if (at !== undefined) readAt.set(key, at)
      }
      if (emit && at !== undefined) {
        seqno = at
        yield snapshot(false)
      }
    }
    at ??= (await tx.getSeqno(signal)).seqno
    for (const key of keys) readAt.set(key, at)
    return at
  }

  // snapshot is the set as read so far.
  function snapshot(complete: boolean): KeyedSet<T> {
    yielded = true
    return { seqno, keys: members, rows, complete }
  }

  // relist replaces the members with keys, returning the added and removed.
  function relist(keys: string[]) {
    const next = new Set(keys)
    const added = keys.filter((key) => !memberSet.has(key))
    const removed = members.filter((key) => !next.has(key))
    for (const key of removed) {
      edit().delete(key)
      readAt.delete(key)
      unfollowed.delete(key)
    }
    members = keys
    memberSet = next
    return { added, removed }
  }

  // follow updates the names the feed follows for added and removed members.
  // A member read at at whose names are not yet followed waits in unfollowed
  // for the followed event, which says from which seqno they are.
  function follow(at: bigint, added: string[], removed: string[]) {
    if (!sub) {
      sub = engine.watchChanges(
        at,
        mergeNames(source.members, source.names(members)),
        signal,
      )
      events = sub[Symbol.asyncIterator]()
      if (sub.pending !== 0n) {
        for (const key of members) unfollowed.add(key)
        listUnfollowed = true
      }
      return
    }
    if (added.length === 0 && removed.length === 0) return
    sub.update(source.names(added), source.names(removed))
    for (const key of added) unfollowed.add(key)
  }

  // withTx runs body on one read transaction and discards it. body yields
  // only the pages of a first read; the set a wake ends with is yielded
  // after the discard, so a transaction never waits on the reader.
  async function* withTx<R>(
    body: (tx: Tx) => AsyncGenerator<KeyedSet<T>, R>,
  ): AsyncGenerator<KeyedSet<T>, R> {
    const tx = await engine.newTransaction(false, signal)
    try {
      return yield* body(tx)
    } finally {
      try {
        await tx.discard()
      } finally {
        tx.release()
      }
    }
  }

  try {
    let full = true
    for (;;) {
      // Read the whole set at one revision.
      if (full) {
        full = false
        yield* withTx(async function* (tx) {
          const { added, removed } = relist(await source.list(tx, signal))
          const at = yield* read(tx, members, true)
          seqno = listedAt = at
          follow(at, added, removed)
        })
        yield snapshot(true)
        continue
      }

      // Take the burst: the event that woke the reader and every one queued.
      const next = await events!.next()
      if (next.done) return
      const burst = [next.value, ...sub!.drain()]

      // Route each event to the values and the listing it may change. An
      // unknown change reads everything again, but the followed events
      // after it in the burst still settle what is followed.
      const dirty = new Set<string>()
      let list = false
      for (const event of burst) {
        if (event.case === 'followed') {
          for (const key of unfollowed) {
            if ((readAt.get(key) ?? 0n) < event.afterSeqno) dirty.add(key)
          }
          if (listUnfollowed && listedAt < event.afterSeqno) list = true
          if (sub!.pending === 0n) {
            unfollowed.clear()
            listUnfollowed = false
          }
          continue
        }
        const change = event.change
        if (change.unknown) full = true
        if (full) continue
        const at = change.seqno ?? 0n
        const route = source.route(change, memberSet)
        if (route.list && at > listedAt) list = true
        for (const key of route.keys) {
          if (memberSet.has(key) && at > (readAt.get(key) ?? 0n)) {
            dirty.add(key)
          }
        }
      }
      if (full || (!list && dirty.size === 0)) continue

      // Read what changed at one revision, then yield once.
      yield* withTx(async function* (tx) {
        let added: string[] = []
        let removed: string[] = []
        if (list) {
          ;({ added, removed } = relist(await source.list(tx, signal)))
          for (const key of added) dirty.add(key)
          for (const key of removed) dirty.delete(key)
        }
        const at = yield* read(tx, [...dirty], false)
        seqno = at
        if (list) listedAt = at
        follow(at, added, removed)
      })
      yield snapshot(true)
    }
  } finally {
    await events?.return?.()
    sub?.close()
  }
}

/**
 * readKeyedSet reads a keyed set once from reader, such as an immutable World
 * snapshot that has no change feed to follow.
 */
export async function readKeyedSet<T>(
  reader: KeyedSetReader,
  source: KeyedSetSource<T>,
  signal: AbortSignal,
): Promise<KeyedSet<T>> {
  const keys = await source.list(reader, signal)
  const rows = new Map<string, T>()
  let seqno: bigint | undefined
  for await (const page of source.read(reader, keys, signal)) {
    seqno ??= page.seqno
    for (const [key, value] of page.rows) {
      if (value !== undefined) rows.set(key, value)
    }
  }
  seqno ??= (await reader.getSeqno(signal)).seqno
  return { seqno, keys, rows, complete: true }
}

// EntitySetSpec names a set of World objects: those a link joins to an
// origin, or those of one type.
export type EntitySetSpec =
  | {
      readonly case: 'link'
      readonly origin: string
      readonly predicate: string
      readonly direction: 'out' | 'in'
    }
  | { readonly case: 'type'; readonly typeId: string }

// EntityDecoder decodes an object body into a row value. name identifies the
// decoder in the share key, so two decoders of one set never share a watch.
export interface EntityDecoder<T> {
  readonly name: string
  decode(body: Uint8Array): T
}

// EntityRow is a member's decoded body, or why its body could not be
// decoded. One undecodable member fails only its own row.
export type EntityRow<T> =
  | { readonly value: T; readonly error?: undefined }
  | { readonly value?: undefined; readonly error: string }

// messageDecoder decodes bodies as messages of type, each into its row.
export function messageDecoder<T extends Message<T>>(
  type: MessageType<T>,
): EntityDecoder<EntityRow<T>> {
  return {
    name: type.typeName,
    decode: (body) => {
      try {
        return { value: type.fromBinary(body) }
      } catch (err) {
        return { error: err instanceof Error ? err.message : String(err) }
      }
    },
  }
}

// entitySetLinkLimit bounds a link listing, which reads in one response.
const entitySetLinkLimit = 100_000

// entitySetKey names a spec in the share key.
export function entitySetKey(spec: EntitySetSpec): string {
  return spec.case === 'link'
    ? JSON.stringify(['link', spec.origin, spec.predicate, spec.direction])
    : JSON.stringify(['type', spec.typeId])
}

// entitySetPattern is the pattern of the quads that join a member to spec.
function entitySetPattern(spec: EntitySetSpec): ChangePattern {
  if (spec.case === 'type') {
    return changePattern({
      predicate: TypePred,
      obj: keyToIRI(buildTypeObjectKey(spec.typeId)),
    })
  }
  const origin = keyToIRI(spec.origin)
  return spec.direction === 'out'
    ? changePattern({ subject: origin, predicate: spec.predicate })
    : changePattern({ predicate: spec.predicate, obj: origin })
}

// matchesPattern reports whether quad matches pattern.
function matchesPattern(
  pattern: ChangePattern,
  quad: NonNullable<WorldChange['quads']>[number],
): boolean {
  return (
    (!pattern.subject || pattern.subject === quad.subject) &&
    (!pattern.predicate || pattern.predicate === quad.predicate) &&
    (!pattern.obj || pattern.obj === quad.obj) &&
    (!pattern.label || pattern.label === quad.label)
  )
}

// listEntitySet lists spec's members at tx's revision.
async function listEntitySet(
  tx: KeyedSetReader,
  spec: EntitySetSpec,
  signal: AbortSignal,
): Promise<string[]> {
  if (spec.case === 'type') {
    return tx.listObjectsWithType(spec.typeId, signal)
  }
  const resp = await tx.listGraphEdgeBuckets(
    [spec.origin],
    entitySetLinkLimit,
    {
      predicate: spec.predicate,
      direction:
        spec.direction === 'out'
          ? GraphEdgeBucketDirection.OUT
          : GraphEdgeBucketDirection.IN,
      abortSignal: signal,
    },
  )
  const bucket = resp.buckets?.[0]
  const truncated =
    spec.direction === 'out'
      ? bucket?.outgoingTruncated
      : bucket?.incomingTruncated
  if (truncated) {
    throw new Error(`More than ${entitySetLinkLimit} linked objects.`)
  }
  const iris =
    spec.direction === 'out'
      ? (bucket?.outgoing ?? []).map((q) => q.obj ?? '')
      : (bucket?.incoming ?? []).map((q) => q.subject ?? '')
  return [...new Set(iris.filter(Boolean).map(iriToKey))]
}

// entitySetSource is the keyed set source of spec's objects and their
// decoded bodies. A member whose object is missing has no row.
export function entitySetSource<T>(
  spec: EntitySetSpec,
  decoder: EntityDecoder<T>,
): KeyedSetSource<T> {
  const pattern = entitySetPattern(spec)
  return {
    members: { patterns: [pattern] },
    list: (tx, signal) => listEntitySet(tx, spec, signal),
    names: (keys) => ({ keys }),
    route: (change) => ({
      list: !!change.quads?.some((quad) => matchesPattern(pattern, quad)),
      keys: change.keys ?? [],
    }),
    read: async function* (tx, keys, signal) {
      if (keys.length === 0) return
      for await (const page of tx.getObjectBodyPages([...keys], signal)) {
        yield {
          seqno: page.worldSeqno,
          rows: (page.bodies ?? []).map(
            (body) =>
              [
                body.objectKey ?? '',
                body.exists
                  ? decoder.decode(body.body ?? new Uint8Array())
                  : undefined,
              ] as const,
          ),
        }
      }
    },
  }
}

/**
 * watchEntitySet follows spec's objects and their decoded bodies, sharing
 * one watch among every subscriber of the Engine with the same spec and
 * decoder. Every wake costs three calls when only bodies changed: the read
 * transaction, one body batch and its discard; a moved member adds one
 * listing.
 */
export function watchEntitySet<T>(
  engine: Engine,
  spec: EntitySetSpec,
  decoder: EntityDecoder<T>,
  signal?: AbortSignal,
): AsyncIterable<KeyedSet<T>> {
  return engine.watchShared(
    'entity-set:' + entitySetKey(spec) + ':' + decoder.name,
    (shared) => watchKeyedSet(engine, entitySetSource(spec, decoder), shared),
    signal,
  )
}
