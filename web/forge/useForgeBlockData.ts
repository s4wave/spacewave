import { useMemo } from 'react'
import {
  useResource,
  type Resource,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import type { IObjectState } from '@s4wave/sdk/world/object-state.js'
import type { MessageType, Message } from '@aptre/protobuf-es-lite'

// useForgeBlockData reads and decodes a forge entity's block data from objectState.
// Returns the decoded proto message or undefined if loading/missing.
export function useForgeBlockData<T extends Message<T>>(
  objectState: IObjectState | undefined,
  messageType: MessageType<T>,
): T | undefined {
  const resource: Resource<T | null> = useResource(
    async (signal) => {
      if (!objectState) return null
      using cursor = await objectState.accessWorldState(undefined, signal)
      const resp = await cursor.unmarshal({}, signal)
      if (!resp.found || !resp.data?.length) return null
      return messageType.fromBinary(resp.data)
    },
    [objectState, messageType],
  )
  return useMemo(() => resource.value ?? undefined, [resource.value])
}
