// The screen a removed wizard's tab shows, by how the World says it ended.

type Gone = {
  ending: string
  view: string
  fresh: boolean
}

export const title = 'Removed wizard: how it ended'

const views: Record<string, string> = {
  checking: 'Checking how it ended, Close tab',
  unreadable: 'Check failed, Retry and Close tab',
  finished: 'Finished, Open and Close tab',
  deleted: 'Deleted, Open Space and Close tab',
}

export const state = (v: Gone) => views[v.view] ?? v.view

const steps: Record<string, string> = {
  Read: 'the watch reads the ending',
  ReadFails: 'the first read fails',
  Retry: 'presses Retry',
  DeleteTarget: 'the object it finished as is deleted',
}

export const step = (action: string, from: Gone, to: Gone) =>
  state(from) === state(to) ? null : (steps[action] ?? null)
