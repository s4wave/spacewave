import { useCallback, useMemo } from 'react'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useMappedResource } from '@aptre/bldr-sdk/hooks/useMappedResource.js'

import { getUnixFSFileInfoKind } from '@s4wave/sdk/unixfs/file-kind.js'
import { normalizeUnixFSLookupPath } from '@s4wave/sdk/unixfs/path.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import type {
  FileEntry,
  FileEntryDetails,
  GetFileEntryDetailsCallback,
} from '@s4wave/web/editors/file-browser/types.js'
import {
  convertDirEntriesToFileEntries,
  useUnixFSRootHandle,
  useUnixFSHandle,
  useUnixFSHandleReaddir,
  useUnixFSHandleStat,
} from '@s4wave/web/hooks/useUnixFSHandle.js'

function getTrackedHandlePath(handle: {
  getPath?: () => string
}): string | null {
  return typeof handle.getPath === 'function' ? handle.getPath() : null
}

interface UnixFSBrowserResourcesOptions {
  worldState: Resource<IWorldState>
  unixfsId: string
  displayPath: string
}

export function useUnixFSBrowserResources({
  worldState,
  unixfsId,
  displayPath,
}: UnixFSBrowserResourcesOptions) {
  const rootHandle = useUnixFSRootHandle(worldState, unixfsId)
  const pathHandle = useUnixFSHandle(rootHandle, displayPath)
  const statResource = useUnixFSHandleStat(pathHandle)

  const fileKind =
    statResource.loading || statResource.value === null
      ? null
      : (statResource.value.fileKind ??
        getUnixFSFileInfoKind(statResource.value.info))
  const isDir = fileKind === null ? null : fileKind === 'directory'
  const normalizedDisplayPath = normalizeUnixFSLookupPath(displayPath)

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

  const readdirResource = useUnixFSHandleReaddir(directoryHandle)
  const entriesResource = useMappedResource(readdirResource, (entries) =>
    entries ? convertDirEntriesToFileEntries(entries) : null,
  )
  const fileEntries = useMemo(
    () => entriesResource.value ?? [],
    [entriesResource.value],
  )

  // The readdir stream already carries per-entry size and mod time, so the
  // Date/Size columns resolve from it instead of a per-row stat round trip.
  const entryDetails = useMemo(() => {
    const details = new Map<string, FileEntryDetails>()
    for (const entry of readdirResource.value ?? []) {
      if (!entry.name) continue
      const modTimeSec = Number(entry.modTime ?? 0n)
      details.set(entry.name, {
        modTime: modTimeSec > 0 ? new Date(modTimeSec * 1000) : undefined,
        size: entry.size === undefined ? undefined : Number(entry.size),
      })
    }
    return details
  }, [readdirResource.value])
  const getEntryDetails = useCallback<GetFileEntryDetailsCallback>(
    (_index: number, entry: FileEntry) =>
      Promise.resolve(entryDetails.get(entry.id) ?? null),
    [entryDetails],
  )

  return {
    rootHandle,
    pathHandle,
    statResource,
    entriesResource,
    fileEntries,
    getEntryDetails,
    isDir,
  }
}
