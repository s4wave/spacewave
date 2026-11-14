// The life of pack 1, from its first append to its eviction, through a crash
// and recovery.

type PackAhead = {
  up: boolean
  file: Record<string, string>
  fin: Record<string, string>
  remote: number[]
  entries: number[]
  reg: number[]
}

export const title = 'One pack: appended, finalized, uploaded, evicted'

// Name pack 1's state from its file, its finalize step and the remote's view.
// TLA+ sequences arrive as records keyed by position.
const pack = (v: PackAhead) => {
  const has = (s: number[]) => s.includes(1)
  if (v.file['1'] === 'gone') return 'evicted'
  if (has(v.reg)) return 'registered'
  if (has(v.entries)) return 'entry written'
  if (has(v.remote)) return 'object uploaded'
  if (v.file['1'] === 'final') return 'finalized'
  if (v.fin['1'] === 'index') return 'index written'
  return 'open'
}

export const state = (v: PackAhead) => (v.up ? pack(v) : `${pack(v)}, crashed`)

export const step = (action: string, from: PackAhead, to: PackAhead) => {
  if (state(from) === state(to)) return null
  switch (action) {
    case 'Crash':
      return 'crash'
    case 'Recover':
      return from.file['1'] === 'open'
        ? 'recover: truncate to the last good record, finalize'
        : 'recover'
    case 'FinIndex':
      return 'write and sync the index'
    case 'FinRename':
      return 'rename and sync the directory'
    case 'Upload':
      return 'upload the file'
    case 'Entry':
      return 'write the entry'
    case 'Register':
      return 'register its keys'
    case 'Evict':
      return 'unlink'
    default:
      return action
  }
}
