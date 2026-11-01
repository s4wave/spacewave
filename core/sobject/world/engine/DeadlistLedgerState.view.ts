// One record, as save generations carry its name and blocks.
type Ledger = { status: string }
// title names the record lifecycle.
export const title = 'A deadlist record waits, becomes due, or loses its name'
// state projects the record state from its bounded chart model.
export const state = (v: Ledger) => v.status
// step labels only transitions that change the record state.
export const step = (action: string, from: Ledger, to: Ledger) =>
  state(from) === state(to) ? null : action
