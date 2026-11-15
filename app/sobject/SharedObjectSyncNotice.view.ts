// Where replay stands, as the sync notice shows it.

type Replay = {
  phase: string
}

export const title = 'Space replay and the sync notice'

export const state = (v: Replay) => {
  if (v.phase === 'opening') return 'Opening, read only'
  if (v.phase === 'handler') return 'Waiting for an app, read only'
  if (v.phase === 'approval') return 'Waiting for approval, read only'
  return 'Caught up'
}

export const step = (action: string, from: Replay, to: Replay) => {
  // A step that keeps the drawn state draws no edge.
  if (state(from) === state(to)) return null

  // Each action names the edge it draws.
  if (action === 'Open') return 'replay catches up'
  if (action === 'Stall') return 'reaches an unknown change'
  if (action === 'NeedApproval') return 'plugin not approved'
  if (action === 'Approve') return 'person approves'
  if (action === 'Pass') return 'handler arrives'
  if (action === 'Restart') return 'app restarts'
  return null
}
