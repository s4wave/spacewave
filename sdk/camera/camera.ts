import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'

import { CameraResourceServiceClient } from './camera_srpc.pb.js'
import type { WatchCameraResponse, WatchFramesResponse } from './camera.pb.js'

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

  // watchFrames plays the Camera's frame track on its Device: Opened, then the
  // newest picture each time the caller reads one. It ends when the Device
  // closes the track and throws when the track fails; abortSignal ends it.
  public watchFrames(
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchFramesResponse> {
    return this.service.WatchFrames({}, abortSignal)
  }
}
