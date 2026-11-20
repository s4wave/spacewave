import { Codec } from '@s4wave/core/frametrack/frametrack.pb.js'
import type { Camera } from '@s4wave/sdk/camera/camera.pb.js'
import type { Device } from '@s4wave/sdk/device/device.pb.js'
import { useParams } from '@s4wave/web/router/router.js'

import {
  CameraView,
  type CameraViewState,
} from '@s4wave/app/camera/CameraView.js'
import type {
  FrameSource,
  FrameSourceEvent,
} from '@s4wave/app/frame/play-frames.js'

// camera is a Camera on a placed camera node of a Device.
const camera: Camera = {
  name: 'Workbench Camera',
  deviceObjectKey: 'devices/forge-a',
  capabilityId: 'flowgraph-node/flowgraphs/workbench/camera',
  createdAt: new Date(2026, 10, 19, 9, 41),
}

// device is the Device the Camera's capability is on.
const device: Device = { label: 'forge-a' }

// noop stands in for a Retry press.
const noop = () => {}

// aborted resolves once signal aborts, so a fixture track stays open.
function aborted(signal: AbortSignal): Promise<void> {
  return new Promise((resolve) =>
    signal.addEventListener('abort', () => resolve(), { once: true }),
  )
}

// testPicture encodes one JPEG test picture whose bar has moved by step.
async function testPicture(step: number): Promise<FrameSourceEvent> {
  const canvas = new OffscreenCanvas(640, 360)
  const ctx = canvas.getContext('2d')!
  ctx.fillStyle = '#20242c'
  ctx.fillRect(0, 0, 640, 360)
  ctx.fillStyle = '#e8a33d'
  ctx.fillRect((step * 8) % 640, 140, 80, 80)
  const blob = await canvas.convertToBlob({ type: 'image/jpeg' })
  return {
    kind: 'frame',
    header: { codec: Codec.JPEG },
    frame: { data: new Uint8Array(await blob.arrayBuffer()), keyframe: true },
  }
}

// sources are the fixture frame tracks, kept stable so the track runs once.
const sources: Record<string, FrameSource> = {
  // starting opens and sends no picture.
  async *starting(signal) {
    yield { kind: 'opened' }
    await aborted(signal)
  },
  // live sends a moving picture ten times a second.
  async *live(signal) {
    yield { kind: 'opened' }
    for (let step = 0; !signal.aborted; step++) {
      yield await testPicture(step)
      await new Promise((resolve) => setTimeout(resolve, 100))
    }
  },
  // stalled sends one picture and then none, so it pauses.
  async *stalled(signal) {
    yield { kind: 'opened' }
    yield await testPicture(30)
    await aborted(signal)
  },
  // failed is refused once opened.
  async *failed() {
    yield { kind: 'opened' }
    throw new Error('The Device refused the frame track.')
  },
}

// picture is the online state each track fixture shares.
const picture: CameraViewState = { body: 'picture', camera, device }

// views are the fixture states, each one production reaches.
const views: Record<string, CameraViewState> = {
  connecting: picture,
  starting: picture,
  live: picture,
  stalled: picture,
  failed: picture,
  offline: { body: 'offline', camera, device },
  waiting: {
    body: 'status',
    status: { state: 'waiting', what: 'the camera', rows: 0 },
  },
  'read-failed': {
    body: 'status',
    status: {
      state: 'failed',
      message: 'Could not open the camera: stream closed',
      retry: noop,
    },
  },
}

/**
 * CameraViewerDebug renders the Camera viewer in one fixture state, with the
 * viewport as the pane, for the viewer sweep: live (a moving test picture),
 * connecting, starting (opened, no picture yet), stalled (the last picture
 * held), failed (the track refused, with Retry), offline (the Device away),
 * waiting (the handle loading) and read-failed (the handle failed, with
 * Retry). An unknown or missing state shows live.
 */
export function CameraViewerDebug() {
  // Read the fixture state from the route.
  const { state = 'live' } = useParams()
  const key = state in views ? state : 'live'
  return (
    <div className="viewer-pane h-full w-full">
      <CameraView key={key} view={views[key]} source={sources[key]} />
    </div>
  )
}
