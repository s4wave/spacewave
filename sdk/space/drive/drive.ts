import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'

import { DriveResourceServiceClient } from './drive_srpc.pb.js'
import type { Drive, WatchDriveStateResponse } from './drive.pb.js'

// DriveTypeID is the object type ID for Drive objects.
export const DriveTypeID = 'spacewave/drive'

// DRIVE_OBJECT_KEY is the default Drive object key used by quickstart.
export const DRIVE_OBJECT_KEY = 'drive'

// INIT_DRIVE_OP_ID is the operation type ID for InitDriveOp.
export const INIT_DRIVE_OP_ID = 'space/world/init-drive'

// DriveHandle represents a handle to a Drive resource.
export class DriveHandle extends Resource {
  private service: DriveResourceServiceClient

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new DriveResourceServiceClient(resourceRef.client)
  }

  // watchDriveState streams Drive state changes.
  public async *watchDriveState(
    abortSignal?: AbortSignal,
  ): AsyncIterable<Drive | undefined> {
    const stream = this.service.WatchDriveState({}, abortSignal)
    for await (const resp of stream as AsyncIterable<WatchDriveStateResponse>) {
      yield resp.state
    }
  }
}
