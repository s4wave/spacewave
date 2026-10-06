import { useCallback, useRef, type ChangeEvent } from 'react'

import type { FSHandle } from '@s4wave/sdk/unixfs/handle.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { FileEntry } from '@s4wave/web/editors/file-browser/types.js'
import { toast } from '@s4wave/web/ui/toaster.js'

import { downloadUnixFSSelection } from './download.js'
import type { UploadManager } from './useUploadManager.js'

interface UnixFSBrowserTransferOptions {
  httpPathPrefix: string
  sessionIndex: number
  spaceId: string | null
  unixfsId: string
  displayPath: string
  pathHandle: Resource<FSHandle>
  uploadManager: UploadManager | null
}

/**
 * useUnixFSBrowserTransfer returns the download handler and the hidden file
 * input handlers that feed the session upload manager.
 */
export function useUnixFSBrowserTransfer({
  httpPathPrefix,
  sessionIndex,
  spaceId,
  unixfsId,
  displayPath,
  pathHandle,
  uploadManager,
}: UnixFSBrowserTransferOptions) {
  const fileInputRef = useRef<HTMLInputElement>(null)

  const handleDownload = useCallback(
    (entries: FileEntry[]) => {
      if (!sessionIndex || !spaceId || entries.length === 0) return
      void downloadUnixFSSelection({
        httpPathPrefix,
        sessionIndex,
        sharedObjectId: spaceId,
        objectKey: unixfsId,
        currentPath: displayPath,
        entries,
      }).catch((err: unknown) => {
        console.error('failed to download unixfs selection', err)
        toast.error('Download failed', { description: String(err) })
      })
    },
    [displayPath, sessionIndex, spaceId, unixfsId, httpPathPrefix],
  )

  // handleUploadFiles opens the native file picker for uploading.
  const handleUploadFiles = useCallback(() => {
    fileInputRef.current?.click()
  }, [])

  const handleFileInputChange = useCallback(
    (e: ChangeEvent<HTMLInputElement>) => {
      const files = e.target.files
      if (!files || files.length === 0) return
      if (pathHandle.value) {
        uploadManager?.addFiles(pathHandle.value, Array.from(files))
      }
      e.target.value = ''
    },
    [uploadManager, pathHandle.value],
  )

  return {
    fileInputRef,
    handleDownload,
    handleUploadFiles,
    handleFileInputChange,
  }
}
