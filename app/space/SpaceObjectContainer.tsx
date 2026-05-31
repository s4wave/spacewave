import { useMemo, useCallback } from 'react'
import { resolvePath, type To } from '@s4wave/web/router/router.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import {
  SpaceContentsContext,
  useSessionIndex,
} from '@s4wave/web/contexts/contexts.js'
import { pluginPathPrefix } from '@s4wave/app/urls.js'
import { ObjectViewer } from '@s4wave/web/object/ObjectViewer.js'
import type { ObjectInfo } from '@s4wave/web/object/object.pb.js'
import { getQuickstartInitialObjectHandoff } from '@s4wave/app/quickstart/session-handoff.js'

// SpaceObjectContainer displays an object within a space.
export function SpaceObjectContainer() {
  const {
    spaceId,
    objectKey,
    objectPath,
    spaceState,
    spaceWorldResource,
    navigateToRoot,
    navigateToSubPath,
  } = SpaceContainerContext.useContext()
  const sessionIndex = useSessionIndex()
  const spaceContentsResource = SpaceContentsContext.useContext()

  const routerPath = '/' + (objectPath || '')

  const handleViewerNavigate = useCallback(
    (to: To) => {
      const resolved = resolvePath(routerPath, to)
      const stripped = resolved.replace(/^\//, '')
      const key = objectKey ?? ''
      const full = stripped ? key + '/-/' + stripped : key
      navigateToSubPath(full)
    },
    [routerPath, objectKey, navigateToSubPath],
  )

  const objectType = useMemo(() => {
    const stateType = spaceState.worldContents?.objects?.find(
      (obj) => obj.objectKey === objectKey,
    )?.objectType
    if (stateType) {
      return stateType
    }
    return (
      getQuickstartInitialObjectHandoff(sessionIndex, spaceId, objectKey)
        ?.objectType ?? ''
    )
  }, [objectKey, sessionIndex, spaceId, spaceState.worldContents?.objects])

  const objectInfo: ObjectInfo = useMemo(
    () => ({
      info: objectKey
        ? {
            case: 'worldObjectInfo' as const,
            value: {
              objectKey,
              ...(objectType ? { objectType } : {}),
            },
          }
        : { case: undefined, value: undefined },
    }),
    [objectKey, objectType],
  )

  const exportUrl = useMemo(
    () =>
      sessionIndex != null && spaceId
        ? `${pluginPathPrefix}/export/u/${sessionIndex}/so/${encodeURIComponent(spaceId)}`
        : undefined,
    [sessionIndex, spaceId],
  )

  return (
    <ObjectViewer
      objectInfo={objectInfo}
      worldState={spaceWorldResource}
      spaceContents={spaceContentsResource}
      path={routerPath}
      exportUrl={exportUrl}
      onNavigate={handleViewerNavigate}
      onBreadcrumbClick={navigateToRoot}
      stateNamespace={['objectViewer', objectKey ?? 'none']}
    />
  )
}
