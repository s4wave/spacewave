// The camera and display viewers' handle, Device presence and frame track,
// as drawn.

type Viewer = { handle: string; device: string; track: string }

export const title = 'Frame viewer: handle, Device and frame track'

const drawn: Record<string, string> = {
  connecting: 'Connecting to the Device',
  starting: 'Waiting for the first picture',
  live: 'Live picture',
  stalled: 'Last picture held, paused',
  failed: 'Track failed, Retry shown',
}

export const state = (v: Viewer) => {
  if (v.handle === 'waiting') return 'Opening'
  if (v.handle === 'failed') return 'Could not open, Retry shown'
  if (v.device === 'offline') return 'Device offline, reconnects on return'
  return drawn[v.track]
}

export const step = (action: string, from: Viewer, to: Viewer) => {
  // A step that keeps the drawn state draws no edge.
  if (state(from) === state(to)) return null
  if (action === 'HandleOpen') return 'handle opens'
  if (action === 'HandleFail') return 'handle fails'
  if (action === 'RetryHandle') return 'presses Retry'
  if (action === 'DeviceLeave') return 'Device goes offline'
  if (action === 'DeviceReturn') return 'Device returns'
  if (action === 'RetryTrack') return 'presses Retry'
  if (action !== 'TrackStep') return null
  if (to.track === 'failed') return 'track refused or broken'
  if (to.track === 'starting') return 'track opens'
  if (to.track === 'stalled') return 'no frames for a second'
  return from.track === 'stalled' ? 'frames resume' : 'first keyframe decodes'
}
