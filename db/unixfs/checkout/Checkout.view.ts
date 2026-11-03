type State = { owner: string; published: boolean; pc: Record<string, string> }

/** title describes the exclusion boundary between Core processes. */
export const title = 'One writer per destination across Core processes'

/** state shows publication and the writer holding the filesystem lease. */
export const state = (value: State) =>
  value.owner === 'none'
    ? value.published
      ? 'Complete checkout, writer released'
      : 'No writer'
    : value.pc[value.owner]

/** step leaves out a step that changes no publication or worker phase. */
export const step = (action: string, from: State, to: State) =>
  state(from) === state(to) ? null : action
