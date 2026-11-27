// A linked file's row in the Drive view, as Bob, a member, sees it. The
// charted config turns RowPresence off, so an opened file reads "Opened"
// until rows learn whether this device holds the file.

type Status = {
  disk: number
  pub: number
  online: boolean
  linked: string
  cver: number
  lapOnline: boolean
  copy: number
  file: Record<string, string>
  have: Record<string, number>
}

export const title = 'Linked file row: what a member sees'

// reachable mirrors the spec's HolderReachable for Bob in the charted
// config, which has no cloud: the linking Device awake with its folder not
// rejected, or Alice's laptop awake.
const reachable = (v: Status) =>
  (v.online && v.linked !== 'rejected') || v.lapOnline

export const state = (v: Status) => {
  const f = v.file.member
  if (f === 'none') {
    return v.linked === 'indexing'
      ? 'No row yet, folder indexing'
      : 'No row yet'
  }
  if (f === 'present') return 'Opened'
  if (!reachable(v)) return 'Unavailable, holders named'
  if (f === 'filling') return 'Opening'
  if (f === 'unavailable') return 'Could not read, Retry shown'
  return 'Listed, Open shown'
}

const steps: Record<string, string> = {
  Edit: 'the owner edits it',
  Index: 'the edit is published',
  Indexed: 'the folder finishes indexing',
  Wake: "the owner's Device wakes",
  Sleep: "the owner's Device sleeps",
  Upload: 'Spacewave Cloud receives it',
  Backfill: "the owner's laptop copies it",
  PressKeep: 'Alice switches a full copy on or off',
  LaptopSleep: "the owner's laptop sleeps",
  LaptopWake: "the owner's laptop wakes",
  Appear: 'the row appears',
  PressOpen: 'opens it',
  Filled: 'its chunks arrive',
  NoHolder: 'no holder answers',
  Update: 'the update arrives',
}

export const step = (action: string, from: Status, to: Status) => {
  // A step that keeps the drawn state draws no edge.
  if (state(from) === state(to)) return null
  return steps[action.replace(/\(.*$/, '')] ?? null
}
