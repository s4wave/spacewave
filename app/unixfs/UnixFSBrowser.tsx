import {
  type ChangeEvent,
  type ComponentType,
  type MouseEvent,
  useCallback,
  useDeferredValue,
  useMemo,
  useReducer,
  useRef,
} from 'react'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import { ObjectLayoutTab } from '@s4wave/sdk/layout/world/world.pb.js'
import { MknodType } from '@s4wave/sdk/unixfs/index.js'
import type { FSHandle } from '@s4wave/sdk/unixfs/handle.js'
import {
  getUnixFSParentPath,
  joinUnixFSDisplayPath,
} from '@s4wave/sdk/unixfs/path.js'
import { ObjectInfo, UnixfsObjectInfo } from '@s4wave/web/object/object.pb.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { FileEntry } from '@s4wave/web/editors/file-browser/types.js'
import type { ListItem } from '@s4wave/web/ui/list'
import { toast } from '@s4wave/web/ui/toaster.js'
import { useTabContext } from '@s4wave/web/object/TabContext.js'
import { UnixFSPathLoadingCard } from '@s4wave/app/loading/wrappers/UnixFSPathLoadingCard.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { useHistory } from '@s4wave/web/router/HistoryRouter.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { useSessionIndex } from '@s4wave/web/contexts/contexts.js'
import { useSessionUploadManager } from '@s4wave/app/session/SessionUploadManagerContext.js'
import {
  type SessionSyncStatusView,
  useSessionSyncStatus,
} from '../session/SessionSyncStatusContext.js'
import { UnixFSFileViewer } from './UnixFSFileViewer.js'
import type { ContextMenuState } from './UnixFSContextMenu.js'
import { UnixFSBrowserDialogs } from './UnixFSBrowserDialogs.js'
import { UnixFSBrowserDropSurface } from './UnixFSBrowserDropSurface.js'
import { UnixFSBrowserShell } from './UnixFSBrowserShell.js'
import { UnixFSDirectoryListing } from './UnixFSDirectoryListing.js'
import { UnixFSBrowserUnavailableState } from './UnixFSBrowserUnavailableState.js'
import {
  buildUnixFSFileInlineURL,
  downloadUnixFSSelection,
} from './download.js'
import {
  buildUnixFSMoveItems,
  moveUnixFSItemsFromDirectory,
  type UnixFSMoveItem,
} from './move.js'
import { useUnixFSBrowserCommands } from './useUnixFSBrowserCommands.js'
import { useUnixFSBrowserDrag } from './useUnixFSBrowserDrag.js'
import { useUnixFSBrowserDragTargets } from './useUnixFSBrowserDragTargets.js'
import { useUnixFSBrowserResources } from './useUnixFSBrowserResources.js'
import { useUnixFSInlineEntryRenderer } from './useUnixFSInlineEntryRenderer.js'
import { useUnixFSDeleteKeyHandler } from './useUnixFSDeleteKeyHandler.js'
import { useUnixFSStartupBoundaries } from './useUnixFSStartupBoundaries.js'

export interface UnixFSBrowserBodyProps {
  rootHandle: Resource<FSHandle>
  unixfsId: string
  currentPath: string
}

export interface UnixFSBrowserDirectoryHeaderProps {
  currentPath: string
  entries: FileEntry[]
  onNewFolder: () => void
  onOpen: (entries: FileEntry[]) => void
  onUploadFiles: () => void
}

// UnixFSBrowserProps are the props passed to the UnixFSBrowser component.
export interface UnixFSBrowserProps {
  // unixfsId is the identifier for the UnixFS on the bus (the object key).
  unixfsId: string
  // basePath is the base path within the UnixFS.
  basePath: string
  // currentPath is the current navigation path within the tab.
  currentPath: string
  // mimeTypeOverride is the optional file MIME type carried by UnixfsObjectInfo.
  mimeTypeOverride?: string
  // worldState is the world state resource for accessing typed objects.
  worldState: Resource<IWorldState>
  // browserBody replaces the browser's default file list/file viewer body.
  browserBody?: ComponentType<UnixFSBrowserBodyProps>
  // directoryHeader renders owner-specific content above the generic file list.
  directoryHeader?: ComponentType<UnixFSBrowserDirectoryHeaderProps>
}

function buildUnixFSLoadingStageLabel({
  rootLoading,
  pathLoading,
  statLoading,
  entriesLoading,
}: {
  rootLoading: boolean
  pathLoading: boolean
  statLoading: boolean
  entriesLoading: boolean
}): string {
  if (rootLoading) return 'mounting UnixFS root'
  if (pathLoading) return 'resolving path'
  if (statLoading) return 'reading path metadata'
  if (entriesLoading) return 'reading directory entries'
  return 'waiting for filesystem resource'
}

function UnixFSLoadingDiagnostics({
  stageLabel,
  status,
}: {
  stageLabel: string
  status: SessionSyncStatusView
}) {
  return (
    <div
      className="text-foreground-alt/60 max-w-xl space-y-1 text-center font-mono text-[0.68rem] leading-relaxed"
      data-testid="unixfs-loading-diagnostics"
    >
      <div>Stage: {stageLabel}</div>
      <div>
        Pack reads: ranges {status.packRangeLabel}; index tail{' '}
        {status.packIndexTailLabel}
      </div>
      <div>
        Lookup: {status.packLookupLabel}; cache {status.packIndexCacheLabel}
      </div>
    </div>
  )
}

interface UnixFSBrowserState {
  pendingName: string | null
  contextMenu: ContextMenuState | null
  selectedIds: string[]
  newFolderName: string | null
  newFileName: string | null
  deleteTargets: FileEntry[] | null
  moveDialogItems: UnixFSMoveItem[] | null
  renamingEntry: FileEntry | null
  isDragging: boolean
  folderDropEntryId: string | null
}

type UnixFSBrowserAction =
  | { type: 'set-pending-name'; name: string | null }
  | { type: 'set-context-menu'; menu: ContextMenuState | null }
  | { type: 'set-selected-ids'; ids: string[] }
  | { type: 'start-rename'; entry: FileEntry }
  | { type: 'clear-rename' }
  | { type: 'request-delete'; entries: FileEntry[] }
  | { type: 'clear-delete' }
  | { type: 'request-move'; items: UnixFSMoveItem[] }
  | { type: 'clear-move' }
  | { type: 'complete-move' }
  | { type: 'start-new-folder' }
  | { type: 'set-new-folder-name'; name: string }
  | { type: 'clear-new-folder' }
  | { type: 'start-new-file' }
  | { type: 'set-new-file-name'; name: string }
  | { type: 'clear-new-file' }
  | { type: 'set-dragging'; dragging: boolean }
  | { type: 'set-folder-drop-entry'; id: string | null }

const initialUnixFSBrowserState: UnixFSBrowserState = {
  pendingName: null,
  contextMenu: null,
  selectedIds: [],
  newFolderName: null,
  newFileName: null,
  deleteTargets: null,
  moveDialogItems: null,
  renamingEntry: null,
  isDragging: false,
  folderDropEntryId: null,
}

function unixFSBrowserReducer(
  state: UnixFSBrowserState,
  action: UnixFSBrowserAction,
): UnixFSBrowserState {
  switch (action.type) {
    case 'set-pending-name':
      return { ...state, pendingName: action.name }
    case 'set-context-menu':
      return { ...state, contextMenu: action.menu }
    case 'set-selected-ids':
      return { ...state, selectedIds: action.ids }
    case 'start-rename':
      return { ...state, renamingEntry: action.entry }
    case 'clear-rename':
      return { ...state, renamingEntry: null }
    case 'request-delete':
      return { ...state, deleteTargets: action.entries }
    case 'clear-delete':
      return { ...state, deleteTargets: null }
    case 'request-move':
      return {
        ...state,
        contextMenu: null,
        moveDialogItems: action.items,
      }
    case 'clear-move':
      return { ...state, moveDialogItems: null }
    case 'complete-move':
      return { ...state, selectedIds: [], moveDialogItems: null }
    case 'start-new-folder':
      return {
        ...state,
        contextMenu: null,
        newFolderName: '',
        newFileName: null,
        renamingEntry: null,
      }
    case 'set-new-folder-name':
      return { ...state, newFolderName: action.name }
    case 'clear-new-folder':
      return { ...state, newFolderName: null }
    case 'start-new-file':
      return {
        ...state,
        contextMenu: null,
        newFolderName: null,
        newFileName: '',
        renamingEntry: null,
      }
    case 'set-new-file-name':
      return { ...state, newFileName: action.name }
    case 'clear-new-file':
      return { ...state, newFileName: null }
    case 'set-dragging':
      return { ...state, isDragging: action.dragging }
    case 'set-folder-drop-entry':
      return { ...state, folderDropEntryId: action.id }
  }
}

// UnixFSBrowser renders a UnixFS filesystem browser for use in layout tabs.
export function UnixFSBrowser(props: UnixFSBrowserProps) {
  return useUnixFSBrowserElement(props)
}

function useUnixFSBrowserElement({
  unixfsId,
  basePath,
  currentPath,
  mimeTypeOverride,
  worldState,
  browserBody: BrowserBody,
  directoryHeader: DirectoryHeader,
}: UnixFSBrowserProps) {
  const tabContext = useTabContext()
  const spaceCtx = SpaceContainerContext.useContextSafe()
  const spaceId = spaceCtx?.spaceId ?? null
  const sessionIndex = useSessionIndex()
  const syncStatus = useSessionSyncStatus()
  const displayPath = currentPath || basePath || '/'

  // Navigation history for back/forward support
  const history = useHistory()

  const {
    rootHandle,
    pathHandle,
    statResource,
    entriesResource,
    fileEntries,
    isDir,
  } = useUnixFSBrowserResources({
    worldState,
    unixfsId,
    displayPath,
  })

  const [state, dispatch] = useReducer(
    unixFSBrowserReducer,
    initialUnixFSBrowserState,
  )
  const {
    pendingName,
    contextMenu,
    selectedIds,
    newFolderName,
    newFileName,
    deleteTargets,
    moveDialogItems,
    renamingEntry,
    isDragging,
    folderDropEntryId,
  } = state
  const deferredFileEntries = useDeferredValue(fileEntries)
  const renameRef = useRef('')

  // Uploads are owned by the session, not this viewer, so an in-flight upload
  // and its feedback survive navigating away from this folder. Null when this
  // browser is mounted outside a session (display, debug harness); uploads are
  // simply unavailable there.
  const uploadManager = useSessionUploadManager()

  // File input ref for upload button
  const fileInputRef = useRef<HTMLInputElement>(null)

  const handleListStateChange = useCallback(
    (state: { selectedIds?: string[] }) => {
      dispatch({ type: 'set-selected-ids', ids: state.selectedIds ?? [] })
    },
    [],
  )

  const selectedEntries = useMemo(
    () => fileEntries.filter((entry) => selectedIds.includes(entry.id)),
    [fileEntries, selectedIds],
  )
  const inlineFileURL = useMemo(() => {
    if (isDir !== false || !statResource.value || !sessionIndex || !spaceId) {
      return undefined
    }
    return buildUnixFSFileInlineURL(
      sessionIndex,
      spaceId,
      unixfsId,
      displayPath,
    )
  }, [displayPath, isDir, sessionIndex, spaceId, statResource.value, unixfsId])
  const effectiveMimeType = mimeTypeOverride || statResource.value?.mimeType

  const { getDragEnvelope, getDownloadDragTarget } =
    useUnixFSBrowserDragTargets({
      displayPath,
      selectedEntries,
      sessionIndex,
      spaceId,
      unixfsId,
    })

  // Get navigate function from router context
  const navigate = useNavigate()

  // Handle navigating back in history
  const handleBack = useCallback(() => {
    dispatch({ type: 'set-pending-name', name: null })
    history?.goBack()
  }, [history])

  // Handle navigating forward in history
  const handleForward = useCallback(() => {
    dispatch({ type: 'set-pending-name', name: null })
    history?.goForward()
  }, [history])

  // Handle navigating up one directory level
  const handleUp = useCallback(() => {
    if (displayPath === '/') return
    dispatch({ type: 'set-pending-name', name: null })
    navigate({ path: getUnixFSParentPath(displayPath) })
  }, [displayPath, navigate])

  // Handle path change from toolbar (user edited path directly)
  const handlePathChange = useCallback(
    (newPath: string) => {
      dispatch({ type: 'set-pending-name', name: null })
      navigate({ path: newPath })
    },
    [navigate],
  )

  // Handle opening files/directories
  const handleOpen = useCallback(
    (entries: FileEntry[]) => {
      if (!entries.length) return

      // Single item open: navigate in same tab
      if (entries.length === 1) {
        const entry = entries[0]
        dispatch({ type: 'set-pending-name', name: entry.id })
        navigate({ path: './' + entry.name })
        return
      }

      // Multiple items: open new tabs for each
      if (!tabContext) return
      for (const entry of entries) {
        const filePath = joinUnixFSDisplayPath(displayPath, entry.name)
        const objectInfo: ObjectInfo = {
          info: {
            case: 'unixfsObjectInfo',
            value: {
              unixfsId,
              path: filePath,
            } satisfies UnixfsObjectInfo,
          },
        }
        const tabData = ObjectLayoutTab.toBinary({
          objectInfo,
          path: '',
        })
        void tabContext.addTab({
          tab: {
            id: `tab-${Date.now()}-${Math.random().toString(36).slice(2, 9)}`,
            name: entry.name,
            enableClose: true,
            data: tabData,
          },
          select: entry === entries[0],
        })
      }
    },
    [displayPath, navigate, tabContext, unixfsId],
  )
  // Handle retry for root handle, path handle, stat, and entries
  const handleRetry = useCallback(() => {
    if (rootHandle.error) {
      rootHandle.retry()
    } else if (pathHandle.error) {
      pathHandle.retry()
    } else if (statResource.error) {
      statResource.retry()
    } else if (entriesResource.error) {
      entriesResource.retry()
    }
  }, [rootHandle, pathHandle, statResource, entriesResource])

  const handleContextMenu = useCallback(
    (item: ListItem<FileEntry>, event: MouseEvent) => {
      const entry = item.data ?? null
      const actionEntries =
        entry && selectedIds.includes(entry.id) && selectedEntries.length > 0
          ? selectedEntries
          : entry
            ? [entry]
            : []
      dispatch({
        type: 'set-context-menu',
        menu: {
          position: { x: event.clientX, y: event.clientY },
          entry,
          actionEntries,
          moveItems: buildUnixFSMoveItems(displayPath, actionEntries),
        },
      })
    },
    [displayPath, selectedEntries, selectedIds],
  )

  const handleCloseContextMenu = useCallback(() => {
    dispatch({ type: 'set-context-menu', menu: null })
  }, [])

  const handleBackgroundContextMenu = useCallback(
    (e: MouseEvent<HTMLDivElement>) => {
      e.preventDefault()
      dispatch({
        type: 'set-context-menu',
        menu: {
          position: { x: e.clientX, y: e.clientY },
          entry: null,
          actionEntries: [],
          moveItems: [],
        },
      })
    },
    [],
  )

  const handleDownload = useCallback(
    (entries: FileEntry[]) => {
      if (!sessionIndex || !spaceId || entries.length === 0) return
      void downloadUnixFSSelection({
        sessionIndex,
        sharedObjectId: spaceId,
        objectKey: unixfsId,
        currentPath: displayPath,
        entries,
      }).catch((err: unknown) => {
        console.error('failed to download unixfs selection', err)
        toast.error('Download failed', { description: String(err) })
      })
    },
    [displayPath, sessionIndex, spaceId, unixfsId],
  )

  // handleStartRename activates inline rename for a file entry.
  const handleStartRename = useCallback((entry: FileEntry) => {
    renameRef.current = entry.name
    dispatch({ type: 'start-rename', entry })
  }, [])

  const handleConfirmRename = useCallback(async () => {
    if (!renamingEntry || !pathHandle.value) return
    const newName = renameRef.current.trim()
    if (!newName || newName === renamingEntry.name) {
      dispatch({ type: 'clear-rename' })
      return
    }
    if (newName.includes('/') || newName.includes('\\')) return

    await pathHandle.value.rename(renamingEntry.name, newName)
    dispatch({ type: 'clear-rename' })
    dispatch({ type: 'set-context-menu', menu: null })
  }, [pathHandle.value, renamingEntry])

  const handleCancelRename = useCallback(() => {
    dispatch({ type: 'clear-rename' })
  }, [])

  // handleRequestDelete opens the delete confirmation dialog for the given entries.
  const handleRequestDelete = useCallback((entries: FileEntry[]) => {
    dispatch({ type: 'request-delete', entries })
  }, [])

  const handleRequestMove = useCallback((moveItems: UnixFSMoveItem[]) => {
    if (moveItems.length === 0) return
    dispatch({ type: 'request-move', items: moveItems })
  }, [])

  const handleConfirmDelete = useCallback(async () => {
    if (!deleteTargets || !pathHandle.value) return
    const names = deleteTargets.map((e) => e.name)
    await pathHandle.value.remove(names)
    dispatch({ type: 'clear-delete' })
  }, [pathHandle.value, deleteTargets])

  const handleConfirmMove = useCallback(
    async (destinationPath: string) => {
      const root = rootHandle.value
      const sourceParent = pathHandle.value
      if (!root || !sourceParent || !moveDialogItems) return
      await moveUnixFSItemsFromDirectory(
        root,
        sourceParent,
        displayPath,
        moveDialogItems,
        destinationPath,
      )
      dispatch({ type: 'complete-move' })
    },
    [displayPath, moveDialogItems, pathHandle.value, rootHandle.value],
  )

  const handleCancelDelete = useCallback(() => {
    dispatch({ type: 'clear-delete' })
  }, [])

  // handleNewFolder opens the inline new-folder input.
  const handleNewFolder = useCallback(() => {
    dispatch({ type: 'start-new-folder' })
  }, [])

  const handleNewFolderConfirm = useCallback(
    async (name: string) => {
      const folderName = name.trim()
      if (!folderName || !pathHandle.value) return
      await pathHandle.value.mkdirAll([folderName])
      dispatch({ type: 'clear-new-folder' })
    },
    [pathHandle.value],
  )

  const handleNewFolderCancel = useCallback(() => {
    dispatch({ type: 'clear-new-folder' })
  }, [])

  // handleNewFile opens the inline new-file input.
  const handleNewFile = useCallback(() => {
    dispatch({ type: 'start-new-file' })
  }, [])

  const handleNewFileConfirm = useCallback(
    async (name: string) => {
      const fileName = name.trim()
      if (!fileName || !pathHandle.value) return
      await pathHandle.value.mknod([fileName], MknodType.FILE)
      dispatch({ type: 'clear-new-file' })
    },
    [pathHandle.value],
  )

  const handleNewFileCancel = useCallback(() => {
    dispatch({ type: 'clear-new-file' })
  }, [])

  // handleUploadFiles opens the native file picker for uploading.
  const handleUploadFiles = useCallback(() => {
    fileInputRef.current?.click()
  }, [])

  const handleFileInputChange = useCallback(
    (e: ChangeEvent<HTMLInputElement>) => {
      const files = e.target.files
      if (!files || files.length === 0) return
      if (pathHandle.value) {
        uploadManager?.addFiles(pathHandle.value, Array.from(files))
      }
      e.target.value = ''
    },
    [uploadManager, pathHandle.value],
  )

  const setDragging = useCallback((dragging: boolean) => {
    dispatch({ type: 'set-dragging', dragging })
  }, [])

  const setFolderDropEntryId = useCallback((id: string | null) => {
    dispatch({ type: 'set-folder-drop-entry', id })
  }, [])

  const {
    handleDragOver,
    handleDragLeave,
    handleDrop,
    handleEntryDragOver,
    handleEntryDragLeave,
    handleEntryDrop,
    handlePathTargetDragOver,
    handlePathTargetDrop,
  } = useUnixFSBrowserDrag({
    unixfsId,
    displayPath,
    rootHandle: rootHandle.value,
    sourceParentHandle: pathHandle.value,
    uploadManager,
    folderDropEntryId,
    setDragging,
    setFolderDropEntryId,
  })
  const handleKeyDown = useUnixFSDeleteKeyHandler({
    selectedEntries,
    onDelete: handleRequestDelete,
  })

  useUnixFSBrowserCommands({
    selectedEntries,
    canGoBack: history?.canGoBack ?? false,
    canGoForward: history?.canGoForward ?? false,
    canGoUp: displayPath !== '/',
    onNewFile: handleNewFile,
    onNewFolder: handleNewFolder,
    onUploadFiles: handleUploadFiles,
    onOpen: handleOpen,
    onRename: handleStartRename,
    onDownload: handleDownload,
    onDelete: handleRequestDelete,
    onBack: handleBack,
    onForward: handleForward,
    onUp: handleUp,
  })
  const isLoading =
    rootHandle.loading ||
    pathHandle.loading ||
    statResource.loading ||
    (isDir === true && entriesResource.loading)

  // Build entries with new folder/file inline input prepended
  const displayEntries = useMemo(() => {
    const prepend: FileEntry[] = []
    if (newFolderName !== null) {
      prepend.push({ id: '__new-folder__', name: '', isDir: true })
    }
    if (newFileName !== null) {
      prepend.push({ id: '__new-file__', name: '', isDir: false })
    }
    if (prepend.length === 0) return fileEntries
    return [...prepend, ...fileEntries]
  }, [fileEntries, newFolderName, newFileName])
  useUnixFSStartupBoundaries({
    displayPath,
    displayEntries,
    isDir,
    isLoading,
    rootHandle: rootHandle.value,
    sharedObjectId: spaceId,
  })

  const handleNewFolderNameChange = useCallback((name: string) => {
    dispatch({ type: 'set-new-folder-name', name })
  }, [])

  const handleNewFileNameChange = useCallback((name: string) => {
    dispatch({ type: 'set-new-file-name', name })
  }, [])

  const renderEntry = useUnixFSInlineEntryRenderer({
    newFolderName,
    newFileName,
    renamingEntry,
    renameRef,
    onConfirmRename: handleConfirmRename,
    onCancelRename: handleCancelRename,
    onNewFolderNameChange: handleNewFolderNameChange,
    onNewFileNameChange: handleNewFileNameChange,
    onNewFolderConfirm: handleNewFolderConfirm,
    onNewFolderCancel: handleNewFolderCancel,
    onNewFileConfirm: handleNewFileConfirm,
    onNewFileCancel: handleNewFileCancel,
  })

  const contextMenuProps = useMemo(
    () => ({
      state: contextMenu,
      onClose: handleCloseContextMenu,
      onOpen: handleOpen,
      onDownload: handleDownload,
      onMove: handleRequestMove,
      onRename: handleStartRename,
      onDelete: handleRequestDelete,
      onNewFolder: handleNewFolder,
      onUploadFiles: handleUploadFiles,
    }),
    [
      contextMenu,
      handleCloseContextMenu,
      handleOpen,
      handleDownload,
      handleRequestMove,
      handleStartRename,
      handleRequestDelete,
      handleNewFolder,
      handleUploadFiles,
    ],
  )
  const loadingStageLabel = useMemo(
    () =>
      buildUnixFSLoadingStageLabel({
        rootLoading: rootHandle.loading,
        pathLoading: pathHandle.loading,
        statLoading: statResource.loading,
        entriesLoading: isDir === true && entriesResource.loading,
      }),
    [
      entriesResource.loading,
      isDir,
      pathHandle.loading,
      rootHandle.loading,
      statResource.loading,
    ],
  )
  const loadingDiagnostics = (
    <UnixFSLoadingDiagnostics
      stageLabel={loadingStageLabel}
      status={syncStatus}
    />
  )
  const handleCancelMove = useCallback(() => {
    dispatch({ type: 'clear-move' })
  }, [])
  const dialogs = (
    <UnixFSBrowserDialogs
      contextMenuProps={contextMenuProps}
      fileInputRef={fileInputRef}
      onFileInputChange={handleFileInputChange}
      deleteTargets={deleteTargets}
      onCancelDelete={handleCancelDelete}
      onConfirmDelete={handleConfirmDelete}
      moveRootHandle={rootHandle.value}
      moveDialogItems={moveDialogItems}
      onCancelMove={handleCancelMove}
      onConfirmMove={handleConfirmMove}
    />
  )
  const shellNavigationProps = {
    currentPath: displayPath,
    onPathChange: handlePathChange,
    onBack: handleBack,
    onForward: handleForward,
    onUp: handleUp,
    canGoBack: history?.canGoBack ?? false,
    canGoForward: history?.canGoForward ?? false,
    canGoUp: displayPath !== '/',
    upDropPath: getUnixFSParentPath(displayPath),
    onPathTargetDragOver: handlePathTargetDragOver,
    onPathTargetDrop: handlePathTargetDrop,
  }
  const shellActionProps = {
    ...shellNavigationProps,
    onNewFolder: handleNewFolder,
    onUploadFiles: handleUploadFiles,
  }
  // During directory transitions, keep showing previous entries with a loading indicator
  if (isLoading && deferredFileEntries.length > 0) {
    return (
      <UnixFSBrowserShell
        {...shellActionProps}
        interactive
        onKeyDown={handleKeyDown}
      >
        <UnixFSBrowserDropSurface
          isDragging={isDragging}
          floatingDiagnostics={loadingDiagnostics}
          onContextMenu={handleBackgroundContextMenu}
          onDragOver={handleDragOver}
          onDragLeave={handleDragLeave}
          onDrop={(e) => void handleDrop(e)}
        >
          {BrowserBody ? (
            <BrowserBody
              rootHandle={rootHandle}
              unixfsId={unixfsId}
              currentPath={displayPath}
            />
          ) : (
            <UnixFSDirectoryListing
              currentPath={displayPath}
              entries={deferredFileEntries}
              displayEntries={deferredFileEntries}
              loadingId={entriesResource.loading ? pendingName : null}
              onOpen={handleOpen}
              onContextMenu={handleContextMenu}
              onStateChange={handleListStateChange}
              onNewFolder={handleNewFolder}
              onUploadFiles={handleUploadFiles}
              getDragEnvelope={getDragEnvelope}
              getDownloadDragTarget={getDownloadDragTarget}
              dropTargetEntryId={folderDropEntryId}
              onEntryDragOver={handleEntryDragOver}
              onEntryDragLeave={handleEntryDragLeave}
              onEntryDrop={handleEntryDrop}
            />
          )}
        </UnixFSBrowserDropSurface>
        {dialogs}
      </UnixFSBrowserShell>
    )
  }

  // Show fullscreen loading state only on initial load
  if (isLoading) {
    return (
      <UnixFSBrowserShell {...shellActionProps}>
        <div className="bg-file-back flex min-h-0 flex-1 flex-col items-center justify-center gap-3 overflow-hidden p-6">
          <div className="w-full max-w-sm">
            <UnixFSPathLoadingCard
              root={rootHandle}
              lookup={pathHandle}
              stat={statResource}
              entries={isDir === true ? entriesResource : null}
              path={displayPath}
            />
          </div>
          {loadingDiagnostics}
        </div>
      </UnixFSBrowserShell>
    )
  }

  // Show error state
  const error =
    rootHandle.error ??
    pathHandle.error ??
    statResource.error ??
    entriesResource.error
  if (error) {
    return (
      <UnixFSBrowserUnavailableState
        kind="error"
        shellProps={shellNavigationProps}
        error={error}
        onRetry={handleRetry}
      />
    )
  }

  // Show placeholder if UnixFS object not found
  if (!rootHandle.value) {
    return (
      <UnixFSBrowserUnavailableState kind="not-found" unixfsId={unixfsId} />
    )
  }

  if (BrowserBody) {
    return (
      <UnixFSBrowserShell
        {...shellActionProps}
        interactive
        onKeyDown={handleKeyDown}
      >
        <UnixFSBrowserDropSurface
          isDragging={isDragging}
          onContextMenu={handleBackgroundContextMenu}
          onDragOver={handleDragOver}
          onDragLeave={handleDragLeave}
          onDrop={(e) => void handleDrop(e)}
        >
          <BrowserBody
            rootHandle={rootHandle}
            unixfsId={unixfsId}
            currentPath={displayPath}
          />
        </UnixFSBrowserDropSurface>
        {dialogs}
      </UnixFSBrowserShell>
    )
  }

  // Render file viewer for files
  if (isDir === false && statResource.value) {
    return (
      <UnixFSFileViewer
        path={displayPath}
        stat={{
          ...statResource.value,
          mimeType: effectiveMimeType ?? statResource.value.mimeType,
        }}
        rootHandle={rootHandle}
        inlineFileURL={inlineFileURL}
      />
    )
  }

  // Render directory listing
  return (
    <UnixFSBrowserShell
      {...shellActionProps}
      interactive
      onKeyDown={handleKeyDown}
    >
      <UnixFSBrowserDropSurface
        isDragging={isDragging}
        onContextMenu={handleBackgroundContextMenu}
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        onDrop={(e) => void handleDrop(e)}
      >
        <UnixFSDirectoryListing
          currentPath={displayPath}
          entries={fileEntries}
          displayEntries={displayEntries}
          DirectoryHeader={DirectoryHeader}
          renderEntry={renderEntry}
          onOpen={handleOpen}
          onContextMenu={handleContextMenu}
          onStateChange={handleListStateChange}
          onNewFolder={handleNewFolder}
          onUploadFiles={handleUploadFiles}
          getDragEnvelope={getDragEnvelope}
          getDownloadDragTarget={getDownloadDragTarget}
          dropTargetEntryId={folderDropEntryId}
          onEntryDragOver={handleEntryDragOver}
          onEntryDragLeave={handleEntryDragLeave}
          onEntryDrop={handleEntryDrop}
        />
      </UnixFSBrowserDropSurface>
      {dialogs}
    </UnixFSBrowserShell>
  )
}
