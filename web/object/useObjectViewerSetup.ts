import { useEffect, useMemo, useRef } from 'react'

import {
  useResource,
  type Resource,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import { resourceOf } from '@aptre/bldr-sdk/hooks/index.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { RootContext } from '@s4wave/web/contexts/contexts.js'
import { getObjectType } from '@s4wave/sdk/world/types/types.js'
import { formatObjectRef } from '@s4wave/sdk/world/object-ref.js'
import { useWorldQuery } from '@s4wave/web/hooks/useWorldQuery.js'
import {
  useAllViewers,
  getViewersForType,
} from '@s4wave/web/hooks/useViewerRegistry.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import type { IObjectState } from '@s4wave/sdk/world/object-state.js'
import type { Root } from '@s4wave/sdk/root/root.js'
import type { ObjectRef } from '@go/github.com/s4wave/spacewave/db/bucket/bucket.pb.js'
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

// formatRootRef formats ref for the Root Ref row, or returns undefined when
// formatting fails. The object is present either way, so a failure costs only
// that row and never the object.
async function formatRootRef(
  root: Root,
  ref: ObjectRef | undefined,
  signal: AbortSignal,
): Promise<string | undefined> {
  try {
    return await formatObjectRef(root, ref, signal)
  } catch {
    signal.throwIfAborted()
    return undefined
  }
}

// useObjectState derives the object's state from its watch and its handle.
// A failed watch is the object's state: before its first value it says
// nothing about whether the object exists, so it shows as an error, never
// as missing. After a value it keeps the handle as last. A watch reopening
// after a failure keeps the handle and carries that failure as reopening.
// found is set once the watch found the object the handle binds.
function useObjectState(
  watched: Resource<WatchedObject | null>,
  handle: Resource<IObjectState>,
  found: boolean,
): Resource<IObjectState | null> {
  const handleValue = found ? handle.value : null
  const loading = watched.loading || (found && handle.loading)
  const watchError = watched.error
  const reopening = watched.loading ? watched.reopening : undefined
  const watchRetry = watched.retry
  return useMemo<Resource<IObjectState | null>>(
    () =>
      watchError
        ? resourceOf({
            value: null,
            loading: false,
            error: watchError,
            last: handleValue,
            retry: watchRetry,
          })
        : resourceOf({ ...handle, value: handleValue, loading, reopening }),
    [handle, handleValue, loading, reopening, watchError, watchRetry],
  )
}

// idleWorld opens no watch, for a source that names no World object.
const idleWorld: Resource<IWorldState> = resourceOf({
  value: null,
  loading: false,
  error: null,
  retry: () => {},
})

interface UseObjectViewerSetupOptions {
  typeIDHint?: string
}

// useObjectViewerSetup loads the ObjectViewer chain for a world object.
// Shared by SpaceObjectContainer and CanvasObjectNode.
//
// Every object key is watched, whatever viewer shows it: the watch is the one
// source of whether the object exists, its type and its failure, so every
// viewer of a World object can tell missing from failed. With no key there
// is no World object: nothing is watched and the type is the hint, or empty.
export function useObjectViewerSetup(
  worldState: Resource<IWorldState>,
  objectKey: string | undefined,
  options: UseObjectViewerSetupOptions = {},
): ObjectViewerSetup {
  const rootResource = RootContext.useContext()
  const space = SpaceContainerContext.useContextSafe()
  const allViewers = useAllViewers(rootResource, space?.spaceState.engineId)

  // The watch formats with the current Root. A reloaded Root with the same id
  // formats the same refs, so it swaps in under the running watch, which
  // neither restarts nor clears what it shows.
  const root = rootResource.value
  const rootId = root?.id
  const currentRoot = useRef(root)
  useEffect(() => {
    currentRoot.current = root
  }, [root])

  // Follow whether the object exists, its type and its formatted root on
  // each World revision that changes them. The watch is the one source of
  // all three, so the root shown never blanks while a new one formats.
  const watched = useWorldQuery(
    objectKey ? worldState : idleWorld,
    async (world, signal): Promise<WatchedObject | null> => {
      if (!objectKey) return null
      using object = await world.getObject(objectKey, signal)
      if (!object) return null
      const [rootRefResp, typeID] = await Promise.all([
        object.getRootRef(signal),
        getObjectType(world, objectKey, signal),
      ])
      const formatRoot = currentRoot.current
      const rootRef = formatRoot
        ? await formatRootRef(formatRoot, rootRefResp.rootRef, signal)
        : undefined
      return { key: objectKey, typeID, rootRef }
    },
    [objectKey, rootId],
    {
      key: `object-viewer:${objectKey}:root:${rootId ?? 'none'}`,
      equals: sameWatchedObject,
    },
  )
  // A watch that fails after answering keeps its last answer, so the viewer
  // keeps the object, its type and its root beside the failure.
  const watchedObject = watched.value ?? (watched.error ? watched.last : null)
  const watchedKey = watchedObject?.key

  // Derive the handle from the watch. A live World, one with an Engine as
  // useWorldQuery decides, binds a handle that reads the current revision on
  // each call, so there is no second read that could land on another
  // revision. A World without an Engine is immutable, so reading it again
  // agrees with the watch.
  const handle = useResource(
    worldState,
    async (world, signal, cleanup) => {
      if (!watchedKey) return null
      if (world.getEngine?.() && world.bindObject) {
        return world.bindObject(watchedKey)
      }
      return cleanup(await world.getObject(watchedKey, signal))
    },
    [watchedKey],
  )

  const objectState = useObjectState(watched, handle, !!watchedKey)

  // The hint stands in for the type only while the watch has not answered.
  const typeID = objectKey
    ? (watchedObject?.typeID ??
      (watched.loading ? options.typeIDHint : undefined))
    : (options.typeIDHint ?? '')
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
