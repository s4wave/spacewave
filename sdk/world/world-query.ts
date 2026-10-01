import { Engine } from './engine.js'
import type { IWorldState } from './world-state.js'

/** WorldQuery reads one bounded answer from an immutable World snapshot. */
export type WorldQuery<T> = (
  world: IWorldState,
  signal?: AbortSignal,
) => Promise<T>

/** WorldQuerySnapshot binds a query result to the World revision it read. */
export interface WorldQuerySnapshot<T> {
  readonly value: T
  readonly seqno: bigint
}

/**
 * watchWorldQuery runs a query on each World revision. Every run reads one
 * read transaction, so its result is consistent, and the next run waits on
 * the live Engine for the revision after the one it read. A commit during a
 * run satisfies that wait immediately. The iterator retains its own Engine
 * reference and releases every transaction before yielding or waiting.
 */
export async function* watchWorldQuery<T>(
  owner: Engine,
  query: WorldQuery<T>,
  signal?: AbortSignal,
): AsyncIterable<WorldQuerySnapshot<T>> {
  using engine = new Engine(owner.resourceRef.createRef(owner.id))
  for (;;) {
    // Pin the result and its wait position to one revision.
    signal?.throwIfAborted()
    // eslint-disable-next-line react-doctor/async-await-in-loop -- Each iteration releases its snapshot before waiting for the next revision.
    const tx = await engine.newTransaction(false, signal)
    let snapshot: WorldQuerySnapshot<T>
    try {
      const { seqno } = await tx.getSeqno(signal)
      snapshot = { seqno, value: await query(tx, signal) }
    } finally {
      // Cancellation must not prevent the pinned transaction from being discarded.
      try {
        await tx.discard()
      } finally {
        tx.release()
      }
    }

    signal?.throwIfAborted()
    yield snapshot
    // eslint-disable-next-line react-doctor/async-await-in-loop -- This is the live Engine wait, never a wait on the immutable read transaction.
    await engine.waitSeqno(snapshot.seqno + 1n, signal)
  }
}
