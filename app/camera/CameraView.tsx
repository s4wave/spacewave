import { LuCamera } from 'react-icons/lu'

import { ViewerBar } from '@s4wave/web/object/ViewerBar.js'

import { FrameObjectBody } from '../frame/FrameObjectBody.js'
import type { FrameObjectState } from '../frame/frame-object-state.js'
import type { FrameSource } from '../frame/play-frames.js'

/** CameraViewProps declares the Camera's watched state and view-only source. */
export interface CameraViewProps {
  view: FrameObjectState
  source?: FrameSource
}

/** CameraView shows the Camera's Device and its picture or current wait. */
export function CameraView({ view, source }: CameraViewProps) {
  const name = view.body === 'status' ? undefined : view.name
  const device = view.body === 'status' ? undefined : view.device
  return (
    <div className="bg-background-primary flex h-full w-full flex-col">
      <ViewerBar
        icon={<LuCamera />}
        title={name || 'Camera'}
        description={
          device?.label ? (
            <>
              on <span>{device.label}</span>
            </>
          ) : undefined
        }
      />
      <FrameObjectBody view={view} source={source} label="Camera" />
    </div>
  )
}
