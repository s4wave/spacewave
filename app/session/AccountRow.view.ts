// One account row in the session picker, from loading to a saved name.

type Row = {
  kind: string
  loaded: boolean
  name: string
  edit: string
  draft: string
}

export const title = 'Session picker: one account row'

export const state = (v: Row) => {
  if (!v.loaded) return 'Loading'
  if (v.edit === 'editing') return 'Editing the name'
  if (v.edit === 'saving') return 'Saving'
  if (v.edit === 'failed') return 'Save failed'
  if (v.kind === 'cloud') return 'Cloud account'
  if (v.name) return 'Named'
  if (v.kind === 'pin') return 'Locked: named after unlocking'
  return 'Unnamed local account'
}

const steps: Record<string, string> = {
  Load: 'metadata arrives',
  Start: 'presses Name',
  Cancel: 'presses Cancel',
  Save: 'presses Save',
  Saved: 'the watch shows the name',
  SaveFailed: 'the save fails',
  Renamed: 'renamed elsewhere',
}

export const step = (action: string, from: Row, to: Row) => {
  // Typing and picking the account change nothing the row draws.
  if (action === 'Select' || action === 'Type') return null
  if (state(from) === state(to)) return null
  return steps[action] ?? null
}
