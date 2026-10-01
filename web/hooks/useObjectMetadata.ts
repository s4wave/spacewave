import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { ObjectMetadata } from '@s4wave/sdk/world/world.pb.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

import { useWorldQuery } from './useWorldQuery.js'

/**
 * useObjectMetadata follows the type and parent of one World object. The value
 * is null when the key is empty or no object has it.
 */
export function useObjectMetadata(
  world: Resource<IWorldState>,
  objectKey: string,
): Resource<ObjectMetadata | null> {
  return useWorldQuery(
    world,
    async (state, signal) => {
      if (!objectKey) return null
      // A key sorts first among the keys it prefixes, so a one-object page
      // under the key answers both existence and metadata in one read.
      const page = await state.listObjects(
        { prefix: objectKey, limit: 1 },
        signal,
      )
      const object = page.objects?.[0]
      return object?.objectKey === objectKey ? object : null
    },
    [objectKey],
  )
}
