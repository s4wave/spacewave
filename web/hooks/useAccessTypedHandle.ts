import {
  useResource,
  type Resource,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import { Resource as SDKResource } from '@aptre/bldr-sdk/resource/resource.js'
import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

// useAccessTypedHandle creates an SDK handle for a world object's typed resource.
export function useAccessTypedHandle<T extends SDKResource>(
  worldState: Resource<IWorldState>,
  objectKey: string,
  HandleClass: new (ref: ClientResourceRef) => T,
  typeId?: string,
): Resource<T> {
  return useResource(
    worldState,
    async (world, signal, cleanup) => {
      if (!world) return null
      const access = await world.accessTypedObject(objectKey, signal)
      if (!access.resourceId) return null
      if (typeId && access.typeId !== typeId) return null
      const resourceRef = world.getResourceRef()
      const ref = resourceRef.createRef(access.resourceId)
      return cleanup(new HandleClass(ref))
    },
    [objectKey],
  )
}
