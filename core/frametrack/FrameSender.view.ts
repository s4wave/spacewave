// The Device's frame sender: what it does with each frame the source makes.

// Seq is a TLA+ sequence that arrives as an array, or as {} when empty.
type Seq = number[] | Record<string, never>

type Sender = { queue: Seq; open: boolean }

export const title = 'Frame sender: groups and keyframe wait'

const queued = (v: Sender) => Object.keys(v.queue).length

export const state = (v: Sender) => {
  const mode = v.open ? 'Group open' : 'Waiting for a keyframe'
  return `${mode}, ${queued(v)} queued`
}

export const step = (action: string, from: Sender, to: Sender) => {
  // A step that keeps the drawn state draws no edge.
  if (state(from) === state(to)) return null
  if (action === 'Write') return 'writes a frame'
  if (action !== 'Produce') return null
  if (to.open && !from.open) return 'keyframe opens a group'
  if (from.open && !to.open)
    return 'viewer behind: drops a frame, ends the group'
  return queued(to) > queued(from)
    ? 'queues a frame'
    : 'new keyframe resets the group'
}
