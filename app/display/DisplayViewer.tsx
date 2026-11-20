import { useCallback, useMemo } from 'react'

import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { DisplayHandle, DisplayTypeID } from '@s4wave/sdk/display/display.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import {
  getObjectKey,
  type ObjectViewerComponentProps,
} from '@s4wave/web/object/object.js'
import { useObjectName } from '@s4wave/web/object/ObjectViewerContext.js'

import { FrameObjectBody } from '../frame/FrameObjectBody.js'
import { frameObjectState } from '../frame/frame-object-state.js'
import type { FrameSource } from '../frame/play-frames.js'

/** DisplayViewer shows one Device display and forwards live input. */
export function DisplayViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  // Open the Display handle the watch and live connection share.
  const handle = useAccessTypedHandle(
    worldState,
    getObjectKey(objectInfo),
    DisplayHandle,
    DisplayTypeID,
  )

  // Follow the Display's name and Device without persisting the peer ID.
  const streamFactory = useCallback(
    (h: DisplayHandle, signal: AbortSignal) => h.watchDisplay(signal),
    [],
  )
  const watch = useStreamingResource(handle, streamFactory, [])
  useObjectName(watch.value?.display?.name)

  // Input uses the same RPC as the pictures, only while the track is mounted.
  const source = useMemo<FrameSource | undefined>(() => {
    const h = handle.value
    return h
      ? {
          kind: 'control',
          open: (input, signal) => h.connectDisplay(input, signal),
        }
      : undefined
  }, [handle.value])
  const view = frameObjectState(
    handle,
    watch,
    'display',
    watch.value?.display,
    watch.value?.device,
  )
  return (
    <div className="bg-background-primary flex h-full w-full flex-col">
      <FrameObjectBody view={view} source={source} label="Display" />
    </div>
  )
}
