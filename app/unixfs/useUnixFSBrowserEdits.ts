import { useCallback, type RefObject } from 'react'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { MknodType } from '@s4wave/sdk/unixfs/index.js'
import type { FSHandle } from '@s4wave/sdk/unixfs/handle.js'
import type { FileEntry } from '@s4wave/web/editors/file-browser/types.js'

import { moveUnixFSItemsFromDirectory, type UnixFSMoveItem } from './move.js'
import type {
  UnixFSBrowserDispatch,
  UnixFSBrowserState,
} from './useUnixFSBrowserState.js'

interface UnixFSBrowserRenameOptions {
  pathHandle: Resource<FSHandle>
  renamingEntry: FileEntry | null
  renameRef: RefObject<string>
  dispatch: UnixFSBrowserDispatch
}

/** useUnixFSBrowserRename returns the handlers of the inline rename. */
export function useUnixFSBrowserRename({
  pathHandle,
  renamingEntry,
  renameRef,
  dispatch,
}: UnixFSBrowserRenameOptions) {
  // handleStartRename activates inline rename for a file entry.
  const handleStartRename = useCallback(
    (entry: FileEntry) => {
      renameRef.current = entry.name
      dispatch({ type: 'start-rename', entry })
    },
    [dispatch, renameRef],
  )

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
  }, [dispatch, pathHandle.value, renameRef, renamingEntry])

  const handleCancelRename = useCallback(() => {
    dispatch({ type: 'clear-rename' })
  }, [dispatch])

  return { handleStartRename, handleConfirmRename, handleCancelRename }
}

interface UnixFSBrowserEditsOptions {
  rootHandle: Resource<FSHandle>
  pathHandle: Resource<FSHandle>
  displayPath: string
  state: UnixFSBrowserState
  dispatch: UnixFSBrowserDispatch
  // clearSelection empties the file list's selection.
  clearSelection: () => void
}

/**
 * useUnixFSBrowserEdits returns the handlers that delete, move, and create
 * entries in the current directory.
 */
export function useUnixFSBrowserEdits({
  rootHandle,
  pathHandle,
  displayPath,
  state,
  dispatch,
  clearSelection,
}: UnixFSBrowserEditsOptions) {
  const { deleteTargets, moveDialogItems } = state

  // handleRequestDelete opens the delete confirmation dialog for the entries.
  const handleRequestDelete = useCallback(
    (entries: FileEntry[]) => {
      dispatch({ type: 'request-delete', entries })
    },
    [dispatch],
  )

  const handleConfirmDelete = useCallback(async () => {
    if (!deleteTargets || !pathHandle.value) return
    await pathHandle.value.remove(deleteTargets.map((e) => e.name))
    dispatch({ type: 'clear-delete' })
  }, [deleteTargets, dispatch, pathHandle.value])

  const handleCancelDelete = useCallback(() => {
    dispatch({ type: 'clear-delete' })
  }, [dispatch])

  const handleRequestMove = useCallback(
    (moveItems: UnixFSMoveItem[]) => {
      if (moveItems.length === 0) return
      dispatch({ type: 'request-move', items: moveItems })
    },
    [dispatch],
  )

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
      clearSelection()
      dispatch({ type: 'clear-move' })
    },
    [
      clearSelection,
      dispatch,
      displayPath,
      moveDialogItems,
      pathHandle.value,
      rootHandle.value,
    ],
  )

  const handleCancelMove = useCallback(() => {
    dispatch({ type: 'clear-move' })
  }, [dispatch])

  // handleNewFolder opens the inline new-folder input.
  const handleNewFolder = useCallback(() => {
    dispatch({ type: 'start-new-folder' })
  }, [dispatch])

  const handleNewFolderConfirm = useCallback(
    async (name: string) => {
      const folderName = name.trim()
      if (!folderName || !pathHandle.value) return
      await pathHandle.value.mkdirAll([folderName])
      dispatch({ type: 'clear-new-folder' })
    },
    [dispatch, pathHandle.value],
  )

  const handleNewFolderCancel = useCallback(() => {
    dispatch({ type: 'clear-new-folder' })
  }, [dispatch])

  const handleNewFolderNameChange = useCallback(
    (name: string) => {
      dispatch({ type: 'set-new-folder-name', name })
    },
    [dispatch],
  )

  // handleNewFile opens the inline new-file input.
  const handleNewFile = useCallback(() => {
    dispatch({ type: 'start-new-file' })
  }, [dispatch])

  const handleNewFileConfirm = useCallback(
    async (name: string) => {
      const fileName = name.trim()
      if (!fileName || !pathHandle.value) return
      await pathHandle.value.mknod([fileName], MknodType.FILE)
      dispatch({ type: 'clear-new-file' })
    },
    [dispatch, pathHandle.value],
  )

  const handleNewFileCancel = useCallback(() => {
    dispatch({ type: 'clear-new-file' })
  }, [dispatch])

  const handleNewFileNameChange = useCallback(
    (name: string) => {
      dispatch({ type: 'set-new-file-name', name })
    },
    [dispatch],
  )

  return {
    handleRequestDelete,
    handleConfirmDelete,
    handleCancelDelete,
    handleRequestMove,
    handleConfirmMove,
    handleCancelMove,
    handleNewFolder,
    handleNewFolderConfirm,
    handleNewFolderCancel,
    handleNewFolderNameChange,
    handleNewFile,
    handleNewFileConfirm,
    handleNewFileCancel,
    handleNewFileNameChange,
  }
}
