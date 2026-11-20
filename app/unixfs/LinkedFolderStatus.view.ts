// A linked file's row in the Drive view, as Bob, a member, sees it.

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

// serves mirrors the spec's Serves for Bob: the linking Device, Spacewave
// Cloud, or Alice's laptop holding the version and able to answer.
const serves = (v: Status) =>
  v.pub > 0 &&
  ((v.online && v.linked !== 'rejected' && v.disk === v.pub) ||
    v.cver === v.pub ||
    (v.lapOnline && v.copy === v.pub))

export const state = (v: Status) => {
  const f = v.file.member
  if (f === 'none') {
    return v.linked === 'indexing'
      ? 'No row yet, folder indexing'
      : 'No row yet'
  }
  if (f === 'filling') return 'Opening'
  if (f === 'present') {
    return v.have.member < v.pub
      ? 'On this device, update on its way'
      : 'On this device'
  }
  if (f === 'unavailable' && !serves(v)) {
    return v.online && v.linked !== 'rejected'
      ? 'Newer version on its way'
      : 'Unavailable, holders named'
  }
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
