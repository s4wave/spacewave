import { ObjectRef } from '@go/github.com/s4wave/spacewave/db/bucket/bucket.pb.js'
import { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import {
  Resource,
  type ResourceDebugInfo,
} from '@aptre/bldr-sdk/resource/resource.js'

import {
  WorldStateResourceService,
  WorldStateResourceServiceClient,
  TypedObjectResourceServiceClient,
} from './world_srpc.pb.js'
import { ObjectState, type IObjectState } from './object-state.js'
import { ObjectIterator } from './object_iterator.js'
import { BucketLookupCursor } from '../bucket/lookup/lookup.js'
import type { LookupGraphQuadsResponse } from './world.pb.js'

// TypedObjectAccess represents access to a typed resource from a world object.
// The resourceId can be used with resourceRef.createRef() to access the typed resource.
export interface TypedObjectAccess {
  // resourceId is the ID of the typed resource.
  resourceId: number
  // typeId is the type identifier of the object.
  typeId: string
}

// RenameObjectOptions configures a world object rename request.
export interface RenameObjectOptions {
  descendants?: boolean
  abortSignal?: AbortSignal
}

// IWorldState contains the world state interface.
// Represents the full state read/write operations interface to the world.
export interface IWorldState {
  // getResourceRef returns the resource ref for creating child resources.
  // This is used to create typed object resources from accessTypedObject results.
  getResourceRef(): ClientResourceRef

  // GetReadOnly returns if the state is read-only
  getReadOnly(): boolean

  // GetSeqno returns the current seqno of the world state
  // This is also the sequence number of the most recent change
  // Initializes at 0 for initial world state
  getSeqno(abortSignal?: AbortSignal): Promise<{ seqno: bigint }>

  // WaitSeqno waits for the seqno of the world state to be >= value
  // Returns the seqno when the condition is reached
  // If value == 0, this might return immediately unconditionally
  waitSeqno(
    seqno: bigint,
    abortSignal?: AbortSignal,
  ): Promise<{ seqno: bigint }>

  // BuildStorageCursor builds a cursor to the world storage with an empty ref
  // The cursor should be released independently of the WorldState
  // Be sure to call Release on the cursor when done
  buildStorageCursor(abortSignal?: AbortSignal): Promise<BucketLookupCursor>

  // AccessWorldState builds a bucket lookup cursor with an optional ref
  // If the ref is empty, returns a cursor pointing to the root world state
  // The lookup cursor will be released after cb returns
  accessWorldState(
    ref?: ObjectRef,
    abortSignal?: AbortSignal,
  ): Promise<BucketLookupCursor>

  // CreateObject creates a object with a key and initial root ref
  // Returns ErrObjectExists if the object already exists
  // Appends a OBJECT_SET change to the changelog
  createObject(
    key: string,
    rootRef: ObjectRef,
    abortSignal?: AbortSignal,
  ): Promise<IObjectState>

  // GetObject looks up an object by key
  // Returns null if not found
  getObject(
    key: string,
    abortSignal?: AbortSignal,
  ): Promise<IObjectState | null>

  // IterateObjects returns an iterator with the given object key prefix
  // The prefix is NOT clipped from the output keys
  // Keys are returned in sorted order
  // Must call Next() or Seek() before valid
  // Call Close when done with the iterator
  // Any init errors will be available via the iterator's Err() method
  iterateObjects(
    prefix?: string,
    reversed?: boolean,
    abortSignal?: AbortSignal,
  ): Promise<ObjectIterator>

  // RenameObject renames an object key and associated graph quads.
  renameObject(
    oldKey: string,
    newKey: string,
    options?: AbortSignal | RenameObjectOptions,
  ): Promise<IObjectState>

  // DeleteObject deletes an object and associated graph quads by ID
  // Calls DeleteGraphObject internally
  // Returns deleted=false if not found
  deleteObject(
    key: string,
    abortSignal?: AbortSignal,
  ): Promise<{ deleted: boolean }>

  // SetGraphQuad sets a quad in the graph store
  // Subject: must be an existing object IRI: <object-key>
  // Predicate: a predicate string, e.g. IRI: <ref>
  // Object: an existing object IRI: <object-key>
  // If already exists, returns nil
  setGraphQuad(
    subject: string,
    predicate: string,
    obj: string,
    label?: string,
    abortSignal?: AbortSignal,
  ): Promise<void>

  // DeleteGraphQuad deletes a quad from the graph store
  // Note: if quad did not exist, returns nil
  deleteGraphQuad(
    subject: string,
    predicate: string,
    obj: string,
    label?: string,
    abortSignal?: AbortSignal,
  ): Promise<void>

  // LookupGraphQuads searches for graph quads in the store
  // If the filter fields are empty, matches any for that field
  // If not found, returns empty list
  // If limit is set, stops after finding that number of matching quads
  lookupGraphQuads(
    subject?: string,
    predicate?: string,
    obj?: string,
    label?: string,
    limit?: number,
    abortSignal?: AbortSignal,
  ): Promise<LookupGraphQuadsResponse>

  // ListObjectsWithType lists object keys with the given type identifier.
  listObjectsWithType(
    typeID: string,
    abortSignal?: AbortSignal,
  ): Promise<string[]>

  // DeleteGraphObject deletes all quads with Subject or Object set to value
  // Note: value should be the object key, NOT the object key <iri> format
  deleteGraphObject(objectKey: string, abortSignal?: AbortSignal): Promise<void>

  // ApplyWorldOp applies a batch operation at the world level
  // The handling of the operation is operation-type specific
  // Returns the seqno following the operation execution
  // If nil is returned for the error, implies success
  // If sysErr is set, the error is treated as a transient system error
  // Must support recursive calls to ApplyWorldOp / ApplyObjectOp
  applyWorldOp(
    opTypeId: string,
    opData: Uint8Array,
    opSender: string,
    abortSignal?: AbortSignal,
  ): Promise<{ seqno: bigint; sysErr: boolean }>

  // AccessTypedObject looks up an object, determines its type via graph quad,
  // and returns access to a typed resource that implements the type-specific RPC service.
  // The returned resourceId can be used with resourceRef.createRef() to access the typed resource.
  // For example, an ObjectLayout object returns access to a LayoutHost resource.
  accessTypedObject(
    objectKey: string,
    abortSignal?: AbortSignal,
  ): Promise<TypedObjectAccess>
}

// WorldStateResource represents the full state read/write interface to the world.
// WorldState implements all world state operations (maps to WorldState in Go).
//
// In the Go implementation (hydra/world/world-state.go), WorldState provides:
// - GetReadOnly() bool
// - WorldStorage: BuildStorageCursor, AccessWorldState
// - WorldStateObject: CreateObject, GetObject, IterateObjects, RenameObject, DeleteObject
// - WorldStateGraph: SetGraphQuad, DeleteGraphQuad, LookupGraphQuads, DeleteGraphObject
// - WorldStateOp: ApplyWorldOp
// - WorldWaitSeqno: GetSeqno, WaitSeqno
//
// Concurrent calls to WorldState functions should be supported.
export class WorldStateResource extends Resource implements IWorldState {
  protected service: WorldStateResourceService
  private readOnly: boolean

  constructor(resourceRef: ClientResourceRef, meta?: { readOnly?: boolean }) {
    super(resourceRef)
    this.service = new WorldStateResourceServiceClient(resourceRef.client)
    this.readOnly = meta?.readOnly ?? false
  }

  // WorldState implementation

  // getResourceRef returns the resource ref for creating child resources.
  public getResourceRef(): ClientResourceRef {
    return this.resourceRef
  }

  // GetReadOnly returns if the transaction is read-only.
  // Returns stored metadata without RPC call.
  public getReadOnly(): boolean {
    return this.readOnly
  }

  // WorldWaitSeqno implementation

  // GetSeqno returns the current sequence number of the world state.
  // This is also the sequence number of the most recent change.
  // Initializes at 0 for initial world state.
  public async getSeqno(abortSignal?: AbortSignal): Promise<{ seqno: bigint }> {
    const response = await this.service.GetSeqno({}, abortSignal)
    return { seqno: response.seqno ?? 0n }
  }

  // WaitSeqno waits for the world state sequence number to reach or exceed the specified value.
  // Returns the seqno when the condition is reached.
  // If seqno == 0, this might return immediately unconditionally.
  public async waitSeqno(
    seqno: bigint,
    abortSignal?: AbortSignal,
  ): Promise<{ seqno: bigint }> {
    const response = await this.service.WaitSeqno({ seqno }, abortSignal)
    return { seqno: response.seqno ?? 0n }
  }

  // WorldStorage implementation

  // BuildStorageCursor builds a cursor to the world storage with an empty ref.
  // The cursor should be released independently of the Tx.
  // Be sure to call Release on the cursor when done.
  public async buildStorageCursor(
    abortSignal?: AbortSignal,
  ): Promise<BucketLookupCursor> {
    const response = await this.service.BuildStorageCursor({}, abortSignal)
    return this.resourceRef.createResource(
      response.resourceId ?? 0,
      BucketLookupCursor,
    )
  }

  // AccessWorldState builds a bucket lookup cursor with an optional ref.
  // If the ref is empty, returns a cursor pointing to the root world state.
  // The lookup cursor will be released after cb returns.
  public async accessWorldState(
    ref?: ObjectRef,
    abortSignal?: AbortSignal,
  ): Promise<BucketLookupCursor> {
    const response = await this.service.AccessWorldState({ ref }, abortSignal)
    return this.resourceRef.createResource(
      response.resourceId ?? 0,
      BucketLookupCursor,
    )
  }

  // WorldStateObject implementation

  // CreateObject creates a new object in the world with the specified key and initial data.
  // Returns ErrObjectExists if the object already exists.
  // Appends a OBJECT_SET change to the changelog.
  // Returns an ObjectState resource for the created object.
  public async createObject(
    key: string,
    rootRef: ObjectRef,
    abortSignal?: AbortSignal,
  ): Promise<ObjectState> {
    const response = await this.service.CreateObject(
      { objectKey: key, rootRef },
      abortSignal,
    )
    return this.resourceRef.createResource(
      response.resourceId ?? 0,
      ObjectState,
      { objectKey: response.objectKey },
    )
  }

  // GetObject retrieves an object from the world by its key.
  // Returns an ObjectState resource if found, or null if not found.
  public async getObject(
    key: string,
    abortSignal?: AbortSignal,
  ): Promise<ObjectState | null> {
    const response = await this.service.GetObject(
      { objectKey: key },
      abortSignal,
    )
    if (!response.found) {
      return null
    }
    return this.resourceRef.createResource(
      response.resourceId ?? 0,
      ObjectState,
      { objectKey: response.objectKey },
    )
  }

  // IterateObjects returns an iterator with the given object key prefix.
  // The prefix is NOT clipped from the output keys.
  // Keys are returned in sorted order.
  // Must call Next() or Seek() before valid.
  // Call Close when done with the iterator.
  // Any init errors will be available via the iterator's Err() method.
  // Returns an ObjectIterator resource for iterating over matching objects.
  public async iterateObjects(
    prefix?: string,
    reversed?: boolean,
    abortSignal?: AbortSignal,
  ): Promise<ObjectIterator> {
    const response = await this.service.IterateObjects(
      {
        prefix,
        reversed,
      },
      abortSignal,
    )
    return this.resourceRef.createResource(
      response.resourceId ?? 0,
      ObjectIterator,
    )
  }

  // RenameObject renames an object key and associated graph quads.
  public async renameObject(
    oldKey: string,
    newKey: string,
    options?: AbortSignal | RenameObjectOptions,
  ): Promise<ObjectState> {
    const renameOptions = normalizeRenameObjectOptions(options)
    const response = await this.service.RenameObject(
      {
        oldObjectKey: oldKey,
        newObjectKey: newKey,
        descendants: renameOptions.descendants,
      },
      renameOptions.abortSignal,
    )
    return this.resourceRef.createResource(
      response.resourceId ?? 0,
      ObjectState,
      { objectKey: response.objectKey },
    )
  }

  // DeleteObject removes an object and all associated graph quads from the world.
  // Calls DeleteGraphObject internally.
  // Returns deleted=false if not found.
  public async deleteObject(
    key: string,
    abortSignal?: AbortSignal,
  ): Promise<{ deleted: boolean }> {
    const response = await this.service.DeleteObject(
      { objectKey: key },
      abortSignal,
    )
    return { deleted: response.deleted ?? false }
  }

  // WorldStateGraph implementation

  // SetGraphQuad adds or updates a quad in the graph store.
  // Subject: must be an existing object IRI: <object-key>
  // Predicate: a predicate string, e.g. IRI: <ref>
  // Object: an existing object IRI: <object-key>
  // If already exists, returns nil.
  public async setGraphQuad(
    subject: string,
    predicate: string,
    obj: string,
    label?: string,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    await this.service.SetGraphQuad(
      {
        quad: { subject, predicate, obj, label },
      },
      abortSignal,
    )
  }

  // DeleteGraphQuad removes a specific quad from the graph store.
  // Note: if quad did not exist, returns nil.
  public async deleteGraphQuad(
    subject: string,
    predicate: string,
    obj: string,
    label?: string,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    await this.service.DeleteGraphQuad(
      {
        quad: { subject, predicate, obj, label },
      },
      abortSignal,
    )
  }

  // LookupGraphQuads searches for graph quads matching the specified filter criteria.
  // If the filter fields are empty, matches any for that field.
  // If not found, returns empty list.
  // If limit is set, stops after finding that number of matching quads.
  public async lookupGraphQuads(
    subject?: string,
    predicate?: string,
    obj?: string,
    label?: string,
    limit?: number,
    abortSignal?: AbortSignal,
  ) {
    return await this.service.LookupGraphQuads(
      {
        filter: { subject, predicate, obj, label },
        limit,
      },
      abortSignal,
    )
  }

  // ListObjectsWithType lists object keys with the given type identifier.
  public async listObjectsWithType(
    typeID: string,
    abortSignal?: AbortSignal,
  ): Promise<string[]> {
    const response = await this.service.ListObjectsWithType(
      { typeId: typeID },
      abortSignal,
    )
    return response.objectKeys ?? []
  }

  // DeleteGraphObject removes all graph quads that reference the specified object key.
  // Note: objectKey should be the object key, NOT the object key <iri> format.
  public async deleteGraphObject(
    objectKey: string,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    await this.service.DeleteGraphObject({ objectKey }, abortSignal)
  }

  // WorldStateOp implementation

  // ApplyWorldOp applies a batch operation at the world level.
  // The handling of the operation is operation-type specific.
  // Returns the seqno following the operation execution.
  // If nil is returned for the error, implies success.
  // If sysErr is set, the error is treated as a transient system error.
  // Must support recursive calls to ApplyWorldOp / ApplyObjectOp.
  public async applyWorldOp(
    opTypeId: string,
    opData: Uint8Array,
    opSender: string,
    abortSignal?: AbortSignal,
  ): Promise<{ seqno: bigint; sysErr: boolean }> {
    const response = await this.service.ApplyWorldOp(
      { opTypeId, opData, opSender },
      abortSignal,
    )
    return { seqno: response.seqno ?? 0n, sysErr: response.sysErr ?? false }
  }

  // TypedObjectResourceService implementation

  // AccessTypedObject looks up an object, determines its type via graph quad,
  // and returns access to a typed resource that implements the type-specific RPC service.
  // The returned resourceId can be used with resourceRef.createRef() to access the typed resource.
  // For example, an ObjectLayout object returns access to a LayoutHost resource.
  public async accessTypedObject(
    objectKey: string,
    abortSignal?: AbortSignal,
  ): Promise<TypedObjectAccess> {
    const typedService = new TypedObjectResourceServiceClient(
      this.resourceRef.client,
    )
    const response = await typedService.AccessTypedObject(
      { objectKey },
      abortSignal,
    )
    return {
      resourceId: response.resourceId ?? 0,
      typeId: response.typeId ?? '',
    }
  }

  // getDebugInfo returns debug information for devtools.
  public getDebugInfo(): ResourceDebugInfo {
    return { label: this.readOnly ? '(read-only)' : undefined }
  }
}

// normalizeRenameObjectOptions normalizes legacy AbortSignal arguments.
export function normalizeRenameObjectOptions(
  options?: AbortSignal | RenameObjectOptions,
): RenameObjectOptions {
  if (!options) return {}
  if (options instanceof AbortSignal) {
    return { abortSignal: options }
  }
  return options
}
