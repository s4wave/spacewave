import {
  type KeyboardEvent,
  useCallback,
  useDeferredValue,
  useMemo,
  useReducer,
  type MouseEvent,
  type DragEvent,
  useRef,
} from 'react'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import { ObjectLayoutTab } from '@s4wave/sdk/layout/world/world.pb.js'
import { MknodType } from '@s4wave/sdk/unixfs/index.js'
import { ObjectInfo, UnixfsObjectInfo } from '@s4wave/web/object/object.pb.js'
import {
  useUnixFSRootHandle,
  useUnixFSHandle,
  useUnixFSHandleEntries,
  useUnixFSHandleStat,
} from '@s4wave/web/hooks/useUnixFSHandle.js'
import { type Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useMappedResource } from '@aptre/bldr-sdk/hooks/useMappedResource.js'
import { FileList } from '@s4wave/web/editors/file-browser/FileList.js'
import { Toolbar } from '@s4wave/web/editors/file-browser/Toolbar.js'
import type { FileEntry } from '@s4wave/web/editors/file-browser/types.js'
import type { RenderEntryCallback } from '@s4wave/web/editors/file-browser/FileListEntry.js'
import type { ListItem } from '@s4wave/web/ui/list'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'
import { cn } from '@s4wave/web/style/utils.js'
import { LuCheck, LuUpload, LuX } from 'react-icons/lu'
import { useCommand } from '@s4wave/web/command/useCommand.js'
import { useIsTabActive } from '@s4wave/web/contexts/TabActiveContext.js'
import { useTabContext } from '@s4wave/web/object/TabContext.js'
import { UnixFSFileViewer } from './UnixFSFileViewer.js'
import {
  UnixFSContextMenu,
  type ContextMenuState,
} from './UnixFSContextMenu.js'
import { UnixFSMoveDialog } from './UnixFSMoveDialog.js'
import { UnixFSPathLoadingCard } from '@s4wave/app/loading/wrappers/UnixFSPathLoadingCard.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { useHistory } from '@s4wave/web/router/HistoryRouter.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { useSessionIndex } from '@s4wave/web/contexts/contexts.js'
import {
  buildUnixFSSelectionDownloadDragTarget,
  buildUnixFSFileInlineURL,
  downloadUnixFSSelection,
} from './download.js'
import { useUploadManager } from './useUploadManager.js'
import { UploadProgressBottomBar } from './UploadProgressBottomBar.js'
import { hasNativeFileDrag } from '@s4wave/web/dnd/app-drag.js'
import { isDownloadURLDragSupported } from '@s4wave/web/dnd/download-url-drag.js'
import {
  buildUnixFSEntryAppDragEnvelope,
  buildUnixFSSelectionAppDragEnvelope,
  readUnixFSMovableAppDragItems,
} from './unixfs-app-drag.js'
import { extractNativeUploadSelection } from './native-upload.js'
import {
  buildUnixFSMoveItems,
  getUnixFSBaseName,
  moveUnixFSItems,
  type UnixFSMoveItem,
  validateUnixFSMove,
} from './move.js'
import {
  type SessionSyncStatusView,
  useSessionSyncStatus,
} from '../session/SessionSyncStatusContext.js'

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
}

// joinPath joins path segments, handling leading slashes.
function joinPath(base: string, ...segments: string[]): string {
  let result = base
  for (const segment of segments) {
    if (result.endsWith('/')) {
      result = result + segment
    } else {
      result = result + '/' + segment
    }
  }
  return result
}

function normalizeHandlePath(path: string): string {
  return !path || path === '/' || path === '.' ? '' : path
}

function getTrackedHandlePath(handle: {
  getPath?: () => string
}): string | null {
  return typeof handle.getPath === 'function' ? handle.getPath() : null
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
export function UnixFSBrowser({
  unixfsId,
  basePath,
  currentPath,
  mimeTypeOverride,
  worldState,
}: UnixFSBrowserProps) {
  const tabContext = useTabContext()
  const spaceCtx = SpaceContainerContext.useContextSafe()
  const spaceId = spaceCtx?.spaceId ?? null
  const sessionIndex = useSessionIndex()
  const syncStatus = useSessionSyncStatus()
  const displayPath = currentPath || basePath || '/'

  // Navigation history for back/forward support
  const history = useHistory()

  // Get the root FSHandle for this UnixFS object
  const rootHandle = useUnixFSRootHandle(worldState, unixfsId)

  // Get a handle for the current display path
  const pathHandle = useUnixFSHandle(rootHandle, displayPath)

  // Stat the path to determine if it's a file or directory
  const statResource = useUnixFSHandleStat(pathHandle)

  // Determine if path is a directory - only after stat completes
  // CRITICAL: Also check statResource.loading to avoid using stale data during path changes
  const isDir =
    statResource.loading || statResource.value === null ?
      null
    : (statResource.value.info.isDir ?? false)
  const normalizedDisplayPath = normalizeHandlePath(displayPath)

  const directoryHandle = useMappedResource(
    pathHandle,
    (h) => {
      if (isDir !== true) return null
      const handlePath = getTrackedHandlePath(h)
      if (handlePath !== null && handlePath !== normalizedDisplayPath) {
        return null
      }
      return h
    },
    [normalizedDisplayPath, isDir],
  )

  const entriesResource = useUnixFSHandleEntries(directoryHandle)

  // Convert to FileEntry format expected by FileList
  const fileEntries = useMemo(() => {
    if (!entriesResource.value) return []
    return entriesResource.value.map(
      (entry): FileEntry => ({
        id: entry.id,
        name: entry.name,
        isDir: entry.isDir,
        isSymlink: entry.isSymlink,
      }),
    )
  }, [entriesResource.value])

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

  // Upload manager
  const uploadManager = useUploadManager(pathHandle.value ?? null, 5)

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

  const getDragEnvelope = useCallback(
    (entry: FileEntry, { selectedIds }: { selectedIds: string[] }) => {
      const dragEntries =
        selectedIds.includes(entry.id) && selectedEntries.length > 1 ?
          selectedEntries
        : [entry]
      if (dragEntries.length === 1) {
        return buildUnixFSEntryAppDragEnvelope({
          entry,
          currentPath: displayPath,
          sessionIndex,
          spaceId,
          unixfsId,
        })
      }
      return buildUnixFSSelectionAppDragEnvelope({
        entries: dragEntries,
        currentPath: displayPath,
        sessionIndex,
        spaceId,
        unixfsId,
        movableEntryIds: dragEntries.map((entry) => entry.id),
      })
    },
    [displayPath, selectedEntries, sessionIndex, spaceId, unixfsId],
  )
  const getDownloadDragTarget = useCallback(
    (entry: FileEntry, { selectedIds }: { selectedIds: string[] }) => {
      if (!sessionIndex || !spaceId) {
        return null
      }
      if (!isDownloadURLDragSupported(navigator.userAgent)) {
        return null
      }
      const dragEntries =
        selectedIds.includes(entry.id) && selectedEntries.length > 1 ?
          selectedEntries
        : [entry]
      return buildUnixFSSelectionDownloadDragTarget({
        sessionIndex,
        sharedObjectId: spaceId,
        objectKey: unixfsId,
        currentPath: displayPath,
        entries: dragEntries,
      })
    },
    [displayPath, selectedEntries, sessionIndex, spaceId, unixfsId],
  )

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
    navigate({ path: joinPath(displayPath, '..') })
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
        const filePath = joinPath(displayPath, entry.name)
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
        entry && selectedIds.includes(entry.id) && selectedEntries.length > 0 ?
          selectedEntries
        : entry ? [entry]
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
      if (!root || !moveDialogItems) return
      await moveUnixFSItems(root, moveDialogItems, destinationPath)
      dispatch({ type: 'complete-move' })
    },
    [moveDialogItems, rootHandle.value],
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
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const files = e.target.files
      if (!files || files.length === 0) return
      uploadManager.addFiles(Array.from(files))
      e.target.value = ''
    },
    [uploadManager],
  )

  // handleDragOver accepts the drag event to enable file drops.
  const handleDragOver = useCallback((e: DragEvent<HTMLDivElement>) => {
    if (!hasNativeFileDrag(e.dataTransfer)) {
      dispatch({ type: 'set-dragging', dragging: false })
      return
    }
    e.preventDefault()
    e.stopPropagation()
    dispatch({ type: 'set-dragging', dragging: true })
  }, [])

  const handleDragLeave = useCallback((e: DragEvent<HTMLDivElement>) => {
    e.preventDefault()
    e.stopPropagation()
    if (e.currentTarget.contains(e.relatedTarget as Node)) return
    dispatch({ type: 'set-dragging', dragging: false })
  }, [])

  const handleDrop = useCallback(
    async (e: DragEvent<HTMLDivElement>) => {
      if (!hasNativeFileDrag(e.dataTransfer)) {
        dispatch({ type: 'set-dragging', dragging: false })
        return
      }
      e.preventDefault()
      e.stopPropagation()
      dispatch({ type: 'set-dragging', dragging: false })
      const dataTransfer = e.dataTransfer
      if (!dataTransfer) return
      const selection = await extractNativeUploadSelection(dataTransfer)
      if (selection.files.length === 0 && selection.directories.length === 0) {
        return
      }
      uploadManager.addFiles(selection.files, selection.directories)
    },
    [uploadManager],
  )

  const getAcceptedFolderMove = useCallback(
    (entry: FileEntry, dataTransfer: DataTransfer | null | undefined) => {
      if (!entry.isDir) return null

      const movableItems = readUnixFSMovableAppDragItems(dataTransfer)
      if (movableItems.length === 0) return null
      if (movableItems.some((item) => item.value.unixfsId !== unixfsId)) {
        return null
      }

      const destPath = joinPath(displayPath, entry.name)
      const moveItems = movableItems.map((movableItem) => {
        const sourceName =
          movableItem.label ?? getUnixFSBaseName(movableItem.value.path)
        return {
          id: movableItem.id,
          name: sourceName,
          path: movableItem.value.path,
          isDir: movableItem.value.isDir,
        }
      })
      if (moveItems.some((item) => !item.name)) return null
      const validation = validateUnixFSMove(moveItems, destPath)
      if (!validation.accepted) {
        return null
      }

      return {
        destinationPath: destPath,
        items: moveItems,
      }
    },
    [displayPath, unixfsId],
  )

  const handleEntryDragOver = useCallback(
    (entry: FileEntry, e: DragEvent<HTMLDivElement>) => {
      const acceptedMove = getAcceptedFolderMove(entry, e.dataTransfer)
      if (!acceptedMove) {
        if (folderDropEntryId === entry.id) {
          dispatch({ type: 'set-folder-drop-entry', id: null })
        }
        return false
      }
      if (folderDropEntryId !== entry.id) {
        dispatch({ type: 'set-folder-drop-entry', id: entry.id })
      }
      return true
    },
    [folderDropEntryId, getAcceptedFolderMove],
  )

  const handleEntryDragLeave = useCallback(
    (entry: FileEntry, e: DragEvent<HTMLDivElement>) => {
      if (e.currentTarget.contains(e.relatedTarget as Node)) return
      if (folderDropEntryId !== entry.id) return
      dispatch({ type: 'set-folder-drop-entry', id: null })
    },
    [folderDropEntryId],
  )

  const handleEntryDrop = useCallback(
    (entry: FileEntry, e: DragEvent<HTMLDivElement>) => {
      const acceptedMove = getAcceptedFolderMove(entry, e.dataTransfer)
      dispatch({ type: 'set-folder-drop-entry', id: null })
      const root = rootHandle.value
      if (!acceptedMove || !root) return

      void (async () => {
        await moveUnixFSItems(
          root,
          acceptedMove.items,
          acceptedMove.destinationPath,
        )
      })()
    },
    [getAcceptedFolderMove, rootHandle.value],
  )

  // Delete key handler
  const handleKeyDown = useCallback(
    (e: KeyboardEvent<HTMLDivElement>) => {
      if (e.key === 'Delete' || e.key === 'Backspace') {
        if (selectedEntries.length > 0) {
          e.preventDefault()
          dispatch({ type: 'request-delete', entries: selectedEntries })
        }
      }
    },
    [selectedEntries],
  )

  // Command registrations for file browser operations.
  const isTabActive = useIsTabActive()

  useCommand({
    commandId: 'spacewave.file.new-file',
    label: 'New File',
    menuPath: 'File/New File',
    menuGroup: 2,
    menuOrder: 1,
    active: isTabActive,
    handler: handleNewFile,
  })

  useCommand({
    commandId: 'spacewave.file.new-folder',
    label: 'New Folder',
    menuPath: 'File/New Folder',
    menuGroup: 2,
    menuOrder: 2,
    active: isTabActive,
    handler: handleNewFolder,
  })

  useCommand({
    commandId: 'spacewave.file.upload',
    label: 'Upload',
    menuPath: 'File/Upload',
    menuGroup: 2,
    menuOrder: 3,
    active: isTabActive,
    handler: handleUploadFiles,
  })

  useCommand({
    commandId: 'spacewave.file.open',
    label: 'Open Selected',
    description: 'Open the selected file or directory',
    menuPath: 'File/Open Selected',
    menuGroup: 1,
    menuOrder: 1,
    active: isTabActive,
    enabled: selectedEntries.length > 0,
    handler: useCallback(() => {
      if (selectedEntries.length > 0) handleOpen(selectedEntries)
    }, [selectedEntries, handleOpen]),
  })

  useCommand({
    commandId: 'spacewave.file.rename',
    label: 'Rename',
    description: 'Rename the selected file or directory',
    keybinding: 'F2',
    menuPath: 'File/Rename',
    menuGroup: 3,
    menuOrder: 1,
    active: isTabActive,
    enabled: selectedEntries.length === 1,
    handler: useCallback(() => {
      if (selectedEntries.length === 1) handleStartRename(selectedEntries[0])
    }, [selectedEntries, handleStartRename]),
  })

  useCommand({
    commandId: 'spacewave.file.download',
    label: 'Download',
    description: 'Download the selected file or selection',
    menuPath: 'File/Download',
    menuGroup: 3,
    menuOrder: 2,
    active: isTabActive,
    enabled: selectedEntries.length > 0,
    handler: useCallback(() => {
      if (selectedEntries.length > 0) {
        handleDownload(selectedEntries)
      }
    }, [selectedEntries, handleDownload]),
  })

  useCommand({
    commandId: 'spacewave.file.delete',
    label: 'Delete',
    menuPath: 'Edit/Delete',
    menuGroup: 40,
    menuOrder: 1,
    active: isTabActive,
    enabled: selectedEntries.length > 0,
    handler: useCallback(() => {
      if (selectedEntries.length > 0) {
        dispatch({ type: 'request-delete', entries: selectedEntries })
      }
    }, [selectedEntries]),
  })

  useCommand({
    commandId: 'spacewave.nav.back',
    label: 'Navigate Back',
    keybinding: 'Alt+ArrowLeft',
    menuPath: 'File/Navigate Back',
    menuGroup: 30,
    menuOrder: 1,
    active: isTabActive,
    enabled: history?.canGoBack ?? false,
    handler: handleBack,
  })

  useCommand({
    commandId: 'spacewave.nav.forward',
    label: 'Navigate Forward',
    keybinding: 'Alt+ArrowRight',
    menuPath: 'File/Navigate Forward',
    menuGroup: 30,
    menuOrder: 2,
    active: isTabActive,
    enabled: history?.canGoForward ?? false,
    handler: handleForward,
  })

  useCommand({
    commandId: 'spacewave.nav.up',
    label: 'Navigate Up',
    keybinding: 'Alt+ArrowUp',
    menuPath: 'File/Navigate Up',
    menuGroup: 30,
    menuOrder: 3,
    active: isTabActive,
    enabled: displayPath !== '/',
    handler: handleUp,
  })

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

  // renderEntry overrides the default entry renderer to show an inline input
  // for renaming an existing entry or naming a new folder/file. Returns
  // undefined when no inline input is active so the file list uses the
  // default renderer.
  const renderEntry: RenderEntryCallback | undefined = useMemo(() => {
    if (newFolderName === null && newFileName === null && !renamingEntry) {
      return undefined
    }
    return ({ entry, defaultNode }) => {
      const isNewFolder =
        entry.id === '__new-folder__' && newFolderName !== null
      const isNewFile = entry.id === '__new-file__' && newFileName !== null
      const isRenaming = !!renamingEntry && entry.id === renamingEntry.id
      const isNewItem = isNewFolder || isNewFile

      if (!isNewItem && !isRenaming) return defaultNode

      if (isRenaming) {
        return (
          <div
            className="rename-actions flex min-w-[120px] flex-1 items-center gap-0.5 overflow-hidden"
            onClick={(e) => e.stopPropagation()}
            onMouseDown={(e) => e.stopPropagation()}
          >
            <input
              ref={(el) => {
                if (!el) return
                el.focus()
                const name = renameRef.current
                const lastDot = name.lastIndexOf('.')
                el.setSelectionRange(0, lastDot > 0 ? lastDot : name.length)
              }}
              className="bg-background text-foreground border-brand min-w-0 flex-1 rounded border px-1.5 py-0.5 text-xs outline-none"
              defaultValue={renameRef.current}
              onChange={(e) => {
                renameRef.current = e.target.value
              }}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault()
                  void handleConfirmRename()
                }
                if (e.key === 'Escape') {
                  e.preventDefault()
                  handleCancelRename()
                }
                e.stopPropagation()
              }}
              onBlur={(e) => {
                const related = e.relatedTarget as HTMLElement | null
                if (related?.closest('.rename-actions')) return
                handleCancelRename()
              }}
            />
            <button
              tabIndex={0}
              className="text-brand hover:text-brand-highlight shrink-0 p-0.5"
              onClick={(e) => {
                e.preventDefault()
                e.stopPropagation()
                void handleConfirmRename()
              }}
            >
              <LuCheck className="h-3 w-3" />
            </button>
            <button
              tabIndex={0}
              className="text-foreground-alt hover:text-foreground shrink-0 p-0.5"
              onClick={(e) => {
                e.preventDefault()
                e.stopPropagation()
                handleCancelRename()
              }}
            >
              <LuX className="h-3 w-3" />
            </button>
          </div>
        )
      }

      // New folder or new file inline input.
      const value = isNewFolder ? newFolderName! : newFileName!
      const placeholder = isNewFolder ? 'Folder name' : 'File name'
      const handleConfirm = isNewFolder ?
          handleNewFolderConfirm
        : handleNewFileConfirm
      const handleCancel = isNewFolder ?
          handleNewFolderCancel
        : handleNewFileCancel
      const dispatchType =
        isNewFolder ? 'set-new-folder-name' : 'set-new-file-name'

      return (
        <div
          className="flex min-w-[120px] flex-1 items-center gap-2 overflow-hidden"
          onClick={(e) => e.stopPropagation()}
          onMouseDown={(e) => e.stopPropagation()}
        >
          <input
            autoFocus
            className="bg-background text-foreground border-brand min-w-0 flex-1 rounded border px-1 py-0 text-xs outline-none"
            value={value}
            onChange={(e) =>
              dispatch({ type: dispatchType, name: e.target.value })
            }
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault()
                void handleConfirm(e.currentTarget.value)
              }
              if (e.key === 'Escape') {
                e.preventDefault()
                handleCancel()
              }
              e.stopPropagation()
            }}
            onBlur={(e) => {
              if (e.currentTarget.value.trim()) {
                void handleConfirm(e.currentTarget.value)
              } else {
                handleCancel()
              }
            }}
            placeholder={placeholder}
          />
        </div>
      )
    }
  }, [
    newFolderName,
    newFileName,
    renamingEntry,
    handleConfirmRename,
    handleCancelRename,
    handleNewFolderConfirm,
    handleNewFolderCancel,
    handleNewFileConfirm,
    handleNewFileCancel,
  ])

  // Context menu props shared between states
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

  // Determine loading state - include stat loading
  const isLoading =
    rootHandle.loading ||
    pathHandle.loading ||
    statResource.loading ||
    (isDir === true && entriesResource.loading)
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

  // Hidden file input for uploads
  const fileInput = (
    <input
      ref={fileInputRef}
      type="file"
      multiple
      className="hidden"
      onChange={handleFileInputChange}
    />
  )

  // Delete confirmation dialog
  const deleteDialog = (
    <Dialog
      open={deleteTargets !== null}
      onOpenChange={(open) => {
        if (!open) handleCancelDelete()
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            Delete {deleteTargets?.length === 1 ? 'item' : 'items'}
          </DialogTitle>
          <DialogDescription>
            {deleteTargets?.length === 1 ?
              `Are you sure you want to delete "${deleteTargets[0].name}"?`
            : `Are you sure you want to delete ${deleteTargets?.length ?? 0} items?`
            }{' '}
            This action cannot be undone.
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <button
            className="hover:bg-accent rounded-md px-4 py-2 text-xs"
            onClick={handleCancelDelete}
          >
            Cancel
          </button>
          <button
            className="bg-destructive text-destructive-foreground hover:bg-destructive/90 rounded-md px-4 py-2 text-xs"
            onClick={() => void handleConfirmDelete()}
          >
            Delete
          </button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )

  const moveDialog =
    moveDialogItems && rootHandle.value ?
      <UnixFSMoveDialog
        rootHandle={rootHandle.value}
        moveItems={moveDialogItems}
        onOpenChange={(open) => {
          if (!open) {
            dispatch({ type: 'clear-move' })
          }
        }}
        onConfirm={handleConfirmMove}
      />
    : null

  // Drag overlay
  const dragOverlay = isDragging && (
    <div className="border-brand/50 bg-brand/5 pointer-events-none absolute inset-0 z-10 flex items-center justify-center rounded-md border-2 border-dashed">
      <div className="flex flex-col items-center gap-2">
        <LuUpload className="text-brand h-8 w-8" />
        <span className="text-brand text-sm font-medium">
          Drop files to upload
        </span>
      </div>
    </div>
  )

  // During directory transitions, keep showing previous entries with a loading indicator
  if (isLoading && deferredFileEntries.length > 0) {
    return (
      <div
        data-testid="unixfs-browser"
        className="flex h-full w-full flex-col overflow-hidden"
        onKeyDown={handleKeyDown}
      >
        <Toolbar
          currentPath={displayPath}
          onPathChange={handlePathChange}
          onNavigate={handlePathChange}
          onBack={handleBack}
          onForward={handleForward}
          onUp={handleUp}
          canGoBack={history?.canGoBack ?? false}
          canGoForward={history?.canGoForward ?? false}
          canGoUp={displayPath !== '/'}
          onNewFolder={handleNewFolder}
          onUploadFiles={handleUploadFiles}
        />
        <div
          className="bg-file-back relative flex min-h-0 flex-1 flex-col overflow-hidden"
          onContextMenu={handleBackgroundContextMenu}
          onDragOver={handleDragOver}
          onDragLeave={handleDragLeave}
          onDrop={(e) => void handleDrop(e)}
        >
          <FileList
            entries={deferredFileEntries}
            onOpen={handleOpen}
            onContextMenu={handleContextMenu}
            onStateChange={handleListStateChange}
            loadingId={entriesResource.loading ? pendingName : null}
            getDragEnvelope={getDragEnvelope}
            getDownloadDragTarget={getDownloadDragTarget}
            dropTargetEntryId={folderDropEntryId}
            onEntryDragOver={handleEntryDragOver}
            onEntryDragLeave={handleEntryDragLeave}
            onEntryDrop={handleEntryDrop}
          />
          <div className="border-foreground/5 bg-background/85 pointer-events-none absolute bottom-3 left-1/2 z-10 -translate-x-1/2 rounded-md border px-3 py-2 shadow-sm backdrop-blur">
            {loadingDiagnostics}
          </div>
          {dragOverlay}
        </div>
        <UnixFSContextMenu {...contextMenuProps} />
        <UploadProgressBottomBar uploadManager={uploadManager} />
        {fileInput}
        {deleteDialog}
        {moveDialog}
      </div>
    )
  }

  // Show fullscreen loading state only on initial load
  if (isLoading) {
    return (
      <div
        data-testid="unixfs-browser"
        className="flex h-full w-full flex-col overflow-hidden"
      >
        <Toolbar
          currentPath={displayPath}
          onPathChange={handlePathChange}
          onNavigate={handlePathChange}
          onBack={handleBack}
          onForward={handleForward}
          onUp={handleUp}
          canGoBack={history?.canGoBack ?? false}
          canGoForward={history?.canGoForward ?? false}
          canGoUp={displayPath !== '/'}
          onNewFolder={handleNewFolder}
          onUploadFiles={handleUploadFiles}
        />
        <div className="bg-file-back flex min-h-0 flex-1 flex-col items-center justify-center gap-3 overflow-hidden p-6">
          <div className="w-full max-w-sm">
            <UnixFSPathLoadingCard
              root={rootHandle}
              lookup={pathHandle}
              stat={statResource}
              entries={
                isDir === true ? (entriesResource as Resource<unknown>) : null
              }
              path={displayPath}
            />
          </div>
          {loadingDiagnostics}
        </div>
      </div>
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
      <div
        data-testid="unixfs-browser"
        className="flex h-full w-full flex-col overflow-hidden"
      >
        <div className="bg-file-back flex min-h-0 flex-1 flex-col items-center justify-center overflow-hidden">
          <div className="text-destructive text-xs">Error loading files</div>
          <div className="text-foreground-alt/70 mt-1 text-xs">
            {error.message ?? 'Unknown error'}
          </div>
          <button
            className="text-brand mt-2 text-xs underline"
            onClick={handleRetry}
          >
            Retry
          </button>
        </div>
      </div>
    )
  }

  // Show placeholder if UnixFS object not found
  if (!rootHandle.value) {
    return (
      <div
        data-testid="unixfs-browser"
        className="flex h-full w-full flex-col overflow-hidden"
      >
        <div className="bg-file-back flex min-h-0 flex-1 flex-col items-center justify-center overflow-hidden">
          <div className="text-foreground-alt text-sm">
            UnixFS object not found
          </div>
          <div className="text-foreground-alt/70 mt-1 text-xs">
            Object: {unixfsId || 'none'}
          </div>
          <div className="text-foreground-alt/70 mt-2 text-xs">
            Create a drive via quickstart to initialize demo content.
          </div>
        </div>
      </div>
    )
  }

  // Render file viewer for files
  if (isDir === false && statResource.value) {
    return (
      <>
        <UnixFSFileViewer
          path={displayPath}
          stat={{
            ...statResource.value,
            mimeType: effectiveMimeType ?? statResource.value.mimeType,
          }}
          rootHandle={rootHandle}
          inlineFileURL={inlineFileURL}
        />
        <UploadProgressBottomBar uploadManager={uploadManager} />
      </>
    )
  }

  // Render directory listing
  return (
    <div
      data-testid="unixfs-browser"
      className="flex h-full w-full flex-col overflow-hidden"
      onKeyDown={handleKeyDown}
    >
      <Toolbar
        currentPath={displayPath}
        onPathChange={handlePathChange}
        onNavigate={handlePathChange}
        onBack={handleBack}
        onForward={handleForward}
        onUp={handleUp}
        canGoBack={history?.canGoBack ?? false}
        canGoForward={history?.canGoForward ?? false}
        canGoUp={displayPath !== '/'}
        onNewFolder={handleNewFolder}
        onUploadFiles={handleUploadFiles}
      />

      <div
        className={cn(
          'bg-file-back relative flex min-h-0 flex-1 flex-col overflow-hidden',
        )}
        onContextMenu={handleBackgroundContextMenu}
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        onDrop={(e) => void handleDrop(e)}
      >
        <FileList
          entries={displayEntries}
          onOpen={handleOpen}
          onContextMenu={handleContextMenu}
          onStateChange={handleListStateChange}
          renderEntry={renderEntry}
          currentPath={displayPath}
          getDragEnvelope={getDragEnvelope}
          getDownloadDragTarget={getDownloadDragTarget}
          dropTargetEntryId={folderDropEntryId}
          onEntryDragOver={handleEntryDragOver}
          onEntryDragLeave={handleEntryDragLeave}
          onEntryDrop={handleEntryDrop}
        />
        {dragOverlay}
      </div>

      <UnixFSContextMenu {...contextMenuProps} />
      <UploadProgressBottomBar uploadManager={uploadManager} />
      {fileInput}
      {deleteDialog}
      {moveDialog}
    </div>
  )
}
