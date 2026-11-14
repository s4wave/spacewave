import { useAppEnvironment } from '@s4wave/web/sdk/app/environment.js'
import { useMemo, useCallback } from 'react'
import {
  useResource,
  useResourceValue,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import { resolvePath, type To, useNavigate } from '@s4wave/web/router/router.js'
import {
  isLocalNavigation,
  isWorldObjectNavigation,
} from '@s4wave/web/router/HistoryRouter.js'
import { parseObjectUri } from '@s4wave/sdk/space/object-uri.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import {
  SpaceContentsContext,
  useSessionIndex,
} from '@s4wave/web/contexts/contexts.js'
import { pluginPathPrefix } from '@s4wave/app/urls.js'
import { ObjectViewer } from '@s4wave/web/object/ObjectViewer.js'
import { ObjectViewerLoadingState } from '@s4wave/web/object/ObjectViewerLoadingState.js'
import type { ObjectInfo } from '@s4wave/web/object/object.pb.js'
import { getQuickstartInitialObjectHandoff } from '@s4wave/app/quickstart/session-handoff.js'
import { useObjectMetadata } from '@s4wave/web/hooks/useObjectMetadata.js'
import { resourceContent } from '@aptre/bldr-sdk/hooks/index.js'

// The outer route's viewer subpath is world/-/<inner-key>/-/<viewer-path>.
const nestedRoutePrefix = 'world/-/'

/** NestedWorldObjectViewer owns the read-only nested World opened for this outer key. */
function NestedWorldObjectViewer({
  outerKey,
  innerKey,
  innerPath,
  spaceWorldResource,
  navigateToSubPath,
  navigateToRoot,
}: {
  outerKey: string
  innerKey: string
  innerPath: string
  spaceWorldResource: ReturnType<
    typeof SpaceContainerContext.useContext
  >['spaceWorldResource']
  navigateToSubPath: (path: string) => void
  navigateToRoot: () => void
}) {
  // The nested World follows the outer object's revisions, so it opens once
  // per outer key and its watches survive republication.
  const worldState = useResource(
    spaceWorldResource,
    async (world, signal, cleanup) => {
      const nestedWorld = await world.openNestedWorld(outerKey, signal)
      return cleanup(nestedWorld)
    },
    [outerKey],
    { enabled: Boolean(innerKey) },
  )
  const world = useResourceValue(worldState)

  // Keep local navigation inside the nested viewer route.
  const navigate = useNavigate()
  const routeFor = useCallback(
    (key: string, path = '') =>
      `${outerKey}/-/world/-/${key}${path ? '/-/' + path : ''}`,
    [outerKey],
  )
  const handleNavigate = useCallback(
    (to: To) => {
      // Open sibling objects inside the same nested World.
      if (isWorldObjectNavigation(to)) {
        navigateToSubPath(routeFor(to.objectKey, to.path))
        return
      }

      // Leave the nested viewer for global destinations.
      if (to.path.startsWith('/') && !isLocalNavigation(to)) {
        navigate(to)
        return
      }

      // Resolve local destinations against the inner viewer path.
      const resolved = resolvePath('/' + innerPath, to).replace(/^\//, '')
      navigateToSubPath(routeFor(innerKey, resolved))
    },
    [navigate, innerPath, innerKey, navigateToSubPath, routeFor],
  )

  // Explain an empty selection before showing World status.
  if (!innerKey) {
    return <p role="status">No nested object selected.</p>
  }

  // Offer a retry when opening the sub-World fails.
  if (worldState.error) {
    return (
      <div role="alert">
        Unable to open nested World: {worldState.error.message}
        <button type="button" onClick={worldState.retry}>
          Try again
        </button>
      </div>
    )
  }

  // Wait for the sub-World before mounting its object viewer.
  if (!world) {
    return <ObjectViewerLoadingState />
  }

  // Render the inner object with its own viewer state and outer breadcrumb.
  return (
    <ObjectViewer
      objectInfo={{
        info: {
          case: 'worldObjectInfo',
          value: { objectKey: innerKey },
        },
      }}
      worldState={worldState}
      path={'/' + innerPath}
      onNavigate={handleNavigate}
      onBreadcrumbClick={navigateToRoot}
      stateNamespace={['objectViewer', outerKey, innerKey]}
    />
  )
}

/** SpaceObjectContainer displays an object within a space. */
export function SpaceObjectContainer() {
  // Read the selected object and the Space it lives in.
  const environment = useAppEnvironment()
  const {
    spaceId,
    objectKey,
    objectPath,
    spaceWorldResource,
    navigateToRoot,
    navigateToSubPath,
  } = SpaceContainerContext.useContext()
  const sessionIndex = useSessionIndex()
  const navigate = useNavigate()
  const spaceContentsResource = SpaceContentsContext.useContext()

  // Split a nested World route from the outer object's viewer path.
  const routerPath = '/' + (objectPath || '')
  const nestedRoute = objectPath?.startsWith(nestedRoutePrefix)
    ? parseObjectUri(objectPath.slice(nestedRoutePrefix.length))
    : null

  // Route viewer navigation to sibling objects, global pages or subpaths.
  const handleViewerNavigate = useCallback(
    (to: To) => {
      // Open sibling objects in the same Space.
      if (isWorldObjectNavigation(to)) {
        navigateToSubPath(
          to.path ? to.objectKey + '/-/' + to.path : to.objectKey,
        )
        return
      }

      // Leave the Space for global destinations.
      if (to.path.startsWith('/') && !isLocalNavigation(to)) {
        navigate(to)
        return
      }

      // Resolve local destinations against the object viewer path.
      const resolved = resolvePath(routerPath, to)
      const stripped = resolved.replace(/^\//, '')
      const key = objectKey ?? ''
      const full = stripped ? key + '/-/' + stripped : key
      navigateToSubPath(full)
    },
    [navigate, routerPath, objectKey, navigateToSubPath],
  )

  // Prefer the World's object type, falling back to the quickstart handoff.
  const metadata = useObjectMetadata(spaceWorldResource, objectKey ?? '')
  const worldObjectType = resourceContent(metadata)?.typeId
  const objectType = useMemo(() => {
    if (worldObjectType) {
      return worldObjectType
    }
    return (
      getQuickstartInitialObjectHandoff(
        sessionIndex,
        spaceId,
        objectKey,
        environment.instanceKey,
      )?.objectType ?? ''
    )
  }, [
    objectKey,
    sessionIndex,
    spaceId,
    worldObjectType,
    environment.instanceKey,
  ])

  // Describe the selected object to the viewer.
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

  // Link the Space export for the current session.
  const exportUrl = useMemo(
    () =>
      sessionIndex != null && spaceId
        ? `${pluginPathPrefix}/export/u/${sessionIndex}/so/${encodeURIComponent(spaceId)}`
        : undefined,
    [sessionIndex, spaceId],
  )

  // Show an object inside a nested World through its own viewer.
  if (nestedRoute && objectKey) {
    return (
      <NestedWorldObjectViewer
        key={objectKey}
        outerKey={objectKey}
        innerKey={nestedRoute.objectKey}
        innerPath={nestedRoute.path}
        spaceWorldResource={spaceWorldResource}
        navigateToSubPath={navigateToSubPath}
        navigateToRoot={navigateToRoot}
      />
    )
  }

  // Wait for the object's type rather than open it with none, as the first
  // render after the object changes would.
  if (objectKey && !objectType && metadata.loading) {
    return <ObjectViewerLoadingState />
  }

  // Show the selected Space object.
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
