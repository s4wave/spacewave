import { useEffect, useRef } from 'react'

import type { FSHandle } from '@s4wave/sdk/unixfs/handle.js'
import type { FileEntry } from '@s4wave/web/editors/file-browser/types.js'
import { markAppStartupBoundary } from '@s4wave/app/quickstart/startup-boundary.js'
import {
  beginDriveSpaceOpenRegion,
  endDriveSpaceOpenRegion,
  endDriveSpaceOpenTrace,
} from '@s4wave/app/trace/drive-space-open-trace.js'

interface UnixFSStartupBoundariesOptions {
  displayPath: string
  displayEntries: FileEntry[]
  isDir: boolean | null
  isLoading: boolean
  rootHandle: FSHandle | null | undefined
  sharedObjectId?: string | null
}

export function useUnixFSStartupBoundaries({
  displayPath,
  displayEntries,
  isDir,
  isLoading,
  rootHandle,
  sharedObjectId,
}: UnixFSStartupBoundariesOptions) {
  const visibleEntryNames = displayEntries.map((entry) => entry.name).join('\n')
  const boundaryMarks = useRef({
    browserMounted: false,
    firstFileRow: false,
    seededFile: false,
  })

  useEffect(() => {
    if (isDir !== true || isLoading || !rootHandle) return
    if (!boundaryMarks.current.browserMounted) {
      boundaryMarks.current.browserMounted = true
      markAppStartupBoundary('unixfs.browser-mounted', {
        path: displayPath,
      })
    }
    if (!boundaryMarks.current.firstFileRow && displayEntries.length > 0) {
      boundaryMarks.current.firstFileRow = true
      markAppStartupBoundary('unixfs.first-file-row', {
        path: displayPath,
        entryCount: displayEntries.length,
        firstEntryName: displayEntries[0]?.name ?? null,
      })
      if (sharedObjectId) {
        beginDriveSpaceOpenRegion(sharedObjectId, 'first-listing-render', {
          path: displayPath,
          entryCount: displayEntries.length,
          firstEntryName: displayEntries[0]?.name ?? null,
        })
        endDriveSpaceOpenRegion(sharedObjectId, 'first-listing-render', {
          path: displayPath,
          entryCount: displayEntries.length,
          firstEntryName: displayEntries[0]?.name ?? null,
        })
        endDriveSpaceOpenTrace(sharedObjectId, {
          path: displayPath,
          entryCount: displayEntries.length,
          firstEntryName: displayEntries[0]?.name ?? null,
        })
      }
    }
    if (!boundaryMarks.current.seededFile && visibleEntryNames) {
      boundaryMarks.current.seededFile = true
      markAppStartupBoundary('unixfs.seeded-file-visible', {
        path: displayPath,
        fileName: displayEntries[0]?.name ?? null,
      })
    }
  }, [
    displayEntries,
    displayPath,
    isDir,
    isLoading,
    rootHandle,
    sharedObjectId,
    visibleEntryNames,
  ])
}
