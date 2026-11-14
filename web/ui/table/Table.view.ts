// The table: which columns show at its own width, the shed facts, the keys.

type Table = {
  width: number
  n: number
  sort: string
  sel: number[]
  focus: number
  anchor: number
}

export const title = 'Table: columns by width, shed facts, and the keys'

const columns: Record<number, string> = {
  0: 'Name',
  16: 'Name, State',
  22: 'Name, State',
  28: 'Name, State, Owner',
  40: 'Name, State, Owner',
}

const facts: Record<number, string> = {
  0: 'State, Owner, Note',
  16: 'Owner, Note',
  22: 'Owner, Note',
  28: 'Note',
  40: 'Note',
}

// state draws three regions: the columns, the selected row's facts, the rows.
export const state = (v: Table) => {
  let rows = 'none selected'
  if (v.n === 0) rows = 'No rows'
  else if (v.sel.length > 1) rows = 'a range selected'
  else if (v.sel.length === 1) rows = 'one row selected'
  else if (v.focus !== 0) rows = 'one row focused'
  return {
    Columns: columns[v.width] ?? String(v.width),
    Facts: v.sel.length === 1 ? (facts[v.width] ?? '') : 'none',
    Rows: rows,
  }
}

const steps: Record<string, string> = {
  Resize: 'the pane is resized',
  Changed: 'another writer changes the records',
  Home: 'presses Home',
  End: 'presses End',
  ShiftHome: 'presses Shift+Home',
  ShiftEnd: 'presses Shift+End',
  ModHome: 'presses Ctrl+Home',
  ModEnd: 'presses Ctrl+End',
}

export const step = (action: string, from: Table, to: Table) => {
  // Sorting reorders the rows but changes nothing the spec draws.
  if (action === 'Sort') return null
  if (JSON.stringify(state(from)) === JSON.stringify(state(to))) return null
  return steps[action] ?? null
}
