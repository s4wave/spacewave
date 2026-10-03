import { KeyValueStore, KVImplType } from '../../../db/kvtx/block/kvtx.pb.js'
import type { WorldStateResource } from '../../world/world-state.js'
import { getObjectType, setObjectType } from '../../world/types/types.js'
import { KvStore, KvStoreTypeID } from '../kv.js'

export class KvObjectTypeError extends Error {
  constructor() {
    super('The existing object has a different ObjectType')
    this.name = 'KvObjectTypeError'
  }
}

// openWorldKvStore binds the typed store to this state, including a supplied Tx.
// Creation changes only this state and never replaces an existing ObjectType.
export async function openWorldKvStore(
  state: WorldStateResource,
  key: string,
  create: boolean,
  signal?: AbortSignal,
): Promise<KvStore | undefined> {
  const object = await state.getObject(key, signal)
  if (object) {
    object.release()
    if ((await getObjectType(state, key, signal)) !== KvStoreTypeID)
      throw new KvObjectTypeError()
  } else {
    if (!create) return undefined
    using stage = await state.stageWorldState(signal)
    using cursor = await stage.buildStorageCursor(signal)
    const root = await cursor.putBlock(
      {
        data: KeyValueStore.toBinary({
          implType: KVImplType.KV_IMPL_TYPE_IAVL,
        }),
      },
      signal,
    )
    const storage = await cursor.getRef(signal)
    if (!root.ref) throw new Error('kv/store: missing initial root')
    const created = await state.createObject(
      key,
      { ...storage.ref, rootRef: root.ref },
      signal,
    )
    created.release()
    await setObjectType(state, key, KvStoreTypeID, signal)
  }
  const access = await state.accessTypedObject(key, signal)
  const store = state.resourceRef.createResource(access.resourceId, KvStore)
  if (access.typeId !== KvStoreTypeID) {
    store.release()
    throw new KvObjectTypeError()
  }
  return store
}
