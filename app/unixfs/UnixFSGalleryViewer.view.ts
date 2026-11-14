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
  // A failed watch or handle draws its notice above the kept index, which
  // then makes no empty claim.
  const failing = v.watch === 'failed' || v.watch === 'capped'
  const body = v.found
    ? v.session && v.space
      ? 'Tiles'
      : 'Tiles, no previews note'
    : failing
      ? 'No tiles'
      : 'No images'
  const notes = [
    failing ? 'Could not read notice, Retry shown' : '',
    v.watch === 'reconnecting' || v.watch === 'waiting' ? 'reading again' : '',
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
  if (action === 'Cap') return 'backoff reaches its cap'
  if (action === 'RootFail') return 'folder handle fails'
  if (action === 'RootReload') return 'folder handle reloads'
  if (action === 'Detach') return 'session leaves'
  if (action === 'LeaveSpace') return 'outside a Space'
  if (action === 'Navigate') return 'opens another folder'
  if (action === 'Retry') return 'presses Retry'
  return null
}
