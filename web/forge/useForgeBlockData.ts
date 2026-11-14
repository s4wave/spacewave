import { useMemo } from 'react'
import {
  useResource,
  type Resource,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import type { IObjectState } from '@s4wave/sdk/world/object-state.js'
import type { MessageType, Message } from '@aptre/protobuf-es-lite'

// useForgeBlock reads and decodes a forge entity's block data from
// objectState. The resource's value is null when the object holds no block,
// and its loading and error say why a value has not arrived.
export function useForgeBlock<T extends Message<T>>(
  objectState: IObjectState | undefined,
  blockTypeID: string,
  messageType: MessageType<T>,
): Resource<T | null> {
  return useResource(
    async (signal) => {
      if (!objectState) return null
      using cursor = await objectState.accessWorldState(undefined, signal)
      const resp = await cursor.unmarshal({ blockType: blockTypeID }, signal)
      if (!resp.found || !resp.data?.length) return null
      return messageType.fromBinary(resp.data)
    },
    [objectState, blockTypeID, messageType],
  )
}

// useForgeBlockData is useForgeBlock's value alone, for a viewer that shows
// nothing different while the block loads or fails: the decoded message, or
// undefined if loading, failed or missing.
export function useForgeBlockData<T extends Message<T>>(
  objectState: IObjectState | undefined,
  blockTypeID: string,
  messageType: MessageType<T>,
): T | undefined {
  const resource = useForgeBlock(objectState, blockTypeID, messageType)
  return useMemo(() => resource.value ?? undefined, [resource.value])
}
