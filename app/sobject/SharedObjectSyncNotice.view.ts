// The replay wait the sync notice shows.

type Wait = {
  wait: string
}

export const title = 'Space waits for a change handler'

export const state = (v: Wait) => {
  if (v.wait === 'handler') return 'Waiting for an app'
  if (v.wait === 'approval') return 'Waiting for approval'
  return 'Up to date'
}

export const step = (action: string, from: Wait, to: Wait) => {
  // A step that keeps the drawn state draws no edge.
  if (state(from) === state(to)) return null

  // Each action names the edge it draws.
  if (action === 'Stall') return 'reaches an unknown change'
  if (action === 'NeedApproval') return 'plugin not approved'
  if (action === 'Approve') return 'person approves'
  if (action === 'Pass') return 'handler arrives'
  if (action === 'Restart') return 'app restarts'
  return null
}
