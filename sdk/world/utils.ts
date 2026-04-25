import { compareUint8Arrays } from '@aptre/bldr'
import { ObjectRef } from '@go/github.com/s4wave/spacewave/db/bucket/bucket.pb.js'
import { BucketLookupCursor } from '../bucket/lookup/lookup.js'
import { BlockCursor } from '../block/cursor/cursor.js'
import { EngineWorldState } from './engine-state.js'
import { type IObjectState } from './object-state.js'

// AccessObjectCallback is called with a block cursor to modify object data.
// The block transaction will be written automatically after the callback completes.
export type AccessObjectCallback = (cursor: BlockCursor) => Promise<void>

// accessObject accesses or creates an ObjectRef using the provided callback.
// If ref is undefined or has an empty rootRef, creates a new empty object.
// The block transaction is written upon completion and the updated ObjectRef is returned.
export async function accessObject(
  worldState: BucketLookupCursor,
  ref: ObjectRef | undefined,
  cb: AccessObjectCallback,
  abortSignal?: AbortSignal,
): Promise<ObjectRef> {
  const { transaction, cursor } = await worldState.buildTransaction(
    {},
    abortSignal,
  )

  // If ref is empty, mark the cursor position as empty
  if (!ref || !ref.rootRef || !ref.rootRef.hash) {
    await cursor.markDirty(abortSignal)
  }

  // Execute the callback to modify the block
  await cb(cursor)

  // Write the transaction and get the updated root ref
  const refResp = await worldState.getRef(abortSignal)
  const writeResp = await transaction.write({ clearTree: true }, abortSignal)

  return {
    bucketId: refResp.ref?.bucketId ?? '',
    rootRef: writeResp.rootRef,
  }
}

// createWorldObject creates a new object in the world with the given key.
// Returns the created ObjectState and the resulting ObjectRef.
export async function createWorldObject(
  world: EngineWorldState,
  worldState: BucketLookupCursor,
  key: string,
  cb: AccessObjectCallback,
  abortSignal?: AbortSignal,
): Promise<{ objectState: IObjectState; objectRef: ObjectRef }> {
  // Check if object already exists
  const existing = await world.getObject(key, abortSignal)
  if (existing) {
    throw new Error(`Object already exists: ${key}`)
  }

  // Create the object data
  const objectRef = await accessObject(worldState, undefined, cb, abortSignal)

  // Create the object in the world
  const objectState = await world.createObject(key, objectRef, abortSignal)

  return { objectState, objectRef }
}

// accessWorldObject attempts to look up an object in the world state.
// If the object does not exist, the cursor will be empty and the object will be created.
// If updateWorld is true and the result is different, will update the object's root ref.
// Returns the modified object ref and a dirty flag indicating if changes were made.
export async function accessWorldObject(
  world: EngineWorldState,
  worldState: BucketLookupCursor,
  key: string,
  updateWorld: boolean,
  cb: AccessObjectCallback,
  abortSignal?: AbortSignal,
): Promise<{ objectRef: ObjectRef; dirty: boolean }> {
  const obj = await world.getObject(key, abortSignal)

  // Create object from scratch if it doesn't exist
  if (!obj) {
    const initRef = await accessObject(worldState, undefined, cb, abortSignal)
    if (updateWorld) {
      await world.createObject(key, initRef, abortSignal)
    }
    return { objectRef: initRef, dirty: true }
  }

  return await accessObjectState(obj, updateWorld, cb, abortSignal)
}

// accessObjectState accesses and updates a world object handle if updateWorld is set.
// If updateWorld is true and the result is different, will update the object's root ref.
// Returns the modified object ref and a dirty flag indicating if changes were made.
export async function accessObjectState(
  obj: IObjectState,
  updateWorld: boolean,
  cb: AccessObjectCallback,
  abortSignal?: AbortSignal,
): Promise<{ objectRef: ObjectRef; dirty: boolean }> {
  const initRefResp = await obj.getRootRef(abortSignal)
  const initRef = initRefResp.rootRef

  if (!initRef) {
    throw new Error('Object has no root ref')
  }

  using worldStateResource = await obj.accessWorldState(initRef, abortSignal)
  const outRef = await accessObject(
    worldStateResource,
    initRef,
    cb,
    abortSignal,
  )

  // Check if the ref changed
  let dirty = false
  if (initRef.bucketId && initRef.bucketId !== outRef.bucketId) {
    dirty = true
  }
  const initHash = initRef.rootRef?.hash
  const outHash = outRef.rootRef?.hash
  if (
    initHash?.hashType !== outHash?.hashType ||
    !compareUint8Arrays(initHash?.hash, outHash?.hash)
  ) {
    dirty = true
  }

  if (updateWorld && dirty) {
    await obj.setRootRef(outRef, abortSignal)
  }

  return { objectRef: outRef, dirty }
}
