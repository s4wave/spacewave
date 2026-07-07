import { useCallback } from 'react'

import type { FileEntry } from '@s4wave/web/editors/file-browser/types.js'
import { isDownloadURLDragSupported } from '@s4wave/web/dnd/download-url-drag.js'

import { buildUnixFSSelectionDownloadDragTarget } from './download.js'
import {
  buildUnixFSEntryAppDragEnvelope,
  buildUnixFSSelectionAppDragEnvelope,
} from './unixfs-app-drag.js'

interface UnixFSBrowserDragTargetsOptions {
  displayPath: string
  selectedEntries: FileEntry[]
  sessionIndex: number | null
  spaceId: string | null
  unixfsId: string
}

export function useUnixFSBrowserDragTargets({
  displayPath,
  selectedEntries,
  sessionIndex,
  spaceId,
  unixfsId,
}: UnixFSBrowserDragTargetsOptions) {
  const getDragEnvelope = useCallback(
    (entry: FileEntry, { selectedIds }: { selectedIds: string[] }) => {
      const dragEntries =
        selectedIds.includes(entry.id) && selectedEntries.length > 1
          ? selectedEntries
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
        selectedIds.includes(entry.id) && selectedEntries.length > 1
          ? selectedEntries
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

  return { getDragEnvelope, getDownloadDragTarget }
}
