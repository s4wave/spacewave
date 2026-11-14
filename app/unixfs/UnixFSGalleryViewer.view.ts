// The gallery's image index watch, as drawn.

type Gallery = {
  watch: string
  kept: boolean
  found: boolean
  truncated: boolean
  issues: boolean
  session: boolean
}

export const title = 'Gallery: the image index watch'

export const state = (v: Gallery) => {
  // Until an index arrives, the wait or the failure takes the body.
  if (!v.kept) {
    return v.watch === 'failed' ? 'Could not read, Retry shown' : 'Loading'
  }
  const body = v.found
    ? v.session
      ? 'Tiles'
      : 'Tiles, no session note'
    : 'No images'
  const notes = [
    v.watch === 'reconnecting' ? 'reading again' : '',
    v.truncated ? 'limit note' : '',
    v.issues ? 'issues note' : '',
  ].filter(Boolean)
  return notes.length ? `${body}, ${notes.join(', ')}` : body
}

export const step = (action: string, from: Gallery, to: Gallery) => {
  // A step that keeps the drawn state draws no edge.
  if (state(from) === state(to)) return null

  // Each action names the edge it draws.
  if (action === 'Yield') return 'index arrives'
  if (action === 'Fail') return 'read fails'
  if (action === 'Drop') return 'watch drops'
  if (action === 'Detach') return 'session leaves'
  if (action === 'Retry') return 'presses Retry'
  return null
}
