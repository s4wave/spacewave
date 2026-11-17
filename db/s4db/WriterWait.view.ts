// WriterWaitState shows one local background step yielding to foreground work.
type WriterWaitState = {
  holder: string
  waiting: boolean
  steps: number
  yielded: boolean
}

export const title = 'Background steps yield to a waiting writer'
export const state = (v: WriterWaitState) =>
  `${v.holder}${v.yielded ? ', background yielded' : ''}${
    v.waiting ? `, writer waiting after ${v.steps} background spans` : ''
  }`
export const step = (action: string) => action
