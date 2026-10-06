import { useCallback, useEffect, useMemo, useState } from 'react'
import { useWatchStateRpc } from '@aptre/bldr-react'
import {
  useResource,
  useResourceValue,
  type Resource,
} from '@aptre/bldr-sdk/hooks/useResource.js'

import { useSessionList } from '@s4wave/app/hooks/useSessionList.js'
import { pluginPathPrefix } from '@s4wave/app/urls.js'
import { parseObjectUri } from '@s4wave/sdk/space/object-uri.js'
import { Space } from '@s4wave/sdk/space/space.js'
import {
  SpaceState,
  WatchSpaceStateRequest,
} from '@s4wave/sdk/space/space.pb.js'
import type { Session } from '@s4wave/sdk/session/session.js'
import {
  WatchResourcesListRequest,
  WatchResourcesListResponse,
} from '@s4wave/sdk/session/session.pb.js'
import type { EngineWorldState } from '@s4wave/sdk/world/engine-state.js'
import { useObjectMetadata } from '@s4wave/web/hooks/useObjectMetadata.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import {
  SessionContext,
  SessionIndexContext,
  SharedObjectBodyContext,
  SharedObjectContext,
  SpaceContentsContext,
  SpaceContext,
} from '@s4wave/web/contexts/contexts.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import type { To } from '@s4wave/web/router/router.js'
import { resolvePath } from '@s4wave/web/router/router.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import type { LoadingState } from '@s4wave/web/ui/loading/types.js'
import { ObjectViewer } from '@s4wave/web/object/ObjectViewer.js'
import { ObjectViewerNotFoundState } from '@s4wave/web/object/ObjectViewerNotFoundState.js'
import type { ObjectInfo } from '@s4wave/web/object/object.pb.js'

interface DisplayStatusCardProps {
  state: LoadingState
  title: string
  detail?: string
  error?: string
  onRetry?: () => void
}

function DisplayStatusCard({
  state,
  title,
  detail,
  error,
  onRetry,
}: DisplayStatusCardProps) {
  return (
    <div className="flex h-full min-h-0 w-full flex-1 items-center justify-center p-6">
      <div className="w-full max-w-sm">
        <LoadingCard
          view={{
            state,
            title,
            detail,
            error,
            onRetry,
          }}
        />
      </div>
    </div>
  )
}

interface DisplayComponentNotFoundStateProps {
  componentID: string
  objectKey: string
  typeID: string
}

function DisplayComponentNotFoundState({
  componentID,
  objectKey,
  typeID,
}: DisplayComponentNotFoundStateProps) {
  return (
    <div className="bg-background-primary flex h-full w-full flex-1 items-center justify-center p-4">
      <div className="border-foreground/6 bg-background-card/30 flex max-w-sm items-start gap-3 rounded-lg border p-3.5 backdrop-blur-sm">
        <div className="bg-destructive/10 text-destructive flex size-8 shrink-0 items-center justify-center rounded-md">
          <span className="text-sm font-semibold" aria-hidden="true">
            !
          </span>
        </div>
        <div className="min-w-0">
          <p className="text-foreground text-sm font-semibold tracking-tight select-none">
            Display component not found
          </p>
          <p className="text-foreground-alt/60 mt-1 text-xs leading-relaxed break-words">
            {componentID} is not installed for {objectKey} ({typeID}).
          </p>
        </div>
      </div>
    </div>
  )
}

interface DisplayLocation {
  pathname: string
  search: string
}

interface DisplayTarget {
  path: string
  componentID?: string
  routePathTarget: boolean
}

function decodeDisplayRoutePath(path: string): string {
  try {
    return decodeURIComponent(path)
  } catch {
    return path
  }
}

function parseDisplayTarget(location: DisplayLocation): DisplayTarget {
  const params = new URLSearchParams(location.search)
  const queryPath = params.get('path')
  if (queryPath !== null) {
    return {
      path: queryPath,
      componentID: params.get('component') ?? undefined,
      routePathTarget: false,
    }
  }
  const routePathTarget = location.pathname.startsWith('/display/')
  return {
    path: routePathTarget
      ? decodeDisplayRoutePath(location.pathname.slice('/display/'.length))
      : '',
    componentID: params.get('component') ?? undefined,
    routePathTarget,
  }
}

function setDisplayTargetPath(
  url: URL,
  path: string,
  componentID: string | undefined,
  routePathTarget: boolean,
) {
  if (routePathTarget) {
    url.pathname = path
      ? `/display/${path.split('/').map(encodeURIComponent).join('/')}`
      : '/display'
    url.searchParams.delete('path')
  } else if (path) {
    url.searchParams.set('path', path)
  } else {
    url.searchParams.delete('path')
  }
  if (componentID) {
    url.searchParams.set('component', componentID)
  } else {
    url.searchParams.delete('component')
  }
}

function useDisplayLocation(): [DisplayLocation, () => void] {
  const [displayLocation, setDisplayLocation] = useState<DisplayLocation>(
    () => ({
      pathname: window.location.pathname,
      search: window.location.search,
    }),
  )
  const syncDisplayLocation = useCallback(
    () =>
      setDisplayLocation({
        pathname: window.location.pathname,
        search: window.location.search,
      }),
    [],
  )
  useEffect(() => {
    window.addEventListener('popstate', syncDisplayLocation)
    window.addEventListener('hashchange', syncDisplayLocation)
    return () => {
      window.removeEventListener('popstate', syncDisplayLocation)
      window.removeEventListener('hashchange', syncDisplayLocation)
    }
  }, [syncDisplayLocation])
  return [displayLocation, syncDisplayLocation]
}

/** useKioskSession mounts the first local session for kiosk display. */
function useKioskSession() {
  const rootResource = useRootResource()
  const sessionList = useSessionList()
  const firstSession = sessionList.value?.sessions?.[0]
  const selectedSessionIndex = firstSession
    ? (firstSession.sessionIndex ?? 1)
    : undefined

  const sessionResource = useResource(
    rootResource,
    async (root, signal, cleanup) => {
      if (selectedSessionIndex == null) return null
      const result = await root.mountSessionByIdx(
        { sessionIdx: selectedSessionIndex },
        signal,
      )
      return result ? cleanup(result.session) : null
    },
    [
      selectedSessionIndex,
      firstSession?.sessionRef?.providerResourceRef?.providerId,
      firstSession?.sessionRef?.providerResourceRef?.providerAccountId,
    ],
  )

  return { sessionList, selectedSessionIndex, sessionResource }
}

/**
 * useKioskSpace mounts the first Space of the session with its shared object
 * body, world state, and contents.
 */
function useKioskSpace(sessionResource: Resource<Session | null>) {
  const session = useResourceValue(sessionResource)
  const resourcesList = useWatchStateRpc(
    useCallback(
      (req: WatchResourcesListRequest, signal: AbortSignal) =>
        session?.watchResourcesList(req, signal) ?? null,
      [session],
    ),
    {},
    WatchResourcesListRequest.equals,
    WatchResourcesListResponse.equals,
  )
  const sharedObjectId =
    resourcesList?.spacesList?.[0]?.entry?.ref?.providerResourceRef?.id ?? ''
  const sharedObjectResource = useResource(
    sessionResource,
    async (mountedSession: Session | null, signal, cleanup) => {
      if (!mountedSession || !sharedObjectId) return null
      const result = await mountedSession.mountSharedObject(
        { sharedObjectId },
        signal,
      )
      return result ? cleanup(result) : null
    },
    [sharedObjectId],
  )
  const sharedObjectBodyResource = useResource(
    sharedObjectResource,
    async (sharedObject, signal, cleanup) => {
      if (!sharedObject) return null
      return cleanup(await sharedObject.mountSharedObjectBody({}, signal))
    },
    [sharedObjectId],
  )
  const spaceResource = useResource(
    sharedObjectBodyResource,
    (sharedObjectBody, _signal, cleanup) => {
      if (!sharedObjectBody) return Promise.resolve(null)
      return Promise.resolve(
        cleanup(
          new Space(
            sharedObjectBody.resourceRef.createRef(sharedObjectBody.id),
          ),
        ),
      )
    },
    [sharedObjectId],
  )
  const space = useResourceValue(spaceResource)
  const spaceWorldResource = useResource(
    spaceResource,
    async (mountedSpace, signal, cleanup) => {
      if (!mountedSpace) return null
      return cleanup(await mountedSpace.accessWorldState(true, signal))
    },
    [sharedObjectId],
  )
  const spaceContentsResource = useResource(
    spaceResource,
    async (mountedSpace, signal, cleanup) => {
      if (!mountedSpace) return null
      return cleanup(await mountedSpace.mountSpaceContents(signal))
    },
    [sharedObjectId],
  )
  const spaceState = useWatchStateRpc(
    useCallback(
      (req: WatchSpaceStateRequest, signal: AbortSignal) =>
        space?.watchSpaceState(req, signal) ?? null,
      [space],
    ),
    {},
    WatchSpaceStateRequest.equals,
    SpaceState.equals,
  )
  const spaceWorld = useResourceValue(spaceWorldResource)

  return {
    resourcesList,
    sharedObjectId,
    sharedObjectResource,
    sharedObjectBodyResource,
    spaceResource,
    spaceWorldResource,
    spaceContentsResource,
    spaceState,
    spaceWorld,
  }
}

interface UseDisplayNavigationParams {
  displayTarget: DisplayTarget
  objectKey: string
  viewerPath: string
  syncDisplayLocation: () => void
}

/** useDisplayNavigation projects navigation into the display URL. */
function useDisplayNavigation({
  displayTarget,
  objectKey,
  viewerPath,
  syncDisplayLocation,
}: UseDisplayNavigationParams) {
  const replaceDisplayPath = useCallback(
    (path: string) => {
      const next = new URL(window.location.href)
      setDisplayTargetPath(
        next,
        path,
        displayTarget.componentID,
        displayTarget.routePathTarget,
      )
      window.history.replaceState({}, '', next)
      syncDisplayLocation()
    },
    [
      displayTarget.componentID,
      displayTarget.routePathTarget,
      syncDisplayLocation,
    ],
  )
  const navigateViewerPath = useCallback(
    (to: To) => {
      const resolved = resolvePath(viewerPath, to)
      const stripped = resolved.replace(/^\//, '')
      const fullPath = stripped ? `${objectKey}/-/${stripped}` : objectKey
      replaceDisplayPath(fullPath)
    },
    [objectKey, replaceDisplayPath, viewerPath],
  )
  const navigateToRoot = useCallback(() => {
    replaceDisplayPath('')
  }, [replaceDisplayPath])
  const navigateToObjects = useCallback(
    (objectKeys: string[]) => {
      if (objectKeys.length === 0) return
      replaceDisplayPath(objectKeys[0])
    },
    [replaceDisplayPath],
  )
  const navigateToSubPath = useCallback(
    (subpath: string) => {
      replaceDisplayPath(subpath ? `${objectKey}/-/${subpath}` : objectKey)
    },
    [objectKey, replaceDisplayPath],
  )
  const buildObjectUrls = useCallback(
    (objectKeys: string[]): string[] =>
      objectKeys.map((key) => {
        const next = new URL(window.location.href)
        setDisplayTargetPath(
          next,
          key,
          displayTarget.componentID,
          displayTarget.routePathTarget,
        )
        return next.toString()
      }),
    [displayTarget.componentID, displayTarget.routePathTarget],
  )

  return {
    navigateViewerPath,
    navigateToRoot,
    navigateToObjects,
    navigateToSubPath,
    buildObjectUrls,
  }
}

// useDisplayController owns kiosk resource mounting, URL projection, and
// navigation callbacks.
function useDisplayController() {
  const [displayLocation, syncDisplayLocation] = useDisplayLocation()
  const displayTarget = useMemo(
    () => parseDisplayTarget(displayLocation),
    [displayLocation],
  )
  const parsedPath = useMemo(
    () => parseObjectUri(displayTarget.path),
    [displayTarget.path],
  )
  const { sessionList, selectedSessionIndex, sessionResource } =
    useKioskSession()
  const space = useKioskSpace(sessionResource)
  const { sharedObjectId, spaceWorldResource } = space

  const retryDisplay = useCallback(() => {
    sessionList.retry()
    sessionResource.retry()
    space.sharedObjectResource.retry()
    space.sharedObjectBodyResource.retry()
    space.spaceResource.retry()
    space.spaceWorldResource.retry()
    space.spaceContentsResource.retry()
  }, [sessionList, sessionResource, space])

  const objectEntryResource = useObjectMetadata(
    spaceWorldResource,
    parsedPath.objectKey,
  )
  const objectEntry = objectEntryResource.value
  const objectType = objectEntry?.typeId ?? ''
  const objectInfo: ObjectInfo = useMemo(
    () => ({
      info: parsedPath.objectKey
        ? {
            case: 'worldObjectInfo' as const,
            value: {
              objectKey: parsedPath.objectKey,
              ...(objectType ? { objectType } : {}),
            },
          }
        : { case: undefined, value: undefined },
    }),
    [objectType, parsedPath.objectKey],
  )
  const viewerPath = '/' + (parsedPath.path || '')
  const exportUrl = useMemo(
    () =>
      sharedObjectId
        ? `${pluginPathPrefix}/export/u/${selectedSessionIndex}/so/${encodeURIComponent(sharedObjectId)}`
        : undefined,
    [selectedSessionIndex, sharedObjectId],
  )
  const stateNamespace = useMemo(
    () => [
      'display',
      sharedObjectId || 'none',
      parsedPath.objectKey || 'none',
      displayTarget.componentID ?? 'default',
    ],
    [displayTarget.componentID, parsedPath.objectKey, sharedObjectId],
  )
  const renderMissingDisplayComponent = useCallback(
    (componentID: string, objectKey: string, typeID: string) => (
      <DisplayComponentNotFoundState
        componentID={componentID}
        objectKey={objectKey}
        typeID={typeID}
      />
    ),
    [],
  )
  const navigation = useDisplayNavigation({
    displayTarget,
    objectKey: parsedPath.objectKey,
    viewerPath,
    syncDisplayLocation,
  })
  const buildExportUrl = useCallback(() => exportUrl ?? '', [exportUrl])

  return {
    ...space,
    ...navigation,
    buildExportUrl,
    displayTarget,
    exportUrl,
    objectEntry,
    objectEntryResource,
    objectInfo,
    parsedPath,
    renderMissingDisplayComponent,
    retryDisplay,
    selectedSessionIndex,
    sessionList,
    sessionResource,
    stateNamespace,
    viewerPath,
  }
}

type DisplayController = ReturnType<typeof useDisplayController>

/** firstMountError returns the first mount error, or null when none failed. */
function firstMountError(c: DisplayController): Error | null {
  const errors = [
    c.sessionList.error,
    c.sessionResource.error,
    c.sharedObjectResource.error,
    c.sharedObjectBodyResource.error,
    c.spaceResource.error,
    c.spaceWorldResource.error,
    c.spaceContentsResource.error,
  ]
  return errors.find((err) => !!err) ?? null
}

/** isSpaceMounting reports whether any Space resource is loading or unset. */
function isSpaceMounting(c: DisplayController): boolean {
  const mounted = [
    c.sharedObjectResource,
    c.sharedObjectBodyResource,
    c.spaceResource,
    c.spaceWorldResource,
    c.spaceContentsResource,
  ]
  return (
    mounted.some((resource) => resource.loading || !resource.value) ||
    c.objectEntryResource.loading ||
    !c.spaceState?.ready
  )
}

const SPACE_LOADING_STATUS: DisplayStatusCardProps = {
  state: 'active',
  title: 'Loading display Space',
  detail: 'Mounting the first Space and watching its world state.',
}

/** resolveDisplayStatus returns the mount status card, or null once mounted. */
function resolveDisplayStatus(
  c: DisplayController,
): DisplayStatusCardProps | null {
  const mountError = firstMountError(c)
  if (mountError) {
    return {
      state: 'error',
      title: 'Failed to load display',
      error: mountError.message,
      onRetry: c.retryDisplay,
    }
  }
  if (c.sessionList.loading) {
    return {
      state: 'loading',
      title: 'Loading sessions',
      detail: 'Resolving the default kiosk session.',
    }
  }
  if ((c.sessionList.value?.sessions?.length ?? 0) === 0) {
    return {
      state: 'error',
      title: 'No session available',
      detail: 'Display mode needs one local session to mount a Space.',
      onRetry: c.retryDisplay,
    }
  }
  if (c.sessionResource.loading || !c.sessionResource.value) {
    return {
      state: 'loading',
      title: 'Loading session',
      detail: 'Mounting the default kiosk session.',
    }
  }
  if (!c.resourcesList) {
    return {
      state: 'loading',
      title: 'Loading Spaces',
      detail: 'Watching the session resource list.',
    }
  }
  if (!c.sharedObjectId) {
    return {
      state: 'error',
      title: 'No Space available',
      detail: 'Display mode needs at least one Space in the selected session.',
      onRetry: c.retryDisplay,
    }
  }
  return isSpaceMounting(c) ? SPACE_LOADING_STATUS : null
}

/** DisplayViewer provides the mounted Space resources to the object viewer. */
function DisplayViewer({
  controller,
  spaceState,
}: {
  controller: DisplayController
  spaceState: SpaceState
}) {
  const {
    buildExportUrl,
    buildObjectUrls,
    displayTarget,
    exportUrl,
    navigateToObjects,
    navigateToRoot,
    navigateToSubPath,
    navigateViewerPath,
    objectInfo,
    parsedPath,
    renderMissingDisplayComponent,
    selectedSessionIndex,
    sessionResource,
    sharedObjectBodyResource,
    sharedObjectId,
    sharedObjectResource,
    spaceContentsResource,
    spaceResource,
    spaceWorld,
    spaceWorldResource,
    stateNamespace,
    viewerPath,
  } = controller

  return (
    <SessionIndexContext.Provider value={selectedSessionIndex ?? 0}>
      <SessionContext.Provider resource={sessionResource}>
        <SharedObjectContext.Provider resource={sharedObjectResource}>
          <SharedObjectBodyContext.Provider resource={sharedObjectBodyResource}>
            <SpaceContext.Provider resource={spaceResource}>
              <SpaceContentsContext.Provider resource={spaceContentsResource}>
                <SpaceContainerContext.Provider
                  spaceId={sharedObjectId}
                  spaceState={spaceState}
                  spaceWorldResource={spaceWorldResource}
                  spaceWorld={spaceWorld as EngineWorldState}
                  navigateToRoot={navigateToRoot}
                  navigateToObjects={navigateToObjects}
                  buildObjectUrls={buildObjectUrls}
                  buildExportUrl={buildExportUrl}
                  objectKey={parsedPath.objectKey}
                  objectPath={parsedPath.path || undefined}
                  navigateToSubPath={navigateToSubPath}
                >
                  <ObjectViewer
                    objectInfo={objectInfo}
                    worldState={spaceWorldResource}
                    spaceContents={spaceContentsResource}
                    standalone
                    bottomBarId="displayObjectViewer"
                    path={viewerPath}
                    exportUrl={exportUrl}
                    preferredComponentID={displayTarget.componentID}
                    stateNamespace={stateNamespace}
                    onNavigate={navigateViewerPath}
                    renderMissingComponent={
                      displayTarget.componentID
                        ? renderMissingDisplayComponent
                        : undefined
                    }
                  />
                </SpaceContainerContext.Provider>
              </SpaceContentsContext.Provider>
            </SpaceContext.Provider>
          </SharedObjectBodyContext.Provider>
        </SharedObjectContext.Provider>
      </SessionContext.Provider>
    </SessionIndexContext.Provider>
  )
}

// DisplayContent selects mount states and provides the mounted Space resources
// to the requested object viewer.
function DisplayContent({ controller }: { controller: DisplayController }) {
  const status = resolveDisplayStatus(controller)
  const { parsedPath, objectEntry, displayTarget, spaceState } = controller
  if (status || !spaceState) {
    return <DisplayStatusCard {...(status ?? SPACE_LOADING_STATUS)} />
  }

  if (!parsedPath.objectKey || !objectEntry) {
    return (
      <ObjectViewerNotFoundState
        objectKey={parsedPath.objectKey || displayTarget.path || 'Display path'}
      />
    )
  }

  return <DisplayViewer controller={controller} spaceState={spaceState} />
}

// DisplayContainer renders the kiosk display route outside the session tree.
export function DisplayContainer() {
  const controller = useDisplayController()
  return (
    <div className="bg-background-primary h-full w-full">
      <DisplayContent controller={controller} />
    </div>
  )
}
