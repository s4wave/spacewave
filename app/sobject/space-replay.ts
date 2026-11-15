import { createContext, useContext } from 'react'

import type { SOReplayState } from '@s4wave/core/sobject/sobject.pb.js'

// SpaceReplayContext carries the mounted Space's watched replay state, as
// SharedObjectHealth.replay reports it. The container that watches the
// Space's health provides it; undefined means the first health has not
// arrived. Outside a Space no provider exists and nothing is held.
export const SpaceReplayContext = createContext<{
  replay: SOReplayState | undefined
} | null>(null)

// useSaveHold returns why a write to the mounted Space waits, or null when the
// Space takes writes. A Space takes writes only once replay is caught up, so a
// Space whose health has not arrived yet waits as one that is opening.
export function useSaveHold(): string | null {
  const space = useContext(SpaceReplayContext)
  if (!space) return null
  return describeSaveHold(space.replay)
}

// describeSaveHold maps a replay state to the reason saving waits.
export function describeSaveHold(
  replay: SOReplayState | undefined,
): string | null {
  switch (replay?.state?.case) {
    case 'caughtUp':
      return null
    case 'waiting':
      return 'Saving waits until this Space catches up'
    default:
      return 'Saving waits until this Space opens'
  }
}
