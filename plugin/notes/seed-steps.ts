import type { ObjectRef } from '@go/github.com/s4wave/spacewave/db/bucket/bucket.pb.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import { WorldOperationRejection } from '@s4wave/sdk/world/errors.js'
import { getObjectType, setObjectType } from '@s4wave/sdk/world/types/types.js'
import { FSNodeTypeID } from '@go/github.com/s4wave/spacewave/db/unixfs/block/fstree_typeid.pb.js'
import { InitUnixFSOp } from '@s4wave/core/space/world/ops/ops.pb.js'
import { INIT_UNIXFS_OP_ID } from '@s4wave/core/space/world/ops/init-unixfs.js'
import { createObjectWithBlockData } from './object-block.js'
import { uploadSeedTree, type SeedFile } from './unixfs-seed.js'

// SEED_KEY_TAKEN_CODE rejects a seed that finds a key it would write already
// holding an object it cannot keep.
export const SEED_KEY_TAKEN_CODE = 'notes/seed/key-taken'

// SEED_UNSTAGED_CODE rejects a staged seed op missing the staged root of an
// object it writes.
export const SEED_UNSTAGED_CODE = 'notes/seed/unstaged'

// SeedObject is one object a seed op writes: its key, the type it gives the
// object, and the step that writes it. keepFound marks a companion that a
// seed with a free primary keeps as it is, whatever its type, when its key is
// taken, instead of rejecting the op.
export interface SeedObject {
  key: string
  typeId: string
  label: string
  keepFound?: boolean
  write(worldState: IWorldState, abortSignal?: AbortSignal): Promise<void>
}

// applySeed writes a seed made of a primary object, such as a notebook, and
// the companions it reads, such as its UnixFS root. It decides once, from the
// primary, before writing anything:
//
//   - primary present with its type: the seed ran before, so every companion
//     must be present with its type, and all are kept as they are;
//   - primary absent: every companion key must be free, so a new primary never
//     adopts an object left by an earlier one, such as the files of a deleted
//     notebook; a keepFound companion whose key is taken is kept instead;
//   - primary present with another type, or with none: rejection.
//
// A rejection names the key and the next step, and the World discards the
// op, so the seed never reports an object it did not make. The primary and
// its companions are written in one World op, after every check, so a primary
// with its type is the whole result of an earlier seed.
export async function applySeed(
  worldState: IWorldState,
  primary: SeedObject,
  companions: SeedObject[],
  abortSignal?: AbortSignal,
): Promise<void> {
  const [primaryType, ...companionTypes] = await Promise.all(
    [primary, ...companions].map((object) =>
      readObjectType(worldState, object.key, abortSignal),
    ),
  )
  if (primaryType === primary.typeId) {
    const i = companionTypes.findIndex(
      (found, i) => found !== companions[i].typeId,
    )
    if (i === -1) return
    const found = companionTypes[i]
    throw keyTaken(
      `The ${primary.typeId} ${primary.key} already exists, but its ` +
        `${companions[i].typeId} ${companions[i].key} ` +
        (found === null ? 'is missing' : describeFound(found)) +
        `. Rename or delete ${primary.key}, then try again.`,
    )
  }
  if (primaryType !== null) {
    throw keyTaken(
      `The object ${primary.key} already exists ${describeFound(primaryType)}` +
        `, so a ${primary.typeId} cannot be created there. ` +
        'Rename or delete that object, then try again.',
    )
  }
  const taken = companionTypes.findIndex(
    (found, i) => found !== null && !companions[i].keepFound,
  )
  if (taken !== -1) {
    const key = companions[taken].key
    throw keyTaken(
      `The object ${key} was left by an earlier object, so the new ` +
        `${primary.typeId} ${primary.key} cannot use it. ` +
        `Delete or rename ${key}, then try again.`,
    )
  }
  const writes = companions.filter((_, i) => companionTypes[i] === null)
  for (const object of [...writes, primary]) {
    // eslint-disable-next-line react-doctor/async-await-in-loop -- the writes share one World transaction and run in order.
    await runSeedStep(object.label, () => object.write(worldState, abortSignal))
  }
}

// seedUnixfsRoot is the seed step that creates the UnixFS root at key
// holding files.
export function seedUnixfsRoot(
  label: string,
  key: string,
  files: SeedFile[],
  timestamp: Date,
): SeedObject {
  return {
    key,
    typeId: FSNodeTypeID,
    label,
    async write(worldState, abortSignal) {
      await worldState.applyWorldOp(
        INIT_UNIXFS_OP_ID,
        InitUnixFSOp.toBinary({ objectKey: key, timestamp }),
        '',
        abortSignal,
      )
      await uploadSeedTree(worldState, key, files, undefined, abortSignal)
    },
  }
}

// seedTypedObject is the seed step that creates the object at key with data
// as its root block and typeId as its type.
export function seedTypedObject(
  label: string,
  key: string,
  data: Uint8Array,
  typeId: string,
): SeedObject {
  return {
    key,
    typeId,
    label,
    async write(worldState, abortSignal) {
      await createObjectWithBlockData(worldState, key, data, abortSignal)
      await setObjectType(worldState, key, typeId, abortSignal)
    },
  }
}

// seedStagedObject is the seed step that creates the object at key with
// root, content its sender built in a World stage, and typeId as its type.
// Under the World writer it only adopts the root. A missing root rejects the
// op, since the op cannot build content itself.
export function seedStagedObject(
  label: string,
  key: string,
  typeId: string,
  root: ObjectRef | undefined,
): SeedObject {
  if (!root?.rootRef) {
    throw new WorldOperationRejection(
      SEED_UNSTAGED_CODE,
      `The ${typeId} ${key} has no staged content, so it cannot be created.`,
    )
  }
  return {
    key,
    typeId,
    label,
    async write(worldState, abortSignal) {
      // The created object handle is local to this call; the World owns its
      // root.
      using _created = await worldState.createObject(key, root, abortSignal)
      await setObjectType(worldState, key, typeId, abortSignal)
    },
  }
}

// readObjectType returns the type of the object at key, '' for an object with
// no type, or null when the key is free.
async function readObjectType(
  worldState: IWorldState,
  key: string,
  abortSignal?: AbortSignal,
): Promise<string | null> {
  const object = await worldState.getObject(key, abortSignal)
  if (!object) return null
  object.release()
  return getObjectType(worldState, key, abortSignal)
}

// describeFound words the type found at a key for a rejection.
function describeFound(found: string): string {
  return found ? `as a ${found}` : 'with no type'
}

function keyTaken(message: string): WorldOperationRejection {
  return new WorldOperationRejection(SEED_KEY_TAKEN_CODE, message)
}

// runSeedStep runs one seed step, naming the step in its error and keeping
// the original error as the cause.
async function runSeedStep(
  label: string,
  cb: () => Promise<void>,
): Promise<void> {
  try {
    await cb()
  } catch (err) {
    throw new Error(
      label + ': ' + (err instanceof Error ? err.message : String(err)),
      { cause: err },
    )
  }
}
