import { useCallback, useEffect, useRef } from 'react'
import { LuBox } from 'react-icons/lu'

import {
  parseObjectUri,
  SUBPATH_DELIMITER,
} from '@s4wave/sdk/space/object-uri.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import { useOpenCommand } from '@s4wave/web/command/CommandContext.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { useWorldQuery } from '@s4wave/web/hooks/useWorldQuery.js'
import {
  isHiddenSpaceObject,
  OBJECT_KEY_DELIMITER,
} from '@s4wave/web/space/object-tree.js'
import { EmptyState } from '@s4wave/web/ui/EmptyState.js'
import { toast } from '@s4wave/web/ui/toaster.js'

import { SpaceObjectBrowser } from './SpaceObjectBrowser.js'
import { applySpaceIndexPath } from './space-settings.js'

// spaceIndexPageSize is how many keys one index lookup reads at a time.
const spaceIndexPageSize = 100

// SpaceIndexResolution is where the root route of a Space leads.
export interface SpaceIndexResolution {
  // path is the index path to open, or empty to show the object list.
  path: string
  // stale is set when path replaces a configured index that no longer exists.
  stale: boolean
  // hasVisibleObjects is set when the Space has an object to list.
  hasVisibleObjects: boolean
}

// resolveSpaceIndex resolves the configured index path against the World. A
// missing index object is replaced by its lowest numbered sibling, such as
// files-1 for files, or else by the first visible object.
export async function resolveSpaceIndex(
  world: IWorldState,
  indexPath: string | undefined,
  signal?: AbortSignal,
): Promise<SpaceIndexResolution> {
  const first = await firstVisibleObjectKey(world, signal)
  const hasVisibleObjects = !!first
  const path = indexPath ?? ''
  if (!path || path === '/') {
    return { path: '', stale: false, hasVisibleObjects }
  }

  const parsed = parseObjectUri(path)
  const indexObjectKey = parsed.objectKey
  if (!indexObjectKey) {
    return { path: '', stale: true, hasVisibleObjects }
  }
  if (await isVisibleObject(world, indexObjectKey, signal)) {
    return { path, stale: false, hasVisibleObjects }
  }

  const replacement =
    (await lowestNumberedObjectKey(world, indexObjectKey, signal)) || first
  if (!replacement) {
    return { path: '', stale: true, hasVisibleObjects }
  }
  return {
    path: parsed.path
      ? replacement + SUBPATH_DELIMITER + parsed.path
      : replacement,
    stale: true,
    hasVisibleObjects,
  }
}

// isVisibleObject returns whether key is an object shown in the Space.
async function isVisibleObject(
  world: IWorldState,
  key: string,
  signal?: AbortSignal,
): Promise<boolean> {
  const { objects } = await world.listObjects({ prefix: key, limit: 1 }, signal)
  const object = objects?.[0]
  return object?.objectKey === key && !isHiddenSpaceObject(key, object.typeId)
}

// firstVisibleObjectKey returns the first key shown in the Space, or empty.
// Hidden objects are few, so the scan ends within a page or two.
async function firstVisibleObjectKey(
  world: IWorldState,
  signal?: AbortSignal,
): Promise<string> {
  let startAfter = ''
  for (;;) {
    const { objects, more } = await world.listObjects(
      { startAfter, limit: spaceIndexPageSize },
      signal,
    )
    for (const object of objects ?? []) {
      const key = object.objectKey ?? ''
      if (key && !isHiddenSpaceObject(key, object.typeId)) return key
      startAfter = key
    }
    if (!more || !objects?.length) return ''
  }
}

// lowestNumberedObjectKey returns the visible key-N sibling with the lowest N
// on the first listing page, or empty.
async function lowestNumberedObjectKey(
  world: IWorldState,
  key: string,
  signal?: AbortSignal,
): Promise<string> {
  const prefix = key + '-'
  const { objects } = await world.listObjects(
    { prefix, delimiter: OBJECT_KEY_DELIMITER, limit: spaceIndexPageSize },
    signal,
  )
  const numbered = (objects ?? [])
    .flatMap((object) => {
      const objectKey = object.objectKey ?? ''
      if (isHiddenSpaceObject(objectKey, object.typeId)) return []
      const suffix = Number(objectKey.slice(prefix.length))
      return Number.isInteger(suffix) && suffix > 0
        ? [{ objectKey, suffix }]
        : []
    })
    .sort((a, b) => a.suffix - b.suffix)
  return numbered[0]?.objectKey ?? ''
}

// SpaceIndex handles the root route of a space.
export function SpaceIndex() {
  const { spaceState, spaceWorld, spaceWorldResource, navigateToSubPath } =
    SpaceContainerContext.useContext()
  const openCommand = useOpenCommand()
  const redirectedIndexPathRef = useRef<string | null>(null)
  const repairedIndexPathRef = useRef<string | null>(null)

  const indexPath = spaceState.settings?.indexPath
  const indexResolution = useWorldQuery(
    spaceWorldResource,
    (world, signal) => resolveSpaceIndex(world, indexPath, signal),
    [indexPath],
  ).value
  const redirectPath =
    indexResolution?.path && indexResolution.path !== '/'
      ? indexResolution.path
      : null
  const stale = !!indexResolution?.stale

  const handleCreateClick = useCallback(() => {
    openCommand('spacewave.create-object')
  }, [openCommand])

  useEffect(() => {
    if (!redirectPath) {
      redirectedIndexPathRef.current = null
      return
    }
    if (redirectedIndexPathRef.current === redirectPath) {
      return
    }
    redirectedIndexPathRef.current = redirectPath
    navigateToSubPath(redirectPath)
  }, [navigateToSubPath, redirectPath])

  useEffect(() => {
    if (!stale || !redirectPath || !spaceWorld) {
      return
    }
    const repairKey = `${indexPath ?? ''}->${redirectPath}`
    if (repairedIndexPathRef.current === repairKey) {
      return
    }
    repairedIndexPathRef.current = repairKey
    void applySpaceIndexPath(spaceWorld, redirectPath, {
      expectedIndexPath: indexPath ?? '',
    }).then(
      () => {
        const label = parseObjectUri(redirectPath).objectKey || redirectPath
        toast.success(`Default object updated to ${label}`)
      },
      () => {},
    )
  }, [stale, redirectPath, spaceWorld, indexPath])

  if (!indexResolution || redirectPath) {
    return null
  }

  if (indexResolution.hasVisibleObjects) {
    return (
      <div className="flex min-h-0 flex-1 flex-col">
        <EmptyState
          className="shrink-0"
          icon={<LuBox className="text-foreground-alt size-7" />}
          title="Choose an object"
          description="Select an object to view, or add a new one."
          action={{
            label: 'Add an object',
            onClick: handleCreateClick,
          }}
        />
        <div className="min-h-0 flex-1 overflow-auto px-4 pb-4">
          <SpaceObjectBrowser embedded={true} />
        </div>
      </div>
    )
  }

  return (
    <EmptyState
      className="fine-pointer:[&_button]:min-h-0 flex-1 [&_button]:min-h-11"
      icon={<LuBox className="text-foreground-alt size-7" />}
      title="Empty Space"
      description="This space has no objects yet."
      action={{
        label: 'Create your first object',
        onClick: handleCreateClick,
      }}
    />
  )
}
