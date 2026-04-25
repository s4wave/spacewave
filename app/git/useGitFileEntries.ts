import { useMemo } from 'react'

import type { FSHandle } from '@s4wave/sdk/unixfs/handle.js'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import {
  useUnixFSHandle,
  useUnixFSHandleEntries,
  useUnixFSHandleStat,
  useUnixFSHandleTextContent,
} from '@s4wave/web/hooks/useUnixFSHandle.js'
import type { FileEntry } from '@s4wave/web/editors/file-browser/types.js'

// useGitFileEntries encapsulates the shared file-entry mapping, stat,
// directory transition tracking, and README content for git viewers.
export function useGitFileEntries(
  rootHandleResource: Resource<FSHandle>,
  displayPath: string,
  readmePath: string | undefined,
  entriesEnabled?: boolean,
) {
  const pathHandle = useUnixFSHandle(rootHandleResource, displayPath)
  const statResource = useUnixFSHandleStat(pathHandle)

  const isDir =
    statResource.loading || statResource.value === null ?
      null
    : (statResource.value.info.isDir ?? false)

  const entriesResource = useUnixFSHandleEntries(pathHandle, {
    enabled: isDir === true && (entriesEnabled ?? true),
  })

  const fileEntries = useMemo(() => {
    if (!entriesResource.value) return []
    return entriesResource.value.map(
      (entry): FileEntry => ({
        id: entry.id,
        name: entry.name,
        isDir: entry.isDir,
        isSymlink: entry.isSymlink,
      }),
    )
  }, [entriesResource.value])
  return {
    pathHandle,
    statResource,
    isDir,
    entriesResource,
    fileEntries,
    readmeContent: useReadmeContent(rootHandleResource, readmePath),
  }
}

// useReadmeContent fetches README text content from the tree.
function useReadmeContent(
  rootHandleResource: Resource<FSHandle>,
  readmePath: string | undefined,
) {
  const readmeHandle = useUnixFSHandle(rootHandleResource, readmePath ?? '')
  return useUnixFSHandleTextContent(readmeHandle)
}
