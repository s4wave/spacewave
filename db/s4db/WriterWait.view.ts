// WriterWaitState is one foreground request contending with a remote scheduler.
type WriterWaitState = { holder: string; waiting: boolean; steps: number }

// title states the counterexample's scope.
export const title = 'A remote background process can reacquire before a waiting writer'

// state keeps each completed background span visible while a writer waits.
export const state = (v: WriterWaitState) =>
  `${v.holder}${v.waiting ? `, writer waiting after ${v.steps} background spans` : ''}`

// step names the lock operation.
export const step = (action: string) => action
