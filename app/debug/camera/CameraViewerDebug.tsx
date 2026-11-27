import {
  Codec,
  type FrameEvent,
} from '@s4wave/core/frametrack/frametrack.pb.js'
import type { Device } from '@s4wave/sdk/device/device.pb.js'
import { useParams } from '@s4wave/web/router/router.js'

import { CameraView } from '@s4wave/app/camera/CameraView.js'
import type { FrameObjectState } from '@s4wave/app/frame/frame-object-state.js'
import type {
  FrameSource,
  OpenFrameTrack,
} from '@s4wave/app/frame/play-frames.js'

// name is the Camera shown in the debug fixture.
const name = 'Workbench Camera'

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
async function testPicture(step: number): Promise<FrameEvent> {
  const canvas = new OffscreenCanvas(640, 360)
  const ctx = canvas.getContext('2d')!
  ctx.fillStyle = '#20242c'
  ctx.fillRect(0, 0, 640, 360)
  ctx.fillStyle = '#e8a33d'
  ctx.fillRect((step * 8) % 640, 140, 80, 80)
  const blob = await canvas.convertToBlob({ type: 'image/jpeg' })
  return {
    state: {
      case: 'frame',
      value: {
        header: { codec: Codec.JPEG },
        frame: {
          data: new Uint8Array(await blob.arrayBuffer()),
          keyframe: true,
        },
      },
    },
  }
}

// tracks are the fixture frame tracks, kept stable so the track runs once.
const tracks: Record<string, OpenFrameTrack> = {
  // starting opens and sends no picture.
  async *starting(signal) {
    yield { state: { case: 'opened', value: {} } }
    await aborted(signal)
  },
  // live sends a moving picture ten times a second.
  async *live(signal) {
    yield { state: { case: 'opened', value: {} } }
    for (let step = 0; !signal.aborted; step++) {
      yield await testPicture(step)
      await new Promise((resolve) => setTimeout(resolve, 100))
    }
  },
  // stalled sends one picture and then none, so the track pauses.
  async *stalled(signal) {
    yield { state: { case: 'opened', value: {} } }
    yield await testPicture(30)
    await aborted(signal)
  },
  // failed is refused once opened.
  async *failed() {
    yield { state: { case: 'opened', value: {} } }
    throw new Error('The Device refused the frame track.')
  },
}

// sources bind the fixture tracks to view-only playback.
const sources = Object.fromEntries(
  Object.entries(tracks).map(([key, open]) => [
    key,
    { kind: 'view', open } satisfies FrameSource,
  ]),
)

// picture is the online state each track fixture shares.
const picture: FrameObjectState = { body: 'picture', name, device }

// views are the fixture states, each one production reaches.
const views: Record<string, FrameObjectState> = {
  connecting: picture,
  starting: picture,
  live: picture,
  stalled: picture,
  failed: picture,
  offline: { body: 'offline', name, device },
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
