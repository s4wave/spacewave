// What a resource hook reports for one child of one parent.

type Hook = {
  key: number
  parent: string
  status: string
  val: number
  valRel: boolean
  keepable: boolean
  last: number
  lastRel: boolean
}

export const title = 'Resource hooks: value, last and failure'

// reported is the hook's state as resourceOf builds it: an error wins.
const reported = (v: Hook) =>
  v.parent === 'failed' || v.status === 'failed'
    ? 'failed'
    : v.parent === 'loading' || v.status === 'loading'
      ? 'loading'
      : 'ready'

// lastOf is the value a failure keeps as last.
const lastOf = (v: Hook) =>
  v.val === 0 ? v.last : v.keepable && !v.valRel ? v.val : 0

export const state = (v: Hook) => {
  const r = reported(v)
  if (r === 'failed') {
    return lastOf(v) !== 0 ? 'Failed over last, Retry' : 'Failed, Retry'
  }
  if (v.val === 0) return r === 'ready' ? 'Ready, empty' : 'Loading'
  const kept = v.val !== v.key ? ', previous deps' : ''
  return r === 'ready' ? 'Ready' : `Loading over value${kept}`
}

export const step = (action: string, from: Hook, to: Hook) => {
  // A step that keeps the drawn state draws no edge.
  if (state(from) === state(to)) return null

  // Each action names the edge it draws.
  if (action === 'ParentReady') return 'parent ready'
  if (action === 'Start') return 'run starts'
  if (action === 'ParentLoad') return 'parent reloads'
  if (action === 'ParentFail') return 'parent fails'
  if (action === 'ParentRetry') return 'Retry the parent'
  if (action === 'Emit') return 'value arrives'
  if (action === 'EndEmpty') return 'source holds nothing'
  if (action === 'StreamFail') return 'run fails'
  if (action === 'Reopen') return 'backoff ends'
  if (action === 'Retry') return 'presses Retry'
  if (action === 'Release') return 'handle released'
  if (action === 'DepsChange') return 'deps change'
  return null
}
