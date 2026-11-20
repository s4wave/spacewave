import { useCallback } from 'react'

import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { useObjectName } from '@s4wave/web/object/ObjectViewerContext.js'

import { CameraHandle, CameraTypeID } from '@s4wave/sdk/camera/camera.js'
import type { WatchCameraResponse } from '@s4wave/sdk/camera/camera.pb.js'
import { DeviceAvailabilityState } from '@s4wave/sdk/device/device.pb.js'

import { CameraView, type CameraViewState } from './CameraView.js'

export { CameraTypeID }

// CameraViewer shows a Camera object: its name and Device in the bar, and
// its picture while the Device is online.
export function CameraViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  // Open the typed handle the Camera watch reads.
  const handle = useAccessTypedHandle(
    worldState,
    getObjectKey(objectInfo),
    CameraHandle,
    CameraTypeID,
  )

  // Watch the Camera with its Device's availability.
  const streamFactory = useCallback(
    (h: CameraHandle, signal: AbortSignal) => h.watchCamera(signal),
    [],
  )
  const watch = useStreamingResource(handle, streamFactory, [])
  useObjectName(watch.value?.camera?.name)

  // Draw one state derived from the handle and the watch. The frame track
  // has no source until the Camera serves its frames.
  return <CameraView view={cameraViewState(handle, watch)} />
}

// cameraViewState derives what the viewer draws, as FrameViewer.tla models
// it. The handle's status takes the body until it opens and the watch
// reports. The frame track runs only while the Device is online, so a
// Device whose availability is unknown or offline is a wait, not a failure.
export function cameraViewState(
  handle: Resource<CameraHandle>,
  watch: Resource<WatchCameraResponse | null>,
): CameraViewState {
  // The handle opens first, and its failure is the viewer's.
  if (handle.error) {
    return {
      body: 'status',
      status: {
        state: 'failed',
        message: `Could not open the camera: ${handle.error.message}`,
        retry: handle.retry,
      },
    }
  }
  if (watch.error) {
    return {
      body: 'status',
      status: {
        state: 'failed',
        message: `Could not read the camera: ${watch.error.message}`,
        retry: watch.retry,
      },
    }
  }
  const camera = watch.value?.camera
  if (!handle.value || !camera) {
    return {
      body: 'status',
      status: { state: 'waiting', what: 'the camera', rows: 0 },
    }
  }

  // The Device's availability decides whether the track runs.
  const device = watch.value?.device
  if (device?.state !== DeviceAvailabilityState.ONLINE) {
    return { body: 'offline', camera, device: device?.device }
  }
  return { body: 'picture', camera, device: device.device }
}
