// A wizard's Finish control in one tab, beside the wizard and its object.

type Finish = {
  control: Record<string, string>
  wizard: boolean
  objects: number
}

export const title = 'Wizard Finish: one atomic list'

export const state = (v: Finish) => {
  // The chart follows tab a; tab b acts as the other writer.
  const c = v.control.a
  if (c === 'gone' || (!v.wizard && (c === 'idle' || c === 'failed'))) {
    return 'Wizard gone, Close tab shown'
  }
  if (c === 'idle') return 'Finish shown'
  if (c === 'finishing') return 'Finishing'
  if (c === 'failed') return 'Failed, says why, Retry shown'
  return 'Done, object opens'
}

export const step = (action: string, from: Finish, to: Finish) => {
  // A step that keeps the drawn state draws no edge.
  if (state(from) === state(to)) return null

  // Each action names the edge it draws.
  if (action === 'Press') return 'presses Finish or Retry'
  if (action === 'Commit') return 'list commits, or is refused as gone'
  if (action === 'Discard') return 'a write fails, list discarded'
  if (action === 'Reload') return 'tab reloads'
  if (action === 'OrphanEnds') return 'a list from before a reload ends'
  if (action === 'MoveIndex') return 'another writer moves the index'
  if (action === 'Watch') return null
  return null
}
