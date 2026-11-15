import { useMemo } from 'react'

import {
  useResource,
  type Resource,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import { resourceOf } from '@aptre/bldr-sdk/hooks/index.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { RootContext } from '@s4wave/web/contexts/contexts.js'
import { getObjectType } from '@s4wave/sdk/world/types/types.js'
import { formatObjectRef } from '@s4wave/sdk/world/object-ref.js'
import { EngineWorldState } from '@s4wave/sdk/world/engine-state.js'
import { useWorldQuery } from '@s4wave/web/hooks/useWorldQuery.js'
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

// WatchedObject is what the viewer shows of one World object.
interface WatchedObject {
  key: string
  typeID: string
  // rootRef is the formatted root, or undefined before the Root loads.
  rootRef: string | undefined
}

// sameWatchedObject reports whether two reads show the same object.
function sameWatchedObject(
  prev: WatchedObject | null,
  next: WatchedObject | null,
): boolean {
  if (!prev || !next) return prev === next
  return (
    prev.key === next.key &&
    prev.typeID === next.typeID &&
    prev.rootRef === next.rootRef
  )
}

// idleWorld opens no watch, for a viewer that loads no object state.
const idleWorld: Resource<IWorldState> = resourceOf({
  value: null,
  loading: false,
  error: null,
  retry: () => {},
})

interface UseObjectViewerSetupOptions {
  typeIDHint?: string
  loadObjectState?: boolean
}

// useObjectViewerSetup loads the ObjectViewer chain for a world object.
// Shared by SpaceObjectContainer and CanvasObjectNode.
export function useObjectViewerSetup(
  worldState: Resource<IWorldState>,
  objectKey: string | undefined,
  options: UseObjectViewerSetupOptions = {},
): ObjectViewerSetup {
  const rootResource = RootContext.useContext()
  const space = SpaceContainerContext.useContextSafe()
  const allViewers = useAllViewers(rootResource, space?.spaceState.engineId)
  const loadObjectState = options.loadObjectState ?? true

  // Follow whether the object exists, its type and its formatted root on
  // each World revision that changes them. The watch is the one source of
  // all three, so the root shown never blanks while a new one formats.
  const root = rootResource.value
  const watched = useWorldQuery(
    loadObjectState ? worldState : idleWorld,
    async (world, signal): Promise<WatchedObject | null> => {
      if (!objectKey) return null
      using object = await world.getObject(objectKey, signal)
      if (!object) return null
      const [rootRefResp, typeID] = await Promise.all([
        object.getRootRef(signal),
        getObjectType(world, objectKey, signal),
      ])
      const rootRef = root
        ? await formatObjectRef(root, rootRefResp.rootRef, signal)
        : undefined
      return { key: objectKey, typeID, rootRef }
    },
    [objectKey, root],
    { key: 'object-viewer:' + objectKey, equals: sameWatchedObject },
  )
  const watchedObject = watched.value
  const watchedKey = watchedObject?.key

  // Derive the handle from the watch. A live World's handle reads the
  // current revision on each call, so it binds to the watched key with no
  // second read that could land on another revision. A World without an
  // Engine is immutable, so reading it again agrees with the watch.
  const handle = useResource(
    worldState,
    async (world, signal, cleanup) => {
      if (!watchedKey) return null
      if (world instanceof EngineWorldState) {
        return world.bindObject(watchedKey)
      }
      return cleanup(await world.getObject(watchedKey, signal))
    },
    [watchedKey],
    { enabled: loadObjectState },
  )

  const handleValue = watchedKey ? handle.value : null
  const objectStateLoading = loadObjectState
    ? watched.loading || (!!watchedKey && handle.loading)
    : handle.loading
  const objectState = useMemo<Resource<IObjectState | null>>(
    () =>
      resourceOf({
        ...handle,
        value: handleValue,
        loading: objectStateLoading,
      }),
    [handle, handleValue, objectStateLoading],
  )

  const typeID = loadObjectState
    ? (watchedObject?.typeID ??
      (watched.loading ? options.typeIDHint : undefined))
    : options.typeIDHint
  const availableComponents = useMemo(
    () => getViewersForType(typeID ?? '', allViewers),
    [typeID, allViewers],
  )

  return {
    objectState,
    typeID,
    rootRef: watchedObject?.rootRef,
    visibleComponents: availableComponents,
  }
}
