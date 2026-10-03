import { ObjectRef } from '@go/github.com/s4wave/spacewave/db/bucket/bucket.pb.js'
import { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'
import {
  WorldStageResourceService,
  WorldStageResourceServiceClient,
} from './world_srpc.pb.js'
import { BucketLookupCursor } from '../bucket/lookup/lookup.js'

// WorldStage builds cursors whose writes a staging scope owns.
//
// Blocks written through the stage stay alive until it is released, so a
// caller can build object data outside a World transaction and adopt it in a
// later one. Hold the stage until the transaction that references the built
// roots has returned, then release it. Blocks no committed parent references
// are reclaimed afterward.
export class WorldStage extends Resource {
  private service: WorldStageResourceService

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new WorldStageResourceServiceClient(resourceRef.client)
  }

  // buildStorageCursor builds a staged cursor to the world storage.
  // Release the cursor independently of the stage.
  public async buildStorageCursor(
    abortSignal?: AbortSignal,
  ): Promise<BucketLookupCursor> {
    const response = await this.service.BuildStorageCursor({}, abortSignal)
    return this.resourceRef.createResource(
      response.resourceId ?? 0,
      BucketLookupCursor,
    )
  }

  // accessWorldState builds a staged cursor with an optional ref.
  // If the ref is empty, returns a cursor pointing to the root world state.
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
}
