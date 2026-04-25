import { useMemo } from 'react'

import {
  useResource,
  type Resource,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import type { Message, MessageType } from '@aptre/protobuf-es-lite'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import type { ForgeLinkedEntity } from '@s4wave/web/forge/useForgeLinkedEntities.js'

export interface ForgeDecodedLinkedEntity<T extends Message<T>> {
  entity: ForgeLinkedEntity
  data: T
}

export function useForgeDecodedLinkedEntities<T extends Message<T>>(
  worldState: Resource<IWorldState>,
  entities: ForgeLinkedEntity[],
  messageType: MessageType<T>,
): { items: ForgeDecodedLinkedEntity<T>[]; loading: boolean } {
  const resource = useResource(
    worldState,
    async (world, signal) => {
      if (!world) return []

      const items = await Promise.all(
        entities.map(async (entity) => {
          using objectState = await world.getObject(entity.objectKey, signal)
          if (!objectState) return null
          using cursor = await objectState.accessWorldState(undefined, signal)
          const resp = await cursor.unmarshal({}, signal)
          if (!resp.found || !resp.data?.length) return null
          return {
            entity,
            data: messageType.fromBinary(resp.data),
          }
        }),
      )

      return items.filter(
        (item): item is ForgeDecodedLinkedEntity<T> => item !== null,
      )
    },
    [entities, messageType],
  )

  return useMemo(
    () => ({
      items: resource.value ?? [],
      loading: resource.loading,
    }),
    [resource.loading, resource.value],
  )
}
