// The Canvas viewer's handle, state watch and edits, as drawn.

type Canvas = {
  handle: string
  watch: string
  kept: boolean
  nodes: boolean
  pending: boolean
  action: string
}

export const title = 'Canvas viewer: handle, state watch and edits'

export const state = (v: Canvas) => {
  // Until a canvas arrives, the wait or the failure takes the body.
  if (!v.kept) {
    if (v.handle === 'failed') return 'Could not open, Retry shown'
    if (v.watch === 'failed') return 'Could not read, Retry shown'
    return 'Loading the canvas'
  }
  const stage = v.nodes ? 'Canvas' : 'Empty canvas, hint shown'
  const read =
    v.handle === 'failed'
      ? ', open failed notice with Retry'
      : v.watch === 'reconnecting'
        ? ', reconnecting line'
        : ''
  const edits = v.pending ? ', edits sending' : ''
  const sent = v.action === 'failed' ? ', send failed notice' : ''
  return stage + read + edits + sent
}

export const step = (action: string, from: Canvas, to: Canvas) => {
  // A step that keeps the drawn state draws no edge.
  if (state(from) === state(to)) return null

  // Each action names the edge it draws.
  if (action === 'HandleOpen') return 'handle opens'
  if (action === 'HandleFail') return 'handle fails'
  if (action === 'RetryHandle') return 'presses Retry'
  if (action === 'WatchArrive') return 'canvas arrives'
  if (action === 'WatchDrop') return 'watch drops'
  if (action === 'WatchFail') return 'watch fails'
  if (action === 'RetryWatch') return 'retries the read'
  if (action === 'Edit') return 'edits'
  if (action === 'Saved') return 'edits saved'
  if (action === 'SendFail') return 'send fails'
  return null
}
