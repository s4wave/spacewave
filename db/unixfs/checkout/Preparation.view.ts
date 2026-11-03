type State = { phase: string }

/** title describes the shared preparation lifecycle. */
export const title = 'One preparation, independent callers'

/** state folds caller bookkeeping into the worker phase. */
export const state = (value: State) => value.phase

/** step keeps only visible lifecycle changes. */
export const step = (action: string, from: State, to: State) =>
  from.phase === to.phase || action === 'Crash' ? null : action
