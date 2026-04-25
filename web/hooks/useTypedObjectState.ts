import { useMemo } from 'react'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { Resource as SDKResource } from '@aptre/bldr-sdk/resource/resource.js'
import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

import { useAccessTypedHandle } from './useAccessTypedHandle.js'

// useTypedObjectState composes useAccessTypedHandle + useStreamingResource +
// source derivation into a single hook. Shared by notebook, docs, and blog viewers.
export function useTypedObjectState<
  THandle extends SDKResource,
  TState extends { sources?: { ref?: string }[] },
>(
  worldState: Resource<IWorldState>,
  objectKey: string,
  HandleClass: new (ref: ClientResourceRef) => THandle,
  typeId: string,
  watchFactory: (handle: THandle, signal: AbortSignal) => AsyncIterable<TState>,
) {
  const resource = useAccessTypedHandle(
    worldState,
    objectKey,
    HandleClass,
    typeId,
  )

  const state = useStreamingResource(resource, watchFactory, [])

  const sources = useMemo(
    () => state.value?.sources ?? [],
    [state.value?.sources],
  )

  return { resource, state, sources }
}
