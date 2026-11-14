import { ObjectRef } from '@go/github.com/s4wave/spacewave/db/bucket/bucket.pb.js'
import { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'
import { SharedStreams } from '@aptre/bldr-sdk/resource/shared-streams.js'
import {
  EngineResourceService,
  EngineResourceServiceClient,
  WatchWorldStateResourceService,
  WatchWorldStateResourceServiceClient,
  TypedObjectResourceServiceClient,
} from './world_srpc.pb.js'
import type { WorldMutation } from './world.pb.js'
import { BucketLookupCursor } from '../bucket/lookup/lookup.js'
import { WorldStage } from './stage.js'
import { throwTypedObjectError } from './typed-object-grant-retired-error.js'
import {
  Tx,
  WorldStateResource,
  type TypedObjectAccess,
} from './world-state.js'

// Engine is the top-level resource for the World data structure.
// Engine implements a transactional world state container.
// Provides:
// - NewTransaction(ctx, write bool) (Tx, error)
// - WorldStorage for bucket cursor access (BuildStorageCursor, AccessWorldState)
// - WorldWaitSeqno for sequence number waiting (GetSeqno, WaitSeqno)
// - WatchWorldState for reactive change tracking (via WatchWorldStateResourceService)
//
// This TypeScript implementation wraps EngineResourceService and WatchWorldStateResourceService.
export class Engine extends Resource {
  private service: EngineResourceService
  private watchService: WatchWorldStateResourceService
  // streams multicasts the keyed watches derived from this Engine.
  private readonly streams = new SharedStreams()

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new EngineResourceServiceClient(resourceRef.client)
    this.watchService = new WatchWorldStateResourceServiceClient(
      resourceRef.client,
    )
  }

  // getEngineInfo returns information about the world engine.
  public async getEngineInfo(abortSignal?: AbortSignal) {
    return await this.service.GetEngineInfo({}, abortSignal)
  }

  // sync fences committed changes to the engine's durable storage.
  public async sync(abortSignal?: AbortSignal): Promise<boolean> {
    const response = await this.service.Sync({}, abortSignal)
    return response.fenced ?? false
  }

  // getWorldRootSnapshot returns the current committed World root.
  public async getWorldRootSnapshot(abortSignal?: AbortSignal) {
    return await this.service.GetWorldRootSnapshot({}, abortSignal)
  }

  // watchWorldRootSnapshots streams committed World root snapshots.
  public watchWorldRootSnapshots(abortSignal?: AbortSignal) {
    return this.service.WatchWorldRootSnapshots({}, abortSignal)
  }

  // NewTransaction creates a new transaction against the world state.
  // Set write=true if the transaction will perform write operations.
  // Always call discard() when done with the transaction.
  // Read transactions retain one revision; EngineWorldState supplies live reads.
  // Note: Engine might return a read-only transaction even if write=true.
  public async newTransaction(
    write: boolean,
    abortSignal?: AbortSignal,
  ): Promise<Tx> {
    const response = await this.service.NewTransaction({ write }, abortSignal)
    return this.resourceRef.createResource(response.resourceId ?? 0, Tx, {
      readOnly: response.readOnly,
    })
  }

  /** executeWorldOp accepts a complete operation through World's stale-base retry owner. */
  public async executeWorldOp(
    opTypeId: string,
    opData: Uint8Array,
    abortSignal?: AbortSignal,
  ) {
    return this.service.ExecuteWorldOp({ opTypeId, opData }, abortSignal)
  }

  /**
   * executeWorldBatch applies the mutations in order in one write
   * transaction, held only inside the call, and commits it. A refused
   * mutation writes nothing; the response names it with failedIndex and its
   * code, and throwBatchError turns it into an error.
   */
  public async executeWorldBatch(
    mutations: WorldMutation[],
    abortSignal?: AbortSignal,
  ) {
    return this.service.ExecuteWorldBatch({ mutations }, abortSignal)
  }

  // GetSeqno returns the current sequence number of the world state.
  // This is also the sequence number of the most recent change.
  // Initializes at 0 for initial world state.
  public async getSeqno(abortSignal?: AbortSignal) {
    return await this.service.GetSeqno({}, abortSignal)
  }

  // WaitSeqno waits for the world state sequence number to reach or exceed the specified value.
  // Returns the seqno when the condition is reached.
  // If seqno == 0, this might return immediately unconditionally.
  public async waitSeqno(seqno: bigint, abortSignal?: AbortSignal) {
    return await this.service.WaitSeqno({ seqno }, abortSignal)
  }

  // BuildStorageCursor builds a cursor to the world storage with an empty ref.
  // The cursor should be released independently of the Engine.
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

  // stageWorldState opens a staging scope for writes outside a transaction.
  // Build through the stage, adopt the result in a transaction, then release
  // the stage after that transaction returns.
  public async stageWorldState(abortSignal?: AbortSignal): Promise<WorldStage> {
    const response = await this.service.StageWorldState({}, abortSignal)
    return this.resourceRef.createResource(response.resourceId ?? 0, WorldStage)
  }

  // accessTypedObject looks up an object, determines its type via graph quad,
  // and returns access to a typed resource that implements the type-specific RPC service.
  // The returned resourceId can be used with resourceRef.createRef() to access the typed resource.
  // For example, an ObjectLayout object returns access to a LayoutHost resource.
  public async accessTypedObject(
    objectKey: string,
    abortSignal?: AbortSignal,
  ): Promise<TypedObjectAccess> {
    // Acquire a typed child through this granting engine mount.
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

  // watchWorldState yields a read snapshot of the head, then a new one after
  // each commit that changes what was read through the latest snapshot. The
  // server records those reads, and a read made after the commit that changed
  // it still ends the wait. Once the next snapshot is yielded, the server
  // releases the previous one. The caller owns each snapshot and releases it
  // once its reads are done.
  public async *watchWorldState(
    abortSignal?: AbortSignal,
  ): AsyncIterable<WorldStateResource> {
    const stream = this.watchService.WatchWorldState({}, abortSignal)
    for await (const response of stream) {
      yield this.resourceRef.createResource(
        response.resourceId ?? 0,
        WorldStateResource,
      )
    }
  }

  // watchShared subscribes to the stream at key, opening it with open when no
  // subscriber of this Engine holds it. Every message must be a full snapshot.
  public watchShared<T>(
    key: string,
    open: (signal: AbortSignal) => AsyncIterable<T>,
    abortSignal?: AbortSignal,
  ): AsyncIterable<T> {
    return this.streams.watch(key, open, abortSignal)
  }
}
