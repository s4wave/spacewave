import { useMemo } from 'react'

import {
  useResource,
  type Resource,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

// useWorldObjectMessageState reads a typed notes object block directly from the
// world and exposes the parsed message plus its sources.
export function useWorldObjectMessageState<
  TState extends { sources?: { ref?: string }[] },
>(
  worldState: Resource<IWorldState>,
  objectKey: string,
  parse: (data: Uint8Array) => TState,
) {
  const state = useResource(
    worldState,
    async (world, signal) => {
      if (!world || !objectKey) return null
      const objectState = await world.getObject(objectKey, signal)
      if (!objectState) return null
      using _ = objectState
      using cursor = await objectState.accessWorldState(undefined, signal)
      const blockResp = await cursor.getBlock({}, signal)
      if (!blockResp.found || !blockResp.data) return null
      return parse(blockResp.data)
    },
    [objectKey, parse],
  )

  const sources = useMemo(
    () => state.value?.sources ?? [],
    [state.value?.sources],
  )

  return { state, sources }
}
