import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import type { IObjectState } from '@s4wave/sdk/world/object-state.js'

/**
 * setObjectBlockData replaces an opaque Notes protobuf through its existing bucket.
 * These blocks contain no nested block references and need no mutable block tree.
 * The supplied World transaction owns acceptance of the new object root.
 */
export async function setObjectBlockData(
  objectState: IObjectState,
  data: Uint8Array,
  signal?: AbortSignal,
): Promise<void> {
  // Write bytes using the object's existing bucket and transform configuration.
  const current = await objectState.getRootRef(signal)
  using cursor = await objectState.accessWorldState(current.rootRef, signal)
  const written = await cursor.putBlock({ data }, signal)

  // Publish the reference only after the block is available in storage.
  await objectState.setRootRef(
    { ...current.rootRef, rootRef: written.ref },
    signal,
  )
}

/** createObjectWithBlockData creates an opaque Notes block in the supplied World transaction. */
export async function createObjectWithBlockData(
  worldState: IWorldState,
  objectKey: string,
  data: Uint8Array,
  signal?: AbortSignal,
): Promise<void> {
  // Write the canonical protobuf into this World's storage.
  using cursor = await worldState.buildStorageCursor(signal)
  const [current, written] = await Promise.all([
    cursor.getRef(signal),
    cursor.putBlock({ data }, signal),
  ])

  // The created object handle is local to this call; the World owns its root.
  using _object = await worldState.createObject(
    objectKey,
    {
      ...current.ref,
      rootRef: written.ref,
    },
    signal,
  )
}
