import type { ObjectRef } from '@go/github.com/s4wave/spacewave/db/bucket/bucket.pb.js'
import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'

import { Engine } from './engine.js'
import { type IObjectState } from './object-state.js'
import { watchWorldQuery } from './world-query.js'
import {
  Tx,
  type IWorldState,
  type ListGraphEdgeBucketsOptions,
  type ListObjectsWithTypePage,
  type RenameObjectOptions,
  type TypedObjectAccess,
} from './world-state.js'
import { ObjectIterator } from './object_iterator.js'
import { BucketLookupCursor } from '../bucket/lookup/lookup.js'
import type { WorldStage } from './stage.js'
import { throwBatchError } from './errors.js'
import {
  WorldErrorCode,
  type ExecuteWorldBatchResponse,
  type WorldMutation,
  type GetRootRefResponse,
  type ListGraphEdgeBucketsResponse,
  type ListObjectsRequest,
  type ListObjectsResponse,
  type ListObjectsWithTypeResponse,
  type ObjectBody,
  type SearchObjectsWithTypeResponse,
  type ObjectMetadata,
  type SetRootRefResponse,
  type ApplyObjectOpResponse,
  type IncrementRevResponse,
  type WaitRevResponse,
  type TypeCountEntry,
  type WatchTypeCountsResponse,
} from './world.pb.js'

// runReadOp performs a read with a short-lived read transaction on the engine
// owned by state.
async function runReadOp<T>(
  state: EngineWorldState,
  abortSignal: AbortSignal | undefined,
  cb: (tx: Tx) => Promise<T>,
): Promise<T> {
  const tx = await state.getEngine().newTransaction(false, abortSignal)
  try {
    return await cb(tx)
  } finally {
    // Cleanup outlives caller cancellation and releases the pinned snapshot.
    try {
      await tx.discard()
    } finally {
      tx.release()
    }
  }
}

// sendMutation applies one mutation as an ExecuteWorldBatch of its own, whose
// write transaction lives only inside the call, and returns the response,
// refused or not.
async function sendMutation(
  state: EngineWorldState,
  mutation: WorldMutation['mutation'],
  abortSignal: AbortSignal | undefined,
): Promise<ExecuteWorldBatchResponse> {
  if (!state.canWrite()) {
    throw new Error('EngineWorldState is read-only')
  }
  return state.getEngine().executeWorldBatch([{ mutation }], abortSignal)
}

// applyMutation applies one mutation and throws the refusal of a refused one.
async function applyMutation(
  state: EngineWorldState,
  mutation: WorldMutation['mutation'],
  abortSignal: AbortSignal | undefined,
): Promise<ExecuteWorldBatchResponse> {
  const response = await sendMutation(state, mutation, abortSignal)
  throwBatchError(response)
  return response
}

// applyObjectMutation applies one object mutation and returns the object's
// revision after it. An applied reply without the revision, as from a host
// that predates batch results, throws rather than report revision zero, which
// a caller waiting on the revision would take as already reached.
async function applyObjectMutation(
  state: EngineWorldState,
  mutation: WorldMutation['mutation'],
  abortSignal: AbortSignal | undefined,
): Promise<{ rev: bigint }> {
  const response = await applyMutation(state, mutation, abortSignal)
  const rev = response.results?.[0]?.rev
  if (rev === undefined) {
    throw new Error('ExecuteWorldBatch reply carries no object revision')
  }
  return { rev }
}

// EngineWorldState implements IWorldState on top of an Engine. Each write is
// one ExecuteWorldBatch round trip, and each read a short-lived read
// transaction. No write transaction is held across a round trip.
export class EngineWorldState implements IWorldState {
  private engine: Engine
  private write: boolean
  private ownsEngine: boolean

  constructor(engine: Engine, write: boolean, ownsEngine = false) {
    this.engine = engine
    this.write = write
    this.ownsEngine = ownsEngine
  }

  // getResourceRef returns the resource ref for creating child resources.
  public getResourceRef(): ClientResourceRef {
    return this.engine.resourceRef
  }

  // getReadOnly returns if the state is read-only.
  public getReadOnly(): boolean {
    return !this.write
  }

  // getSeqno returns the current seqno of the world state.
  public async getSeqno(abortSignal?: AbortSignal): Promise<{ seqno: bigint }> {
    const response = await this.engine.getSeqno(abortSignal)
    return { seqno: response.seqno ?? 0n }
  }

  // waitSeqno waits for the seqno of the world state to be >= value.
  public async waitSeqno(
    seqno: bigint,
    abortSignal?: AbortSignal,
  ): Promise<{ seqno: bigint }> {
    const response = await this.engine.waitSeqno(seqno, abortSignal)
    return { seqno: response.seqno ?? 0n }
  }

  // buildStorageCursor builds a cursor to the world storage with an empty ref.
  public async buildStorageCursor(
    abortSignal?: AbortSignal,
  ): Promise<BucketLookupCursor> {
    return await this.engine.buildStorageCursor(abortSignal)
  }

  // accessWorldState builds a bucket lookup cursor with an optional ref.
  public async accessWorldState(
    ref?: ObjectRef,
    abortSignal?: AbortSignal,
  ): Promise<BucketLookupCursor> {
    return this.engine.accessWorldState(ref, abortSignal)
  }

  // stageWorldState opens a staging scope on the engine.
  public async stageWorldState(abortSignal?: AbortSignal): Promise<WorldStage> {
    if (!this.write) {
      throw new Error('EngineWorldState is read-only')
    }
    return this.engine.stageWorldState(abortSignal)
  }

  /**
   * openNestedWorld opens a read-only nested World that follows a typed outer
   * object.
   */
  public async openNestedWorld(
    objectKey: string,
    abortSignal?: AbortSignal,
  ): Promise<IWorldState & Disposable> {
    return this.performRead(abortSignal, (tx) =>
      tx.openNestedWorld(objectKey, abortSignal),
    )
  }

  /**
   * openOuterWorld rejects a top-level state through its short-lived
   * transaction.
   */
  public async openOuterWorld(
    abortSignal?: AbortSignal,
  ): Promise<IWorldState & Disposable> {
    return this.performRead(abortSignal, (tx) => tx.openOuterWorld(abortSignal))
  }

  // applyWorldOp applies a batch operation at the world level.
  public async applyWorldOp(
    opTypeId: string,
    opData: Uint8Array,
    opSender: string,
    abortSignal?: AbortSignal,
  ): Promise<{ seqno: bigint; sysErr: boolean }> {
    // A system error throws, so an applied op never reports sysErr.
    const response = await applyMutation(
      this,
      { case: 'op', value: { opTypeId, opData, opSender } },
      abortSignal,
    )
    return { seqno: response.seqno ?? 0n, sysErr: false }
  }

  // createObject creates an object with a key and initial root ref.
  public async createObject(
    key: string,
    rootRef: ObjectRef,
    abortSignal?: AbortSignal,
  ): Promise<IObjectState> {
    await applyMutation(
      this,
      { case: 'createObject', value: { objectKey: key, rootRef } },
      abortSignal,
    )
    return this.bindObject(key)
  }

  // getObject looks up an object by key.
  public async getObject(
    key: string,
    abortSignal?: AbortSignal,
  ): Promise<IObjectState | null> {
    const found = await this.performRead(abortSignal, async (tx) => {
      using obj = await tx.getObject(key, abortSignal)
      return obj !== null
    })

    if (!found) {
      return null
    }

    return this.bindObject(key)
  }

  // bindObject returns a handle to the object at key on the live World
  // without checking that it exists. The handle holds no snapshot: each call
  // reads the current revision and fails if the object is absent there.
  public bindObject(key: string): IObjectState {
    return new EngineWorldStateObject(this, key)
  }

  // iterateObjects returns an iterator with the given object key prefix.
  public async iterateObjects(
    prefix?: string,
    reversed?: boolean,
    abortSignal?: AbortSignal,
  ): Promise<ObjectIterator> {
    const tx = await this.engine.newTransaction(false, abortSignal)
    try {
      const iterator = await tx.iterateObjects(prefix, reversed, abortSignal)
      return new TxOwnedObjectIterator(iterator, tx)
    } catch (error) {
      tx.release()
      throw error
    }
  }

  // renameObject renames an object key and associated graph quads.
  public async renameObject(
    oldKey: string,
    newKey: string,
    options?: RenameObjectOptions,
  ): Promise<IObjectState> {
    await applyMutation(
      this,
      {
        case: 'renameObject',
        value: {
          oldObjectKey: oldKey,
          newObjectKey: newKey,
          descendants: options?.descendants,
        },
      },
      options?.abortSignal,
    )
    return this.bindObject(newKey)
  }

  // deleteObject deletes an object and associated graph quads by ID.
  public async deleteObject(
    key: string,
    abortSignal?: AbortSignal,
  ): Promise<{ deleted: boolean }> {
    const response = await sendMutation(
      this,
      { case: 'deleteObject', value: { objectKey: key } },
      abortSignal,
    )
    if (response.errorCode === WorldErrorCode.OBJECT_NOT_FOUND) {
      return { deleted: false }
    }
    throwBatchError(response)
    return { deleted: true }
  }

  // setGraphQuad sets a quad in the graph store.
  public async setGraphQuad(
    subject: string,
    predicate: string,
    obj: string,
    label?: string,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    await applyMutation(
      this,
      {
        case: 'setGraphQuad',
        value: { quad: { subject, predicate, obj, label } },
      },
      abortSignal,
    )
  }

  // deleteGraphQuad deletes a quad from the graph store.
  public async deleteGraphQuad(
    subject: string,
    predicate: string,
    obj: string,
    label?: string,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    await applyMutation(
      this,
      {
        case: 'deleteGraphQuad',
        value: { quad: { subject, predicate, obj, label } },
      },
      abortSignal,
    )
  }

  // lookupGraphQuads searches for graph quads in the store.
  public async lookupGraphQuads(
    subject?: string,
    predicate?: string,
    obj?: string,
    label?: string,
    limit?: number,
    abortSignal?: AbortSignal,
  ) {
    return this.performRead(abortSignal, async (tx) => {
      return await tx.lookupGraphQuads(
        subject,
        predicate,
        obj,
        label,
        limit,
        abortSignal,
      )
    })
  }

  // listGraphEdgeBuckets lists grouped inbound/outbound graph edges for origin
  // object keys.
  public async listGraphEdgeBuckets(
    originObjectKeys: string[],
    limitPerOrigin: number,
    options: ListGraphEdgeBucketsOptions = {},
  ): Promise<ListGraphEdgeBucketsResponse> {
    return this.performRead(options.abortSignal, async (tx) => {
      return await tx.listGraphEdgeBuckets(
        originObjectKeys,
        limitPerOrigin,
        options,
      )
    })
  }

  // getObjectBodies reads object root bodies from one World revision.
  public async getObjectBodies(
    objectKeys: string[],
    abortSignal?: AbortSignal,
  ): Promise<ObjectBody[]> {
    return this.performRead(abortSignal, async (tx) => {
      return await tx.getObjectBodies(objectKeys, abortSignal)
    })
  }

  // listObjectsWithType lists object keys with the given type identifier.
  public async listObjectsWithType(
    typeID: string,
    abortSignal?: AbortSignal,
    limit?: number,
  ): Promise<string[]> {
    return this.performRead(abortSignal, async (tx) => {
      return await tx.listObjectsWithType(typeID, abortSignal, limit)
    })
  }

  // listObjectsWithTypePage returns one page of the type's keys and titles.
  public async listObjectsWithTypePage(
    typeID: string,
    page: ListObjectsWithTypePage,
    abortSignal?: AbortSignal,
  ): Promise<ListObjectsWithTypeResponse> {
    return this.performRead(abortSignal, (tx) =>
      tx.listObjectsWithTypePage(typeID, page, abortSignal),
    )
  }

  // listTypeCounts returns the number of objects of each type.
  public async listTypeCounts(
    abortSignal?: AbortSignal,
  ): Promise<TypeCountEntry[]> {
    return this.performRead(abortSignal, (tx) => tx.listTypeCounts(abortSignal))
  }

  // searchObjectsWithType streams pages of the type's matching objects from
  // one committed snapshot.
  public searchObjectsWithType(
    typeID: string,
    query: string,
    pageSize?: number,
    abortSignal?: AbortSignal,
  ): AsyncIterable<SearchObjectsWithTypeResponse> {
    return this.engine.searchObjectsWithType(
      typeID,
      query,
      pageSize,
      abortSignal,
    )
  }

  // watchTypeCounts streams the type counts of the Engine's committed World.
  public watchTypeCounts(
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchTypeCountsResponse> {
    return this.engine.watchTypeCounts(abortSignal)
  }

  // listObjects returns one page of objects under prefix in key order.
  public async listObjects(
    request: ListObjectsRequest,
    abortSignal?: AbortSignal,
  ): Promise<ListObjectsResponse> {
    return this.performRead(abortSignal, async (tx) => {
      return await tx.listObjects(request, abortSignal)
    })
  }

  // getObjectMetadataBatch returns the type, parent and declarations of each
  // key, in request order.
  public async getObjectMetadataBatch(
    objectKeys: string[],
    abortSignal?: AbortSignal,
    declared = false,
  ): Promise<ObjectMetadata[]> {
    return this.performRead(abortSignal, async (tx) => {
      return await tx.getObjectMetadataBatch(objectKeys, abortSignal, declared)
    })
  }

  // deleteGraphObject deletes all quads with Subject or Object set to value.
  public async deleteGraphObject(
    objectKey: string,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    await applyMutation(
      this,
      { case: 'deleteGraphObject', value: { objectKey } },
      abortSignal,
    )
  }

  // setObjectType sets an object's one type in one batch.
  public async setObjectType(
    objectKey: string,
    typeID: string,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    await applyMutation(
      this,
      { case: 'setObjectType', value: { objectKey, typeId: typeID } },
      abortSignal,
    )
  }

  // accessTypedObject looks up an object, determines its type via graph quad,
  // and returns access to a typed resource that implements the type-specific RPC service.
  // The returned resourceId can be used with resourceRef.createRef() to access the typed resource.
  // For example, an ObjectLayout object returns access to a LayoutHost resource.
  public async accessTypedObject(
    objectKey: string,
    abortSignal?: AbortSignal,
  ): Promise<TypedObjectAccess> {
    return this.engine.accessTypedObject(objectKey, abortSignal)
  }

  // performRead performs a read with a short-lived read transaction.
  private performRead<T>(
    abortSignal: AbortSignal | undefined,
    cb: (tx: Tx) => Promise<T>,
  ): Promise<T> {
    return runReadOp(this, abortSignal, cb)
  }

  // canWrite reports whether this wrapper allows write operations.
  public canWrite(): boolean {
    return this.write
  }

  // getEngine returns the underlying Engine (for advanced usage).
  public getEngine(): Engine {
    return this.engine
  }

  // released reports whether the underlying Engine resource was released.
  public get released(): boolean {
    return this.engine.released
  }

  // release releases the owned Engine resource, when this wrapper created it.
  public release(): void {
    if (this.ownsEngine && !this.engine.released) {
      this.engine.release()
    }
  }

  // Symbol.dispose releases any owned Engine resource.
  [Symbol.dispose](): void {
    this.release()
  }
}

// EngineWorldStateObject wraps an EngineWorldState to provide ObjectState
// operations.
class EngineWorldStateObject implements IObjectState {
  private engineState: EngineWorldState
  private objectKey: string

  constructor(engineState: EngineWorldState, key: string) {
    this.engineState = engineState
    this.objectKey = key
  }

  public getKey(): string {
    return this.objectKey
  }

  public async getRootRef(
    abortSignal?: AbortSignal,
  ): Promise<GetRootRefResponse> {
    return runReadOp(this.engineState, abortSignal, async (tx) => {
      using obj = await tx.getObject(this.objectKey, abortSignal)
      if (!obj) {
        throw new Error(`Object not found: ${this.objectKey}`)
      }
      return await obj.getRootRef(abortSignal)
    })
  }

  public async setRootRef(
    rootRef: ObjectRef,
    abortSignal?: AbortSignal,
  ): Promise<SetRootRefResponse> {
    return applyObjectMutation(
      this.engineState,
      { case: 'setRootRef', value: { objectKey: this.objectKey, rootRef } },
      abortSignal,
    )
  }

  public async accessWorldState(
    ref?: ObjectRef,
    abortSignal?: AbortSignal,
  ): Promise<BucketLookupCursor> {
    const tx = await this.engineState
      .getEngine()
      .newTransaction(false, abortSignal)
    try {
      const obj = await tx.getObject(this.objectKey, abortSignal)
      if (!obj) {
        throw new Error(`Object not found: ${this.objectKey}`)
      }
      try {
        const cursor = await obj.accessWorldState(ref, abortSignal)
        return new TxOwnedBucketLookupCursor(cursor, tx)
      } finally {
        obj.release()
      }
    } catch (err) {
      await tx.discard(abortSignal).catch(() => {})
      tx.release()
      throw err
    }
  }

  public async applyObjectOp(
    opTypeId: string,
    opData: Uint8Array,
    opSender: string,
    abortSignal?: AbortSignal,
  ): Promise<ApplyObjectOpResponse> {
    return applyObjectMutation(
      this.engineState,
      {
        case: 'applyObjectOp',
        value: { objectKey: this.objectKey, opTypeId, opData, opSender },
      },
      abortSignal,
    )
  }

  public async incrementRev(
    abortSignal?: AbortSignal,
  ): Promise<IncrementRevResponse> {
    return applyObjectMutation(
      this.engineState,
      { case: 'incrementRev', value: { objectKey: this.objectKey } },
      abortSignal,
    )
  }

  public async waitRev(
    rev: bigint,
    ignoreNotFound?: boolean,
    abortSignal?: AbortSignal,
  ): Promise<WaitRevResponse> {
    // Reread the root only after a commit that changes this object.
    const roots = watchWorldQuery(
      this.engineState.getEngine(),
      async (world, signal) => {
        using obj = await world.getObject(this.objectKey, signal)
        if (!obj && !ignoreNotFound) {
          throw new Error(`Object not found: ${this.objectKey}`)
        }
        return await obj?.getRootRef(signal)
      },
      abortSignal,
    )
    for await (const { value: root } of roots) {
      if (root && (root.rev ?? 0n) >= rev) {
        return { rev: root.rev }
      }
    }
    abortSignal?.throwIfAborted()
    throw new Error('World watch ended')
  }

  // release is a no-op for EngineWorldStateObject since it doesn't own any
  // resources.
  public release(): void {
    // No-op: EngineWorldStateObject is not a Resource
  }

  // Symbol.dispose is a no-op for EngineWorldStateObject.
  [Symbol.dispose](): void {
    // No-op: EngineWorldStateObject is not a Resource
  }
}

class TxOwnedBucketLookupCursor extends BucketLookupCursor {
  private cursor: BucketLookupCursor
  private tx: Tx
  private releasedCursor = false

  constructor(cursor: BucketLookupCursor, tx: Tx) {
    super(cursor.resourceRef)
    this.cursor = cursor
    this.tx = tx
  }

  public release(abortSignal?: AbortSignal): void {
    if (this.releasedCursor) return
    this.releasedCursor = true
    this.cursor.release(abortSignal)
    void this.tx
      .discard(abortSignal)
      .catch(() => {})
      .finally(() => {
        this.tx.release()
      })
  }
}

// TxOwnedObjectIterator keeps its immutable snapshot alive until close or release.
class TxOwnedObjectIterator extends ObjectIterator {
  private closed = false

  constructor(
    private readonly iterator: ObjectIterator,
    private readonly tx: Tx,
  ) {
    super(iterator.resourceRef)
  }

  public async close(abortSignal?: AbortSignal): Promise<void> {
    if (this.closed) return
    try {
      await this.iterator.close(abortSignal)
    } finally {
      this.release()
    }
  }

  public release(): void {
    if (this.closed) return
    this.closed = true
    this.iterator.release()
    this.tx.release()
  }
}
