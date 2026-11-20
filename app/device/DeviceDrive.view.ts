// The file browser over a folder served live from a Device, as drawn.

type Drive = { handle: string; device: string; root: string; editing: boolean }

export const title = 'Device folder: handle, Device and root'

export const state = (v: Drive) => {
  if (v.handle === 'waiting') return 'Opening'
  if (v.handle === 'failed') return 'Could not open, Retry shown'
  if (v.device === 'offline') return 'Device offline, reconnects on return'
  if (v.root === 'connecting') return 'Connecting to the Device'
  if (v.root === 'failed') return 'Folder unavailable, Retry shown'
  return v.editing ? 'Files, editing enabled' : 'Files'
}

export const step = (action: string, from: Drive, to: Drive) => {
  // A step that keeps the drawn state draws no edge.
  if (state(from) === state(to)) return null
  const named: Record<string, string> = {
    HandleOpen: 'handle opens',
    HandleFail: 'handle fails',
    RetryHandle: 'presses Retry',
    DeviceLeave: 'Device goes offline',
    DeviceReturn: 'Device returns',
    RetryRoot: 'presses Retry',
    EnableEditing: 'presses Enable editing',
  }
  if (action !== 'RootStep') return named[action] ?? null
  return to.root === 'ready' ? 'folder opens' : 'folder refused or broken'
}
