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
  // Until a canvas arrives, the wait or the failure takes the body; queued
  // edits wait off the stage for the next canvas.
  const queued = v.pending ? ', edits queued' : ''
  if (!v.kept) {
    if (v.handle === 'failed') return 'Could not open, Retry shown' + queued
    if (v.watch === 'failed') return 'Could not read, Retry shown' + queued
    return 'Loading the canvas' + queued
  }
  // A failed source draws its notice over the kept canvas, which makes no
  // empty claim; a source reading again draws a line.
  const failed =
    v.handle === 'failed'
      ? ', Could not open notice, Retry shown'
      : v.watch === 'capped'
        ? ', Could not read notice, Retry shown'
        : ''
  const stage = v.nodes
    ? 'Canvas'
    : failed
      ? 'Empty canvas'
      : 'Empty canvas, hint shown'
  const read =
    !failed && (v.watch === 'reconnecting' || v.handle === 'waiting')
      ? ', reconnecting line'
      : ''
  const edits = v.pending ? ', edits sending' : ''
  const sent = v.action === 'failed' ? ', send failed notice' : ''
  return stage + failed + read + edits + sent
}

export const step = (action: string, from: Canvas, to: Canvas) => {
  // A step that keeps the drawn state draws no edge.
  if (state(from) === state(to)) return null

  // Each action names the edge it draws.
  if (action === 'HandleOpen') return 'handle opens'
  if (action === 'HandleFail') return 'handle fails'
  if (action === 'HandleReload') return 'handle reloads'
  if (action === 'RetryHandle') return 'presses Retry'
  if (action === 'WatchArrive') return 'canvas arrives'
  if (action === 'WatchDrop') return 'watch drops'
  if (action === 'WatchCap') return 'backoff reaches its cap'
  if (action === 'WatchFail') return 'watch fails'
  if (action === 'RetryWatch') return 'retries the read'
  if (action === 'Edit') return 'edits'
  if (action === 'Saved') return 'edits saved'
  if (action === 'Dismiss') return 'dismisses'
  if (action === 'SendFail') return 'send fails'
  return null
}
