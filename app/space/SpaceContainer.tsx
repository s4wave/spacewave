import { useMemo, useCallback, useEffect, useState } from 'react'
import { joinPath } from '@aptre/bldr'
import { DebugInfo, useWatchStateRpc } from '@aptre/bldr-react'
import { LuArrowUp, LuBuilding2, LuPlus } from 'react-icons/lu'
import { PiAppStoreLogoLight } from 'react-icons/pi'

import { useNavigate, useParams, useRouter } from '@s4wave/web/router/router.js'
import { setAppPath } from '@s4wave/web/router/app-path.js'
import {
  RootContext,
  SessionContext,
  SharedObjectBodyContext,
  SharedObjectContext,
  SpaceContext,
  SpaceContentsContext,
  useSessionIndex,
} from '@s4wave/web/contexts/contexts.js'
import { useIsTabActive } from '@s4wave/web/contexts/TabActiveContext.js'
import { SpacewaveOrgListContext } from '@s4wave/web/contexts/SpacewaveOrgListContext.js'
import {
  useResource,
  useResourceValue,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import { StateNamespaceProvider } from '@s4wave/web/state/index.js'
import {
  parseObjectUri,
  SUBPATH_DELIMITER,
} from '@s4wave/sdk/space/object-uri.js'
import { SpaceSoMeta } from '@s4wave/core/space/space.pb.js'
import { Space } from '@s4wave/sdk/space/space.js'
import {
  SpaceSharingState,
  SpaceState,
  WatchSpaceSharingStateRequest,
  WatchSpaceStateRequest,
} from '@s4wave/sdk/space/space.pb.js'
import {
  WatchOrganizationStateRequest,
  WatchOrganizationStateResponse,
} from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import {
  WatchResourcesListRequest,
  WatchResourcesListResponse,
} from '@s4wave/sdk/session/session.pb.js'

import { BottomBarLevel } from '@s4wave/web/frame/bottom-bar-level.js'
import { BottomBarItem } from '@s4wave/web/frame/bottom-bar-item.js'
import { bottomBarIconProps } from '@s4wave/web/frame/bottom-icon-props.js'
import { SharedObjectDetails } from '@s4wave/app/sobject/SharedObjectDetails.js'
import { AddUserDialog } from '@s4wave/app/sobject/AddUserDialog.js'
import { DeleteSpaceDialog } from '@s4wave/app/sobject/DeleteSpaceDialog.js'
import { RenameSpaceDialog } from '@s4wave/app/sobject/RenameSpaceDialog.js'
import { useBottomBarSetOpenMenu } from '@s4wave/web/frame/bottom-bar-context.js'
import { useOpenCommand } from '@s4wave/web/command/CommandContext.js'
import { toast } from '@s4wave/web/ui/toaster.js'

import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import {
  getSpaceDocumentTitleName,
  useDocumentTitle,
} from '@s4wave/web/title/DocumentTitleContext.js'
import { pluginPathPrefix } from '@s4wave/app/urls.js'
import { useShellTabs, useTabId } from '@s4wave/app/ShellTabContext.js'
import { SpaceBody } from './SpaceBody.js'
import { SpaceMountingScreen } from './SpaceMountingScreen.js'
import {
  spaceMountDetailFromWorld,
  spaceMountStageFromWorld,
  spaceRouteCanRenderBody,
} from './spaceMountStage.js'
import { SpaceCommands } from './SpaceCommands.js'
import { SpaceObjectBrowser } from './SpaceObjectBrowser.js'
import { SpacePlugins } from './SpacePlugins.js'
import { SpaceSettingsEditor } from './SpaceSettingsEditor.js'
import { SpaceDataSection } from './SpaceTransformSection.js'
import { CreateObjectButton } from './CreateObjectButton.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'
import {
  buildSpaceObjectActionTargets,
  getObjectDisplayName,
} from '@s4wave/web/space/object-tree.js'
import { createSpaceObjectNavigationActions } from '@s4wave/web/space/space-object-navigation-actions.js'
import { useObjectTypeMetadata } from '@s4wave/web/hooks/useObjectTypeMetadata.js'
import { downloadURL } from '@s4wave/web/download.js'
import { canDeleteSpaceObject, canRenameSpace } from './permissions.js'
import {
  consumeQuickstartSpaceContentsHandoff,
  consumeQuickstartSpaceHandoff,
  consumeQuickstartSpaceWorldHandoff,
  releaseQuickstartSharedObjectHandoff,
} from '@s4wave/app/quickstart/session-handoff.js'
import { markQuickstartStartupBoundary } from '@s4wave/app/quickstart/startup-boundary.js'

const quickstartSpaceStartupLabels: Record<string, string> = {
  'quickstart route using space handoff': 'quickstart.space-handoff-used',
  'quickstart space resource created': 'quickstart.space-resource-created',
  'quickstart route using space world handoff':
    'quickstart.space-world-handoff-used',
  'quickstart access world start': 'quickstart.space-world-access-start',
  'quickstart access world finish': 'quickstart.space-world-access-ready',
  'quickstart route using space contents handoff':
    'quickstart.space-contents-handoff-used',
  'quickstart mount contents start': 'quickstart.space-contents-mount-start',
  'quickstart mount contents finish': 'quickstart.space-contents-mount-ready',
}

export function buildObjectRoutePath(
  parentPaths: readonly string[],
  objectKey: string,
): string {
  return joinPath([...parentPaths, SUBPATH_DELIMITER, objectKey], true)
}

function logQuickstartSpaceDiagnostic(
  message: string,
  fields: Record<string, unknown>,
): void {
  const startupLabel = quickstartSpaceStartupLabels[message]
  if (startupLabel) {
    markQuickstartStartupBoundary(startupLabel, fields)
  }
  if (
    !(globalThis as { __s4waveLogQuickstartTiming?: boolean })
      .__s4waveLogQuickstartTiming
  ) {
    return
  }
  console.log(message + ': ' + JSON.stringify(fields))
}

// useSpaceContainerController owns Space mounting, route projection, object
// navigation, sharing policy, and destructive mutations.
function useSpaceContainerController() {
  const rootResource = RootContext.useContext()
  const root = useResourceValue(rootResource)

  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const { providerId } = useSessionInfo(session)
  const orgListCtx = SpacewaveOrgListContext.useContextSafe()
  const sessionIndex = useSessionIndex()
  const tabId = useTabId()
  const { updateTabPath } = useShellTabs()
  const openCommand = useOpenCommand()
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [sharingOpen, setSharingOpen] = useState(false)
  const [renameOpen, setRenameOpen] = useState(false)

  const sharedObjectResource = SharedObjectContext.useContext()
  const sharedObject = useResourceValue(sharedObjectResource)
  const sharedObjectId = sharedObject?.meta.sharedObjectId ?? ''
  const params = useParams()
  const subPath = params['*']
  const { objectKey, path: objectPath } = useMemo(
    () => parseObjectUri(subPath),
    [subPath],
  )

  const sharedObjectBodyResource = SharedObjectBodyContext.useContext()
  const spaceResource = useResource(
    sharedObjectBodyResource,
    (parentSharedObjectBody, _signal, cleanup) => {
      if (!parentSharedObjectBody) {
        return Promise.resolve(null)
      }
      const handoff = consumeQuickstartSpaceHandoff(
        sessionIndex,
        sharedObjectId,
      )
      if (handoff) {
        logQuickstartSpaceDiagnostic('quickstart route using space handoff', {
          sharedObjectId,
          spaceResourceId: handoff.id,
          released: handoff.released,
        })
        return Promise.resolve(cleanup(handoff))
      }
      const space = new Space(
        parentSharedObjectBody.resourceRef.createRef(parentSharedObjectBody.id),
      )
      logQuickstartSpaceDiagnostic('quickstart space resource created', {
        sharedObjectId,
        bodyResourceId: parentSharedObjectBody.id,
      })
      return Promise.resolve(cleanup(space))
    },
    [sessionIndex, sharedObjectId],
  )
  const space = useResourceValue(spaceResource)

  const spaceWorldResource = useResource(
    spaceResource,
    async (space, signal, cleanup) => {
      if (!space) {
        return null
      }
      const handoff = consumeQuickstartSpaceWorldHandoff(
        sessionIndex,
        sharedObjectId,
      )
      if (handoff) {
        logQuickstartSpaceDiagnostic(
          'quickstart route using space world handoff',
          {
            sharedObjectId,
          },
        )
        return cleanup(handoff)
      }
      logQuickstartSpaceDiagnostic('quickstart access world start', {
        sharedObjectId,
        spaceResourceId: space.id,
      })
      const state = await space.accessWorldState(true, signal)
      logQuickstartSpaceDiagnostic('quickstart access world finish', {
        sharedObjectId,
        spaceResourceId: space.id,
      })
      return cleanup(state)
    },
    [sessionIndex, sharedObjectId],
  )
  const spaceWorld = useResourceValue(spaceWorldResource)

  // watch the space state
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
  const spaceContentsResource = useResource(
    spaceResource,
    async (space, signal, cleanup) => {
      if (!space) {
        return null
      }
      const handoff = consumeQuickstartSpaceContentsHandoff(
        sessionIndex,
        sharedObjectId,
      )
      if (handoff) {
        logQuickstartSpaceDiagnostic(
          'quickstart route using space contents handoff',
          {
            sharedObjectId,
          },
        )
        return cleanup(handoff)
      }
      logQuickstartSpaceDiagnostic('quickstart mount contents start', {
        sharedObjectId,
        spaceResourceId: space.id,
      })
      const contents = await space.mountSpaceContents(signal)
      logQuickstartSpaceDiagnostic('quickstart mount contents finish', {
        sharedObjectId,
        spaceResourceId: space.id,
      })
      return cleanup(contents)
    },
    [sessionIndex, sharedObjectId],
  )
  // Subscribe this component to contents resource updates; consumers read
  // the resource through SpaceContentsContext.
  const _spaceContents = useResourceValue(spaceContentsResource)
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
  const spaceSharingState = useWatchStateRpc(
    useCallback(
      (req: WatchSpaceSharingStateRequest, signal: AbortSignal) =>
        space?.watchSpaceSharingState(req, signal) ?? null,
      [space],
    ),
    {},
    WatchSpaceSharingStateRequest.equals,
    SpaceSharingState.equals,
  )

  useEffect(() => {
    return () => {
      releaseQuickstartSharedObjectHandoff(sessionIndex, sharedObjectId)
    }
  }, [sessionIndex, sharedObjectId])

  const canManageSharing = spaceSharingState?.canManage ?? false
  const spaceOrgId = useMemo(() => {
    const orgs = orgListCtx?.organizations ?? []
    for (const org of orgs) {
      if (!org.id || !org.spaceIds) {
        continue
      }
      const spaceIds = new Set(org.spaceIds)
      if (!spaceIds.has(sharedObjectId)) {
        continue
      }
      return org.id
    }
    return ''
  }, [orgListCtx?.organizations, sharedObjectId])
  const spaceOrgState = useWatchStateRpc(
    useCallback(
      (req: WatchOrganizationStateRequest, signal: AbortSignal) => {
        const orgId = req.orgId ?? spaceOrgId
        if (!session || !orgId) return null
        return session.spacewave.watchOrganizationState(orgId, signal)
      },
      [session, spaceOrgId],
    ),
    spaceOrgId ? { orgId: spaceOrgId } : {},
    WatchOrganizationStateRequest.equals,
    WatchOrganizationStateResponse.equals,
  )

  const navigate = useNavigate()
  const routerContext = useRouter()
  const path = routerContext?.path ?? ''

  // Memoize parentPaths to avoid changing dependencies on every render
  const parentPaths = useMemo(
    () => routerContext?.parentPaths ?? [],
    [routerContext?.parentPaths],
  )
  const currentLevelPath =
    parentPaths.length > 0 ? joinPath(parentPaths, true) : path

  // Normalize: if subPath is non-empty but objectKey is empty (e.g. bare "-"),
  // redirect to the clean URL without the trailing subpath delimiter.
  useEffect(() => {
    if (subPath && !objectKey) {
      navigate({ path: joinPath([...parentPaths], true), replace: true })
    }
  }, [subPath, objectKey, navigate, parentPaths])

  const buildObjectUrls = useCallback(
    (objectKeys: string[]): string[] =>
      objectKeys.map((objectKey) =>
        new URL(
          joinPath([...parentPaths, 'k', objectKey], true),
          window.location.origin,
        ).toString(),
      ),
    [parentPaths],
  )

  const navigateToRoot = useCallback(() => {
    navigate({ path: joinPath([...parentPaths], true) })
  }, [navigate, parentPaths])

  // Namespace for space-specific state within the tab's state atom
  const spaceNamespace = useMemo(
    () => (sharedObjectId ? ['space', sharedObjectId] : []),
    [sharedObjectId],
  )

  const navigateToObjects = useCallback(
    (objectKeys: string[]) => {
      if (objectKeys.length === 0) return

      navigate({ path: buildObjectRoutePath(parentPaths, objectKeys[0]) })
    },
    [navigate, parentPaths],
  )

  const navigateToSubPath = useCallback(
    (subpath: string) => {
      navigate({
        path: joinPath([...parentPaths, SUBPATH_DELIMITER, subpath], true),
      })
    },
    [navigate, parentPaths],
  )

  const setOpenMenu = useBottomBarSetOpenMenu()

  const handleCloseDetails = useCallback(() => {
    setOpenMenu?.('')
  }, [setOpenMenu])

  const spaceName = useMemo(() => {
    const currentEntry = resourcesList?.spacesList?.find(
      (entry) => entry.entry?.ref?.providerResourceRef?.id === sharedObjectId,
    )
    if (currentEntry?.spaceMeta?.name) {
      return currentEntry.spaceMeta.name
    }
    const bodyMeta = sharedObject?.meta?.sharedObjectMeta?.bodyMeta
    if (!bodyMeta || bodyMeta.length === 0) return sharedObjectId
    const meta = SpaceSoMeta.fromBinary(bodyMeta)
    return meta.name || sharedObjectId
  }, [resourcesList, sharedObject, sharedObjectId])
  const titleSpaceName = getSpaceDocumentTitleName(spaceName, sharedObjectId)
  const isTabActive = useIsTabActive()
  useDocumentTitle(
    {
      view: objectKey ? getObjectDisplayName(objectKey) || 'Object' : undefined,
      space: titleSpaceName,
    },
    { active: isTabActive, priority: 10 },
  )
  const canRename = canRenameSpace(providerId, canManageSharing)
  const canDeleteObjects = canDeleteSpaceObject(
    providerId,
    spaceSharingState?.viewerRole,
  )
  const canRenderBody = spaceRouteCanRenderBody(
    !!root,
    !!space,
    !!spaceWorld,
    !!spaceState?.ready,
    objectKey,
  )
  const routeSpaceState = useMemo<SpaceState | null>(() => {
    if (spaceState?.ready) {
      return spaceState
    }
    if (canRenderBody && objectKey) {
      return { ready: true }
    }
    return spaceState
  }, [canRenderBody, objectKey, spaceState])
  const objectTypeMetadataById = useObjectTypeMetadata(rootResource)
  const spaceObjectTargets = useMemo(
    () =>
      buildSpaceObjectActionTargets(
        routeSpaceState?.worldContents?.objects ?? [],
        objectTypeMetadataById,
      ),
    [routeSpaceState?.worldContents?.objects, objectTypeMetadataById],
  )
  const objectCount = spaceObjectTargets.length

  const handleRenameStart = useCallback(() => {
    if (!canRename) return
    setRenameOpen(true)
  }, [canRename])

  const handleRenameConfirm = useCallback(
    async (newName: string) => {
      if (!session) return
      await session.renameSpace({
        sharedObjectId,
        displayName: newName,
      })
    },
    [session, sharedObjectId],
  )

  const sharedObjectButton = useCallback(
    (selected: boolean, onClick: () => void, className?: string) => (
      <BottomBarItem
        selected={selected}
        onClick={onClick}
        className={className}
        aria-label={
          selected ? 'Close shared object menu' : 'Open shared object menu'
        }
      >
        {selected ? (
          <LuArrowUp {...bottomBarIconProps} aria-hidden="true" />
        ) : (
          <PiAppStoreLogoLight {...bottomBarIconProps} aria-hidden="true" />
        )}
        <div className="flex-shrink flex-grow truncate">{spaceName}</div>
      </BottomBarItem>
    ),
    [spaceName],
  )

  const sharedObjectDisplayKey = `${sharedObjectId}:${spaceName}`
  const ready = canRenderBody

  const handleSharingClick = useCallback(() => setSharingOpen(true), [])
  const handleDeleteClick = useCallback(() => setDeleteOpen(true), [])

  const redirectTab = useCallback(
    (nextPath: string) => {
      if (tabId) {
        void updateTabPath(tabId, nextPath)
      }
      setAppPath(nextPath)
    },
    [tabId, updateTabPath],
  )
  const switchObjectAtCurrentPosition = useCallback(
    ({ objectKey }: { objectKey: string }) => {
      if (!objectKey) return
      redirectTab(buildObjectRoutePath(parentPaths, objectKey))
    },
    [parentPaths, redirectTab],
  )
  const handleOpenSpaceObject = useCallback(
    (target: { objectKey: string }) => {
      navigateToObjects([target.objectKey])
    },
    [navigateToObjects],
  )
  const handleSwitchSpaceObject = useCallback(
    (target: { objectKey: string }) => {
      switchObjectAtCurrentPosition({ objectKey: target.objectKey })
    },
    [switchObjectAtCurrentPosition],
  )
  const sharedObjectContextMenuItems = useMemo(
    () =>
      ready
        ? createSpaceObjectNavigationActions({
            targets: spaceObjectTargets,
            currentObjectKey: objectKey,
            openDetails: () => {},
            openObject: handleOpenSpaceObject,
            switchObjectHere: handleSwitchSpaceObject,
          })
        : undefined,
    [
      ready,
      spaceObjectTargets,
      objectKey,
      handleOpenSpaceObject,
      handleSwitchSpaceObject,
    ],
  )
  const sharedObjectContextMenuKey = useMemo(
    () =>
      [
        sharedObjectDisplayKey,
        objectKey ?? 'none',
        spaceObjectTargets
          .map((target) => `${target.objectKey}:${target.objectType}`)
          .join('|'),
      ].join(':'),
    [sharedObjectDisplayKey, objectKey, spaceObjectTargets],
  )

  const buildExportUrl = useCallback(
    () =>
      `${pluginPathPrefix}/export/u/${sessionIndex}/so/${encodeURIComponent(sharedObjectId)}`,
    [sessionIndex, sharedObjectId],
  )
  const handleExportClick = useCallback(() => {
    void downloadURL(buildExportUrl()).catch((err: unknown) => {
      console.error('failed to export space', err)
      toast.error('Export failed', { description: String(err) })
    })
  }, [buildExportUrl])
  const handleCreateObject = useCallback(() => {
    openCommand('spacewave.create-object')
  }, [openCommand])

  const handleDeleteConfirm = useCallback(async () => {
    if (!session) return
    const nextPath = `/u/${sessionIndex}`
    redirectTab(nextPath)
    try {
      await session.deleteSpace(sharedObjectId)
    } catch (err) {
      if (path) {
        queueMicrotask(() => redirectTab(path))
      }
      throw err
    }
  }, [session, sharedObjectId, sessionIndex, path, redirectTab])

  const sharedObjectOverlay = useMemo(() => {
    if (!ready || !spaceWorld || !routeSpaceState) return undefined
    return (
      <SpaceContext.Provider resource={spaceResource}>
        <SpaceContentsContext.Provider resource={spaceContentsResource}>
          <SpaceContainerContext.Provider
            spaceId={sharedObjectId}
            spaceName={titleSpaceName}
            spaceWorldResource={spaceWorldResource}
            spaceWorld={spaceWorld}
            navigateToRoot={navigateToRoot}
            navigateToObjects={navigateToObjects}
            switchObjectAtCurrentPosition={switchObjectAtCurrentPosition}
            spaceState={routeSpaceState}
            spaceSharingState={spaceSharingState}
            orgState={spaceOrgState}
            buildObjectUrls={buildObjectUrls}
            buildExportUrl={buildExportUrl}
            objectKey={objectKey}
            objectPath={objectPath || undefined}
            navigateToSubPath={navigateToSubPath}
          >
            <SharedObjectDetails
              displayName={spaceName}
              canRename={canRename}
              canShare={canManageSharing}
              onCloseClick={handleCloseDetails}
              onSharingClick={canManageSharing ? handleSharingClick : undefined}
              onExportClick={handleExportClick}
              onDeleteClick={handleDeleteClick}
              onRenameStart={handleRenameStart}
              orgIndicator={
                spaceOrgId ? (
                  <button
                    onClick={() =>
                      navigate({ path: `../../org/${spaceOrgId}` })
                    }
                    className="bg-brand/10 text-brand hover:bg-brand/20 flex shrink-0 items-center gap-1 rounded px-1.5 py-0.5 text-xs font-medium transition-colors"
                  >
                    <LuBuilding2 className="size-2.5" />
                    <span className="max-w-20 truncate">
                      {spaceOrgState?.organization?.displayName || 'Org'}
                    </span>
                  </button>
                ) : undefined
              }
              orgInfoSection={
                spaceOrgId && spaceOrgState?.organization ? (
                  <div className="space-y-1">
                    <div className="text-foreground-alt mb-0.5 text-xs select-none">
                      Organization
                    </div>
                    <div className="text-foreground flex items-center gap-1.5 text-xs">
                      <LuBuilding2 className="text-brand size-3 shrink-0" />
                      <span className="truncate">
                        {spaceOrgState.organization.displayName || spaceOrgId}
                      </span>
                      <span className="text-foreground-alt/50 text-xs">
                        {spaceOrgState.organization.role === 'org:owner'
                          ? 'Owner'
                          : 'Member'}
                      </span>
                    </div>
                  </div>
                ) : undefined
              }
              objectsBadge={
                <span className="text-foreground-alt/50 text-xs">
                  {objectCount}
                </span>
              }
              objectsActions={
                <button
                  type="button"
                  onClick={handleCreateObject}
                  className="text-foreground-alt hover:text-foreground flex size-4 items-center justify-center transition-colors"
                  aria-label="Create object"
                  title="Create object"
                >
                  <LuPlus className="size-3.5" />
                </button>
              }
              objectsSection={<SpaceObjectBrowser embedded={true} />}
              settingsSection={
                <SpaceSettingsEditor
                  canEdit={true}
                  canRename={canRename}
                  displayName={spaceName}
                  embedded={true}
                  onRenameStart={handleRenameStart}
                />
              }
              dataSection={<SpaceDataSection />}
              pluginsSection={<SpacePlugins />}
            />
          </SpaceContainerContext.Provider>
        </SpaceContentsContext.Provider>
      </SpaceContext.Provider>
    )
  }, [
    ready,
    handleCloseDetails,
    handleSharingClick,
    handleDeleteClick,
    handleExportClick,
    sharedObjectId,
    titleSpaceName,
    spaceWorldResource,
    spaceWorld,
    navigateToRoot,
    navigateToObjects,
    switchObjectAtCurrentPosition,
    routeSpaceState,
    spaceSharingState,
    spaceOrgState,
    buildObjectUrls,
    buildExportUrl,
    objectKey,
    objectPath,
    navigateToSubPath,
    canRename,
    canManageSharing,
    spaceName,
    handleRenameStart,
    objectCount,
    handleCreateObject,
    navigate,
    spaceOrgId,
    spaceResource,
    spaceContentsResource,
  ])

  const handleSharedObjectBreadcrumb = useCallback(() => {
    navigate({ path: currentLevelPath })
  }, [navigate, currentLevelPath])

  return {
    buildExportUrl,
    buildObjectUrls,
    canDeleteObjects,
    canManageSharing,
    canRename,
    deleteOpen,
    handleDeleteConfirm,
    handleRenameConfirm,
    handleRenameStart,
    handleSharedObjectBreadcrumb,
    handleSharingClick,
    navigate,
    navigateToObjects,
    navigateToRoot,
    navigateToSubPath,
    objectKey,
    objectPath,
    providerId,
    ready,
    renameOpen,
    root,
    routeSpaceState,
    setDeleteOpen,
    setRenameOpen,
    setSharingOpen,
    sharedObjectButton,
    sharedObjectContextMenuItems,
    sharedObjectContextMenuKey,
    sharedObjectDisplayKey,
    sharedObjectId,
    sharedObjectOverlay,
    sharingOpen,
    space,
    spaceContentsResource,
    spaceNamespace,
    spaceName,
    spaceOrgId,
    spaceOrgState,
    spaceResource,
    spaceSharingState,
    spaceState,
    spaceWorld,
    spaceWorldResource,
    switchObjectAtCurrentPosition,
    titleSpaceName,
  }
}

// SpaceContainer renders a Space shared-object body from the mount and route
// controller projection.
export function SpaceContainer() {
  const {
    buildExportUrl,
    buildObjectUrls,
    canDeleteObjects,
    canManageSharing,
    canRename,
    deleteOpen,
    handleDeleteConfirm,
    handleRenameConfirm,
    handleRenameStart,
    handleSharedObjectBreadcrumb,
    handleSharingClick,
    navigate,
    navigateToObjects,
    navigateToRoot,
    navigateToSubPath,
    objectKey,
    objectPath,
    providerId,
    ready,
    renameOpen,
    root,
    routeSpaceState,
    setDeleteOpen,
    setRenameOpen,
    setSharingOpen,
    sharedObjectButton,
    sharedObjectContextMenuItems,
    sharedObjectContextMenuKey,
    sharedObjectDisplayKey,
    sharedObjectId,
    sharedObjectOverlay,
    sharingOpen,
    space,
    spaceContentsResource,
    spaceNamespace,
    spaceName,
    spaceOrgId,
    spaceOrgState,
    spaceResource,
    spaceSharingState,
    spaceState,
    spaceWorld,
    spaceWorldResource,
    switchObjectAtCurrentPosition,
    titleSpaceName,
  } = useSpaceContainerController()

  return (
    <StateNamespaceProvider namespace={spaceNamespace}>
      <BottomBarLevel
        id="sharedObject"
        button={sharedObjectButton}
        overlay={sharedObjectOverlay}
        buttonKey={sharedObjectDisplayKey}
        overlayKey={sharedObjectDisplayKey}
        menuLabel={spaceName}
        contextMenuLabel={`${spaceName} actions`}
        contextMenuKey={sharedObjectContextMenuKey}
        contextMenuItems={sharedObjectContextMenuItems}
        onBreadcrumbClick={handleSharedObjectBreadcrumb}
      >
        <DebugInfo>
          Shared Object ID: {sharedObjectId}
          <br />
          Space loaded: {(!!space).toString()}
          <br />
          Space World loaded: {(!!spaceWorld).toString()}
          <br />
          Space state ready: {(!!spaceState?.ready).toString()}
          <br />
          Object key: {objectKey ?? 'none'}
          <br />
          Space state:{' '}
          <pre>
            {spaceState
              ? JSON.stringify(SpaceState.toJson(spaceState), null, 4)
              : 'none'}
          </pre>
        </DebugInfo>
        <SpaceContext.Provider resource={spaceResource}>
          <SpaceContentsContext.Provider resource={spaceContentsResource}>
            {ready && routeSpaceState && spaceWorld ? (
              <SpaceContainerContext.Provider
                spaceId={sharedObjectId}
                spaceName={titleSpaceName}
                spaceWorldResource={spaceWorldResource}
                spaceWorld={spaceWorld}
                navigateToRoot={navigateToRoot}
                navigateToObjects={navigateToObjects}
                switchObjectAtCurrentPosition={switchObjectAtCurrentPosition}
                spaceState={routeSpaceState}
                spaceSharingState={spaceSharingState}
                orgState={spaceOrgState}
                canDeleteObjects={canDeleteObjects}
                buildObjectUrls={buildObjectUrls}
                buildExportUrl={buildExportUrl}
                objectKey={objectKey}
                objectPath={objectPath || undefined}
                navigateToSubPath={navigateToSubPath}
              >
                <SpaceCommands
                  canRename={canRename}
                  canShare={canManageSharing}
                  onRenameSpace={handleRenameStart}
                  onShareSpace={handleSharingClick}
                />
                <CreateObjectButton />
                <SpaceBody />
              </SpaceContainerContext.Provider>
            ) : (
              <SpaceMountingScreen
                stage={spaceMountStageFromWorld(
                  !!root,
                  !!space,
                  !!spaceWorld,
                  !!spaceState?.ready,
                )}
                detail={spaceMountDetailFromWorld(
                  !!root,
                  !!space,
                  !!spaceWorld,
                  !!spaceState?.ready,
                )}
                onBack={() => navigate({ path: '../' })}
              />
            )}
          </SpaceContentsContext.Provider>
        </SpaceContext.Provider>
      </BottomBarLevel>
      <AddUserDialog
        open={sharingOpen}
        onOpenChange={setSharingOpen}
        spaceName={spaceName}
        spaceId={sharedObjectId}
        isCloudProvider={providerId === 'spacewave'}
        orgId={spaceOrgId}
        orgMembers={spaceOrgState?.members ?? []}
        orgMembersLoading={!!spaceOrgId && !spaceOrgState}
      />
      <DeleteSpaceDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        spaceName={spaceName}
        onConfirm={handleDeleteConfirm}
      />
      <RenameSpaceDialog
        open={renameOpen}
        onOpenChange={setRenameOpen}
        spaceName={spaceName}
        onConfirm={handleRenameConfirm}
      />
    </StateNamespaceProvider>
  )
}
