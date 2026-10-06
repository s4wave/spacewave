import { useCallback, type MouseEvent } from 'react'

import type { FileEntry } from '@s4wave/web/editors/file-browser/types.js'
import type { ListItem } from '@s4wave/web/ui/list'

import { buildUnixFSMoveItems } from './move.js'
import type { UnixFSBrowserDispatch } from './useUnixFSBrowserState.js'

interface UnixFSBrowserContextMenuOptions {
  displayPath: string
  selectedEntries: FileEntry[]
  selectedIds: string[]
  dispatch: UnixFSBrowserDispatch
}

/** actionEntriesFor returns the entries a context menu acts on. */
function actionEntriesFor(
  entry: FileEntry | null,
  selectedEntries: FileEntry[],
  selectedIds: string[],
): FileEntry[] {
  if (!entry) return []
  if (selectedIds.includes(entry.id) && selectedEntries.length > 0) {
    return selectedEntries
  }
  return [entry]
}

/**
 * useUnixFSBrowserContextMenu returns the handlers that open and close the
 * context menu for an entry or for the empty background.
 */
export function useUnixFSBrowserContextMenu({
  displayPath,
  selectedEntries,
  selectedIds,
  dispatch,
}: UnixFSBrowserContextMenuOptions) {
  const handleContextMenu = useCallback(
    (item: ListItem<FileEntry>, event: MouseEvent) => {
      const entry = item.data ?? null
      const actionEntries = actionEntriesFor(
        entry,
        selectedEntries,
        selectedIds,
      )
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
    [dispatch, displayPath, selectedEntries, selectedIds],
  )

  const handleCloseContextMenu = useCallback(() => {
    dispatch({ type: 'set-context-menu', menu: null })
  }, [dispatch])

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
    [dispatch],
  )

  return {
    handleContextMenu,
    handleCloseContextMenu,
    handleBackgroundContextMenu,
  }
}
