import { useMemo } from 'react'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useMappedResource } from '@aptre/bldr-sdk/hooks/useMappedResource.js'
import { getUnixFSFileInfoKind } from '@s4wave/sdk/unixfs/file-kind.js'
import { normalizeUnixFSLookupPath } from '@s4wave/sdk/unixfs/path.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import type {
  FileEntryDetails,
  FileEntryDetailsMap,
} from '@s4wave/web/editors/file-browser/types.js'
import {
  useUnixFSRootHandle,
  useUnixFSHandle,
  useUnixFSHandleListing,
  useUnixFSHandleStat,
} from '@s4wave/web/hooks/useUnixFSHandle.js'

// getTrackedHandlePath returns the path a handle tracks, or null when it has none.
function getTrackedHandlePath(handle: {
  getPath?: () => string
}): string | null {
  return typeof handle.getPath === 'function' ? handle.getPath() : null
}

/** UnixFSBrowserResourcesOptions selects the UnixFS path the browser shows. */
interface UnixFSBrowserResourcesOptions {
  worldState: Resource<IWorldState>
  unixfsId: string
  displayPath: string
}

/**
 * useUnixFSBrowserResources follows the handle, stat and directory listing of
 * the browser's display path.
 */
export function useUnixFSBrowserResources({
  worldState,
  unixfsId,
  displayPath,
}: UnixFSBrowserResourcesOptions) {
  // Resolve and stat the display path.
  const rootHandle = useUnixFSRootHandle(worldState, unixfsId)
  const pathHandle = useUnixFSHandle(rootHandle, displayPath)
  const statResource = useUnixFSHandleStat(pathHandle)

  // Classify the path once its stat settles.
  const fileKind =
    statResource.loading || statResource.value === null
      ? null
      : (statResource.value.fileKind ??
        getUnixFSFileInfoKind(statResource.value.info))
  const isDir = fileKind === null ? null : fileKind === 'directory'
  const normalizedDisplayPath = normalizeUnixFSLookupPath(displayPath)

  // List only a directory handle that still tracks the display path.
  const directoryHandle = useMappedResource(
    pathHandle,
    (handle) => {
      if (isDir !== true) return null
      const handlePath = getTrackedHandlePath(handle)
      if (handlePath !== null && handlePath !== normalizedDisplayPath) {
        return null
      }
      return handle
    },
    [normalizedDisplayPath, isDir],
  )

  // Follow the listing's rows, which keep identity while unchanged.
  const listingResource = useUnixFSHandleListing(directoryHandle)
  const entriesResource = useMappedResource(
    listingResource,
    (listing) => listing?.rows ?? null,
    [],
  )
  const fileEntries = useMemo(
    () => entriesResource.value ?? [],
    [entriesResource.value],
  )

  // The Date and Size columns read each entry's size and mod time from the
  // listing, in one pass per listing value with no per-row stat round trip;
  // an entry the listing sent without them shows a dash.
  const entryDetails = useMemo<FileEntryDetailsMap>(() => {
    const details = new Map<string, FileEntryDetails>()
    for (const entry of listingResource.value?.entries ?? []) {
      if (!entry.name) continue
      const modTimeSec = Number(entry.modTime ?? 0n)
      details.set(entry.name, {
        modTime: modTimeSec > 0 ? new Date(modTimeSec * 1000) : undefined,
        size: entry.size === undefined ? undefined : Number(entry.size),
      })
    }
    return details
  }, [listingResource.value])

  return {
    rootHandle,
    pathHandle,
    statResource,
    entriesResource,
    fileEntries,
    entryDetails,
    isDir,
  }
}
