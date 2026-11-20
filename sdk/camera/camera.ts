import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'

import { CameraResourceServiceClient } from './camera_srpc.pb.js'
import type { WatchCameraResponse } from './camera.pb.js'

export { CameraTypeID, CreateCameraOpTypeID } from './camera_typeid.pb.js'

// CameraHandle represents a handle to a Camera resource.
export class CameraHandle extends Resource {
  private service: CameraResourceServiceClient

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new CameraResourceServiceClient(resourceRef.client)
  }

  // watchCamera streams the Camera with its Device's availability, whole on
  // each change.
  public watchCamera(
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchCameraResponse> {
    return this.service.WatchCamera({}, abortSignal)
  }
}
