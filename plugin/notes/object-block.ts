import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import type { IObjectState } from '@s4wave/sdk/world/object-state.js'
import { accessObjectRootWorldState } from '@s4wave/sdk/world/utils.js'

/** readObjectBlockData reads the opaque Notes protobuf at the object root, or null when it is absent. */
export async function readObjectBlockData(
  objectState: IObjectState,
  signal?: AbortSignal,
): Promise<Uint8Array | null> {
  using cursor = await accessObjectRootWorldState(objectState, signal)
  const block = await cursor.getBlock({}, signal)
  return block.found && block.data ? block.data : null
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
  const [current, written] = await Promise.all([
    cursor.getRef(signal),
    cursor.putBlock({ data, opts: { refsKnown: true } }, signal),
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
