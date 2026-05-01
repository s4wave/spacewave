import {
  SharedObjectHealthLayer,
  type SharedObjectHealth,
} from '@s4wave/core/sobject/sobject.pb.js'

// SpaceMountStage is one of the four observable phases of a space mount,
// shared across the SharedObject health load and the SpaceWorld load that
// follows it.
export type SpaceMountStage = 'resolve' | 'mount' | 'sync' | 'ready'

// SpaceMountStageEntry pairs a stage id with its short uppercase label
// rendered by the stepper.
export interface SpaceMountStageEntry {
  id: SpaceMountStage
  label: string
}

// spaceMountStages is the ordered list of stages rendered by the stepper.
export const spaceMountStages: readonly SpaceMountStageEntry[] = [
  { id: 'resolve', label: 'Resolve' },
  { id: 'mount', label: 'Mount' },
  { id: 'sync', label: 'Sync' },
  { id: 'ready', label: 'Ready' },
]

// spaceMountStageIndex returns the position of stage in the ordered stage
// list, or -1 if not found.
export function spaceMountStageIndex(stage: SpaceMountStage): number {
  return spaceMountStages.findIndex((entry) => entry.id === stage)
}

// spaceMountStageFromHealth derives the mount stage from a SharedObject
// health snapshot. Body-layer loads have already passed the resolve step.
export function spaceMountStageFromHealth(
  health: SharedObjectHealth,
): SpaceMountStage {
  if (health.layer === SharedObjectHealthLayer.BODY) {
    return 'mount'
  }
  return 'resolve'
}

// spaceMountStageFromWorld derives the mount stage from the boolean
// readiness flags tracked by SpaceContainer. The shared object body has
// already mounted at this point, so the floor is 'mount'.
export function spaceMountStageFromWorld(
  root: boolean,
  space: boolean,
  spaceWorld: boolean,
  spaceState: boolean,
): SpaceMountStage {
  if (!root) return 'resolve'
  if (!space) return 'mount'
  if (!spaceWorld || !spaceState) return 'sync'
  return 'ready'
}

// spaceMountDetailFromWorld returns the human-readable detail line that
// pairs with spaceMountStageFromWorld. Mirrors the previous
// spaceWorldLoadingDetail helper.
export function spaceMountDetailFromWorld(
  root: boolean,
  space: boolean,
  spaceWorld: boolean,
  spaceState: boolean,
): string {
  if (!root) return 'Waiting for the session root.'
  if (!space) return 'Mounting the space.'
  if (!spaceWorld) return 'Loading the space world state.'
  if (!spaceState) return 'Preparing the space contents.'
  return 'Finishing space load.'
}
