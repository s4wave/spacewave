// ReleaseState is a finite pass split into bounded writer callbacks.
type ReleaseState = { remaining: number; queued: boolean; waiting: boolean }

// title states the scheduler's continuation contract.
export const title = 'Bounded release batches retain their continuation'

// state shows unfinished work and foreground priority.
export const state = (v: ReleaseState) =>
  `${v.remaining} batches, ${v.queued ? 'queued' : 'idle'}, ${v.waiting ? 'writer waiting' : 'writer free'}`

// step names the scheduling operation.
export const step = (action: string) => action
