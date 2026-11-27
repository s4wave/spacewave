import { createContext, useContext } from 'react'

import type { SOReplayState } from '@s4wave/core/sobject/sobject.pb.js'

// SpaceReplayContext carries the mounted Space's watched replay state, as
// SharedObjectHealth.replay reports it. The container that watches the
// Space's health provides the context. An undefined replay state means the
// first health has not arrived. Outside a Space no provider exists and
// nothing is held.
export const SpaceReplayContext = createContext<{
  replay: SOReplayState | undefined
} | null>(null)

// useSaveHold returns why the mounted Space takes no save now, or null when
// it takes one. A save while the Space opens, or before its health arrives,
// waits in the Space's gate and lands once replay catches up, so only a wait
// holds it.
export function useSaveHold(): string | null {
  const space = useContext(SpaceReplayContext)
  if (!space) return null
  return describeSaveHold(space.replay)
}

// describeSaveHold maps a replay state to the reason saving waits, or null
// when a save is taken.
export function describeSaveHold(
  replay: SOReplayState | undefined,
): string | null {
  if (replay?.state?.case === 'waiting') {
    return 'Saving waits until this Space catches up'
  }
  return null
}
