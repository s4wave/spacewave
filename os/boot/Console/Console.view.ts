type ConsoleState = { phase: string; pending: number; acknowledged: number }

// title describes the operator's QEMU session.
export const title = 'Run and keep your files'
// state groups startup and cleanup steps that present the same interaction.
export const state = (v: ConsoleState) => ({
  off: 'Stopped', opening: 'Starting', ready: 'Starting',
  console: 'Console', stopping: 'Stopping', recovery: 'Recovery',
}[v.phase])
// step labels events the operator observes on the console or host.
export const step = (action: string, from: ConsoleState) => ({
  Start: from.phase === 'recovery' ? 'Retry' : 'Start',
  Enter: 'Ready', Write: 'Write', Ack: 'Sync returns', Stop: 'Exit',
  Close: 'Power off', FailOpen: 'Cannot open files', Crash: 'Host stops VM',
}[action] ?? null)
