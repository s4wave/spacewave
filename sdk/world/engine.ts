import { ObjectRef } from '@go/github.com/s4wave/spacewave/db/bucket/bucket.pb.js'
import { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'
import {
  EngineResourceService,
  EngineResourceServiceClient,
  WatchWorldStateResourceService,
  WatchWorldStateResourceServiceClient,
  TypedObjectResourceServiceClient,
} from './world_srpc.pb.js'
import { Tx } from './tx.js'
import { BucketLookupCursor } from '../bucket/lookup/lookup.js'
import { WorldStateResource, type TypedObjectAccess } from './world-state.js'

// Engine is the top-level resource for Hydra's World data structure.
// Engine implements a transactional world state container.
//
// In the Go implementation (hydra/world/engine.go), Engine provides:
// - NewTransaction(ctx, write bool) (Tx, error)
// - WorldStorage for bucket cursor access (BuildStorageCursor, AccessWorldState)
// - WorldWaitSeqno for sequence number waiting (GetSeqno, WaitSeqno)
// - WatchWorldState for reactive change tracking (via WatchWorldStateResourceService)
//
// This TypeScript implementation wraps EngineResourceService and WatchWorldStateResourceService.
export class Engine extends Resource {
  private service: EngineResourceService
  private watchService: WatchWorldStateResourceService

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

  // NewTransaction creates a new transaction against the world state.
  // Set write=true if the transaction will perform write operations.
  // Always call discard() when done with the transaction.
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

  // accessTypedObject looks up an object, determines its type via graph quad,
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

  // WatchWorldState creates a reactive watch that tracks WorldState accesses
  // and re-executes the callback whenever tracked resources change.
  //
  // The callback receives:
  // - worldState: A tracked WorldState resource
  // - abortSignal: Signal that triggers when changes detected or watch stops
  // - cleanup: Function to register disposable resources
  //
  // Returns a cleanup function to stop the watch.
  //
  // As the callback accesses resources, server-side tracking starts immediately.
  // Change detection begins as soon as first access is recorded. When changes occur,
  // the server sends a new resource_id and the callback is re-executed.
  public watchWorldState<T = void>(
    callback: (
      worldState: WorldStateResource,
      abortSignal: AbortSignal,
      cleanup: <R extends Disposable | null | undefined>(resource: R) => R,
    ) => Promise<T>,
  ): () => void {
    return createWatchWorldState(this.watchService, this.resourceRef, callback)
  }
}

// createWatchWorldState implements the reactive watch pattern for WorldState changes.
// This is a common implementation that can be used by any class that has access to
// WatchWorldStateResourceService and can create WorldStateResource instances.
function createWatchWorldState<T = void>(
  watchService: WatchWorldStateResourceService,
  resourceRef: ClientResourceRef,
  callback: (
    worldState: WorldStateResource,
    abortSignal: AbortSignal,
    cleanup: <R extends Disposable | null | undefined>(resource: R) => R,
  ) => Promise<T>,
): () => void {
  let cancelled = false
  const cleanupResources: Disposable[] = []

  // Create abort controller for entire watch
  const watchAbortController = new AbortController()

  const stopWatch = () => {
    cancelled = true
    watchAbortController.abort()
    cleanupResources.forEach((r) => r[Symbol.dispose]())
    cleanupResources.length = 0
  }

  // Start async watch loop
  void (async () => {
    try {
      // Start streaming RPC
      const stream = watchService.WatchWorldState(
        {},
        watchAbortController.signal,
      )

      for await (const response of stream) {
        if (cancelled) break

        // Clean up previous execution's resources
        cleanupResources.forEach((r) => r[Symbol.dispose]())
        cleanupResources.length = 0

        // Create abort controller for this execution
        const execAbortController = new AbortController()

        // Create WorldState resource from resource_id
        const trackedWorldState = resourceRef.createResource(
          response.resourceId ?? 0,
          WorldStateResource,
        )

        // Register cleanup function that returns the resource for chaining
        const cleanup = <R extends Disposable | null | undefined>(
          resource: R,
        ): R => {
          if (resource) {
            cleanupResources.push(resource)
          }
          return resource
        }

        try {
          // Execute callback
          // As the callback accesses resources, server-side tracking starts immediately
          // Change detection begins as soon as first access is recorded
          await callback(trackedWorldState, execAbortController.signal, cleanup)
        } catch (err) {
          // Handle errors in callback (could be abort errors, which are expected)
          if (err instanceof Error && err.name !== 'AbortError') {
            console.error('Error in watchWorldState callback:', err)
          }
        } finally {
          // Dispose tracked WorldState resource
          trackedWorldState[Symbol.dispose]()
        }

        // Server is now monitoring tracked resources for changes
        // When changes occur, server sends new resource_id
        // Loop continues with next iteration
      }
    } catch (err) {
      if (!cancelled && !(err instanceof Error && err.name === 'AbortError')) {
        console.error('WatchWorldState stream error:', err)
      }
    } finally {
      stopWatch()
    }
  })()

  return stopWatch
}
