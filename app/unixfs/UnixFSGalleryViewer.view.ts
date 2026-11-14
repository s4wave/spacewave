// The gallery's image index watch, as drawn.

type Gallery = {
  path: string
  watch: string
  indexPath: string
  found: boolean
  truncated: boolean
  issues: boolean
  session: boolean
  space: boolean
}

export const title = 'Gallery: the image index watch'

export const state = (v: Gallery) => {
  // Until an index arrives, the wait or the failure takes the body.
  if (v.indexPath === 'none') {
    return v.watch === 'failed' ? 'Could not read, Retry shown' : 'Loading'
  }
  const body = v.found
    ? v.session && v.space
      ? 'Tiles'
      : 'Tiles, no previews note'
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
  if (action === 'RootFail') return 'folder handle fails'
  if (action === 'RootReload') return 'folder handle reloads'
  if (action === 'Detach') return 'session leaves'
  if (action === 'LeaveSpace') return 'outside a Space'
  if (action === 'Navigate') return 'opens another folder'
  if (action === 'Retry') return 'presses Retry'
  return null
}
