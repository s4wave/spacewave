// One record, as save generations carry its name and blocks.
type Ledger = { status: string }
export const title = 'A deadlist record waits, becomes due, or loses its name'
export const state = (v: Ledger) => v.status
export const step = (action: string, from: Ledger, to: Ledger) =>
  state(from) === state(to) ? null : action
