// A member's change reaching one file in the owner's linked folder: the node
// fetches a written version, checks the disk still holds the version the member
// replaced, exchanges the files atomically and verifies what it displaced.
// A delete goes to the trash and is verified there.

type Change = { kind: string; base: number; new: number }

type Apply = {
  disk: number
  pending: Change
  phase: string
  held: number
  copies: unknown
  trash: unknown
}

// TLA+ sets arrive as arrays or as 1-indexed records.
const size = (s: unknown): number =>
  Array.isArray(s) ? s.length : Object.keys(s as Record<string, number>).length

// What the folder keeps besides the path.
const kept = (v: Apply) =>
  (size(v.copies) > 0 ? ', a conflict copy beside it' : '') +
  (size(v.trash) > 0 ? ', the old file in the trash' : '')

export const title = "Linked folder: applying a member's change"

export const state = (v: Apply) => {
  if (v.pending.kind === 'none') return (v.disk === 0 ? 'No file at the path' : 'A file at the path') + kept(v)
  const write = v.pending.kind === 'write'
  switch (v.phase) {
    case 'idle':
      return write ? "Member's version waiting" : 'Delete waiting'
    case 'staged':
      return "Member's version fetched"
    case 'restoring':
      return 'Path empty; putting it back'
    case 'checked':
      return write ? 'Disk matches; exchanging' : 'Disk matches; moving to trash'
    case 'swapped':
      return "Checking the displaced file"
    default:
      return 'Checking the trashed file'
  }
}

const words: Record<string, string> = {
  MemberWrite: 'a member saves',
  MemberDelete: 'a member deletes',
  Fetch: "the member's version arrives",
  CheckWrite: 'the disk is checked',
  CheckDelete: 'the disk is checked',
  Restore: "the member's version is put back",
  Exchange: 'the files are exchanged',
  MoveToTrash: 'the file goes to the trash',
  VerifyWrite: 'the displaced file is checked',
  VerifyDelete: 'the trashed file is checked',
}

export const step = (action: string, from: Apply, to: Apply) => {
  const w = words[action]
  if (!w) return null
  if (state(from) === state(to)) return null
  return w
}

export const group = (v: Apply) =>
  v.pending.kind === 'none'
    ? 'Settled'
    : v.phase === 'swapped' || v.phase === 'trashed'
      ? 'Verifying'
      : 'Applying'

export const colors = {
  Settled: 'teal',
  Applying: 'blue',
  Verifying: 'orange',
}
