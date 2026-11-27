// A wizard's Finish control in one tab, beside the wizard and its object.

type Finish = {
  control: Record<string, string>
  wizard: boolean
  objects: number
}

export const title = 'Wizard Finish: one atomic list'

export const state = (v: Finish) => {
  // The chart follows tab a. Tab b acts as the other writer.
  const c = v.control.a
  if (
    c === 'gone' ||
    (!v.wizard && (c === 'idle' || c === 'failed' || c === 'unsupported'))
  ) {
    return 'Wizard gone, Close tab shown'
  }
  if (c === 'idle') return 'Finish shown'
  if (c === 'finishing') return 'Finishing'
  if (c === 'failed') return 'Failed, says why, Retry shown'
  if (c === 'unsupported')
    return 'No build for this device, says where to finish'
  return 'Done, object opens'
}

export const step = (action: string, from: Finish, to: Finish) => {
  // A step that keeps the drawn state draws no edge.
  if (state(from) === state(to)) return null

  // A refused press ends done only when the ending records the object
  // created by a list sent by an earlier press.
  const refused = ['Discard', 'Reject', 'Unsupported']
  if (refused.includes(action) && to.control.a === 'done') {
    return 'list refused, ending names the object'
  }

  // Each action labels the edge the action draws.
  if (action === 'Press') return 'presses Finish or Retry'
  if (action === 'Commit') return 'list commits, or is refused as gone'
  if (action === 'Discard') return 'a write fails, list discarded'
  if (action === 'LostReply') return 'reply lost, ending read'
  if (action === 'Reject') return 'the operation refuses the list, says why'
  if (action === 'Unsupported') return 'the plugin has no build for this device'
  if (action === 'Reload') return 'tab reloads'
  if (action === 'OrphanEnds') return 'a list from before a reload ends'
  if (action === 'MoveIndex') return 'another writer moves the index'
  if (action === 'Watch') return null
  return null
}
