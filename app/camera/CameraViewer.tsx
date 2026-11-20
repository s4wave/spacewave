import { useCallback, useMemo } from 'react'

import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { useObjectName } from '@s4wave/web/object/ObjectViewerContext.js'

import { CameraHandle, CameraTypeID } from '@s4wave/sdk/camera/camera.js'
import type { WatchCameraResponse } from '@s4wave/sdk/camera/camera.pb.js'
import { DeviceAvailabilityState } from '@s4wave/sdk/device/device.pb.js'
import { Codec } from '@s4wave/core/frametrack/frametrack.pb.js'

import type { FrameSource } from '../frame/play-frames.js'
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

  // Play the frame track through the open handle.
  const source = useMemo(
    () => (handle.value ? cameraFrameSource(handle.value) : undefined),
    [handle.value],
  )

  // Draw one state derived from the handle and the watch.
  return <CameraView view={cameraViewState(handle, watch)} source={source} />
}

// frameMimeTypes maps the codecs a canvas can decode as a picture to their
// MIME types.
const frameMimeTypes: Partial<Record<Codec, string>> = {
  [Codec.JPEG]: 'image/jpeg',
}

// cameraFrameSource plays the Camera's frame track through its handle: the
// opened event, then each picture the Device sends, until signal aborts the
// RPC. A picture in a codec the viewer cannot draw fails the track.
function cameraFrameSource(handle: CameraHandle): FrameSource {
  return async function* (signal) {
    for await (const { state } of handle.watchFrames(signal)) {
      switch (state?.case) {
        case 'opened':
          yield { kind: 'opened' }
          break
        case 'frame': {
          const codec = state.value.header?.codec ?? Codec.UNKNOWN
          const mimeType = frameMimeTypes[codec]
          if (!mimeType) {
            throw new Error(
              `The camera sends ${Codec[codec]} pictures, which this viewer cannot show.`,
            )
          }
          yield {
            kind: 'frame',
            data: state.value.frame?.data ?? new Uint8Array(),
            mimeType,
          }
          break
        }
      }
    }
  }
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
