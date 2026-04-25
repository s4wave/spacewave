import { useMemo } from 'react'

import {
  useResource,
  type Resource,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import { RootContext } from '@s4wave/web/contexts/contexts.js'
import { getObjectType } from '@s4wave/sdk/world/types/types.js'
import { formatObjectRef } from '@s4wave/sdk/world/object-ref.js'
import {
  useAllViewers,
  getViewersForType,
} from '@s4wave/web/hooks/useViewerRegistry.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import type { IObjectState } from '@s4wave/sdk/world/object-state.js'
import type { ObjectViewerComponent } from './object.js'

// ObjectViewerSetup is the result of useObjectViewerSetup.
export interface ObjectViewerSetup {
  objectState: Resource<IObjectState | null>
  typeID: string | undefined
  rootRef: string | undefined
  visibleComponents: ObjectViewerComponent[]
}

// useObjectViewerSetup loads the ObjectViewer chain for a world object.
// Shared by SpaceObjectContainer and CanvasObjectNode.
export function useObjectViewerSetup(
  worldState: Resource<IWorldState>,
  objectKey: string | undefined,
): ObjectViewerSetup {
  const rootResource = RootContext.useContext()
  const allViewers = useAllViewers(rootResource)

  const objectState = useResource(
    worldState,
    async (world, signal, cleanup) =>
      world && objectKey ?
        cleanup(await world.getObject(objectKey, signal))
      : null,
    [objectKey],
  )

  const objectInfo = useResource(
    [rootResource, worldState, objectState] as const,
    async ([root, world, object], signal) => {
      if (!world || !object) return null
      const key = object.getKey()
      const rootRefResp = await object.getRootRef(signal)
      const typeID = await getObjectType(world, key, signal)
      return {
        key,
        rootRef: await formatObjectRef(root, rootRefResp.rootRef, signal),
        typeID,
      }
    },
    [],
  )

  const typeID = objectInfo.value?.typeID
  const availableComponents = useMemo(
    () => getViewersForType(typeID ?? '', allViewers),
    [typeID, allViewers],
  )

  return {
    objectState,
    typeID,
    rootRef: objectInfo.value?.rootRef,
    visibleComponents: availableComponents,
  }
}
