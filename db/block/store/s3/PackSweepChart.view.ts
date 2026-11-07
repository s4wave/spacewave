// One pack's way through a sweep cycle: p2, whose block 2 p1 also holds, as
// the pack store and the command record it.

type Sweep = {
  state: Record<string, string>
  cst: Record<string, string>
  moved: Record<string, boolean>
  missed: Record<string, boolean>
}

export const title = 'A pack through the sweep: quarantine, trash, removal'

const pack = 'p2'

export const state = (v: Sweep) => {
  const s = v.state[pack]
  const c = v.cst[pack]
  if (s === 'removed') return 'removed'
  if (s === 'trash') {
    if (v.missed[pack]) return 'trashing, a read missed'
    if (v.moved[pack]) return 'in the trash'
    return c === 'move' ? 'trashing, checked for misses' : 'trashing'
  }
  if (s === 'quar') return c === 'pass' ? 'quarantined, rechecked' : 'quarantined'
  return 'registered'
}

export const step = (action: string, from: Sweep, to: Sweep) => {
  if (state(from) === state(to)) return null
  switch (action) {
    case 'Quarantine':
      return 'Quarantine'
    case 'Recheck':
      return 'the recheck passes'
    case 'Read':
      return to.state[pack] === 'reg' ? 'a fallback read lifts it' : 'a read misses'
    case 'BeginTrash':
      return 'BeginTrash'
    case 'MoveCheck':
      return 'no miss recorded'
    case 'MoveFile':
      return 'the file moves'
    case 'Remove':
      return 'Remove'
    case 'Restore':
      return 'Restore'
    default:
      return action
  }
}
