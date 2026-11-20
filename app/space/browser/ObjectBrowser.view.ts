// The Space object browser: types first, objects by title, relations to
// follow as a graph or a grouped list, search next to the data, and the key
// tree as an expert view.

type Browser = {
  view: string
  load: string
  conn: string
  target: string
  count: number
  rel: string
}

export const title = 'Space object browser'

// A type's objects, search results and the key tree are one kind of screen: a
// list of objects to open. Offline and a failed read are one: the screen says
// which, and offers Retry when the connection is up.
export const state = (v: Browser) => {
  if (v.conn === 'down' || v.load === 'failed')
    return 'Cannot load: offline or Retry'
  if (v.view === 'types') return 'Types and their counts'
  if (v.view === 'object')
    return v.target === 'deleted'
      ? 'Deleted by another writer'
      : `Object and its relations as a ${v.rel}`
  return 'Objects: of a type, search results or key tree'
}

export const step = (action: string, from: Browser, to: Browser) => {
  if (state(from) === state(to)) return null
  if (action === 'Disconnect') return 'connection drops'
  if (action === 'LoadFails') return 'read fails'
  if (action === 'Reconnect') return 'connection returns'
  if (action === 'OtherDeletes') return 'another writer deletes it'
  if (action !== 'Press') return null
  if (from.rel !== to.rel) return 'Graph or List'
  if (to.load === 'loading' && to.view === from.view) return 'Retry'
  if (to.view === 'types') return 'Back'
  if (to.view === 'object') return 'opens an object'
  if (from.view === 'object') return 'Back'
  return 'opens a type, searches or opens the key tree'
}
