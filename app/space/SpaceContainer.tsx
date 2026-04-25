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

import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { pluginPathPrefix } from '@s4wave/app/urls.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { useShellTabs, useTabId } from '@s4wave/app/ShellTabContext.js'
import { SpaceBody } from './SpaceBody.js'
import { SpaceCommands } from './SpaceCommands.js'
import { SpaceObjectBrowser } from './SpaceObjectBrowser.js'
import { SpacePlugins } from './SpacePlugins.js'
import { SpaceSettingsEditor } from './SpaceSettingsEditor.js'
import { SpaceDataSection } from './SpaceTransformSection.js'
import { CreateObjectButton } from './CreateObjectButton.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'
import { isHiddenSpaceObject } from '@s4wave/web/space/object-tree.js'
import { downloadURL } from '@s4wave/web/download.js'
import { canRenameSpace } from './permissions.js'

// SpaceContainer renders a space shared object body.
export function SpaceContainer() {
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

  const sharedObjectBodyResource = SharedObjectBodyContext.useContext()
  const spaceResource = useResource(
    sharedObjectBodyResource,
    (parentSharedObjectBody) =>
      Promise.resolve(
        parentSharedObjectBody ?
          new Space(parentSharedObjectBody.resourceRef)
        : null,
      ),
    [],
  )
  const space = useResourceValue(spaceResource)

  const spaceWorldResource = useResource(
    spaceResource,
    async (space, signal, cleanup) =>
      space ? cleanup(await space.accessWorldState(true, signal)) : null,
    [],
  )
  const spaceWorld = useResourceValue(spaceWorldResource)

  const spaceContentsResource = useResource(
    spaceResource,
    async (space, signal, cleanup) =>
      space ? cleanup(await space.mountSpaceContents(signal)) : null,
    [],
  )
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
      if (!org.id || !org.spaceIds?.includes(sharedObjectId)) {
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

  const params = useParams()
  const navigate = useNavigate()
  const routerContext = useRouter()
  const subPath = params['*']
  const path = routerContext?.path ?? ''

  // Memoize parentPaths to avoid changing dependencies on every render
  const parentPaths = useMemo(
    () => routerContext?.parentPaths ?? [],
    [routerContext?.parentPaths],
  )
  const currentLevelPath =
    parentPaths.length > 0 ? joinPath(parentPaths, true) : path

  const { objectKey, path: objectPath } = useMemo(
    () => parseObjectUri(subPath),
    [subPath],
  )

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

      navigate({
        path: joinPath(
          [...parentPaths, SUBPATH_DELIMITER, objectKeys[0]],
          true,
        ),
      })
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
  const canRename = canRenameSpace(providerId, canManageSharing)
  const objectCount = useMemo(() => {
    const objects = spaceState?.worldContents?.objects ?? []
    return objects.filter(
      (o) => !isHiddenSpaceObject(o.objectKey, o.objectType),
    ).length
  }, [spaceState?.worldContents?.objects])

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
        {selected ?
          <LuArrowUp {...bottomBarIconProps} aria-hidden="true" />
        : <PiAppStoreLogoLight {...bottomBarIconProps} aria-hidden="true" />}
        <div className="flex-shrink flex-grow truncate">{spaceName}</div>
      </BottomBarItem>
    ),
    [spaceName],
  )

  const sharedObjectDisplayKey = `${sharedObjectId}:${spaceName}`

  const handleSharingClick = useCallback(() => setSharingOpen(true), [])
  const handleDeleteClick = useCallback(() => setDeleteOpen(true), [])

  const redirectTab = useCallback(
    (nextPath: string) => {
      if (tabId) {
        updateTabPath(tabId, nextPath)
      }
      setAppPath(nextPath)
    },
    [tabId, updateTabPath],
  )

  const handleExportClick = useCallback(() => {
    downloadURL(
      `${pluginPathPrefix}/export/u/${sessionIndex}/so/${encodeURIComponent(sharedObjectId)}`,
    )
  }, [sessionIndex, sharedObjectId])
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

  const ready = !!root && !!space && !!spaceWorld && spaceState?.ready
  const sharedObjectOverlay = useMemo(() => {
    if (!ready || !spaceWorld || !spaceState) return undefined
    return (
      <SpaceContainerContext.Provider
        spaceId={sharedObjectId}
        spaceWorldResource={spaceWorldResource}
        spaceWorld={spaceWorld}
        navigateToRoot={navigateToRoot}
        navigateToObjects={navigateToObjects}
        spaceState={spaceState}
        spaceSharingState={spaceSharingState}
        orgState={spaceOrgState}
        buildObjectUrls={buildObjectUrls}
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
            spaceOrgId ?
              <button
                onClick={() => navigate({ path: `../../org/${spaceOrgId}` })}
                className="bg-brand/10 text-brand hover:bg-brand/20 flex shrink-0 items-center gap-1 rounded px-1.5 py-0.5 text-[0.6rem] font-medium transition-colors"
              >
                <LuBuilding2 className="h-2.5 w-2.5" />
                <span className="max-w-20 truncate">
                  {spaceOrgState?.organization?.displayName || 'Org'}
                </span>
              </button>
            : undefined
          }
          orgInfoSection={
            spaceOrgId && spaceOrgState?.organization ?
              <div className="space-y-1">
                <div className="text-foreground-alt mb-0.5 text-[0.6rem] select-none">
                  Organization
                </div>
                <div className="text-foreground flex items-center gap-1.5 text-xs">
                  <LuBuilding2 className="text-brand h-3 w-3 shrink-0" />
                  <span className="truncate">
                    {spaceOrgState.organization.displayName || spaceOrgId}
                  </span>
                  <span className="text-foreground-alt/50 text-[0.6rem]">
                    {spaceOrgState.organization.role === 'org:owner' ?
                      'Owner'
                    : 'Member'}
                  </span>
                </div>
              </div>
            : undefined
          }
          objectsBadge={
            <span className="text-foreground-alt/50 text-[0.55rem]">
              {objectCount}
            </span>
          }
          objectsActions={
            <button
              type="button"
              onClick={handleCreateObject}
              className="text-foreground-alt hover:text-foreground flex h-4 w-4 items-center justify-center transition-colors"
              aria-label="Create object"
              title="Create object"
            >
              <LuPlus className="h-3.5 w-3.5" />
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
    )
  }, [
    ready,
    handleCloseDetails,
    handleSharingClick,
    handleDeleteClick,
    handleExportClick,
    sharedObjectId,
    spaceWorldResource,
    spaceWorld,
    navigateToRoot,
    navigateToObjects,
    spaceState,
    spaceSharingState,
    spaceOrgState,
    buildObjectUrls,
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
  ])

  const handleSharedObjectBreadcrumb = useCallback(() => {
    navigate({ path: currentLevelPath })
  }, [navigate, currentLevelPath])

  return (
    <StateNamespaceProvider namespace={spaceNamespace}>
      <BottomBarLevel
        id="sharedObject"
        button={sharedObjectButton}
        overlay={sharedObjectOverlay}
        buttonKey={sharedObjectDisplayKey}
        overlayKey={sharedObjectDisplayKey}
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
            {spaceState ?
              JSON.stringify(SpaceState.toJson(spaceState), null, 4)
            : 'none'}
          </pre>
        </DebugInfo>
        <SpaceContext.Provider resource={spaceResource}>
          <SpaceContentsContext.Provider resource={spaceContentsResource}>
            {ready ?
              <SpaceContainerContext.Provider
                spaceId={sharedObjectId}
                spaceWorldResource={spaceWorldResource}
                spaceWorld={spaceWorld}
                navigateToRoot={navigateToRoot}
                navigateToObjects={navigateToObjects}
                spaceState={spaceState}
                spaceSharingState={spaceSharingState}
                orgState={spaceOrgState}
                buildObjectUrls={buildObjectUrls}
                objectKey={objectKey}
                objectPath={objectPath || undefined}
                navigateToSubPath={navigateToSubPath}
              >
                <SpaceCommands
                  canRename={canRename}
                  onRenameSpace={handleRenameStart}
                />
                <CreateObjectButton />
                <SpaceBody />
              </SpaceContainerContext.Provider>
            : <div className="flex h-full w-full items-center justify-center p-6">
                <div className="w-full max-w-sm">
                  <LoadingCard
                    view={{
                      state: 'active',
                      title: 'Loading space',
                      detail: spaceWorldLoadingDetail(
                        !!root,
                        !!space,
                        !!spaceWorld,
                        !!spaceState?.ready,
                      ),
                    }}
                  />
                </div>
              </div>
            }
          </SpaceContentsContext.Provider>
        </SpaceContext.Provider>
      </BottomBarLevel>
      <AddUserDialog
        open={sharingOpen}
        onOpenChange={setSharingOpen}
        spaceName={spaceName}
        spaceId={sharedObjectId}
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

// spaceWorldLoadingDetail returns a detail line describing which dependency
// is still outstanding when the space mount is ready but the space world
// state has not finished loading.
function spaceWorldLoadingDetail(
  root: boolean,
  space: boolean,
  spaceWorld: boolean,
  spaceState: boolean,
): string {
  if (!root) return 'Waiting for the session root.'
  if (!space) return 'Mounting the space.'
  if (!spaceWorld) return 'Loading the space world state.'
  if (!spaceState) return 'Preparing the space contents.'
  return 'Finishing space load.'
}
