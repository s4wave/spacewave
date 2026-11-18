// The references strip in Spacewave's UnixFS file viewer.

type Strip = { load: string; refs: string; hasMore: boolean }

export const title = 'UnixFS file viewer: references strip'

export const state = (v: Strip) => {
  if (v.load === 'loading') return 'Reading references'
  if (v.load === 'failed') return 'Read failed: Retry'
  const more = v.hasMore ? ', more exist' : ''
  if (v.refs === 'none') return `No references${more}`
  return `References listed${more}`
}

export const step = (action: string) => {
  switch (action) {
    case 'Loaded':
      return 'first page arrives'
    case 'Fail':
      return 'read fails'
    case 'Retry':
      return 'reader presses Retry'
    case 'More':
      return 'reader shows more'
    case 'Arrive':
      return 'a reference commits'
    default:
      // Open changes another tab, not this strip.
      return null
  }
}
