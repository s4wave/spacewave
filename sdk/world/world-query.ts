import { Engine } from './engine.js'
import type { IWorldState } from './world-state.js'

/**
 * WorldQuery reads one bounded answer from an immutable World snapshot. signal
 * aborts the read when its watch ends.
 */
export type WorldQuery<T> = (
  world: IWorldState,
  signal: AbortSignal,
) => Promise<T>

/** WorldQuerySnapshot binds a query result to the World revision it read. */
export interface WorldQuerySnapshot<T> {
  readonly value: T
  readonly seqno: bigint
}

/**
 * watchWorldQuery runs a query on the World head, then again after each commit
 * that changes what the previous run read. Every run reads one read snapshot,
 * so its result is consistent, and the server records the snapshot's reads to
 * decide when to run again. The iterator retains its own Engine reference and
 * releases every snapshot before yielding.
 *
 * Watches of one owner with the same key share one run of the query. The key
 * must identify everything the query reads. Without a key, the watch runs
 * alone.
 */
export function watchWorldQuery<T>(
  owner: Engine,
  query: WorldQuery<T>,
  signal?: AbortSignal,
  key?: string,
): AsyncIterable<WorldQuerySnapshot<T>> {
  if (key === undefined) {
    return runWorldQuery(owner, query, signal)
  }
  return owner.watchShared(
    'world-query:' + key,
    (shared) => runWorldQuery(owner, query, shared),
    signal,
  )
}

// runWorldQuery runs one watch of query on its own Engine reference. A watch
// without a signal reads with a signal of its own that aborts when the watch
// ends, so no signal outlives the watch or is shared between watches.
async function* runWorldQuery<T>(
  owner: Engine,
  query: WorldQuery<T>,
  signal?: AbortSignal,
): AsyncIterable<WorldQuerySnapshot<T>> {
  const own = new AbortController()
  const read = signal ?? own.signal
  try {
    using engine = new Engine(owner.resourceRef.createRef(owner.id))
    for await (const world of engine.watchWorldState(read)) {
      // Pin the result to the snapshot's revision, then release the snapshot.
      let snapshot: WorldQuerySnapshot<T>
      try {
        const { seqno } = await world.getSeqno(read)
        snapshot = { seqno, value: await query(world, read) }
      } finally {
        world[Symbol.dispose]()
      }

      read.throwIfAborted()
      yield snapshot
    }
  } finally {
    own.abort()
  }
}
