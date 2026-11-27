import {
  useResource,
  type Resource,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { useAccessTypedHandle } from './useAccessTypedHandle.js'
import { useMappedResource } from '@aptre/bldr-sdk/hooks/useMappedResource.js'
import { FSHandle } from '@s4wave/sdk/unixfs/handle.js'
import type { FileInfo, DirEntry } from '@s4wave/sdk/unixfs/handle.pb.js'
import {
  getUnixFSDirEntryKind,
  getUnixFSFileInfoKind,
  type UnixFSFileKind,
} from '@s4wave/sdk/unixfs/file-kind.js'
import { normalizeUnixFSLookupPath } from '@s4wave/sdk/unixfs/path.js'
import { UnixFSTypeID } from '@s4wave/sdk/unixfs/type.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import type { FileEntry } from '@s4wave/web/editors/file-browser/types.js'
import type { RegisterCleanup } from '@aptre/bldr-sdk/hooks/useResource.js'

export { UnixFSTypeID }

// useUnixFSRootHandle returns a root FSHandle for the given UnixFS object.
// This hook is the entry point for handle-based UnixFS operations.
export function useUnixFSRootHandle(
  worldState: Resource<IWorldState>,
  objectKey: string | null,
): Resource<FSHandle> {
  return useAccessTypedHandle(worldState, objectKey, FSHandle, UnixFSTypeID)
}

// useUnixFSHandle opens a handle at the given path relative to a root handle.
// The hook returns a new FSHandle for the target path.
export function useUnixFSHandle(
  rootHandle: Resource<FSHandle>,
  path: string,
): Resource<FSHandle> {
  return useResource(
    rootHandle,
    async (root, signal, cleanup) =>
      root ? resolveUnixFSHandle(root, path, signal, cleanup) : null,
    [path],
  )
}

// resolveUnixFSHandle resolves a display path without releasing the root
// handle resource.
export async function resolveUnixFSHandle(
  root: FSHandle,
  path: string,
  signal: AbortSignal,
  cleanup: RegisterCleanup,
): Promise<FSHandle> {
  const lookupPath = normalizeUnixFSLookupPath(path)
  if (!lookupPath) {
    return root
  }
  const { handle } = await root.lookupPath(lookupPath, signal)
  return cleanup(handle)
}

// toFileEntry converts a directory entry to a file browser row.
function toFileEntry(entry: DirEntry): FileEntry {
  const fileKind = getUnixFSDirEntryKind(entry)
  return {
    id: entry.name ?? '',
    name: entry.name ?? '',
    isDir: fileKind === 'directory',
    isSymlink: fileKind === 'symlink',
  }
}

// fileEntryEquals reports whether two directory rows show the same entry.
function fileEntryEquals(a: FileEntry, b: FileEntry): boolean {
  return (
    a.id === b.id &&
    a.name === b.name &&
    a.isDir === b.isDir &&
    a.isSymlink === b.isSymlink
  )
}

/** UnixFSListing is one directory listing with its file browser rows. */
export interface UnixFSListing {
  /** entries are the directory entries in listing order. */
  entries: DirEntry[]
  /** rows are the file browser rows for entries, in the same order. */
  rows: FileEntry[]
}

// watchListing follows a directory listing with its file browser rows. Each
// listing keeps the previous row object for an unchanged entry, so a change
// renders only the rows it changed.
async function* watchListing(
  handle: FSHandle,
  signal: AbortSignal,
): AsyncIterable<UnixFSListing> {
  let previous = new Map<string, FileEntry>()
  for await (const entries of handle.watchReaddir(signal)) {
    // Build the rows, reusing each unchanged one.
    const next = new Map<string, FileEntry>()
    const rows = entries.map((entry) => {
      // Keep the previous row while its entry is unchanged.
      const row = toFileEntry(entry)
      const prev = previous.get(row.id)
      const kept = prev && fileEntryEquals(prev, row) ? prev : row
      next.set(row.id, kept)
      return kept
    })
    previous = next
    yield { entries, rows }
  }
}

// emptyAsyncIterable is a no-op async iterable that yields nothing.
async function* emptyAsyncIterable<T>(): AsyncIterable<T> {}

/**
 * useUnixFSHandleListing follows the directory listing of a handle. The hook
 * returns null while the handle is null or disabled.
 */
export function useUnixFSHandleListing(
  handle: Resource<FSHandle | null>,
  options?: { enabled?: boolean },
): Resource<UnixFSListing | null> {
  // Watch the listing only for an enabled handle.
  const enabled = options?.enabled ?? true
  const handleId = handle.value?.id ?? null
  const gatedHandle = useMappedResource(
    handle,
    (h) => (h && enabled ? h : null),
    [enabled],
  )
  return useStreamingResource(
    gatedHandle,
    (h, signal) =>
      h ? watchListing(h, signal) : emptyAsyncIterable<UnixFSListing>(),
    [enabled, handleId],
  )
}

/**
 * useUnixFSHandleEntries follows the file browser rows of a handle's
 * directory. The hook returns null while the handle is null or disabled.
 */
export function useUnixFSHandleEntries(
  handle: Resource<FSHandle | null>,
  options?: { enabled?: boolean },
): Resource<FileEntry[] | null> {
  const listing = useUnixFSHandleListing(handle, options)
  return useMappedResource(listing, (value) => value?.rows ?? null, [])
}

// StatResult contains the result of a stat operation with derived mime type.
export interface StatResult {
  info: FileInfo
  fileKind?: UnixFSFileKind
  mimeType: string
}

// useUnixFSHandleStat returns file info for the handle's location.
export function useUnixFSHandleStat(
  handle: Resource<FSHandle>,
): Resource<StatResult> {
  return useResource(
    handle,
    async (h, signal) => {
      if (!h) return null
      return readUnixFSHandleStat(h, signal)
    },
    [],
  )
}

// readUnixFSHandleStat uses the root handle's cached metadata when the root
// resource carried it, and otherwise asks the handle.
export async function readUnixFSHandleStat(
  handle: FSHandle,
  signal: AbortSignal,
): Promise<StatResult> {
  // Read the info, then derive its kind and mime type.
  const cached = isRootFSHandle(handle) ? handle.getInfo() : null
  const info =
    cached && Object.keys(cached).length !== 0
      ? cached
      : await handle.getFileInfo(signal)
  const name = info.name ?? ''
  const fileKind = getUnixFSFileInfoKind(info)
  const mimeType =
    fileKind === 'directory' ? 'inode/directory' : getMimeType(name)
  return { info, fileKind, mimeType }
}

function isRootFSHandle(handle: FSHandle): boolean {
  const path = handle.getPath()
  return path === '' || path === '/' || path === '.'
}

// ReadFileResult contains the result of a readFile operation.
export interface ReadFileResult {
  data: Uint8Array
  eof: boolean
}

export const unixFSHandleTextContentMaxBytes = 512 * 1024

// useUnixFSHandleReadFile reads file content from a handle.
export function useUnixFSHandleReadFile(
  handle: Resource<FSHandle>,
  offset?: bigint,
  length?: bigint,
): Resource<ReadFileResult> {
  return useResource(
    handle,
    async (h, signal) => {
      if (!h) return null
      const result = await h.readAt(offset ?? 0n, length ?? 0n, signal)
      return {
        data: result.data,
        eof: result.eof,
      }
    },
    [offset, length],
  )
}

// useUnixFSHandleTextContent reads file content as text.
export function useUnixFSHandleTextContent(
  handle: Resource<FSHandle>,
): Resource<string> {
  const readResource = useUnixFSHandleReadFile(
    handle,
    0n,
    BigInt(unixFSHandleTextContentMaxBytes),
  )
  return useMappedResource(readResource, (r) =>
    new TextDecoder().decode(r.data),
  )
}

// MIME_TYPES maps file extensions to MIME types.
const MIME_TYPES: Record<string, string> = {
  // Text
  txt: 'text/plain',
  md: 'text/markdown',
  markdown: 'text/markdown',
  org: 'text/org',
  html: 'text/html',
  htm: 'text/html',
  css: 'text/css',
  csv: 'text/csv',
  xml: 'text/xml',

  // Code
  js: 'text/javascript',
  mjs: 'text/javascript',
  cjs: 'text/javascript',
  ts: 'text/typescript',
  mts: 'text/typescript',
  cts: 'text/typescript',
  tsx: 'text/typescript-jsx',
  jsx: 'text/javascript-jsx',
  json: 'application/json',
  jsonc: 'application/json',
  yaml: 'text/yaml',
  yml: 'text/yaml',
  toml: 'text/toml',
  go: 'text/x-go',
  py: 'text/x-python',
  rb: 'text/x-ruby',
  rs: 'text/x-rust',
  sh: 'text/x-shellscript',
  bash: 'text/x-shellscript',
  zsh: 'text/x-shellscript',
  fish: 'text/x-shellscript',
  ps1: 'text/x-powershell',
  c: 'text/x-c',
  h: 'text/x-c',
  cpp: 'text/x-c++',
  hpp: 'text/x-c++',
  cc: 'text/x-c++',
  java: 'text/x-java',
  kt: 'text/x-kotlin',
  swift: 'text/x-swift',
  sql: 'text/x-sql',
  proto: 'text/x-protobuf',

  // Images
  png: 'image/png',
  jpg: 'image/jpeg',
  jpeg: 'image/jpeg',
  gif: 'image/gif',
  webp: 'image/webp',
  svg: 'image/svg+xml',
  ico: 'image/x-icon',
  bmp: 'image/bmp',
  tiff: 'image/tiff',
  tif: 'image/tiff',
  avif: 'image/avif',

  // Audio
  mp3: 'audio/mpeg',
  wav: 'audio/wav',
  ogg: 'audio/ogg',
  oga: 'audio/ogg',
  opus: 'audio/ogg',
  flac: 'audio/flac',
  aac: 'audio/aac',
  m4a: 'audio/mp4',
  m4b: 'audio/mp4',
  m4r: 'audio/mp4',
  weba: 'audio/webm',

  // Video
  mp4: 'video/mp4',
  webm: 'video/webm',
  ogv: 'video/ogg',
  mov: 'video/quicktime',
  avi: 'video/x-msvideo',
  mkv: 'video/x-matroska',

  // Documents
  pdf: 'application/pdf',
  doc: 'application/msword',
  docx: 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
  xls: 'application/vnd.ms-excel',
  xlsx: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
  ppt: 'application/vnd.ms-powerpoint',
  pptx: 'application/vnd.openxmlformats-officedocument.presentationml.presentation',
  odt: 'application/vnd.oasis.opendocument.text',
  ods: 'application/vnd.oasis.opendocument.spreadsheet',
  odp: 'application/vnd.oasis.opendocument.presentation',

  // Archives
  zip: 'application/zip',
  tar: 'application/x-tar',
  gz: 'application/gzip',
  bz2: 'application/x-bzip2',
  xz: 'application/x-xz',
  '7z': 'application/x-7z-compressed',
  rar: 'application/vnd.rar',

  // Fonts
  woff: 'font/woff',
  woff2: 'font/woff2',
  ttf: 'font/ttf',
  otf: 'font/otf',
  eot: 'application/vnd.ms-fontobject',

  // Other
  wasm: 'application/wasm',
}

// getMimeType returns the MIME type for a file path based on its extension.
export function getMimeType(path: string): string {
  const lastDot = path.lastIndexOf('.')
  if (lastDot === -1 || lastDot === path.length - 1) {
    return 'application/octet-stream'
  }
  const ext = path.slice(lastDot + 1).toLowerCase()
  return MIME_TYPES[ext] ?? 'application/octet-stream'
}

// isTextMimeType returns true if the mime type represents text content.
export function isTextMimeType(mimeType: string): boolean {
  return (
    mimeType.startsWith('text/') ||
    mimeType === 'application/json' ||
    mimeType === 'application/xml' ||
    mimeType.endsWith('+xml') ||
    mimeType.endsWith('+json')
  )
}

// isImageMimeType returns true if the mime type represents an image.
export function isImageMimeType(mimeType: string): boolean {
  return mimeType.startsWith('image/')
}

// isAudioMimeType returns true if the mime type represents audio.
export function isAudioMimeType(mimeType: string): boolean {
  return mimeType.startsWith('audio/')
}

// isVideoMimeType returns true if the mime type represents video.
export function isVideoMimeType(mimeType: string): boolean {
  return mimeType.startsWith('video/')
}
