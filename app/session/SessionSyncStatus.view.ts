// The chart highlights transfer progress and the independent direct failure.
type Status = {
  cloud: string
  peer: string
  replication: string
  open: boolean
}
export const title = 'Session sync: independent source observations'
export const state = (v: Status) => {
  if (v.replication === 'error') return 'Local copy failed'
  if (v.replication === 'active') return 'Local copy active'
  if (v.cloud === 'error') return 'Cloud sync failed'
  if (v.cloud === 'active') return 'Cloud sync active'
  return v.peer === 'error' ? 'Synced; peer failed' : 'Synced'
}
export const step = (action: string, from: Status, to: Status) => {
  if (state(from) === state(to)) return null
  if (action === 'PeerChanged') return 'peer status changes'
  if (action === 'ReplicationChanged') return 'copy progress changes'
  if (action === 'CloudChanged') return 'cloud progress changes'
  return null
}
