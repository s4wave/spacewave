import { pluginPathPrefix } from '@s4wave/app/urls.js'
import { ExportBatchRequest } from '@s4wave/core/space/http/export/config.pb.js'
import { downloadURL } from '@s4wave/web/download.js'
import type { DownloadDragTarget } from '@s4wave/web/dnd/download-url-drag.js'
import type { FileEntry } from '@s4wave/web/editors/file-browser/types.js'

interface UnixFSSelectionDownloadOpts {
  sessionIndex: number
  sharedObjectId: string
  objectKey: string
  currentPath: string
  entries: FileEntry[]
}

type UnixFSSelectionDownloadDragTargetOpts = UnixFSSelectionDownloadOpts

function normalizeUnixFSPath(path: string): string {
  if (!path || path === '/') {
    return ''
  }
  return path.split('/').filter(Boolean).join('/')
}

function encodePath(path: string): string {
  return path.split('/').filter(Boolean).map(encodeURIComponent).join('/')
}

function buildProjectedObjectPath(
  sessionIndex: number,
  sharedObjectId: string,
  objectKey: string,
  path: string,
): string {
  const projectedPath =
    `u/${sessionIndex}/so/${encodeURIComponent(sharedObjectId)}/-/` +
    encodePath(objectKey)
  const normalizedPath = normalizeUnixFSPath(path)
  if (!normalizedPath) {
    return projectedPath
  }
  return `${projectedPath}/-/${encodePath(normalizedPath)}`
}

function buildBatchFilename(entries: FileEntry[]): string {
  if (entries.length === 1) {
    return `${entries[0].name}.zip`
  }
  return 'selection.zip'
}

function normalizeBatchEntries(entries: FileEntry[]): FileEntry[] {
  const seen = new Set<string>()
  return [...entries]
    .filter((entry) => {
      if (seen.has(entry.name)) {
        return false
      }
      seen.add(entry.name)
      return true
    })
    .sort((a, b) => a.name.localeCompare(b.name))
}

function encodeBase64Url(data: Uint8Array): string {
  let binary = ''
  for (let i = 0; i < data.length; i += 0x8000) {
    binary += String.fromCharCode(...data.subarray(i, i + 0x8000))
  }
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

export function buildUnixFSFileDownloadURL(
  sessionIndex: number,
  sharedObjectId: string,
  objectKey: string,
  path: string,
): string {
  const projectedPath = buildProjectedObjectPath(
    sessionIndex,
    sharedObjectId,
    objectKey,
    path,
  )
  return `${pluginPathPrefix}/fs/${projectedPath}`
}

export function buildUnixFSFileInlineURL(
  sessionIndex: number,
  sharedObjectId: string,
  objectKey: string,
  path: string,
): string {
  return (
    buildUnixFSFileDownloadURL(sessionIndex, sharedObjectId, objectKey, path) +
    '?inline=1'
  )
}

export function buildUnixFSExportURL(
  sessionIndex: number,
  sharedObjectId: string,
  objectKey: string,
  path: string,
): string {
  const projectedPath = buildProjectedObjectPath(
    sessionIndex,
    sharedObjectId,
    objectKey,
    path,
  )
  return `${pluginPathPrefix}/export/${projectedPath}`
}

export function buildUnixFSBatchExportURL(
  sessionIndex: number,
  sharedObjectId: string,
  objectKey: string,
  basePath: string,
  entries: FileEntry[],
): { url: string; filename: string } {
  const normalizedEntries = normalizeBatchEntries(entries)
  const req = ExportBatchRequest.toBinary({
    paths: normalizedEntries.map((entry) => entry.name),
  })
  const baseProjectedPath = buildProjectedObjectPath(
    sessionIndex,
    sharedObjectId,
    objectKey,
    basePath,
  )
  const filename = buildBatchFilename(normalizedEntries)
  const encodedFilename = encodeURIComponent(filename)
  const encodedReq = encodeBase64Url(req)
  return {
    url:
      `${pluginPathPrefix}/export-batch/${baseProjectedPath}/` +
      `${encodedReq}/${encodedFilename}`,
    filename,
  }
}

export function buildUnixFSSelectionDownloadDragTarget({
  sessionIndex,
  sharedObjectId,
  objectKey,
  currentPath,
  entries,
}: UnixFSSelectionDownloadDragTargetOpts): DownloadDragTarget | null {
  const normalizedEntries = normalizeBatchEntries(entries)
  if (normalizedEntries.length === 0) {
    return null
  }

  if (normalizedEntries.length === 1 && !normalizedEntries[0].isDir) {
    const filePath = [
      normalizeUnixFSPath(currentPath),
      normalizedEntries[0].name,
    ]
      .filter(Boolean)
      .join('/')
    return {
      mimeType: 'application/octet-stream',
      filename: normalizedEntries[0].name,
      url: buildUnixFSFileDownloadURL(
        sessionIndex,
        sharedObjectId,
        objectKey,
        filePath,
      ),
    }
  }

  if (normalizedEntries.length === 1) {
    const dirPath = [
      normalizeUnixFSPath(currentPath),
      normalizedEntries[0].name,
    ]
      .filter(Boolean)
      .join('/')
    return {
      mimeType: 'application/zip',
      filename: `${normalizedEntries[0].name}.zip`,
      url: buildUnixFSExportURL(
        sessionIndex,
        sharedObjectId,
        objectKey,
        dirPath,
      ),
    }
  }

  const batchDownload = buildUnixFSBatchExportURL(
    sessionIndex,
    sharedObjectId,
    objectKey,
    currentPath,
    normalizedEntries,
  )
  return {
    mimeType: 'application/zip',
    filename: batchDownload.filename,
    url: batchDownload.url,
  }
}

export function downloadUnixFSSelection({
  sessionIndex,
  sharedObjectId,
  objectKey,
  currentPath,
  entries,
}: UnixFSSelectionDownloadOpts): Promise<void> {
  const normalizedEntries = normalizeBatchEntries(entries)
  if (normalizedEntries.length === 0) {
    return Promise.resolve()
  }

  if (normalizedEntries.length === 1 && !normalizedEntries[0].isDir) {
    const filePath = [
      normalizeUnixFSPath(currentPath),
      normalizedEntries[0].name,
    ]
      .filter(Boolean)
      .join('/')
    downloadURL(
      buildUnixFSFileDownloadURL(
        sessionIndex,
        sharedObjectId,
        objectKey,
        filePath,
      ),
      normalizedEntries[0].name,
    )
    return Promise.resolve()
  }

  if (normalizedEntries.length === 1) {
    const dirPath = [
      normalizeUnixFSPath(currentPath),
      normalizedEntries[0].name,
    ]
      .filter(Boolean)
      .join('/')
    downloadURL(
      buildUnixFSExportURL(sessionIndex, sharedObjectId, objectKey, dirPath),
      `${normalizedEntries[0].name}.zip`,
    )
    return Promise.resolve()
  }

  const batchDownload = buildUnixFSBatchExportURL(
    sessionIndex,
    sharedObjectId,
    objectKey,
    currentPath,
    normalizedEntries,
  )
  downloadURL(batchDownload.url, batchDownload.filename)
  return Promise.resolve()
}
