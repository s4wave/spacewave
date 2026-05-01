import { useCallback, useEffect, useMemo, useRef } from 'react'
import { LuBox } from 'react-icons/lu'
import { EmptyState } from '@s4wave/web/ui/EmptyState.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { useOpenCommand } from '@s4wave/web/command/CommandContext.js'
import {
  parseObjectUri,
  SUBPATH_DELIMITER,
} from '@s4wave/sdk/space/object-uri.js'
import { isHiddenSpaceObject } from '@s4wave/web/space/object-tree.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import { applySpaceIndexPath } from './space-settings.js'

interface SpaceIndexObject {
  objectKey?: string
  objectType?: string
}

export interface SpaceIndexResolution {
  path: string
  stale: boolean
}

export function resolveSpaceIndexPath(
  indexPath: string | undefined,
  objects: SpaceIndexObject[] | undefined,
): SpaceIndexResolution {
  const path = indexPath ?? ''
  if (!path || path === '/') {
    return { path: '', stale: false }
  }

  const parsed = parseObjectUri(path)
  const indexObjectKey = parsed.objectKey
  if (!indexObjectKey) {
    return { path: '', stale: true }
  }

  const visible = (objects ?? []).filter(
    (obj) =>
      !!obj.objectKey && !isHiddenSpaceObject(obj.objectKey, obj.objectType),
  )
  if (visible.some((obj) => obj.objectKey === indexObjectKey)) {
    return { path, stale: false }
  }

  const replacement = findReplacementIndexObject(indexObjectKey, visible)
  if (!replacement) {
    return { path: '', stale: true }
  }

  return {
    path:
      parsed.path ? replacement + SUBPATH_DELIMITER + parsed.path : replacement,
    stale: true,
  }
}

function findReplacementIndexObject(
  missingObjectKey: string,
  objects: SpaceIndexObject[],
): string {
  const numberedPrefix = missingObjectKey + '-'
  const numbered = objects
    .map((obj) => obj.objectKey ?? '')
    .filter((key) => key.startsWith(numberedPrefix))
    .map((key) => ({
      key,
      suffix: Number(key.slice(numberedPrefix.length)),
    }))
    .filter((entry) => Number.isInteger(entry.suffix) && entry.suffix > 0)
    .sort((a, b) => a.suffix - b.suffix || a.key.localeCompare(b.key))
  return numbered[0]?.key ?? objects[0]?.objectKey ?? ''
}

// SpaceIndex handles the root route of a space.
export function SpaceIndex() {
  const { spaceState, spaceWorld, navigateToSubPath } =
    SpaceContainerContext.useContext()
  const openCommand = useOpenCommand()
  const redirectedIndexPathRef = useRef<string | null>(null)
  const repairedIndexPathRef = useRef<string | null>(null)

  const indexPath = spaceState.settings?.indexPath
  const indexResolution = useMemo(
    () =>
      resolveSpaceIndexPath(indexPath, spaceState.worldContents?.objects ?? []),
    [indexPath, spaceState.worldContents?.objects],
  )
  const redirectPath =
    indexResolution.path && indexResolution.path !== '/' ?
      indexResolution.path
    : null

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
    if (!indexResolution.stale || !redirectPath || !spaceWorld) {
      return
    }
    const repairKey = `${indexPath ?? ''}->${redirectPath}`
    if (repairedIndexPathRef.current === repairKey) {
      return
    }
    repairedIndexPathRef.current = repairKey
    void applySpaceIndexPath(
      spaceWorld,
      spaceState.settings,
      redirectPath,
    ).then(
      () => {
        const label = parseObjectUri(redirectPath).objectKey || redirectPath
        toast.success(`Default object updated to ${label}`)
      },
      () => {},
    )
  }, [
    indexResolution.stale,
    redirectPath,
    spaceWorld,
    spaceState.settings,
    indexPath,
  ])

  if (redirectPath) {
    return null
  }

  return (
    <EmptyState
      className="flex-1"
      icon={<LuBox className="text-foreground-alt h-7 w-7" />}
      title="Empty Space"
      description="This space has no objects yet."
      action={{
        label: 'Create your first object',
        onClick: handleCreateClick,
      }}
    />
  )
}
