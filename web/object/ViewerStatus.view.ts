// A viewer's waiting, failed and empty states around the object it shows.

type Status = {
  status: string
  retryable: boolean
  drawn: boolean
}

export const title = 'Object viewer: waiting, failed and empty'

export const state = (v: Status) => {
  if (v.status === 'waiting') {
    return v.drawn ? 'Reading again, record kept' : 'Waiting for the record'
  }
  if (v.status === 'failed') {
    const kept = v.drawn ? ', record kept' : ''
    return v.retryable ? `Failed, Retry shown${kept}` : `Failed for good${kept}`
  }
  if (v.status === 'empty') return 'Empty'
  return 'Ready'
}

export const step = (action: string, from: Status, to: Status) => {
  // A step that keeps the drawn state draws no edge.
  if (state(from) === state(to)) return null

  // Each action names the edge it draws.
  if (action === 'Arrive') return 'record arrives'
  if (action === 'Fail') return 'read fails'
  if (action === 'Reread') return 'reads again'
  if (action === 'Change') return 'another object opens'
  if (action === 'Retry') return 'presses Retry'
  return null
}
