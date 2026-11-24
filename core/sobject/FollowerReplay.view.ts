type Follower = {
  order: number
  ckpt: number
  leader: boolean
  shown: number
  up: boolean
  restarts: number
}

// title names the chart.
export const title = 'A follower replays the operations above the checkpoint'

// state says how far behind the order the follower shows.
export const state = (v: Follower) => {
  const who = v.leader ? 'Leader writing' : 'Leader stopped'
  if (!v.up) return `${who}, follower restarting`
  if (v.shown === v.order) return `${who}, follower shows every operation`
  if (v.shown < v.ckpt) return `${who}, follower behind the checkpoint`
  return `${who}, follower replaying above the checkpoint`
}

// step labels each move as the reader sees it happen.
export const step = (action: string) =>
  (
    {
      Order: 'the leader orders an operation',
      Checkpoint: 'the leader stores a checkpoint',
      Stop: 'the leader stops',
      Install: 'the follower installs the checkpoint',
      Apply: 'the follower applies the next operation',
      Crash: 'the follower restarts',
      Recover: 'the follower reloads the checkpoint',
    } as Record<string, string>
  )[action] ?? null
