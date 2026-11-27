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
  TxResourceServiceClient,
} from './world_srpc.pb.js'
import { throwCommitError, throwOperationError } from './errors.js'
import { KNOWN_ERROR_CODE } from './known-error-code.js'
import { throwTypedObjectError } from './typed-object-grant-retired-error.js'
import { ObjectState, type IObjectState } from './object-state.js'
import { ObjectIterator } from './object_iterator.js'
import { BucketLookupCursor } from '../bucket/lookup/lookup.js'
import { Engine } from './engine.js'
import { WorldStage } from './stage.js'
import { EngineWorldState } from './engine-state.js'
import type {
  ObjectBody,
  ObjectRecordBase,
  GetObjectBodiesBatchResponse,
  GraphEdgeBucketDirection,
  ListGraphEdgeBucketsResponse,
  ListObjectsRequest,
  ListObjectsResponse,
  LookupGraphQuadsResponse,
  ObjectMetadata,
  ListObjectsWithTypeResponse,
  SearchObjectsWithTypeResponse,
  TypeCountEntry,
  WatchTypeCountsResponse,
} from './world.pb.js'

// ListObjectsWithTypePage selects one page of a type's objects.
export interface ListObjectsWithTypePage {
  // startAfter resumes after this key. An empty key starts at the first key.
  startAfter?: string
  // limit bounds the page. Zero lets the server choose.
  limit?: number
  // declared asks for each object's title and state beside its key.
  declared?: boolean
}

// maxObjectBodiesRevisionRetries bounds restarts when writes land between body pages.
const maxObjectBodiesRevisionRetries = 3

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

// ListGraphEdgeBucketsOptions configures grouped graph edge bucket lookup.
export interface ListGraphEdgeBucketsOptions {
  predicate?: string
  direction?: GraphEdgeBucketDirection
  // after resumes just past all edges of this endpoint object key, the
  // previous page's last endpoint. It requires a predicate.
  after?: string
  // reverse lists edges last first: newest first in creation order.
  reverse?: boolean
  abortSignal?: AbortSignal
}

// IWorldState contains the world state interface.
// Represents the full state read/write operations interface to the world.
export interface IWorldState {
  /**
   * getEngine borrows the live owner when available. Immutable snapshots have
   * no Engine.
   */
  getEngine?(): Engine

  /**
   * bindObject returns a handle to the object at key without checking that it
   * exists. Each call on it reads the current revision. Only a live World,
   * one with getEngine, has bindObject.
   */
  bindObject?(key: string): IObjectState

  // getResourceRef returns the resource ref for creating child resources.
  // This is used to create typed object resources from accessTypedObject results.
  getResourceRef(): ClientResourceRef

  // getReadOnly returns if the state is read-only.
  getReadOnly(): boolean

  // getSeqno returns the current seqno of the world state.
  // This is also the sequence number of the most recent change.
  // Initializes at 0 for initial world state.
  getSeqno(abortSignal?: AbortSignal): Promise<{ seqno: bigint }>

  // waitSeqno waits for the seqno of the world state to be >= value.
  // Returns the seqno when the condition is reached.
  // If value == 0, this might return immediately unconditionally.
  waitSeqno(
    seqno: bigint,
    abortSignal?: AbortSignal,
  ): Promise<{ seqno: bigint }>

  // buildStorageCursor builds a cursor to the world storage with an empty ref.
  // The cursor should be released independently of the WorldState.
  // Be sure to call Release on the cursor when done.
  buildStorageCursor(abortSignal?: AbortSignal): Promise<BucketLookupCursor>

  // accessWorldState builds a bucket lookup cursor with an optional ref.
  // If the ref is empty, returns a cursor pointing to the root world state.
  // The lookup cursor will be released after cb returns.
  accessWorldState(
    ref?: ObjectRef,
    abortSignal?: AbortSignal,
  ): Promise<BucketLookupCursor>

  // stageWorldState opens a staging scope for writes outside a transaction.
  // Build through the stage, adopt the result through this state, then
  // release the stage after the adopting call returns. A write transaction's
  // stage writes through the transaction. A read-only state rejects it.
  stageWorldState(abortSignal?: AbortSignal): Promise<WorldStage>

  /** openNestedWorld opens a typed outer object's nested World as a read-only state that follows the object's revisions. Dispose of it independently. */
  openNestedWorld(
    objectKey: string,
    abortSignal?: AbortSignal,
  ): Promise<IWorldState & Disposable>

  /** openOuterWorld grants the enclosing Space World under the same authority. Dispose of it independently. */
  openOuterWorld(abortSignal?: AbortSignal): Promise<IWorldState & Disposable>

  // createObject creates a object with a key and initial root ref.
  // Returns ErrObjectExists if the object already exists.
  // Appends a OBJECT_SET change to the changelog.
  createObject(
    key: string,
    rootRef: ObjectRef,
    abortSignal?: AbortSignal,
  ): Promise<IObjectState>

  // getObject looks up an object by key.
  // Returns null if not found.
  getObject(
    key: string,
    abortSignal?: AbortSignal,
  ): Promise<IObjectState | null>

  // iterateObjects returns an iterator with the given object key prefix.
  // The prefix is NOT clipped from the output keys
  // Keys are returned in sorted order.
  // Must call Next() or Seek() before the iterator is valid.
  // Call Close when done with the iterator.
  // Any init errors will be available via the iterator's Err() method.
  iterateObjects(
    prefix?: string,
    reversed?: boolean,
    abortSignal?: AbortSignal,
  ): Promise<ObjectIterator>

  // renameObject renames an object key and associated graph quads.
  renameObject(
    oldKey: string,
    newKey: string,
    options?: RenameObjectOptions,
  ): Promise<IObjectState>

  // deleteObject deletes an object and associated graph quads by ID.
  // Calls DeleteGraphObject internally.
  // Returns deleted=false if not found.
  deleteObject(
    key: string,
    abortSignal?: AbortSignal,
  ): Promise<{ deleted: boolean }>

  // setGraphQuad sets a quad in the graph store.
  // Subject: must be an existing object IRI: <object-key>
  // Predicate: a predicate string, e.g. IRI: <ref>
  // Object: an existing object IRI: <object-key>
  // If already exists, returns nil.
  setGraphQuad(
    subject: string,
    predicate: string,
    obj: string,
    label?: string,
    abortSignal?: AbortSignal,
  ): Promise<void>

  // deleteGraphQuad deletes a quad from the graph store.
  // Note: if quad did not exist, returns nil.
  deleteGraphQuad(
    subject: string,
    predicate: string,
    obj: string,
    label?: string,
    abortSignal?: AbortSignal,
  ): Promise<void>

  // lookupGraphQuads searches for graph quads in the store.
  // If the filter fields are empty, matches any for that field.
  // If not found, returns empty list.
  // If limit is set, stops after finding that number of matching quads.
  lookupGraphQuads(
    subject?: string,
    predicate?: string,
    obj?: string,
    label?: string,
    limit?: number,
    abortSignal?: AbortSignal,
  ): Promise<LookupGraphQuadsResponse>

  // listGraphEdgeBuckets lists grouped inbound/outbound graph edges for origin
  // object keys.
  // The response contains one bucket per requested origin in request order.
  listGraphEdgeBuckets(
    originObjectKeys: string[],
    limitPerOrigin: number,
    options?: ListGraphEdgeBucketsOptions,
  ): Promise<ListGraphEdgeBucketsResponse>

  /**
   * listObjectsWithType collects the matching keys in stable graph-index
   * order: every key, or the first limit keys when limit is positive.
   */
  listObjectsWithType(
    typeID: string,
    abortSignal?: AbortSignal,
    limit?: number,
  ): Promise<string[]>

  /**
   * listObjectsWithTypePage returns one page of the type's keys after
   * startAfter, with each object's title and state when declared is set and
   * the type declares them. An empty title means the client shows the key.
   */
  listObjectsWithTypePage(
    typeID: string,
    page: ListObjectsWithTypePage,
    abortSignal?: AbortSignal,
  ): Promise<ListObjectsWithTypeResponse>

  /**
   * searchObjectsWithType streams the objects of a type whose key or title
   * holds the query, ignoring case, one page per response. The server walks
   * the type only as far as the pages read, so stop iterating to stop it.
   */
  searchObjectsWithType(
    typeID: string,
    query: string,
    pageSize?: number,
    abortSignal?: AbortSignal,
  ): AsyncIterable<SearchObjectsWithTypeResponse>

  /**
   * listTypeCounts returns the number of objects of each type, sorted by type
   * ID, each marked internal when its type is bookkeeping a browser hides.
   */
  listTypeCounts(abortSignal?: AbortSignal): Promise<TypeCountEntry[]>

  /**
   * watchTypeCounts streams the type counts: every count first, then the
   * counts each change moves, with a zero count for a type whose last object
   * went.
   */
  watchTypeCounts(
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchTypeCountsResponse>

  /**
   * listObjects returns one page of objects under the request prefix in key
   * order, starting after its cursor. A delimiter groups deeper keys into
   * prefixes. The server may return fewer entries than the limit and sets more
   * when entries remain.
   */
  listObjects(
    request: ListObjectsRequest,
    abortSignal?: AbortSignal,
  ): Promise<ListObjectsResponse>

  /**
   * getObjectMetadataBatch returns the type and parent of each key, in request
   * order, and its title, state and time when declared is set and its type
   * declares them.
   */
  getObjectMetadataBatch(
    objectKeys: string[],
    abortSignal?: AbortSignal,
    declared?: boolean,
  ): Promise<ObjectMetadata[]>

  /** getObjectBodies reads object root bodies from one World revision, in request order. */
  getObjectBodies(
    objectKeys: string[],
    abortSignal?: AbortSignal,
  ): Promise<ObjectBody[]>

  // deleteGraphObject deletes all quads with Subject or Object set to value.
  // Note: value should be the object key, NOT the object key <iri> format.
  deleteGraphObject(objectKey: string, abortSignal?: AbortSignal): Promise<void>

  // setObjectType, when present, sets an object's one type in one write:
  // it creates the type's object when absent and removes any other type.
  // setObjectType in types.ts uses it in place of its separate writes.
  setObjectType?(
    objectKey: string,
    typeID: string,
    abortSignal?: AbortSignal,
  ): Promise<void>

  // applyWorldOp applies a batch operation at the world level.
  // The handling of the operation is operation-type specific.
  // Returns the seqno after the operation executes.
  // Throws WorldOperationRejection when the operation refuses the change.
  // If sysErr is set, the error is treated as a transient system error.
  // Must support recursive calls to ApplyWorldOp / ApplyObjectOp.
  applyWorldOp(
    opTypeId: string,
    opData: Uint8Array,
    opSender: string,
    abortSignal?: AbortSignal,
  ): Promise<{ seqno: bigint; sysErr: boolean }>

  // accessTypedObject looks up an object, determines its type via graph quad,
  // and returns access to a typed resource that implements the type-specific
  // RPC service.
  // The returned resourceId can be used with resourceRef.createRef() to access
  // the typed resource.
  // For example, an ObjectLayout object returns access to a LayoutHost
  // resource.
  accessTypedObject(
    objectKey: string,
    abortSignal?: AbortSignal,
  ): Promise<TypedObjectAccess>
}

// WorldStateResource represents the full state read/write interface to the world.
// WorldState implements all world state operations.
// Provides:
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

  // getReadOnly returns if the transaction is read-only.
  // Returns stored metadata without RPC call.
  public getReadOnly(): boolean {
    return this.readOnly
  }

  // WorldWaitSeqno implementation

  // getSeqno returns the current sequence number of the world state.
  // This is also the sequence number of the most recent change.
  // Initializes at 0 for initial world state.
  public async getSeqno(abortSignal?: AbortSignal): Promise<{ seqno: bigint }> {
    const response = await this.service.GetSeqno({}, abortSignal)
    return { seqno: response.seqno ?? 0n }
  }

  // waitSeqno waits for the world state sequence number to reach or exceed the
  // specified value.
  // Returns the seqno when the condition is reached.
  // If seqno == 0, this might return immediately unconditionally.
  public async waitSeqno(
    seqno: bigint,
    abortSignal?: AbortSignal,
  ): Promise<{ seqno: bigint }> {
    const response = await this.service.WaitSeqno({ seqno }, abortSignal)
    return { seqno: response.seqno ?? 0n }
  }

  /** compareObjectRecords compares previous immutable roots with this snapshot. */
  public async compareObjectRecords(
    bases: ObjectRecordBase[],
    signal?: AbortSignal,
  ) {
    return this.service.CompareObjectRecords({ bases }, signal)
  }

  /** getObjectRootRefs returns existence, revision, and root from this snapshot. */
  public async getObjectRootRefs(
    objectKeys: string[],
    abortSignal?: AbortSignal,
  ) {
    const response = await this.service.GetObjectRootRefsBatch(
      { objectKeys },
      abortSignal,
    )
    return response.rootRefs ?? []
  }

  // WorldStorage implementation

  // buildStorageCursor builds a cursor to the world storage with an empty ref.
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

  // accessWorldState builds a bucket lookup cursor with an optional ref.
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

  // stageWorldState opens a staging scope for writes outside a transaction.
  // Release the stage after the call that adopts its build returns.
  public async stageWorldState(abortSignal?: AbortSignal): Promise<WorldStage> {
    const response = await this.service.StageWorldState({}, abortSignal)
    return this.resourceRef.createResource(response.resourceId ?? 0, WorldStage)
  }

  /** openNestedWorld grants a read-only Engine over a typed object's nested World that follows the object's revisions. Dispose of it independently. */
  public async openNestedWorld(
    objectKey: string,
    abortSignal?: AbortSignal,
  ): Promise<EngineWorldState> {
    const response = await this.service.OpenNestedWorld(
      { objectKey },
      abortSignal,
    )
    const engine = this.resourceRef.createResource(
      response.resourceId ?? 0,
      Engine,
    )
    return new EngineWorldState(engine, false, true)
  }

  /** openOuterWorld grants the enclosing Space World under the same authority. Dispose of it independently. */
  public async openOuterWorld(
    abortSignal?: AbortSignal,
  ): Promise<EngineWorldState> {
    const response = await this.service.OpenOuterWorld({}, abortSignal)
    const engine = this.resourceRef.createResource(
      response.resourceId ?? 0,
      Engine,
    )
    return new EngineWorldState(engine, true, true)
  }

  // WorldStateObject implementation

  // createObject creates a new object in the world with the specified key and
  // initial data.
  // Returns ErrObjectExists if the object already exists.
  // Appends a OBJECT_SET change to the changelog.
  // Returns an ObjectState resource for the created object.
  public async createObject(
    key: string,
    rootRef: ObjectRef,
    abortSignal?: AbortSignal,
  ): Promise<ObjectState> {
    const response = await this.service.CreateObject(
      { objectKey: key, rootRef, knownErrorCode: KNOWN_ERROR_CODE },
      abortSignal,
    )
    throwOperationError(response)
    return this.resourceRef.createResource(
      response.resourceId ?? 0,
      ObjectState,
      { objectKey: response.objectKey },
    )
  }

  // getObject retrieves an object from the world by its key.
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

  // iterateObjects returns an iterator with the given object key prefix.
  // The prefix is NOT clipped from the output keys.
  // Keys are returned in sorted order.
  // Must call Next() or Seek() before the iterator is valid.
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

  // renameObject renames an object key and associated graph quads.
  public async renameObject(
    oldKey: string,
    newKey: string,
    options?: RenameObjectOptions,
  ): Promise<ObjectState> {
    const response = await this.service.RenameObject(
      {
        oldObjectKey: oldKey,
        newObjectKey: newKey,
        descendants: options?.descendants,
        knownErrorCode: KNOWN_ERROR_CODE,
      },
      options?.abortSignal,
    )
    throwOperationError(response)
    return this.resourceRef.createResource(
      response.resourceId ?? 0,
      ObjectState,
      { objectKey: response.objectKey },
    )
  }

  // deleteObject removes an object and all associated graph quads from the
  // world.
  // Calls DeleteGraphObject internally.
  // Returns deleted=false if not found.
  public async deleteObject(
    key: string,
    abortSignal?: AbortSignal,
  ): Promise<{ deleted: boolean }> {
    const response = await this.service.DeleteObject(
      { objectKey: key, knownErrorCode: KNOWN_ERROR_CODE },
      abortSignal,
    )
    throwOperationError(response)
    return { deleted: response.deleted ?? false }
  }

  // WorldStateGraph implementation

  // setGraphQuad adds or updates a quad in the graph store.
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
    const response = await this.service.SetGraphQuad(
      {
        quad: { subject, predicate, obj, label },
        knownErrorCode: KNOWN_ERROR_CODE,
      },
      abortSignal,
    )
    throwOperationError(response)
  }

  // deleteGraphQuad removes a specific quad from the graph store.
  // Note: if quad did not exist, returns nil.
  public async deleteGraphQuad(
    subject: string,
    predicate: string,
    obj: string,
    label?: string,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    const response = await this.service.DeleteGraphQuad(
      {
        quad: { subject, predicate, obj, label },
        knownErrorCode: KNOWN_ERROR_CODE,
      },
      abortSignal,
    )
    throwOperationError(response)
  }

  // lookupGraphQuads searches for graph quads matching the specified filter
  // criteria.
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

  // listGraphEdgeBuckets lists grouped inbound/outbound graph edges for origin
  // object keys.
  public async listGraphEdgeBuckets(
    originObjectKeys: string[],
    limitPerOrigin: number,
    options: ListGraphEdgeBucketsOptions = {},
  ): Promise<ListGraphEdgeBucketsResponse> {
    return await this.service.ListGraphEdgeBuckets(
      {
        originObjectKeys,
        predicate: options.predicate,
        limitPerOrigin,
        direction: options.direction,
        after: options.after,
        reverse: options.reverse,
      },
      options.abortSignal,
    )
  }

  /**
   * getObjectBodies reads object root bodies from one World revision, in
   * request order. A body reports exists=false for a missing object. The
   * server streams large responses in pages. A write between pages restarts
   * the read. A body too large for one page fails the read.
   */
  public async getObjectBodies(
    objectKeys: string[],
    abortSignal?: AbortSignal,
  ): Promise<ObjectBody[]> {
    if (objectKeys.length === 0) return []
    for (
      let attempt = 0;
      attempt <= maxObjectBodiesRevisionRetries;
      attempt++
    ) {
      const bodies = await this.readObjectBodyPages(objectKeys, abortSignal)
      if (bodies) return bodies
    }
    throw new Error('The World changed on every attempt to read object bodies.')
  }

  // readObjectBodyPages returns null when the World revision changes between pages.
  private async readObjectBodyPages(
    objectKeys: string[],
    abortSignal?: AbortSignal,
  ): Promise<ObjectBody[] | null> {
    // Collect the pages, giving up if any page reads another World revision.
    const bodies: ObjectBody[] = []
    let worldSeqno: bigint | undefined
    for await (const page of this.getObjectBodyPages(objectKeys, abortSignal)) {
      const pageSeqno = page.worldSeqno ?? 0n
      worldSeqno ??= pageSeqno
      if (pageSeqno !== worldSeqno) return null
      for (const body of page.bodies ?? []) {
        if (body.withheldSize) {
          throw new Error(
            `Object ${body.objectKey} has a ${body.withheldSize}-byte body, too large for one page.`,
          )
        }
        bodies.push(body)
      }
    }
    return bodies
  }

  /**
   * getObjectBodyPages streams object root bodies in request order, one
   * budgeted page at a time, each with the World seqno it read. A body over
   * the page budget arrives withheld, with withheldSize set and no body. On a read
   * transaction every page reads the transaction's one revision.
   */
  public getObjectBodyPages(
    objectKeys: string[],
    abortSignal?: AbortSignal,
  ): AsyncIterable<GetObjectBodiesBatchResponse> {
    return this.service.GetObjectBodiesBatch({ objectKeys }, abortSignal)
  }

  /** listObjectsWithType collects the matching keys in stable graph-index order. */
  public async listObjectsWithType(
    typeID: string,
    abortSignal?: AbortSignal,
    limit = 0,
  ): Promise<string[]> {
    // Walk bounded service pages, resuming from each last subject.
    const keys: string[] = []
    let startAfter = ''
    for (;;) {
      // Request one typed page, no larger than the keys still wanted.
      const pageLimit = limit > 0 ? Math.min(limit - keys.length, 1000) : 1000
      const response = await this.service.ListObjectsWithType(
        { typeId: typeID, startAfter, limit: pageLimit },
        abortSignal,
      )
      const page = response.objectKeys ?? []
      keys.push(...page)

      // Stop at the limit or the end of the type, or resume from the last key.
      if (!response.more || (limit > 0 && keys.length >= limit)) return keys
      startAfter = page[page.length - 1]!
    }
  }

  /** listObjectsWithTypePage returns one page of the type's keys, titles and states. */
  public async listObjectsWithTypePage(
    typeID: string,
    page: ListObjectsWithTypePage,
    abortSignal?: AbortSignal,
  ): Promise<ListObjectsWithTypeResponse> {
    return await this.service.ListObjectsWithType(
      { typeId: typeID, ...page },
      abortSignal,
    )
  }

  /** searchObjectsWithType streams pages of the type's matching objects. */
  public searchObjectsWithType(
    typeID: string,
    query: string,
    pageSize = 0,
    abortSignal?: AbortSignal,
  ): AsyncIterable<SearchObjectsWithTypeResponse> {
    return this.service.SearchObjectsWithType(
      { typeId: typeID, query, pageSize },
      abortSignal,
    )
  }

  /** listTypeCounts returns the number of objects of each type. */
  public async listTypeCounts(
    abortSignal?: AbortSignal,
  ): Promise<TypeCountEntry[]> {
    const response = await this.service.ListTypeCounts({}, abortSignal)
    return response.typeCounts ?? []
  }

  /** watchTypeCounts streams the type counts and then the moved counts. */
  public watchTypeCounts(
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchTypeCountsResponse> {
    return this.service.WatchTypeCounts({}, abortSignal)
  }

  /** listObjects returns one page of objects under prefix in key order. */
  public async listObjects(
    request: ListObjectsRequest,
    abortSignal?: AbortSignal,
  ): Promise<ListObjectsResponse> {
    return this.service.ListObjects(request, abortSignal)
  }

  /** getObjectMetadataBatch returns the type, parent and declarations of each key, in request order. */
  public async getObjectMetadataBatch(
    objectKeys: string[],
    abortSignal?: AbortSignal,
    declared = false,
  ): Promise<ObjectMetadata[]> {
    const response = await this.service.GetObjectMetadataBatch(
      { objectKeys, declared },
      abortSignal,
    )
    return response.metadata ?? []
  }

  // deleteGraphObject removes all graph quads that reference the specified
  // object key.
  // Note: objectKey should be the object key, NOT the object key <iri> format.
  public async deleteGraphObject(
    objectKey: string,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    const response = await this.service.DeleteGraphObject(
      { objectKey, knownErrorCode: KNOWN_ERROR_CODE },
      abortSignal,
    )
    throwOperationError(response)
  }

  // WorldStateOp implementation

  // applyWorldOp applies a batch operation at the world level.
  // The handling of the operation is operation-type specific.
  // Returns the seqno after the operation executes.
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
      { opTypeId, opData, opSender, knownErrorCode: KNOWN_ERROR_CODE },
      abortSignal,
    )
    throwOperationError(response)
    return { seqno: response.seqno ?? 0n, sysErr: response.sysErr ?? false }
  }

  // TypedObjectResourceService implementation

  // accessTypedObject looks up an object, determines its type via graph quad,
  // and returns access to a typed resource that implements the type-specific
  // RPC service.
  // The returned resourceId can be used with resourceRef.createRef() to access
  // the typed resource.
  // For example, an ObjectLayout object returns access to a LayoutHost
  // resource.
  public async accessTypedObject(
    objectKey: string,
    abortSignal?: AbortSignal,
  ): Promise<TypedObjectAccess> {
    // Acquire a typed child through this exact granting World mount.
    const typedService = new TypedObjectResourceServiceClient(
      this.resourceRef.client,
    )
    const response = await typedService.AccessTypedObject(
      { objectKey },
      abortSignal,
    )

    // Restore typed acquisition failures before exposing a child reference.
    throwTypedObjectError(response, abortSignal)
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

// Tx represents a transaction against the world state.
// Tx implements the world state transaction interfaces.
// Provides:
// - WorldState: full state read/write interface (inherited from WorldStateResource)
// - tx.Tx: Commit, Discard operations
//
// A Tx maintains state across multiple RPC calls, enabling complex multi-step
// operations within a single transaction. Always call discard() when done.
//
// Concurrent calls to WorldState functions should be supported.
//
// Tx lives beside WorldStateResource because engine.ts and engine-state.ts
// import this module: a subclass in another module of that cycle could be
// evaluated before its base class exists.
export class Tx extends WorldStateResource {
  private txService: TxResourceServiceClient

  constructor(resourceRef: ClientResourceRef, meta?: { readOnly?: boolean }) {
    super(resourceRef, meta)
    this.txService = new TxResourceServiceClient(resourceRef.client)
  }

  // Transaction operations (tx.Tx interface)

  // commit commits the transaction.
  // After commit, the transaction should be discarded.
  // A commit the World refused throws WorldCommitError. Its superseded field
  // says whether another writer advanced the World first. A commit whose
  // outcome is unknown throws DurabilityUnknownError. A commit with an unknown
  // outcome must never be sent again.
  public async commit(abortSignal?: AbortSignal): Promise<void> {
    throwCommitError(
      await this.txService.CommitTx(
        { knownErrorCode: KNOWN_ERROR_CODE },
        abortSignal,
      ),
    )
  }

  // discard discards the transaction without committing changes.
  // Always call this when done with the transaction.
  public async discard(abortSignal?: AbortSignal): Promise<void> {
    await this.txService.Discard({}, abortSignal)
  }
}
