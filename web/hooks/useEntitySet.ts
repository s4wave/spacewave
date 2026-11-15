import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import {
  entitySetKey,
  entitySetSource,
  readKeyedSet,
  watchEntitySet,
  type EntityDecoder,
  type EntityRow,
  type EntitySetSpec,
  type KeyedSet,
} from '@s4wave/sdk/world/entity-set.js'
import {
  WorldStateResource,
  type IWorldState,
} from '@s4wave/sdk/world/world-state.js'

/**
 * useEntitySet follows the objects spec names and their decoded bodies. A
 * live World is followed through its Engine, waking only on changes to the
 * set's members and its listing; a World snapshot is read once. Hooks on one
 * World with the same spec and decoder share one watch. A watch that fails
 * after its first value reopens and keeps the value meanwhile.
 */
export function useEntitySet<T>(
  world: Resource<IWorldState>,
  spec: EntitySetSpec,
  decoder: EntityDecoder<T>,
): Resource<KeyedSet<T>> {
  return useStreamingResource(
    world,
    async function* (state, signal) {
      const engine = state.getEngine?.()
      if (engine) {
        yield* watchEntitySet(engine, spec, decoder, signal)
        return
      }
      if (!(state instanceof WorldStateResource)) {
        throw new Error('This World can not list its objects.')
      }
      yield await readKeyedSet(state, entitySetSource(spec, decoder), signal)
    },
    // spec and decoder are named by their key and name.
    [entitySetKey(spec), decoder.name],
    { reconnect: true },
  )
}

// entitySetRows returns the members of set whose body decoded, in listing
// order, with their values.
export function entitySetRows<T>(
  set: KeyedSet<EntityRow<T>> | null,
): { objectKey: string; value: T }[] {
  if (!set) return []
  return set.keys.flatMap((objectKey) => {
    const value = set.rows.get(objectKey)?.value
    return value === undefined ? [] : [{ objectKey, value }]
  })
}

// entitySetFailures returns the members of set whose body could not be
// decoded, in listing order, with the reason.
export function entitySetFailures<T>(
  set: KeyedSet<EntityRow<T>> | null,
): { objectKey: string; error: string }[] {
  if (!set) return []
  return set.keys.flatMap((objectKey) => {
    const error = set.rows.get(objectKey)?.error
    return error === undefined ? [] : [{ objectKey, error }]
  })
}
