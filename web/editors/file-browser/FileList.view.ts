// The file list: which columns show at the list's own width, and the keys.

type FileList = {
  width: string
  details: boolean
  n: number
  sort: string
  sel: number[]
  focus: number
  anchor: number
}

export const title = 'File list: columns by width, and the keys'

const columns: Record<string, string> = {
  wide: 'Name, Date Modified, Size',
  medium: 'Name, Size',
  narrow: 'Name',
}

// state draws two regions: the columns the width shows, and the rows.
export const state = (v: FileList) => {
  let rows = 'none selected'
  if (v.n === 0) rows = 'This folder is empty'
  else if (v.sel.length > 1) rows = 'a range selected'
  else if (v.sel.length === 1) rows = 'one row selected'
  else if (v.focus !== 0) rows = 'one row focused'
  const shown = v.details ? (columns[v.width] ?? v.width) : 'Name, no details'
  return { Columns: shown, Rows: rows }
}

const steps: Record<string, string> = {
  Resize: 'the pane is resized',
  Changed: 'another writer changes the folder',
  Home: 'presses Home',
  End: 'presses End',
  ShiftHome: 'presses Shift+Home',
  ShiftEnd: 'presses Shift+End',
  ModHome: 'presses Ctrl+Home',
  ModEnd: 'presses Ctrl+End',
  More: 'presses More on a row',
}

export const step = (action: string, from: FileList, to: FileList) => {
  // Sorting reorders the rows but changes nothing the spec draws.
  if (action === 'Sort') return null
  if (state(from) === state(to)) return null
  return steps[action] ?? null
}
