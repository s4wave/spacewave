// The "keep a full copy" switch on one Device, as Alice sees it.

type Switch = { keep: boolean; sw: string; target: boolean }

export const title = 'Keep a full copy: the switch and its write'

export const state = (v: Switch) => {
  if (v.sw === 'saving') return v.target ? 'Turning on' : 'Turning off'
  if (v.sw === 'failed') return 'Save failed, Retry shown'
  return v.keep ? 'On' : 'Off'
}

const steps: Record<string, string> = {
  Press: 'presses the switch',
  Retry: 'presses Retry',
  Saved: 'the write lands',
  SaveFails: 'the write fails',
  OtherWriter: 'another session changes it',
}

export const step = (action: string, from: Switch, to: Switch) => {
  // A step that keeps the drawn state draws no edge.
  if (state(from) === state(to)) return null
  return steps[action.replace(/\(.*$/, '')] ?? null
}
