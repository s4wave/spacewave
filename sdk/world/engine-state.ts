import type { ObjectRef } from '@go/github.com/s4wave/spacewave/db/bucket/bucket.pb.js'
import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'

import { Engine } from './engine.js'
import { Tx } from './tx.js'
import { type IObjectState } from './object-state.js'
import {
  normalizeRenameObjectOptions,
  type IWorldState,
  type RenameObjectOptions,
  type TypedObjectAccess,
} from './world-state.js'
import { ObjectIterator } from './object_iterator.js'
import { BucketLookupCursor } from '../bucket/lookup/lookup.js'
import type {
  GetRootRefResponse,
  SetRootRefResponse,
  ApplyObjectOpResponse,
  IncrementRevResponse,
  WaitRevResponse,
} from './world.pb.js'

// EngineWorldState implements IWorldState on top of an Engine.
// Short-lived transactions are created for each operation.
// This matches the Go implementation in hydra/world/engine-state.go
export class EngineWorldState implements IWorldState {
  private engine: Engine
  private write: boolean

  constructor(engine: Engine, write: boolean) {
    this.engine = engine
    this.write = write
  }

  // getResourceRef returns the resource ref for creating child resources.
  public getResourceRef(): ClientResourceRef {
    return this.engine.resourceRef
  }

  // getReadOnly returns if the state is read-only
  public getReadOnly(): boolean {
    return !this.write
  }

  // getSeqno returns the current seqno of the world state
  public async getSeqno(abortSignal?: AbortSignal): Promise<{ seqno: bigint }> {
    const response = await this.engine.getSeqno(abortSignal)
    return { seqno: response.seqno ?? 0n }
  }

  // waitSeqno waits for the seqno of the world state to be >= value
  public async waitSeqno(
    seqno: bigint,
    abortSignal?: AbortSignal,
  ): Promise<{ seqno: bigint }> {
    const response = await this.engine.waitSeqno(seqno, abortSignal)
    return { seqno: response.seqno ?? 0n }
  }

  // buildStorageCursor builds a cursor to the world storage with an empty ref
  public async buildStorageCursor(
    abortSignal?: AbortSignal,
  ): Promise<BucketLookupCursor> {
    return await this.engine.buildStorageCursor(abortSignal)
  }

  // accessWorldState builds a bucket lookup cursor with an optional ref
  public async accessWorldState(
    ref?: ObjectRef,
    abortSignal?: AbortSignal,
  ): Promise<BucketLookupCursor> {
    return this.engine.accessWorldState(ref, abortSignal)
  }

  // applyWorldOp applies a batch operation at the world level
  public async applyWorldOp(
    opTypeId: string,
    opData: Uint8Array,
    opSender: string,
    abortSignal?: AbortSignal,
  ): Promise<{ seqno: bigint; sysErr: boolean }> {
    return this.performOp(true, abortSignal, async (tx) => {
      const response = await tx.applyWorldOp(
        opTypeId,
        opData,
        opSender,
        abortSignal,
      )
      return { seqno: response.seqno ?? 0n, sysErr: response.sysErr ?? false }
    })
  }

  // createObject creates an object with a key and initial root ref
  public async createObject(
    key: string,
    rootRef: ObjectRef,
    abortSignal?: AbortSignal,
  ): Promise<IObjectState> {
    await this.performOp(true, abortSignal, async (tx) => {
      await tx.createObject(key, rootRef, abortSignal)
    })
    // Return a new EngineWorldStateObject that wraps this engine + key
    return new EngineWorldStateObject(this, key)
  }

  // getObject looks up an object by key
  public async getObject(
    key: string,
    abortSignal?: AbortSignal,
  ): Promise<IObjectState | null> {
    const found = await this.performOp(false, abortSignal, async (tx) => {
      const obj = await tx.getObject(key, abortSignal)
      return obj !== null
    })

    if (!found) {
      return null
    }

    return new EngineWorldStateObject(this, key)
  }

  // iterateObjects returns an iterator with the given object key prefix
  public async iterateObjects(
    prefix?: string,
    reversed?: boolean,
    abortSignal?: AbortSignal,
  ): Promise<ObjectIterator> {
    // Create a transaction and return the iterator from it
    const tx = await this.engine.newTransaction(false, abortSignal)
    return tx.iterateObjects(prefix, reversed, abortSignal)
  }

  // renameObject renames an object key and associated graph quads
  public async renameObject(
    oldKey: string,
    newKey: string,
    options?: AbortSignal | RenameObjectOptions,
  ): Promise<IObjectState> {
    const renameOptions = normalizeRenameObjectOptions(options)
    await this.performOp(true, renameOptions.abortSignal, async (tx) => {
      const obj = await tx.renameObject(oldKey, newKey, renameOptions)
      obj.release()
    })
    return new EngineWorldStateObject(this, newKey)
  }

  // deleteObject deletes an object and associated graph quads by ID
  public async deleteObject(
    key: string,
    abortSignal?: AbortSignal,
  ): Promise<{ deleted: boolean }> {
    const deleted = await this.performOp(true, abortSignal, async (tx) => {
      const response = await tx.deleteObject(key, abortSignal)
      return response.deleted ?? false
    })
    return { deleted }
  }

  // setGraphQuad sets a quad in the graph store
  public async setGraphQuad(
    subject: string,
    predicate: string,
    obj: string,
    label?: string,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    await this.performOp(true, abortSignal, async (tx) => {
      await tx.setGraphQuad(subject, predicate, obj, label, abortSignal)
    })
  }

  // deleteGraphQuad deletes a quad from the graph store
  public async deleteGraphQuad(
    subject: string,
    predicate: string,
    obj: string,
    label?: string,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    await this.performOp(true, abortSignal, async (tx) => {
      await tx.deleteGraphQuad(subject, predicate, obj, label, abortSignal)
    })
  }

  // lookupGraphQuads searches for graph quads in the store
  public async lookupGraphQuads(
    subject?: string,
    predicate?: string,
    obj?: string,
    label?: string,
    limit?: number,
    abortSignal?: AbortSignal,
  ) {
    return this.performOp(false, abortSignal, async (tx) => {
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

  // listObjectsWithType lists object keys with the given type identifier.
  public async listObjectsWithType(
    typeID: string,
    abortSignal?: AbortSignal,
  ): Promise<string[]> {
    return this.performOp(false, abortSignal, async (tx) => {
      return await tx.listObjectsWithType(typeID, abortSignal)
    })
  }

  // deleteGraphObject deletes all quads with Subject or Object set to value
  public async deleteGraphObject(
    objectKey: string,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    await this.performOp(true, abortSignal, async (tx) => {
      await tx.deleteGraphObject(objectKey, abortSignal)
    })
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

  // performOp performs an operation with a short-lived transaction
  private async performOp<T>(
    write: boolean,
    abortSignal: AbortSignal | undefined,
    cb: (tx: Tx) => Promise<T>,
  ): Promise<T> {
    if (!this.write && write) {
      throw new Error('EngineWorldState is read-only')
    }

    const tx = await this.engine.newTransaction(write, abortSignal)
    try {
      const result = await cb(tx)
      if (write) {
        await tx.commit(abortSignal)
      }
      return result
    } finally {
      // Always discard to clean up (catches panic cases)
      await tx.discard(abortSignal).catch(() => {
        // Ignore errors during cleanup
      })
    }
  }

  // getEngine returns the underlying Engine (for advanced usage)
  public getEngine(): Engine {
    return this.engine
  }

  // release is a no-op for EngineWorldState since it doesn't own any resources
  public release(): void {
    // No-op: EngineWorldState is not a Resource, just a lightweight wrapper
  }

  // Symbol.dispose is a no-op for EngineWorldState
  [Symbol.dispose](): void {
    // No-op: EngineWorldState is not a Resource
  }
}

// EngineWorldStateObject wraps an EngineWorldState to provide ObjectState operations
// This matches the Go implementation in hydra/world/engine-state-object.go
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
    return this.engineState['performOp'](false, abortSignal, async (tx) => {
      const obj = await tx.getObject(this.objectKey, abortSignal)
      if (!obj) {
        throw new Error(`Object not found: ${this.objectKey}`)
      }
      return obj.getRootRef(abortSignal)
    })
  }

  public async setRootRef(
    rootRef: ObjectRef,
    abortSignal?: AbortSignal,
  ): Promise<SetRootRefResponse> {
    return this.engineState['performOp'](true, abortSignal, async (tx) => {
      const obj = await tx.getObject(this.objectKey, abortSignal)
      if (!obj) {
        throw new Error(`Object not found: ${this.objectKey}`)
      }
      return obj.setRootRef(rootRef, abortSignal)
    })
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
    return this.engineState['performOp'](true, abortSignal, async (tx) => {
      const obj = await tx.getObject(this.objectKey, abortSignal)
      if (!obj) {
        throw new Error(`Object not found: ${this.objectKey}`)
      }
      return obj.applyObjectOp(opTypeId, opData, opSender, abortSignal)
    })
  }

  public async incrementRev(
    abortSignal?: AbortSignal,
  ): Promise<IncrementRevResponse> {
    return this.engineState['performOp'](true, abortSignal, async (tx) => {
      const obj = await tx.getObject(this.objectKey, abortSignal)
      if (!obj) {
        throw new Error(`Object not found: ${this.objectKey}`)
      }
      return obj.incrementRev(abortSignal)
    })
  }

  public async waitRev(
    rev: bigint,
    ignoreNotFound?: boolean,
    abortSignal?: AbortSignal,
  ): Promise<WaitRevResponse> {
    return this.engineState['performOp'](false, abortSignal, async (tx) => {
      const obj = await tx.getObject(this.objectKey, abortSignal)
      if (!obj) {
        throw new Error(`Object not found: ${this.objectKey}`)
      }
      return obj.waitRev(rev, ignoreNotFound, abortSignal)
    })
  }

  // release is a no-op for EngineWorldStateObject since it doesn't own any resources
  public release(): void {
    // No-op: EngineWorldStateObject is not a Resource
  }

  // Symbol.dispose is a no-op for EngineWorldStateObject
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
