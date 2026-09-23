import { ObjectRef } from '../../db/bucket/bucket.pb.js'
import { Engine } from '../world/engine.js'
import type { IObjectState } from '../world/object-state.js'

/** ObjectQuery reads a typed projection from one immutable object snapshot. */
export type ObjectQuery<T> = (
  object: IObjectState,
  signal?: AbortSignal,
) => Promise<T>

/** ObjectQuerySnapshot binds a projection, or an absent object, to its World revision. */
export interface ObjectQuerySnapshot<T> {
  readonly value: T | null
  readonly seqno: bigint
}

/**
 * watchObjectQuery projects an existing World object without copying its storage.
 * The root and wait position come from the same read transaction. Unchanged roots
 * skip the callback, including unrelated World edits. The iterator independently
 * retains the Engine and releases every snapshot before yielding or waiting.
 */
export async function* watchObjectQuery<T>(
  owner: Engine,
  objectKey: string,
  query: ObjectQuery<T>,
  signal?: AbortSignal,
): AsyncIterable<ObjectQuerySnapshot<T>> {
  using engine = new Engine(owner.resourceRef.createRef(owner.id))
  let previous: string | undefined
  for (;;) {
    // Pin the value and its wait position before the live World can advance.
    signal?.throwIfAborted()
    // eslint-disable-next-line react-doctor/async-await-in-loop -- Each iteration releases its snapshot before waiting for the next revision.
    const tx = await engine.newTransaction(false, signal)
    let seqno: bigint
    let next: ObjectQuerySnapshot<T> | undefined
    try {
      seqno = (await tx.getSeqno(signal)).seqno
      using object = await tx.getObject(objectKey, signal)
      const root = object ? (await object.getRootRef(signal)).rootRef : null
      const signature = !object
        ? 'missing'
        : root
          ? Array.from(ObjectRef.toBinary(root)).join(',')
          : 'empty'
      if (signature !== previous) {
        next = { seqno, value: object ? await query(object, signal) : null }
        previous = signature
      }
    } finally {
      // Cancellation must not prevent the pinned transaction from being discarded.
      try {
        await tx.discard()
      } finally {
        tx.release()
      }
    }

    // A commit during evaluation or delivery satisfies this wait immediately.
    signal?.throwIfAborted()
    if (next) yield next
    // eslint-disable-next-line react-doctor/async-await-in-loop -- This is the live Engine wait, never a wait on the immutable read transaction.
    await engine.waitSeqno(seqno + 1n, signal)
  }
}
