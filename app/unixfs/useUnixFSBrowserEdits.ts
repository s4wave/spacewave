import { useCallback, type RefObject } from 'react'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { MknodType } from '@s4wave/sdk/unixfs/index.js'
import type { FSHandle } from '@s4wave/sdk/unixfs/handle.js'
import type { FileEntry } from '@s4wave/web/editors/file-browser/types.js'
import { toast } from '@s4wave/web/ui/toaster.js'

import { moveUnixFSItemsFromDirectory, type UnixFSMoveItem } from './move.js'
import type {
  UnixFSBrowserDispatch,
  UnixFSBrowserState,
} from './useUnixFSBrowserState.js'

// runEdit runs one write to the Space, as an upload does: while the Space
// takes no writes it says why and writes nothing, and a write the Space
// refuses says what did not happen. It returns whether the write landed, so
// the edit's input or dialog stays open for another try when it did not.
async function runEdit(
  saveHold: string | null,
  failed: string,
  write: () => Promise<unknown>,
): Promise<boolean> {
  if (saveHold) {
    toast.info(saveHold)
    return false
  }
  try {
    await write()
    return true
  } catch (err) {
    toast.error(failed, { description: errorText(err) })
    return false
  }
}

// errorText is the plain words of a failed write.
function errorText(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}

interface UnixFSBrowserRenameOptions {
  pathHandle: Resource<FSHandle>
  // saveHold is why writes to the Space wait, from useSaveHold, or null.
  saveHold: string | null
  renamingEntry: FileEntry | null
  renameRef: RefObject<string>
  dispatch: UnixFSBrowserDispatch
}

/** useUnixFSBrowserRename returns the handlers of the inline rename. */
export function useUnixFSBrowserRename({
  pathHandle,
  saveHold,
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
    const handle = pathHandle.value
    if (!renamingEntry || !handle) return
    const newName = renameRef.current.trim()
    if (!newName || newName === renamingEntry.name) {
      dispatch({ type: 'clear-rename' })
      return
    }
    if (newName.includes('/') || newName.includes('\\')) return

    const renamed = await runEdit(saveHold, 'Not renamed.', () =>
      handle.rename(renamingEntry.name, newName),
    )
    if (!renamed) return
    dispatch({ type: 'clear-rename' })
    dispatch({ type: 'set-context-menu', menu: null })
  }, [dispatch, pathHandle.value, renameRef, renamingEntry, saveHold])

  const handleCancelRename = useCallback(() => {
    dispatch({ type: 'clear-rename' })
  }, [dispatch])

  return { handleStartRename, handleConfirmRename, handleCancelRename }
}

interface UnixFSBrowserEditsOptions {
  rootHandle: Resource<FSHandle>
  pathHandle: Resource<FSHandle>
  // saveHold is why writes to the Space wait, from useSaveHold, or null.
  saveHold: string | null
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
  saveHold,
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
    const handle = pathHandle.value
    if (!deleteTargets || !handle) return
    const removed = await runEdit(saveHold, 'Nothing was deleted.', () =>
      handle.remove(deleteTargets.map((e) => e.name)),
    )
    if (removed) dispatch({ type: 'clear-delete' })
  }, [deleteTargets, dispatch, pathHandle.value, saveHold])

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
      const moved = await runEdit(saveHold, 'The move did not finish.', () =>
        moveUnixFSItemsFromDirectory(
          root,
          sourceParent,
          displayPath,
          moveDialogItems,
          destinationPath,
        ),
      )
      if (!moved) return
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
      saveHold,
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
      const handle = pathHandle.value
      if (!folderName || !handle) return
      const made = await runEdit(saveHold, 'The folder was not created.', () =>
        handle.mkdirAll([folderName]),
      )
      if (made) dispatch({ type: 'clear-new-folder' })
    },
    [dispatch, pathHandle.value, saveHold],
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
      const handle = pathHandle.value
      if (!fileName || !handle) return
      const made = await runEdit(saveHold, 'The file was not created.', () =>
        handle.mknod([fileName], MknodType.FILE),
      )
      if (made) dispatch({ type: 'clear-new-file' })
    },
    [dispatch, pathHandle.value, saveHold],
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
