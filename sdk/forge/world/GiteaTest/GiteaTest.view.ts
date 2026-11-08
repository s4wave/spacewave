type State = { phase: string; built: boolean }
export const title = 'Keep the repository, issues and build result'
export const state = (v: State) =>
  ({
    off: 'Stopped',
    booting: 'Starting',
    attached: 'Starting',
    running: 'Ready',
    working: 'Working',
    checked: v.built ? 'Build verified' : 'Data recorded',
    'stop-pending': 'Waiting for stop confirmation',
    stopping: 'Stopping',
    recovered: 'Verified',
  })[v.phase]
export const step = (action: string) =>
  ({
    Start: 'Start',
    Serve: 'Open Gitea',
    BeginWork: 'Create issue and push code',
    RecordResults: 'Record issue and commit',
    BuildRepo: 'Run and verify build',
    Stop: 'Stop through graph',
    StopUnavailable: 'Stop unavailable; keep custody',
    RetryStop: 'Retry stop',
    ConfirmStop: 'Stop confirmed',
    Restart: 'Restart the VM',
    Replace: 'Replace the VM',
    Verify: 'Clone and read the issue',
  })[action] ?? null
