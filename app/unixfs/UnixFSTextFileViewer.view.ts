// A text file read, edited in a draft and saved, with another writer.

type TextFile = {
  read: string
  mode: string
  seen: string
  base: string
  dirty: boolean
  saveFailed: boolean
  theirs: boolean
  writable: boolean
  whole: boolean
}

export const title = 'Text file: reading, editing and saving'

export const state = (v: TextFile) => {
  // With no draft the read decides what shows.
  if (v.mode === 'reading') {
    if (v.read === 'waiting') return 'Reading the file'
    if (v.read === 'failed') return 'Read failed, Retry shown'
    if (!v.writable) return 'Shown, read-only'
    if (!v.whole) return 'Preview, too large to edit'
    return 'Shown, Edit offered'
  }

  // With a draft, a running save, then a failed read, then another writer's
  // version, then a failed save decide it.
  if (v.mode === 'saving') return 'Saving'
  if (v.read === 'failed') return 'Editing, read failed, draft kept'
  if (v.seen !== v.base) {
    return v.theirs
      ? 'Changed by another writer, theirs shown'
      : 'Changed by another writer'
  }
  if (v.saveFailed) return 'Save failed, draft kept'
  return v.dirty ? 'Editing, not saved' : 'Editing, no changes'
}

export const step = (action: string, from: TextFile, to: TextFile) => {
  // A step that keeps the drawn state draws no edge.
  if (state(from) === state(to)) return null

  // Each action names the edge it draws.
  const names: Record<string, string> = {
    Arrive: 'read lands',
    Fail: 'read fails',
    Reread: 'reads again',
    OtherWrite: 'another writer saves',
    SaveDone: 'save lands',
    SaveFail: 'save fails',
    Retry: 'presses Retry',
    Edit: 'presses Edit',
    Type: 'types',
    Save: 'presses Save',
    SaveOver: 'presses Save over theirs',
    Discard: 'presses Discard edits',
    ShowTheirs: 'presses Show theirs',
    BackToMine: 'presses Back to my edits',
  }
  return names[action] ?? null
}
