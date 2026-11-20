import { LuCamera } from 'react-icons/lu'

import type { Camera } from '@s4wave/sdk/camera/camera.pb.js'
import type { Device } from '@s4wave/sdk/device/device.pb.js'
import { ViewerBar } from '@s4wave/web/object/ViewerBar.js'
import {
  ViewerStatus,
  type ViewerStatusProps,
} from '@s4wave/web/object/ViewerStatus.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'

import { FrameView } from '../frame/FrameView.js'
import type { FrameSource } from '../frame/play-frames.js'

// CameraViewState is what the Camera viewer draws, derived from its sources
// as FrameViewer.tla models them: the handle's status in place of the
// picture, a wait while the Device is offline, or the frame track once the
// Device is online.
export type CameraViewState =
  | { body: 'status'; status: ViewerStatusProps }
  | { body: 'offline'; camera: Camera; device?: Device }
  | { body: 'picture'; camera: Camera; device?: Device }

export interface CameraViewProps {
  // view is the derived state to draw.
  view: CameraViewState
  // source opens the frame track while the Device is online.
  source?: FrameSource
}

// CameraView shows a Camera: one bar with its name and Device, and under it
// the live picture, or what the viewer waits for.
export function CameraView({ view, source }: CameraViewProps) {
  // The Camera and its Device, once the watch has said them.
  const camera = view.body === 'status' ? undefined : view.camera
  const device = view.body === 'status' ? undefined : view.device
  return (
    <div className="bg-background-primary flex h-full w-full flex-col">
      <ViewerBar
        icon={<LuCamera />}
        title={camera?.name || 'Camera'}
        description={
          device?.label ? (
            <>
              on <span>{device.label}</span>
            </>
          ) : undefined
        }
      />
      {renderBody(view, source)}
    </div>
  )
}

// renderBody draws the viewer body for one state.
function renderBody(view: CameraViewState, source?: FrameSource) {
  switch (view.body) {
    case 'status':
      return <ViewerStatus {...view.status} />
    case 'offline':
      return (
        <div className="relative flex min-h-0 flex-1 items-center justify-center bg-black">
          <LoadingCard
            className="w-full max-w-sm"
            view={{
              state: 'active',
              title: 'Device offline',
              detail: 'The picture reconnects when the Device returns.',
            }}
          />
        </div>
      )
    case 'picture':
      return (
        <FrameView
          source={source}
          label={`${view.camera.name || 'Camera'} picture`}
        />
      )
  }
}
