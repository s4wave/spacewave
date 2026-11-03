import type { StyleXStyles as BottomBarStyles } from '@stylexjs/stylex'
import {
  type ReactNode,
  useMemo,
  useCallback,
  useEffect,
  useState,
} from 'react'
import { joinPath } from '@aptre/bldr'
import { DebugInfo, useWatchStateRpc } from '@aptre/bldr-react'
import { LuArrowUp } from '@s4wave/web/images/icons.js'
import { PiAppStoreLogoLight } from '@s4wave/web/ui/phosphor-icons.js'

import { useNavigate, useParams, useRouter } from '@s4wave/web/router/router.js'
import { useAppNavigation } from '@s4wave/web/sdk/app/environment.js'
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
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { StateNamespaceProvider } from '@s4wave/web/state/index.js'
import {
  parseObjectUri,
  SUBPATH_DELIMITER,
} from '@s4wave/sdk/space/object-uri.js'
import { SpaceSoMeta } from '@s4wave/core/space/space.pb.js'
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
import { SpaceDetails } from './SpaceDetails.js'
import {
  buildObjectRoutePath,
  SpaceTabContextProvider,
} from './SpaceTabContext.js'
import { SpaceMountingScreen } from './SpaceMountingScreen.js'
import {
  spaceMountDetailFromWorld,
  spaceMountStageFromWorld,
  spaceRouteCanRenderBody,
} from './spaceMountStage.js'
import { SpaceCommands } from './SpaceCommands.js'
import { CreateObjectButton } from './CreateObjectButton.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'
import {
  getObjectDisplayName,
  listSpaceObjectTargets,
  type SpaceObjectTargets,
} from '@s4wave/web/space/object-tree.js'
import { createSpaceObjectNavigationActions } from '@s4wave/web/space/space-object-navigation-actions.js'
import { useObjectTypeMetadata } from '@s4wave/web/hooks/useObjectTypeMetadata.js'
import { useWorldQuery } from '@s4wave/web/hooks/useWorldQuery.js'
import { downloadURL } from '@s4wave/web/download.js'
import { canDeleteSpaceObject, canRenameSpace } from './permissions.js'
import {
  useSpaceContentsMount,
  useSpaceMount,
  useSpaceWorldMount,
} from '@s4wave/app/hooks/useSharedMounts.js'

import * as stylex from '@stylexjs/stylex'

const stylexStyles = stylex.create({
  rule1: {
    flexShrink: 1,
    flexGrow: 1,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
  },
})

// noSpaceObjectTargets stands in for the object targets while they load.

const noSpaceObjectTargets: SpaceObjectTargets = { targets: [], more: false }

// useSpaceContainerController owns Space mounting, route projection, object
// navigation, sharing policy, and destructive mutations.
function useSpaceContainerController() {
  const { setAppPath } = useAppNavigation()
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
  const spaceResource = useSpaceMount(sharedObjectBodyResource)
  const space = useResourceValue(spaceResource)

  const spaceWorldResource = useSpaceWorldMount(spaceResource)
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
  const spaceContentsResource = useSpaceContentsMount(spaceResource)
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
  const objectTypeMetadataById = useObjectTypeMetadata(
    rootResource,
    spaceState?.engineId,
  )
  const spaceObjectTargets =
    useWorldQuery(
      spaceWorldResource,
      (world, signal) =>
        listSpaceObjectTargets(world, objectTypeMetadataById, signal),
      [objectTypeMetadataById],
    ).value ?? noSpaceObjectTargets

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
    (selected: boolean, onClick: () => void, sx?: BottomBarStyles) => (
      <BottomBarItem
        selected={selected}
        onClick={onClick}
        sx={sx}
        aria-label={
          selected ? 'Close shared object menu' : 'Open shared object menu'
        }
      >
        {selected ? (
          <LuArrowUp {...bottomBarIconProps} aria-hidden="true" />
        ) : (
          <PiAppStoreLogoLight {...bottomBarIconProps} aria-hidden="true" />
        )}
        <div sx={stylexStyles.rule1}>{spaceName}</div>
      </BottomBarItem>
    ),
    [spaceName],
  )

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
    [tabId, updateTabPath, setAppPath],
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
            targets: spaceObjectTargets.targets,
            moreTargets: spaceObjectTargets.more,
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
    handleCloseDetails,
    handleCreateObject,
    handleDeleteClick,
    handleExportClick,
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
    parentPaths,
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
    sharedObjectId,
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
    spaceObjectTargets,
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
    handleCloseDetails,
    handleCreateObject,
    handleDeleteClick,
    handleExportClick,
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
    parentPaths,
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
    sharedObjectId,
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
    spaceObjectTargets,
    spaceWorldResource,
    switchObjectAtCurrentPosition,
    titleSpaceName,
  } = useSpaceContainerController()

  // The bottom bar item and its details overlay. The overlay renders inside
  // this level, so it reads the same Space state and contexts as the body.
  const renderLevel = (overlay: ReactNode, children: ReactNode) => (
    <BottomBarLevel
      id="sharedObject"
      button={sharedObjectButton}
      overlay={overlay}
      menuLabel={spaceName}
      contextMenuLabel={`${spaceName} actions`}
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
      {children}
    </BottomBarLevel>
  )

  return (
    <StateNamespaceProvider namespace={spaceNamespace}>
      <SpaceContext.Provider resource={spaceResource}>
        <SpaceContentsContext.Provider resource={spaceContentsResource}>
          {ready && routeSpaceState && spaceWorld ? (
            <SpaceContainerContext.Provider
              spaceId={sharedObjectId}
              spaceName={titleSpaceName}
              spaceWorldResource={spaceWorldResource}
              spaceWorld={spaceWorld}
              spaceObjectTargets={spaceObjectTargets}
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
              {renderLevel(
                <SpaceDetails
                  spaceName={spaceName}
                  canRename={canRename}
                  canShare={canManageSharing}
                  orgId={spaceOrgId}
                  onClose={handleCloseDetails}
                  onSharing={handleSharingClick}
                  onExport={handleExportClick}
                  onDelete={handleDeleteClick}
                  onRename={handleRenameStart}
                  onCreateObject={handleCreateObject}
                  onOrgClick={() =>
                    navigate({ path: `../../org/${spaceOrgId}` })
                  }
                />,
                <>
                  <SpaceCommands
                    canRename={canRename}
                    canShare={canManageSharing}
                    onRenameSpace={handleRenameStart}
                    onShareSpace={handleSharingClick}
                  />
                  <CreateObjectButton />
                  <SpaceTabContextProvider parentPaths={parentPaths}>
                    <SpaceBody />
                  </SpaceTabContextProvider>
                </>,
              )}
            </SpaceContainerContext.Provider>
          ) : (
            renderLevel(
              undefined,
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
              />,
            )
          )}
        </SpaceContentsContext.Provider>
      </SpaceContext.Provider>
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
