import type { SOStoreDamage } from '@s4wave/core/sobject/sobject.pb.js'

// StoreDamageSummary is what a person reads about a damaged Space.
export interface StoreDamageSummary {
  title: string
  description: string
  hint: string
  detail: string
}

// summarizeStoreDamage describes a Space whose write the store refused again
// because a block it names is missing, and the step that clears it today: the
// block comes back from a backup or another device. Spacewave retries the write
// about once a minute, and the first one that lands clears the damage.
export function summarizeStoreDamage(
  damage: SOStoreDamage,
): StoreDamageSummary {
  const since = damage.since?.toLocaleString() ?? ''
  return {
    title: "This Space can't save its progress",
    description:
      'Data this Space needs is missing on this device. You can still read it, but it stops saving checkpoints and its place in the history, so it replays more each time it opens.',
    hint: 'Restore the missing data from a backup or another device that holds this Space. Spacewave tries again every minute and resumes saving once the data is back.',
    detail:
      `${damage.operation || 'A write'} was refused ${damage.refusals ?? 0} times` +
      (since ? ` since ${since}` : '') +
      '.',
  }
}
