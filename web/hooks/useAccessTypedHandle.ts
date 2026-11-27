import {
  useResource,
  type Resource,
  type UseResourceOptions,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import { Resource as SDKResource } from '@aptre/bldr-sdk/resource/resource.js'
import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

// typedHandleOptions retries the access when the server or the connection
// releases the handle. The options are shared so the release listener is
// subscribed once per handle instead of once per render.
const typedHandleOptions: UseResourceOptions<SDKResource> = {
  retryOnReleasedResource: {
    reasons: ['server-released', 'connection-lost'],
    getResourceIds: (handle) => [handle.id],
  },
}

// useAccessTypedHandle creates an SDK handle for a world object's typed resource.
// A handle of a different type than typeId resolves to null.
export function useAccessTypedHandle<T extends SDKResource>(
  worldState: Resource<IWorldState>,
  objectKey: string | null,
  HandleClass: new (ref: ClientResourceRef) => T,
  typeId?: string,
): Resource<T> {
  return useResource(
    worldState,
    async (world, signal, cleanup) => {
      if (!world) return null
      if (!objectKey) return null
      const access = await world.accessTypedObject(objectKey, signal)
      if (!access.resourceId) return null
      const ref = world.getResourceRef().createRef(access.resourceId)
      if (typeId && access.typeId !== typeId) {
        ref.release()
        return null
      }
      return cleanup(new HandleClass(ref))
    },
    [objectKey, typeId, HandleClass],
    typedHandleOptions,
  )
}
