import { useMemo, useCallback } from 'react'
import { resolvePath, type To } from '@s4wave/web/router/router.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { useSessionIndex } from '@s4wave/web/contexts/contexts.js'
import { pluginPathPrefix } from '@s4wave/app/urls.js'
import { ObjectViewer } from '@s4wave/web/object/ObjectViewer.js'
import type { ObjectInfo } from '@s4wave/web/object/object.pb.js'

// SpaceObjectContainer displays an object within a space.
export function SpaceObjectContainer() {
  const {
    spaceId,
    objectKey,
    objectPath,
    spaceWorldResource,
    navigateToRoot,
    navigateToSubPath,
  } = SpaceContainerContext.useContext()
  const sessionIndex = useSessionIndex()

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

  const objectInfo: ObjectInfo = useMemo(
    () => ({
      info:
        objectKey ?
          {
            case: 'worldObjectInfo' as const,
            value: { objectKey },
          }
        : { case: undefined, value: undefined },
    }),
    [objectKey],
  )

  const exportUrl = useMemo(
    () =>
      sessionIndex != null && spaceId ?
        `${pluginPathPrefix}/export/u/${sessionIndex}/so/${encodeURIComponent(spaceId)}`
      : undefined,
    [sessionIndex, spaceId],
  )

  return (
    <ObjectViewer
      objectInfo={objectInfo}
      worldState={spaceWorldResource}
      path={routerPath}
      exportUrl={exportUrl}
      onNavigate={handleViewerNavigate}
      onBreadcrumbClick={navigateToRoot}
      stateNamespace={['objectViewer', objectKey ?? 'none']}
    />
  )
}
