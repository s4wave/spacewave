import { useCallback, type DragEvent } from 'react'

import type { FSHandle } from '@s4wave/sdk/unixfs/handle.js'
import { joinUnixFSDisplayPath } from '@s4wave/sdk/unixfs/path.js'
import type { FileEntry } from '@s4wave/web/editors/file-browser/types.js'
import { hasNativeFileDrag } from '@s4wave/web/dnd/app-drag.js'
import type { UploadManager } from '@s4wave/app/unixfs/useUploadManager.js'
import { toast } from '@s4wave/web/ui/toaster.js'

import { extractNativeUploadSelection } from './native-upload.js'
import {
  getUnixFSBaseName,
  moveUnixFSItemsFromDirectory,
  validateUnixFSMove,
} from './move.js'
import { readUnixFSMovableAppDragItems } from './unixfs-app-drag.js'

interface UnixFSBrowserDragOptions {
  unixfsId: string
  displayPath: string
  rootHandle: FSHandle | null | undefined
  sourceParentHandle: FSHandle | null | undefined
  uploadManager: UploadManager | null
  folderDropEntryId: string | null
  setDragging: (dragging: boolean) => void
  setFolderDropEntryId: (id: string | null) => void
}

export function useUnixFSBrowserDrag({
  unixfsId,
  displayPath,
  rootHandle,
  sourceParentHandle,
  uploadManager,
  folderDropEntryId,
  setDragging,
  setFolderDropEntryId,
}: UnixFSBrowserDragOptions) {
  const handleDragOver = useCallback(
    (event: DragEvent<HTMLDivElement>) => {
      if (!hasNativeFileDrag(event.dataTransfer)) {
        setDragging(false)
        return
      }
      event.preventDefault()
      event.stopPropagation()
      setDragging(true)
    },
    [setDragging],
  )

  const handleDragLeave = useCallback(
    (event: DragEvent<HTMLDivElement>) => {
      event.preventDefault()
      event.stopPropagation()
      if (event.currentTarget.contains(event.relatedTarget as Node)) return
      setDragging(false)
    },
    [setDragging],
  )

  const handleDrop = useCallback(
    async (event: DragEvent<HTMLDivElement>) => {
      if (!hasNativeFileDrag(event.dataTransfer)) {
        setDragging(false)
        return
      }
      event.preventDefault()
      event.stopPropagation()
      setDragging(false)
      const selection = await extractNativeUploadSelection(event.dataTransfer)
      if (selection.files.length === 0 && selection.directories.length === 0) {
        return
      }
      if (sourceParentHandle) {
        uploadManager?.addFiles(
          sourceParentHandle,
          selection.files,
          selection.directories,
        )
      }
    },
    [setDragging, sourceParentHandle, uploadManager],
  )

  const getAcceptedPathMove = useCallback(
    (
      destinationPath: string,
      dataTransfer: DataTransfer | null | undefined,
    ) => {
      const movableItems = readUnixFSMovableAppDragItems(dataTransfer)
      if (movableItems.length === 0) return null
      if (movableItems.some((item) => item.value.unixfsId !== unixfsId)) {
        return null
      }

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
      const validation = validateUnixFSMove(moveItems, destinationPath)
      if (!validation.accepted) {
        return null
      }

      return {
        destinationPath,
        items: moveItems,
      }
    },
    [unixfsId],
  )

  const getAcceptedFolderMove = useCallback(
    (entry: FileEntry, dataTransfer: DataTransfer | null | undefined) => {
      if (!entry.isDir) return null
      return getAcceptedPathMove(
        joinUnixFSDisplayPath(displayPath, entry.name),
        dataTransfer,
      )
    },
    [displayPath, getAcceptedPathMove],
  )

  const handleEntryDragOver = useCallback(
    (entry: FileEntry, event: DragEvent<HTMLDivElement>) => {
      const acceptedMove = getAcceptedFolderMove(entry, event.dataTransfer)
      if (!acceptedMove) {
        if (folderDropEntryId === entry.id) {
          setFolderDropEntryId(null)
        }
        return false
      }
      if (folderDropEntryId !== entry.id) {
        setFolderDropEntryId(entry.id)
      }
      return true
    },
    [folderDropEntryId, getAcceptedFolderMove, setFolderDropEntryId],
  )

  const handleEntryDragLeave = useCallback(
    (entry: FileEntry, event: DragEvent<HTMLDivElement>) => {
      if (event.currentTarget.contains(event.relatedTarget as Node)) return
      if (folderDropEntryId !== entry.id) return
      setFolderDropEntryId(null)
    },
    [folderDropEntryId, setFolderDropEntryId],
  )

  const handleEntryDrop = useCallback(
    (entry: FileEntry, event: DragEvent<HTMLDivElement>) => {
      const acceptedMove = getAcceptedFolderMove(entry, event.dataTransfer)
      setFolderDropEntryId(null)
      if (!acceptedMove || !rootHandle || !sourceParentHandle) return

      void (async () => {
        try {
          await moveUnixFSItemsFromDirectory(
            rootHandle,
            sourceParentHandle,
            displayPath,
            acceptedMove.items,
            acceptedMove.destinationPath,
          )
        } catch (err) {
          toast.error('Move failed', { description: String(err) })
        }
      })()
    },
    [
      displayPath,
      getAcceptedFolderMove,
      rootHandle,
      setFolderDropEntryId,
      sourceParentHandle,
    ],
  )

  const handlePathTargetDragOver = useCallback(
    (destinationPath: string, event: DragEvent<HTMLElement>) => {
      const acceptedMove = getAcceptedPathMove(
        destinationPath,
        event.dataTransfer,
      )
      if (!acceptedMove) return false
      event.preventDefault()
      event.stopPropagation()
      event.dataTransfer.dropEffect = 'move'
      return true
    },
    [getAcceptedPathMove],
  )

  const handlePathTargetDrop = useCallback(
    (destinationPath: string, event: DragEvent<HTMLElement>) => {
      const acceptedMove = getAcceptedPathMove(
        destinationPath,
        event.dataTransfer,
      )
      if (!acceptedMove) return
      event.preventDefault()
      event.stopPropagation()
      if (!rootHandle || !sourceParentHandle) return

      void (async () => {
        try {
          await moveUnixFSItemsFromDirectory(
            rootHandle,
            sourceParentHandle,
            displayPath,
            acceptedMove.items,
            acceptedMove.destinationPath,
          )
        } catch (err) {
          toast.error('Move failed', { description: String(err) })
        }
      })()
    },
    [displayPath, getAcceptedPathMove, rootHandle, sourceParentHandle],
  )

  return {
    handleDragOver,
    handleDragLeave,
    handleDrop,
    handleEntryDragOver,
    handleEntryDragLeave,
    handleEntryDrop,
    handlePathTargetDragOver,
    handlePathTargetDrop,
  }
}
