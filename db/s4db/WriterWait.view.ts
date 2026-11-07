// WriterWaitState includes the remote scheduler's foreground companion.
type WriterWaitState = {
  holder: string
  waiting: boolean
  steps: number
  remoteWait: boolean
  failed: boolean
}

// title states the counterexample's scope.
export const title = 'Background steps yield to a waiting writer'

// state shows both the callback bound and the kernel wait graph.
export const state = (v: WriterWaitState) =>
  `${v.holder}${v.remoteWait ? ', background waiting on intent' : ''}${
    v.failed ? ', kernel deadlock' : ''
  }${v.waiting ? `, writer waiting after ${v.steps} background spans` : ''}`

// step names the lock operation.
export const step = (action: string) => action
