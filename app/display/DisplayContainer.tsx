import { useCallback, useMemo, useState, type ReactNode } from 'react'
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

// DisplayContainer renders the seed kiosk display route outside the session tree.
export function DisplayContainer() {
  const rootResource = useRootResource()
  const sessionList = useSessionList()
  const firstSession = sessionList.value?.sessions?.[0]
  const selectedSessionIndex = firstSession
    ? (firstSession.sessionIndex ?? 1)
    : undefined
  const [displaySearch, setDisplaySearch] = useState(
    () => window.location.search,
  )
  const displayTarget = useMemo(() => {
    const params = new URLSearchParams(displaySearch)
    return {
      path: params.get('path') ?? '',
      componentID: params.get('component') ?? undefined,
    }
  }, [displaySearch])
  const parsedPath = useMemo(
    () => parseObjectUri(displayTarget.path),
    [displayTarget.path],
  )

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
    [selectedSessionIndex],
  )
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
  const retryDisplay = useCallback(() => {
    sessionList.retry()
    sessionResource.retry()
    sharedObjectResource.retry()
    sharedObjectBodyResource.retry()
    spaceResource.retry()
    spaceWorldResource.retry()
    spaceContentsResource.retry()
  }, [
    sessionList,
    sessionResource,
    sharedObjectResource,
    sharedObjectBodyResource,
    spaceResource,
    spaceWorldResource,
    spaceContentsResource,
  ])

  const objectEntry = useMemo(
    () =>
      spaceState?.worldContents?.objects?.find(
        (obj) => obj.objectKey === parsedPath.objectKey,
      ),
    [parsedPath.objectKey, spaceState?.worldContents?.objects],
  )
  const objectType = objectEntry?.objectType ?? ''
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
  const replaceDisplayPath = useCallback(
    (path: string) => {
      const next = new URL(window.location.href)
      if (path) {
        next.searchParams.set('path', path)
      } else {
        next.searchParams.delete('path')
      }
      if (displayTarget.componentID) {
        next.searchParams.set('component', displayTarget.componentID)
      } else {
        next.searchParams.delete('component')
      }
      window.history.replaceState({}, '', next)
      setDisplaySearch(next.search)
    },
    [displayTarget.componentID],
  )
  const navigateViewerPath = useCallback(
    (to: To) => {
      const resolved = resolvePath(viewerPath, to)
      const stripped = resolved.replace(/^\//, '')
      const fullPath = stripped
        ? `${parsedPath.objectKey}/-/${stripped}`
        : parsedPath.objectKey
      replaceDisplayPath(fullPath)
    },
    [parsedPath.objectKey, replaceDisplayPath, viewerPath],
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
      replaceDisplayPath(
        subpath ? `${parsedPath.objectKey}/-/${subpath}` : parsedPath.objectKey,
      )
    },
    [parsedPath.objectKey, replaceDisplayPath],
  )

  const mountError =
    sessionList.error?.message ??
    sessionResource.error?.message ??
    sharedObjectResource.error?.message ??
    sharedObjectBodyResource.error?.message ??
    spaceResource.error?.message ??
    spaceWorldResource.error?.message ??
    spaceContentsResource.error?.message ??
    'Unknown error'
  const hasMountError =
    !!sessionList.error ||
    !!sessionResource.error ||
    !!sharedObjectResource.error ||
    !!sharedObjectBodyResource.error ||
    !!spaceResource.error ||
    !!spaceWorldResource.error ||
    !!spaceContentsResource.error

  const buildObjectUrls = useCallback(
    (objectKeys: string[]): string[] =>
      objectKeys.map((objectKey) => {
        const next = new URL(window.location.href)
        next.searchParams.set('path', objectKey)
        if (displayTarget.componentID) {
          next.searchParams.set('component', displayTarget.componentID)
        } else {
          next.searchParams.delete('component')
        }
        return next.toString()
      }),
    [displayTarget.componentID],
  )
  const buildExportUrl = useCallback(() => exportUrl ?? '', [exportUrl])
  let content: ReactNode
  if (hasMountError) {
    content = (
      <DisplayStatusCard
        state="error"
        title="Failed to load display"
        error={mountError}
        onRetry={retryDisplay}
      />
    )
  } else if (sessionList.loading) {
    content = (
      <DisplayStatusCard
        state="loading"
        title="Loading sessions"
        detail="Resolving the default kiosk session."
      />
    )
  } else if ((sessionList.value?.sessions?.length ?? 0) === 0) {
    content = (
      <DisplayStatusCard
        state="error"
        title="No session available"
        detail="Display mode needs one local session to mount a Space."
        onRetry={retryDisplay}
      />
    )
  } else if (sessionResource.loading || !sessionResource.value) {
    content = (
      <DisplayStatusCard
        state="loading"
        title="Loading session"
        detail="Mounting the default kiosk session."
      />
    )
  } else if (!resourcesList) {
    content = (
      <DisplayStatusCard
        state="loading"
        title="Loading Spaces"
        detail="Watching the session resource list."
      />
    )
  } else if (!sharedObjectId) {
    content = (
      <DisplayStatusCard
        state="error"
        title="No Space available"
        detail="Display mode needs at least one Space in the selected session."
        onRetry={retryDisplay}
      />
    )
  } else if (
    sharedObjectResource.loading ||
    sharedObjectBodyResource.loading ||
    spaceResource.loading ||
    spaceWorldResource.loading ||
    spaceContentsResource.loading ||
    !sharedObjectResource.value ||
    !sharedObjectBodyResource.value ||
    !spaceResource.value ||
    !spaceWorldResource.value ||
    !spaceContentsResource.value ||
    !spaceState?.ready
  ) {
    content = (
      <DisplayStatusCard
        state="active"
        title="Loading display Space"
        detail="Mounting the first Space and watching its world state."
      />
    )
  } else if (!parsedPath.objectKey || !objectEntry) {
    content = (
      <ObjectViewerNotFoundState
        objectKey={parsedPath.objectKey || displayTarget.path || 'Display path'}
      />
    )
  } else {
    content = (
      <SessionIndexContext.Provider value={selectedSessionIndex ?? 0}>
        <SessionContext.Provider resource={sessionResource}>
          <SharedObjectContext.Provider resource={sharedObjectResource}>
            <SharedObjectBodyContext.Provider
              resource={sharedObjectBodyResource}
            >
              <SpaceContext.Provider resource={spaceResource}>
                <SpaceContentsContext.Provider resource={spaceContentsResource}>
                  <SpaceContainerContext.Provider
                    spaceId={sharedObjectId}
                    spaceState={spaceState}
                    spaceWorldResource={
                      spaceWorldResource as Resource<EngineWorldState>
                    }
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

  return <div className="bg-background-primary h-full w-full">{content}</div>
}
