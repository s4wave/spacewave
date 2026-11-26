// Two container runs asking one Worker for its one graphics card, as the
// Execution viewer shows each of them.

type Admission = {
  st: Record<string, string>
  holder: string
}

export const title = 'Two runs, one graphics card: what each Execution shows'

const labels: Record<string, string> = {
  pending: 'starting',
  waiting: 'waiting for a graphics card',
  running: 'running on the card',
  stopping: 'stopping',
  uncertain: 'Worker restarting, card held',
  done: 'complete',
  refused: 'refused: no card here fits',
}

export const state = (v: Admission) =>
  `A ${labels[v.st.a] ?? v.st.a}; B ${labels[v.st.b] ?? v.st.b}`

const steps: Record<string, string> = {
  Reserve: 'gets the free card',
  Busy: 'finds the card busy',
  Refuse: 'asks for more than the card has',
  Wake: 'gets the freed card',
  Exit: 'container exits',
  Credit: 'stop confirmed, card freed',
  Crash: 'Worker restarts',
  Recover: 'Worker stops the container',
}

export const step = (action: string, from: Admission, to: Admission) => {
  if (action === 'Done') return null
  const who = from.st.a !== to.st.a ? 'A' : 'B'
  const name = Object.keys(steps).find((k) => action.startsWith(k))
  return name ? `${who} ${steps[name]}` : null
}

export const group = (v: Admission) =>
  Object.values(v.st).includes('refused')
    ? 'refused'
    : Object.values(v.st).includes('waiting')
      ? 'waiting'
      : v.holder === 'none'
        ? 'idle'
        : 'busy'

export const colors = { idle: 'teal', busy: 'blue', waiting: 'orange', refused: 'pink' }
