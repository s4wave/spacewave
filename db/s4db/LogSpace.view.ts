// LogSpaceState is the bounded log admission state.
type LogSpaceState = {
  active: number
  spare: number
  full: boolean
  room: boolean
  output: string
  length: number
  owner: number
}

// title states the full-disk guarantee checked here.
export const title = 'A backed log spare admits bounded full-disk reclamation'

// state shows both capacity and the producer whose cleanup is allowed.
export const state = (v: LogSpaceState) =>
  `owner ${v.owner}: ${v.output}, end ${v.length}, log ${v.active}, spare ${v.spare}, ${
    v.full ? 'disk full' : 'disk available'
  }, ${v.room ? 'tail room' : 'tail full'}`

// step names the responsible allocation transition.
export const step = (action: string) => action
