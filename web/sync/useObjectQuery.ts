import { useMemo } from 'react'

import type { Resource } from '../../bldr/sdk/hooks/useResource.js'
import { useStreamingResource } from '../../bldr/sdk/hooks/useStreamingResource.js'
import {
  watchObjectQuery,
  type ObjectQuery,
} from '../../sdk/sync/object-query.js'
import type { IWorldState } from '../../sdk/world/world-state.js'

/** useObjectQuery projects a canonical object and releases its watch on replacement. */
export function useObjectQuery<T>(
  world: Resource<IWorldState>,
  objectKey: string,
  query: ObjectQuery<T>,
): Resource<T> {
  const result = useStreamingResource(
    world,
    async function* (state, signal) {
      // Live viewers watch immutable snapshots; historical viewers read their supplied state.
      if (!objectKey) return
      const engine = state.getEngine?.()
      if (engine) {
        for await (const snapshot of watchObjectQuery(
          engine,
          objectKey,
          query,
          signal,
        )) {
          yield snapshot.value
        }
      } else {
        using object = await state.getObject(objectKey, signal)
        yield object ? await query(object, signal) : null
      }
    },
    [objectKey, query],
  )

  // Hide a replaced source immediately while its first snapshot is loading.
  return useMemo(
    () => ({
      ...result,
      value: result.loading ? null : result.value,
    }),
    [result],
  )
}
