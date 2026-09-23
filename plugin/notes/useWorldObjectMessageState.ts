import { useMemo } from 'react'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import { accessObjectRootWorldState } from '@s4wave/sdk/world/utils.js'
import { useObjectQuery } from '@s4wave/web/sync/useObjectQuery.js'
import type { ObjectQuery } from '@s4wave/sdk/sync/object-query.js'

/** useWorldObjectMessageState watches the canonical typed block without copying its storage. */
export function useWorldObjectMessageState<
  TState extends { sources?: { ref?: string }[] },
>(
  worldState: Resource<IWorldState>,
  objectKey: string,
  parse: (data: Uint8Array) => TState,
) {
  // A stable decoder runs only when this object's immutable root changes.
  const query = useMemo<ObjectQuery<TState | null>>(
    () => async (object, signal) => {
      using cursor = await accessObjectRootWorldState(object, signal)
      const block = await cursor.getBlock({}, signal)
      return block.found && block.data ? parse(block.data) : null
    },
    [parse],
  )
  const state = useObjectQuery(worldState, objectKey, query)

  // The source list is a projection of the current Notebook, Docs, or Blog block.
  const sources = useMemo(
    () => state.value?.sources ?? [],
    [state.value?.sources],
  )

  return { state, sources }
}
