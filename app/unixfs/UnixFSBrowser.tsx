import { useAppEnvironment } from '@s4wave/web/sdk/app/environment.js'
import {
  type ComponentProps,
  type ComponentType,
  type DragEvent,
  type ReactNode,
  useCallback,
  useDeferredValue,
  useMemo,
  useRef,
} from 'react'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import type { FSHandle } from '@s4wave/sdk/unixfs/handle.js'
import { getUnixFSParentPath } from '@s4wave/sdk/unixfs/path.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { FileEntry } from '@s4wave/web/editors/file-browser/types.js'
import { useFileListState } from '@s4wave/web/editors/file-browser/FileList.js'
import { UnixFSPathLoadingCard } from '@s4wave/app/loading/wrappers/UnixFSPathLoadingCard.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { useSessionIndex } from '@s4wave/web/contexts/contexts.js'
import { useSessionUploadManager } from '@s4wave/app/session/SessionUploadManagerContext.js'
import {
  type SessionSyncStatusView,
  useSessionSyncStatus,
} from '../session/SessionSyncStatusContext.js'
import { UnixFSFileViewer } from './UnixFSFileViewer.js'
import { UnixFSBrowserDialogs } from './UnixFSBrowserDialogs.js'
import { UnixFSBrowserDropSurface } from './UnixFSBrowserDropSurface.js'
import { UnixFSBrowserShell } from './UnixFSBrowserShell.js'
import { UnixFSDirectoryListing } from './UnixFSDirectoryListing.js'
import { UnixFSBrowserUnavailableState } from './UnixFSBrowserUnavailableState.js'
import { buildUnixFSFileInlineURL } from './download.js'
import { useUnixFSBrowserCommands } from './useUnixFSBrowserCommands.js'
import { useUnixFSBrowserContextMenu } from './useUnixFSBrowserContextMenu.js'
import { useUnixFSBrowserDrag } from './useUnixFSBrowserDrag.js'
import { useUnixFSBrowserDragTargets } from './useUnixFSBrowserDragTargets.js'
import {
  useUnixFSBrowserEdits,
  useUnixFSBrowserRename,
} from './useUnixFSBrowserEdits.js'
import { useUnixFSBrowserNavigation } from './useUnixFSBrowserNavigation.js'
import { useUnixFSBrowserResources } from './useUnixFSBrowserResources.js'
import { useUnixFSBrowserState } from './useUnixFSBrowserState.js'
import { useUnixFSBrowserTransfer } from './useUnixFSBrowserTransfer.js'
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
  // directoryHeader renders caller-supplied content above the file list.
  directoryHeader?: ComponentType<UnixFSBrowserDirectoryHeaderProps>
}

// noSelection is the selection of a file list that has none.
const noSelection: string[] = []

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
      className="text-foreground-alt/60 max-w-xl space-y-1 text-center font-mono text-xs leading-relaxed"
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

/** inlineFileURLFor returns the inline URL of the file at the display path. */
function inlineFileURLFor(
  isDir: boolean | null,
  hasStat: boolean,
  sessionIndex: number,
  spaceId: string | null,
  unixfsId: string,
  displayPath: string,
  httpPathPrefix: string,
): string | undefined {
  if (isDir !== false || !hasStat || !sessionIndex || !spaceId) {
    return undefined
  }
  return buildUnixFSFileInlineURL(
    sessionIndex,
    spaceId,
    unixfsId,
    displayPath,
    httpPathPrefix,
  )
}

/**
 * useUnixFSBrowserModel owns the resources, state, and handlers of the
 * browser and returns what the views render.
 */
function useUnixFSBrowserModel({
  unixfsId,
  basePath,
  currentPath,
  worldState,
}: UnixFSBrowserProps) {
  const { httpPathPrefix } = useAppEnvironment()
  const spaceId = SpaceContainerContext.useContextSafe()?.spaceId ?? null
  const sessionIndex = useSessionIndex()
  const syncStatus = useSessionSyncStatus()
  const displayPath = currentPath || basePath || '/'

  // Uploads are owned by the session, not this viewer, so an in-flight upload
  // and its feedback survive navigating away from this folder. Null when this
  // browser is mounted outside a session (display, debug harness); uploads are
  // simply unavailable there.
  const uploadManager = useSessionUploadManager()

  const {
    rootHandle,
    pathHandle,
    statResource,
    entriesResource,
    fileEntries,
    getEntryDetails,
    isDir,
  } = useUnixFSBrowserResources({ worldState, unixfsId, displayPath })

  const [state, dispatch] = useUnixFSBrowserState()
  const { newFolderName, newFileName, renamingEntry } = state
  const listState = useFileListState()
  const [{ selectedIds = noSelection }, updateListState] = listState
  const deferredFileEntries = useDeferredValue(fileEntries)
  const renameRef = useRef('')

  const navigation = useUnixFSBrowserNavigation({
    unixfsId,
    displayPath,
    dispatch,
  })

  const clearSelection = useCallback(
    () => updateListState((prev) => ({ ...prev, selectedIds: [] })),
    [updateListState],
  )

  const selectedEntries = useMemo(() => {
    const selected = new Set(selectedIds)
    return fileEntries.filter((entry) => selected.has(entry.id))
  }, [fileEntries, selectedIds])

  const hasStat = !!statResource.value
  const inlineFileURL = useMemo(
    () =>
      inlineFileURLFor(
        isDir,
        hasStat,
        sessionIndex,
        spaceId,
        unixfsId,
        displayPath,
        httpPathPrefix,
      ),
    [
      displayPath,
      hasStat,
      httpPathPrefix,
      isDir,
      sessionIndex,
      spaceId,
      unixfsId,
    ],
  )

  const { getDragEnvelope, getDownloadDragTarget } =
    useUnixFSBrowserDragTargets({
      displayPath,
      selectedEntries,
      sessionIndex,
      spaceId,
      unixfsId,
    })

  const contextMenu = useUnixFSBrowserContextMenu({
    displayPath,
    selectedEntries,
    selectedIds,
    dispatch,
  })
  const transfer = useUnixFSBrowserTransfer({
    httpPathPrefix,
    sessionIndex,
    spaceId,
    unixfsId,
    displayPath,
    pathHandle,
    uploadManager,
  })
  const rename = useUnixFSBrowserRename({
    pathHandle,
    renamingEntry,
    renameRef,
    dispatch,
  })
  const edits = useUnixFSBrowserEdits({
    rootHandle,
    pathHandle,
    displayPath,
    state,
    dispatch,
    clearSelection,
  })

  const setDragging = useCallback(
    (dragging: boolean) => dispatch({ type: 'set-dragging', dragging }),
    [dispatch],
  )
  const setFolderDropEntryId = useCallback(
    (id: string | null) => dispatch({ type: 'set-folder-drop-entry', id }),
    [dispatch],
  )
  const drag = useUnixFSBrowserDrag({
    unixfsId,
    displayPath,
    rootHandle: rootHandle.value,
    sourceParentHandle: pathHandle.value,
    uploadManager,
    folderDropEntryId: state.folderDropEntryId,
    setDragging,
    setFolderDropEntryId,
  })

  const handleKeyDown = useUnixFSDeleteKeyHandler({
    selectedEntries,
    onDelete: edits.handleRequestDelete,
  })

  useUnixFSBrowserCommands({
    selectedEntries,
    canGoBack: navigation.canGoBack,
    canGoForward: navigation.canGoForward,
    canGoUp: navigation.canGoUp,
    onNewFile: edits.handleNewFile,
    onNewFolder: edits.handleNewFolder,
    onUploadFiles: transfer.handleUploadFiles,
    onOpen: navigation.handleOpen,
    onRename: rename.handleStartRename,
    onDownload: transfer.handleDownload,
    onDelete: edits.handleRequestDelete,
    onBack: navigation.handleBack,
    onForward: navigation.handleForward,
    onUp: navigation.handleUp,
  })

  const entriesLoading = isDir === true && entriesResource.loading
  const isLoading =
    rootHandle.loading ||
    pathHandle.loading ||
    statResource.loading ||
    entriesLoading

  // displayEntries prepends the inline new folder and new file inputs.
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
  })

  const renderEntry = useUnixFSInlineEntryRenderer({
    newFolderName,
    newFileName,
    renamingEntry,
    renameRef,
    onConfirmRename: rename.handleConfirmRename,
    onCancelRename: rename.handleCancelRename,
    onNewFolderNameChange: edits.handleNewFolderNameChange,
    onNewFileNameChange: edits.handleNewFileNameChange,
    onNewFolderConfirm: edits.handleNewFolderConfirm,
    onNewFolderCancel: edits.handleNewFolderCancel,
    onNewFileConfirm: edits.handleNewFileConfirm,
    onNewFileCancel: edits.handleNewFileCancel,
  })

  const contextMenuProps = useMemo(
    () => ({
      state: state.contextMenu,
      onClose: contextMenu.handleCloseContextMenu,
      onOpen: navigation.handleOpen,
      onDownload: transfer.handleDownload,
      onMove: edits.handleRequestMove,
      onRename: rename.handleStartRename,
      onDelete: edits.handleRequestDelete,
      onNewFolder: edits.handleNewFolder,
      onUploadFiles: transfer.handleUploadFiles,
    }),
    [
      state.contextMenu,
      contextMenu.handleCloseContextMenu,
      navigation.handleOpen,
      transfer.handleDownload,
      edits.handleRequestMove,
      rename.handleStartRename,
      edits.handleRequestDelete,
      edits.handleNewFolder,
      transfer.handleUploadFiles,
    ],
  )
  const loadingStageLabel = useMemo(
    () =>
      buildUnixFSLoadingStageLabel({
        rootLoading: rootHandle.loading,
        pathLoading: pathHandle.loading,
        statLoading: statResource.loading,
        entriesLoading,
      }),
    [
      entriesLoading,
      pathHandle.loading,
      rootHandle.loading,
      statResource.loading,
    ],
  )

  const shellNavigationProps = {
    currentPath: displayPath,
    onPathChange: navigation.handlePathChange,
    onBack: navigation.handleBack,
    onForward: navigation.handleForward,
    onUp: navigation.handleUp,
    canGoBack: navigation.canGoBack,
    canGoForward: navigation.canGoForward,
    canGoUp: navigation.canGoUp,
    upDropPath: getUnixFSParentPath(displayPath),
    onPathTargetDragOver: drag.handlePathTargetDragOver,
    onPathTargetDrop: drag.handlePathTargetDrop,
  }

  // failure is the first resource that failed to load, in dependency order.
  const failed = [rootHandle, pathHandle, statResource, entriesResource].find(
    (resource) => resource.error,
  )
  const failure = failed?.error
    ? { error: failed.error, retry: failed.retry }
    : null

  return {
    displayPath,
    unixfsId,
    rootHandle,
    pathHandle,
    statResource,
    entriesResource,
    fileEntries,
    deferredFileEntries,
    displayEntries,
    isDir,
    isLoading,
    failure,
    inlineFileURL,
    renderEntry,
    pendingName: state.pendingName,
    isDragging: state.isDragging,
    handleKeyDown,
    handleBackgroundContextMenu: contextMenu.handleBackgroundContextMenu,
    shellNavigationProps,
    shellActionProps: {
      ...shellNavigationProps,
      onNewFolder: edits.handleNewFolder,
      onUploadFiles: transfer.handleUploadFiles,
    },
    dropHandlers: {
      onDragOver: drag.handleDragOver,
      onDragLeave: drag.handleDragLeave,
      onDrop: (e: DragEvent<HTMLDivElement>) => void drag.handleDrop(e),
    },
    listingProps: {
      currentPath: displayPath,
      getEntryDetails,
      onOpen: navigation.handleOpen,
      onContextMenu: contextMenu.handleContextMenu,
      listState,
      onNewFolder: edits.handleNewFolder,
      onUploadFiles: transfer.handleUploadFiles,
      getDragEnvelope,
      getDownloadDragTarget,
      dropTargetEntryId: state.folderDropEntryId,
      onEntryDragOver: drag.handleEntryDragOver,
      onEntryDragLeave: drag.handleEntryDragLeave,
      onEntryDrop: drag.handleEntryDrop,
    },
    dialogsProps: {
      contextMenuProps,
      fileInputRef: transfer.fileInputRef,
      onFileInputChange: transfer.handleFileInputChange,
      deleteTargets: state.deleteTargets,
      onCancelDelete: edits.handleCancelDelete,
      onConfirmDelete: edits.handleConfirmDelete,
      moveRootHandle: rootHandle.value,
      moveDialogItems: state.moveDialogItems,
      onCancelMove: edits.handleCancelMove,
      onConfirmMove: edits.handleConfirmMove,
    },
    loadingDiagnostics: (
      <UnixFSLoadingDiagnostics
        stageLabel={loadingStageLabel}
        status={syncStatus}
      />
    ),
  }
}

type UnixFSBrowserModel = ReturnType<typeof useUnixFSBrowserModel>

/** UnixFSBrowserInteractive wraps content in the shell, drop surface, and dialogs. */
function UnixFSBrowserInteractive({
  model,
  floatingDiagnostics,
  children,
}: {
  model: UnixFSBrowserModel
  floatingDiagnostics?: ReactNode
  children: ReactNode
}) {
  return (
    <UnixFSBrowserShell
      {...model.shellActionProps}
      interactive
      onKeyDown={model.handleKeyDown}
    >
      <UnixFSBrowserDropSurface
        isDragging={model.isDragging}
        floatingDiagnostics={floatingDiagnostics}
        onContextMenu={model.handleBackgroundContextMenu}
        {...model.dropHandlers}
      >
        {children}
      </UnixFSBrowserDropSurface>
      <UnixFSBrowserDialogs {...model.dialogsProps} />
    </UnixFSBrowserShell>
  )
}

/** UnixFSBrowserListing renders the directory listing with the shared handlers. */
function UnixFSBrowserListing({
  model,
  ...listing
}: { model: UnixFSBrowserModel } & Pick<
  ComponentProps<typeof UnixFSDirectoryListing>,
  'entries' | 'displayEntries' | 'loadingId' | 'DirectoryHeader' | 'renderEntry'
>) {
  return <UnixFSDirectoryListing {...model.listingProps} {...listing} />
}

/** UnixFSBrowserLoading renders the full loading state of the first load. */
function UnixFSBrowserLoading({ model }: { model: UnixFSBrowserModel }) {
  return (
    <UnixFSBrowserShell {...model.shellActionProps}>
      <div className="bg-file-back flex min-h-0 flex-1 flex-col items-center justify-center gap-3 overflow-hidden p-6">
        <div className="w-full max-w-sm">
          <UnixFSPathLoadingCard
            root={model.rootHandle}
            lookup={model.pathHandle}
            stat={model.statResource}
            entries={model.isDir === true ? model.entriesResource : null}
            path={model.displayPath}
          />
        </div>
        {model.loadingDiagnostics}
      </div>
    </UnixFSBrowserShell>
  )
}

// UnixFSBrowser renders a UnixFS filesystem browser for use in layout tabs.
export function UnixFSBrowser(props: UnixFSBrowserProps) {
  const {
    unixfsId,
    mimeTypeOverride,
    browserBody: BrowserBody,
    directoryHeader: DirectoryHeader,
  } = props
  const model = useUnixFSBrowserModel(props)
  const { rootHandle, statResource, displayPath } = model
  const browserBody = BrowserBody && (
    <BrowserBody
      rootHandle={rootHandle}
      unixfsId={unixfsId}
      currentPath={displayPath}
    />
  )

  // During directory transitions, keep showing previous entries with a loading indicator
  if (model.isLoading && model.deferredFileEntries.length > 0) {
    return (
      <UnixFSBrowserInteractive
        model={model}
        floatingDiagnostics={model.loadingDiagnostics}
      >
        {browserBody ?? (
          <UnixFSBrowserListing
            model={model}
            entries={model.deferredFileEntries}
            displayEntries={model.deferredFileEntries}
            loadingId={model.entriesResource.loading ? model.pendingName : null}
          />
        )}
      </UnixFSBrowserInteractive>
    )
  }

  // Show fullscreen loading state only on initial load
  if (model.isLoading) return <UnixFSBrowserLoading model={model} />

  if (model.failure) {
    return (
      <UnixFSBrowserUnavailableState
        kind="error"
        shellProps={model.shellNavigationProps}
        error={model.failure.error}
        onRetry={model.failure.retry}
      />
    )
  }

  if (!rootHandle.value) {
    return (
      <UnixFSBrowserUnavailableState kind="not-found" unixfsId={unixfsId} />
    )
  }

  if (browserBody) {
    return (
      <UnixFSBrowserInteractive model={model}>
        {browserBody}
      </UnixFSBrowserInteractive>
    )
  }

  if (model.isDir === false && statResource.value) {
    return (
      <UnixFSFileViewer
        path={displayPath}
        stat={{
          ...statResource.value,
          mimeType: mimeTypeOverride || statResource.value.mimeType,
        }}
        rootHandle={rootHandle}
        inlineFileURL={model.inlineFileURL}
      />
    )
  }

  return (
    <UnixFSBrowserInteractive model={model}>
      <UnixFSBrowserListing
        model={model}
        entries={model.fileEntries}
        displayEntries={model.displayEntries}
        DirectoryHeader={DirectoryHeader}
        renderEntry={model.renderEntry}
      />
    </UnixFSBrowserInteractive>
  )
}
