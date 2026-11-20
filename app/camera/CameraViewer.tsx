import { useCallback, useMemo } from 'react'

import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { CameraHandle, CameraTypeID } from '@s4wave/sdk/camera/camera.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import {
  getObjectKey,
  type ObjectViewerComponentProps,
} from '@s4wave/web/object/object.js'
import { useObjectName } from '@s4wave/web/object/ObjectViewerContext.js'

import { frameObjectState } from '../frame/frame-object-state.js'
import type { FrameSource } from '../frame/play-frames.js'
import { CameraView } from './CameraView.js'

export { CameraTypeID }

/** CameraViewer shows a view-only picture from the Camera's Device. */
export function CameraViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  // Open the Camera handle the watch and frame stream share.
  const handle = useAccessTypedHandle(
    worldState,
    getObjectKey(objectInfo),
    CameraHandle,
    CameraTypeID,
  )

  // Follow the Camera and its Device in this pane.
  const streamFactory = useCallback(
    (h: CameraHandle, signal: AbortSignal) => h.watchCamera(signal),
    [],
  )
  const watch = useStreamingResource(handle, streamFactory, [])
  useObjectName(watch.value?.camera?.name)

  // The Camera's frame source has no input stream.
  const source = useMemo<FrameSource | undefined>(() => {
    const h = handle.value
    return h
      ? { kind: 'view', open: (signal) => h.watchFrames(signal) }
      : undefined
  }, [handle.value])
  const view = frameObjectState(
    handle,
    watch,
    'camera',
    watch.value?.camera,
    watch.value?.device,
  )
  return <CameraView view={view} source={source} />
}
