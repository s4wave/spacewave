import { useMemo } from 'react'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useMappedResource } from '@aptre/bldr-sdk/hooks/useMappedResource.js'
import {
  getUnixFSDirEntryKind,
  getUnixFSFileInfoKind,
} from '@s4wave/sdk/unixfs/file-kind.js'
import { normalizeUnixFSLookupPath } from '@s4wave/sdk/unixfs/path.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import type { FileEntry } from '@s4wave/web/editors/file-browser/types.js'
import {
  useUnixFSRootHandle,
  useUnixFSHandle,
  useUnixFSHandleEntries,
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

  const entriesResource = useUnixFSHandleEntries(directoryHandle)
  const fileEntries = useMemo(() => {
    if (!entriesResource.value) return []
    return entriesResource.value.map((entry): FileEntry => {
      const entryKind = getUnixFSDirEntryKind(entry)
      return {
        id: entry.id,
        name: entry.name,
        isDir: entryKind === 'directory',
        isSymlink: entryKind === 'symlink',
      }
    })
  }, [entriesResource.value])

  return {
    rootHandle,
    pathHandle,
    statResource,
    entriesResource,
    fileEntries,
    isDir,
  }
}
