import { useCallback, type KeyboardEvent } from 'react'

import type { FileEntry } from '@s4wave/web/editors/file-browser/types.js'

interface UnixFSDeleteKeyHandlerOptions {
  selectedEntries: FileEntry[]
  onDelete: (entries: FileEntry[]) => void
}

export function useUnixFSDeleteKeyHandler({
  selectedEntries,
  onDelete,
}: UnixFSDeleteKeyHandlerOptions) {
  return useCallback(
    (event: KeyboardEvent<HTMLDivElement>) => {
      if (event.key !== 'Delete' && event.key !== 'Backspace') return
      if (selectedEntries.length === 0) return
      event.preventDefault()
      onDelete(selectedEntries)
    },
    [onDelete, selectedEntries],
  )
}
