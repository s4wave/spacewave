import type { BlockRef } from '@go/github.com/s4wave/spacewave/db/block/block.pb.js'
import type { ObjectRef } from '@go/github.com/s4wave/spacewave/db/bucket/bucket.pb.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import type { IObjectState } from '@s4wave/sdk/world/object-state.js'

function buildObjectRootRef(
  currentRef: ObjectRef | undefined,
  rootRef: BlockRef | undefined,
): ObjectRef {
  if (!rootRef) {
    throw new Error('failed to write object root ref')
  }
  return {
    bucketId: currentRef?.bucketId ?? '',
    rootRef,
    transformConf: currentRef?.transformConf,
  }
}

async function runObjectBlockStep<T>(
  label: string,
  cb: () => Promise<T>,
): Promise<T> {
  try {
    return await cb()
  } catch (err) {
    throw new Error(
      label + ': ' + (err instanceof Error ? err.message : String(err)),
    )
  }
}

export async function setObjectBlockData(
  objectState: IObjectState,
  data: Uint8Array,
  abortSignal?: AbortSignal,
): Promise<void> {
  using cursor = await objectState.accessWorldState(undefined, abortSignal)
  const { transaction, cursor: blockCursor } = await cursor.buildTransaction(
    {},
    abortSignal,
  )
  try {
    await runObjectBlockStep('mark existing object root dirty', async () => {
      await blockCursor.markDirty(abortSignal)
    })
    await runObjectBlockStep('write existing object block data', async () => {
      await blockCursor.setBlock(
        {
          data,
          markDirty: true,
        },
        abortSignal,
      )
    })
    const currentRef = await runObjectBlockStep(
      'capture existing object ref',
      async () => (await cursor.getRef(abortSignal)).ref,
    )
    const writeResp = await runObjectBlockStep(
      'commit existing object block',
      async () => transaction.write({ clearTree: true }, abortSignal),
    )
    await runObjectBlockStep('update existing object root ref', async () => {
      await objectState.setRootRef(
        buildObjectRootRef(currentRef, writeResp.rootRef),
        abortSignal,
      )
    })
  } finally {
    blockCursor.release()
    transaction.release()
  }
}

export async function createObjectWithBlockData(
  worldState: IWorldState,
  objectKey: string,
  data: Uint8Array,
  abortSignal?: AbortSignal,
): Promise<void> {
  using cursor = await worldState.buildStorageCursor(abortSignal)
  const putResp = await runObjectBlockStep('put new object block', async () => {
    return cursor.putBlock({ data }, abortSignal)
  })
  const currentRef = await runObjectBlockStep(
    'capture storage cursor ref',
    async () => (await cursor.getRef(abortSignal)).ref,
  )
  await runObjectBlockStep('create world object', async () => {
    await worldState.createObject(
      objectKey,
      buildObjectRootRef(currentRef, putResp.ref),
      abortSignal,
    )
  })
}
