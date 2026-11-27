import type { DependencyList } from 'react'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import {
  useStreamingResource,
  type UseStreamingResourceOptions,
} from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import {
  watchWorldQuery,
  type WorldQuery,
} from '@s4wave/sdk/world/world-query.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

/** UseWorldQueryOptions configures a World query watch. */
export interface UseWorldQueryOptions<
  T,
> extends UseStreamingResourceOptions<T> {
  /** key shares one watch among the hooks on a World that pass it. */
  key?: string
}

/**
 * useWorldQuery runs a bounded World query and re-runs it on each World
 * revision. A live World is watched through its Engine. A World without one,
 * such as a historical snapshot, is read once. deps lists the values the
 * query closes over. Changing one restarts the watch. A watch that fails
 * after its first result reconnects. options can override that or add an
 * equals check that keeps an unchanged result. Hooks on one World that pass
 * the same key share one watch, so the key must identify everything the query
 * reads.
 */
export function useWorldQuery<T>(
  world: Resource<IWorldState>,
  query: WorldQuery<T>,
  deps: DependencyList,
  options?: UseWorldQueryOptions<T>,
): Resource<T> {
  const { key, ...streamOptions } = options ?? {}
  return useStreamingResource(
    world,
    async function* (state, signal) {
      const engine = state.getEngine?.()
      if (!engine) {
        yield await query(state, signal)
        return
      }
      for await (const snapshot of watchWorldQuery(
        engine,
        query,
        signal,
        key,
      )) {
        yield snapshot.value
      }
    },
    deps,
    { reconnect: true, ...streamOptions },
  )
}
