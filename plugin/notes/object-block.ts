import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import type { IObjectState } from '@s4wave/sdk/world/object-state.js'
import { accessObjectRootWorldState } from '@s4wave/sdk/world/utils.js'
import type { BucketLookupCursor } from '@s4wave/sdk/bucket/lookup/lookup.js'
import type { ObjectRef } from '@go/github.com/s4wave/spacewave/db/bucket/bucket.pb.js'

/** readObjectBlockData reads the opaque Notes protobuf at the object root, or null when it is absent. */
export async function readObjectBlockData(
  objectState: IObjectState,
  signal?: AbortSignal,
): Promise<Uint8Array | null> {
  using cursor = await accessObjectRootWorldState(objectState, signal)
  const block = await cursor.getBlock({}, signal)
  return block.found && block.data ? block.data : null
}

/** stageObjectBlock writes data as a block through a staged cursor and returns the object root naming it. */
export async function stageObjectBlock(
  cursor: BucketLookupCursor,
  data: Uint8Array,
  signal?: AbortSignal,
): Promise<ObjectRef> {
  const [current, written] = await Promise.all([
    cursor.getRef(signal),
    cursor.putBlock({ data, opts: { refsKnown: true } }, signal),
  ])
  return { ...current.ref, rootRef: written.ref }
}

/** createObjectWithBlockData creates an opaque Notes block in the supplied World transaction. */
export async function createObjectWithBlockData(
  worldState: IWorldState,
  objectKey: string,
  data: Uint8Array,
  signal?: AbortSignal,
): Promise<void> {
  // Write the canonical protobuf through a stage held until the object exists.
  using stage = await worldState.stageWorldState(signal)
  using cursor = await stage.buildStorageCursor(signal)
  const root = await stageObjectBlock(cursor, data, signal)

  // The created object handle is local to this call; the World owns its root.
  using _object = await worldState.createObject(objectKey, root, signal)
}
