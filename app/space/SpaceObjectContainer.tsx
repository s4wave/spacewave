import { useAppEnvironment } from '@s4wave/web/sdk/app/environment.js'
import { useMemo, useCallback } from 'react'
import {
  useResource,
  useResourceValue,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
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

// The outer route's viewer subpath is world/-/<inner-key>/-/<viewer-path>.
const nestedRoutePrefix = 'world/-/'

/** NestedWorldObjectViewer owns the opened read-only sub-World for this outer key. */
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
  // Observe only revisions of the selected outer object. Capture the World
  // sequence before reading its root so a concurrent publication cannot be missed.
  const outerRevision = useStreamingResource(
    spaceWorldResource,
    async function* (world, signal) {
      if (!innerKey) return

      let lastRev: bigint | undefined
      let seqno = (await world.getSeqno(signal)).seqno
      while (!signal.aborted) {
        const object = await world.getObject(outerKey, signal)
        if (!object) throw new Error(`Object not found: ${outerKey}`)
        let rev: bigint
        try {
          rev = (await object.getRootRef(signal)).rev ?? 0n
        } finally {
          object.release()
        }
        if (rev !== lastRev) {
          lastRev = rev
          yield rev
        }
        seqno = (await world.waitSeqno(seqno + 1n, signal)).seqno
      }
    },
    [outerKey, Boolean(innerKey)],
  )

  // The resource hook owns each immutable snapshot and disposes it on revision.
  const worldState = useResource(
    [spaceWorldResource, outerRevision],
    async ([world], signal, cleanup) => {
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

// SpaceObjectContainer displays an object within a space.
export function SpaceObjectContainer() {
  const environment = useAppEnvironment()
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
  const navigate = useNavigate()
  const spaceContentsResource = SpaceContentsContext.useContext()

  const routerPath = '/' + (objectPath || '')
  const nestedRoute = objectPath?.startsWith(nestedRoutePrefix)
    ? parseObjectUri(objectPath.slice(nestedRoutePrefix.length))
    : null

  const handleViewerNavigate = useCallback(
    (to: To) => {
      if (isWorldObjectNavigation(to)) {
        navigateToSubPath(
          to.path ? to.objectKey + '/-/' + to.path : to.objectKey,
        )
        return
      }
      if (to.path.startsWith('/') && !isLocalNavigation(to)) {
        navigate(to)
        return
      }
      const resolved = resolvePath(routerPath, to)
      const stripped = resolved.replace(/^\//, '')
      const key = objectKey ?? ''
      const full = stripped ? key + '/-/' + stripped : key
      navigateToSubPath(full)
    },
    [navigate, routerPath, objectKey, navigateToSubPath],
  )

  const objectType = useMemo(() => {
    const stateType = spaceState.worldContents?.objects?.find(
      (obj) => obj.objectKey === objectKey,
    )?.objectType
    if (stateType) {
      return stateType
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
    spaceState.worldContents?.objects,
    environment.instanceKey,
  ])

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
