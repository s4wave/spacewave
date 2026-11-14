import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import { WorldOperationRejection } from '@s4wave/sdk/world/errors.js'

import {
  Notebook,
  type NotebookEditOp,
  type NotebookSource,
} from './proto/notebook.pb.js'

// NOTEBOOK_EDIT_OP_ID is the World op that edits one Notebook's sources.
export const NOTEBOOK_EDIT_OP_ID = 'notes/notebook/edit'

// NOTEBOOK_CONFLICT_CODE rejects an edit made against sources the Notebook no
// longer holds, such as a Remove whose index now names another source.
export const NOTEBOOK_CONFLICT_CODE = 'notes/notebook/conflict'

// NOTEBOOK_ORDER_CODE rejects a Reorder whose order is not a permutation of
// the sources it carries.
export const NOTEBOOK_ORDER_CODE = 'notes/notebook/order'

// NOTEBOOK_SOURCE_CODE rejects an Add whose source has no ref.
export const NOTEBOOK_SOURCE_CODE = 'notes/notebook/source'

// NOTEBOOK_MISSING_CODE rejects an edit of a Notebook that does not exist.
export const NOTEBOOK_MISSING_CODE = 'notes/notebook/missing'

// NOTEBOOK_EMPTY_CODE rejects an op that carries no edit.
export const NOTEBOOK_EMPTY_CODE = 'notes/notebook/empty'

/**
 * applyNotebookEdit reads a Notebook, checks the edit against it and writes the
 * result, all through the World state the op was applied with, so the World
 * writer is held from the read to the write and no other edit lands between.
 * It makes six calls on that state: getObject, getRootRef, accessWorldState,
 * getBlock, putBlock and setRootRef; a refused edit stops after getBlock and
 * writes nothing. It calls nothing else while the writer is held. A refusal
 * throws a WorldOperationRejection carrying one of the codes above.
 */
export async function applyNotebookEdit(
  ws: IWorldState,
  op: NotebookEditOp,
  signal?: AbortSignal,
): Promise<void> {
  // Validate input that needs no read before touching the World.
  const apply = planEdit(op.edit)

  // Read the stored Notebook through one cursor at its root.
  using objectState = await ws.getObject(op.objectKey ?? '', signal)
  if (!objectState) {
    throw new WorldOperationRejection(
      NOTEBOOK_MISSING_CODE,
      'notebook not found',
    )
  }
  const { rootRef } = await objectState.getRootRef(signal)
  using cursor = await objectState.accessWorldState(rootRef, signal)
  const block = await cursor.getBlock({}, signal)
  if (!block.found || !block.data) {
    throw new WorldOperationRejection(
      NOTEBOOK_MISSING_CODE,
      'notebook block not found',
    )
  }
  const notebook = Notebook.fromBinary(block.data)

  // Apply the edit, refusing one made against sources no longer stored.
  const sources = apply(notebook.sources ?? [])

  // Write the new block in the object's bucket, then point the object at it.
  const written = await cursor.putBlock(
    {
      data: Notebook.toBinary({ ...notebook, sources }),
      opts: { refsKnown: true },
    },
    signal,
  )
  await objectState.setRootRef({ ...rootRef, rootRef: written.ref }, signal)
}

// planEdit validates an edit and returns the function that applies it to the
// stored sources, refusing with the conflict code when they no longer match
// what the caller saw.
function planEdit(
  edit: NotebookEditOp['edit'],
): (stored: NotebookSource[]) => NotebookSource[] {
  switch (edit?.case) {
    case 'add': {
      const ref = edit.value.source?.ref?.trim() ?? ''
      if (!ref) {
        throw new WorldOperationRejection(
          NOTEBOOK_SOURCE_CODE,
          'source ref is required',
        )
      }
      const source = { name: edit.value.source?.name?.trim() ?? '', ref }
      return (stored) => [...stored, source]
    }
    case 'remove': {
      const { index = 0, source } = edit.value
      return (stored) => {
        if (index >= stored.length || !sameSource(stored[index], source)) {
          throw changed()
        }
        return stored.filter((_, i) => i !== index)
      }
    }
    case 'reorder': {
      const { order = [], sources: seen = [] } = edit.value
      if (!isPermutation(order, seen.length)) {
        throw new WorldOperationRejection(
          NOTEBOOK_ORDER_CODE,
          'source order is not a permutation of the sources',
        )
      }
      return (stored) => {
        if (
          seen.length !== stored.length ||
          !seen.every((source, i) => sameSource(stored[i], source))
        ) {
          throw changed()
        }
        return order.map((i) => stored[i])
      }
    }
    default:
      throw new WorldOperationRejection(NOTEBOOK_EMPTY_CODE, 'no edit given')
  }
}

// changed is the rejection for an edit made against sources no longer stored.
function changed(): WorldOperationRejection {
  return new WorldOperationRejection(
    NOTEBOOK_CONFLICT_CODE,
    'the notebook changed',
  )
}

// sameSource reports whether two sources have the same name and ref.
function sameSource(a?: NotebookSource, b?: NotebookSource): boolean {
  return (
    (a?.name ?? '') === (b?.name ?? '') && (a?.ref ?? '') === (b?.ref ?? '')
  )
}

// isPermutation reports whether order lists each index below n exactly once.
function isPermutation(order: number[], n: number): boolean {
  if (order.length !== n) return false
  const seen = new Set<number>()
  for (const i of order) {
    if (i >= n || seen.has(i)) return false
    seen.add(i)
  }
  return true
}
