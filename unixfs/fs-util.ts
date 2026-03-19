import type { FSCursorDirent } from './fs-cursor.js'
import type { FSHandle } from './fs-handle.js'
import {
  ErrAbsolutePath,
  ErrEmptyPath,
  isUnixFSError,
} from './errors/errors.js'
import { UnixFSErrorType } from './errors/errors.pb.js'
import { splitPath } from './path.js'

// MAX_READ_FILE_SIZE is the maximum size for readFile operations (4GB).
export const MAX_READ_FILE_SIZE = 4n * 1024n * 1024n * 1024n

// newReadFileSizeTooLargeError returns an error for when a file is too large.
export function newReadFileSizeTooLargeError(size: bigint): Error {
  return new Error(`file size too large for readFile: ${size} bytes`)
}

// lookupPathOrThrow is a helper that calls lookupPath and throws on error,
// releasing the partial handle. Returns the handle on success.
async function lookupPathOrThrow(
  signal: AbortSignal,
  h: FSHandle,
  path: string,
): Promise<FSHandle> {
  const result = await h.lookupPath(signal, path)
  if (result.error) {
    result.handle.release()
    throw result.error
  }
  return result.handle
}

// lookupPathPtsOrThrow is a helper that calls lookupPathPts and throws on error,
// releasing the partial handle. Returns the handle on success.
async function lookupPathPtsOrThrow(
  signal: AbortSignal,
  h: FSHandle,
  parts: string[],
): Promise<FSHandle> {
  const result = await h.lookupPathPts(signal, parts)
  if (result.error) {
    result.handle.release()
    throw result.error
  }
  return result.handle
}

// readFile reads the named file and returns the contents.
export async function readFile(
  signal: AbortSignal,
  h: FSHandle,
): Promise<Uint8Array> {
  let size = 0n
  try {
    const info = await h.getFileInfo(signal)
    size = info.size
  } catch {
    // ignore
  }
  if (size === 0n) {
    return new Uint8Array(0)
  }

  // If a file claims a small size, read at least 512 bytes.
  if (size < 512n) {
    size = 512n
  } else if (size > MAX_READ_FILE_SIZE) {
    throw newReadFileSizeTooLargeError(size)
  } else {
    size++ // one byte for final read at EOF
  }

  const chunks: Uint8Array[] = []
  let offset = 0n
  let totalRead = 0
  for (;;) {
    const remaining = Number(size) - totalRead
    if (remaining <= 0) {
      size = size + 512n
    }

    const chunkSize = Math.min(Number(size) - totalRead, 65536)
    if (chunkSize <= 0) break
    const buf = new Uint8Array(chunkSize)
    let n: bigint
    try {
      n = await h.readAt(signal, offset, buf)
    } catch (err) {
      if (isUnixFSError(err, UnixFSErrorType.EOF)) {
        break
      }
      throw err
    }
    if (n === 0n) {
      break
    }
    chunks.push(buf.subarray(0, Number(n)))
    totalRead += Number(n)
    offset += n
    if (n < BigInt(chunkSize)) {
      break
    }
  }

  // Concatenate chunks.
  const result = new Uint8Array(totalRead)
  let pos = 0
  for (const chunk of chunks) {
    result.set(chunk, pos)
    pos += chunk.length
  }
  return result
}

// DEFAULT_CHUNK_SIZE is the default optimal write size (256KB).
const DEFAULT_CHUNK_SIZE = 2048n * 125n

// writeFile writes data to the filesystem handle, which must be a file.
export async function writeFile(
  signal: AbortSignal,
  h: FSHandle,
  data: Uint8Array,
  ts: Date,
): Promise<void> {
  let optimalWriteSize = await h.getOptimalWriteSize(signal)
  if (optimalWriteSize === 0n) {
    optimalWriteSize = DEFAULT_CHUNK_SIZE
  }

  // Truncate the file.
  await h.truncate(signal, 0n, ts)

  // Write data in chunks.
  const dataLen = BigInt(data.length)
  for (let offset = 0n; offset < dataLen; offset += optimalWriteSize) {
    const end = offset + optimalWriteSize < dataLen
      ? offset + optimalWriteSize
      : dataLen
    const chunk = data.subarray(Number(offset), Number(end))
    await h.writeAt(signal, offset, chunk, ts)
  }
}

// statWithPath calls stat on a path in a FSHandle.
// This will traverse symbolic links.
export async function statWithPath(
  signal: AbortSignal,
  h: FSHandle,
  name: string,
): Promise<{
  name: string
  size: bigint
  mode: number
  modTime: Date
  isDir: boolean
}> {
  name = cleanPath(name)
  if (name === '.' || name === '/' || name === '') {
    return h.getFileInfo(signal)
  }

  const fh = await lookupPathOrThrow(signal, h, name)
  try {
    return await fh.getFileInfo(signal)
  } finally {
    fh.release()
  }
}

// lstatWithPath calls lstat on a path in a FSHandle.
// Unlike statWithPath, the final path component is not followed if it is a symlink.
export async function lstatWithPath(
  signal: AbortSignal,
  h: FSHandle,
  name: string,
): Promise<{
  name: string
  size: bigint
  mode: number
  modTime: Date
  isDir: boolean
}> {
  name = cleanPath(name)
  if (name === '.' || name === '/' || name === '') {
    return h.getFileInfo(signal)
  }

  const lastSlash = name.lastIndexOf('/')
  const dir = lastSlash >= 0 ? name.substring(0, lastSlash) : ''
  const base = lastSlash >= 0 ? name.substring(lastSlash + 1) : name

  // Look up the parent directory (follows symlinks for intermediate components).
  let parentHandle: FSHandle
  if (dir === '' || dir === '.' || dir === '/') {
    parentHandle = await h.clone(signal)
  } else {
    parentHandle = await lookupPathOrThrow(signal, h, dir)
  }

  try {
    // Single lookup on the final component does NOT follow symlinks.
    const childHandle = await parentHandle.lookup(signal, base)
    try {
      return await childHandle.getFileInfo(signal)
    } finally {
      childHandle.release()
    }
  } finally {
    parentHandle.release()
  }
}

// renameWithPaths renames using two paths within a FSHandle.
export async function renameWithPaths(
  signal: AbortSignal,
  h: FSHandle,
  oldPath: string,
  newPath: string,
  ts: Date,
): Promise<void> {
  const { parts: oldPathPts, isAbsolute: oldAbs } = splitPath(oldPath)
  if (oldAbs) {
    throw ErrAbsolutePath
  }

  const { parts: newPathPts, isAbsolute: newAbs } = splitPath(newPath)
  if (newAbs) {
    throw ErrAbsolutePath
  }

  if (arraysEqual(oldPathPts, newPathPts)) {
    return
  }

  if (!newPath || !oldPath) {
    throw ErrEmptyPath
  }

  const oldHandle = await lookupPathPtsOrThrow(signal, h, oldPathPts)
  try {
    const parentPathPts = newPathPts.slice(0, -1)
    const destName = newPathPts[newPathPts.length - 1]
    const nextParent = await lookupPathPtsOrThrow(signal, h, parentPathPts)
    try {
      await oldHandle.rename(signal, nextParent, destName, ts)
    } finally {
      nextParent.release()
    }
  } finally {
    oldHandle.release()
  }
}

// removeAllWithPath calls remove on the given path.
export async function removeAllWithPath(
  signal: AbortSignal,
  h: FSHandle,
  filepath: string,
  ts: Date,
): Promise<void> {
  filepath = cleanPath(filepath)
  const lastSlash = filepath.lastIndexOf('/')
  const filedir = lastSlash >= 0 ? filepath.substring(0, lastSlash) : ''
  const filename =
    lastSlash >= 0 ? filepath.substring(lastSlash + 1) : filepath

  const dirHandle = await lookupPathOrThrow(signal, h, filedir || '.')
  try {
    await dirHandle.remove(signal, [filename], ts)
  } finally {
    dirHandle.release()
  }
}

// chmodWithPath calls chmod on the given path.
export async function chmodWithPath(
  signal: AbortSignal,
  h: FSHandle,
  filepath: string,
  mode: number,
  ts: Date,
): Promise<void> {
  const ch = await lookupPathOrThrow(signal, h, filepath)
  try {
    const info = await ch.getFileInfo(signal)
    const oldPerms = info.mode & 0o777
    const setPerms = mode & 0o777
    if (oldPerms !== setPerms) {
      await ch.setPermissions(signal, setPerms, ts)
    }
  } finally {
    ch.release()
  }
}

// setModTimestampWithPath changes the modification timestamp on the given path.
export async function setModTimestampWithPath(
  signal: AbortSignal,
  h: FSHandle,
  filepath: string,
  mtime: Date,
): Promise<void> {
  const ch = await lookupPathOrThrow(signal, h, filepath)
  try {
    await ch.setModTimestamp(signal, mtime)
  } finally {
    ch.release()
  }
}

// readdirAllToEntries reads all directory entries and returns file info.
export async function readdirAllToEntries(
  signal: AbortSignal,
  h: FSHandle,
  skip: bigint,
  limit: number,
): Promise<
  Array<{
    name: string
    size: bigint
    mode: number
    modTime: Date
    isDir: boolean
  }>
> {
  const children: string[] = []
  try {
    await h.readdirAll(signal, skip, (ent: FSCursorDirent) => {
      children.push(ent.getName())
      if (limit > 0 && children.length >= limit) {
        throw new EofSignal()
      }
    })
  } catch (e) {
    if (!(e instanceof EofSignal)) {
      throw e
    }
  }

  const out: Array<{
    name: string
    size: bigint
    mode: number
    modTime: Date
    isDir: boolean
  }> = []
  for (const childName of children) {
    let ch: FSHandle
    try {
      ch = await h.lookup(signal, childName)
    } catch (e) {
      if (isUnixFSError(e, UnixFSErrorType.NOT_EXIST)) {
        continue
      }
      throw e
    }
    try {
      const fi = await ch.getFileInfo(signal)
      out.push(fi)
    } finally {
      ch.release()
    }
  }
  return out
}

// EofSignal is used to signal EOF in readdirAll callbacks.
class EofSignal extends Error {
  constructor() {
    super('EOF')
  }
}

// arraysEqual checks if two string arrays are equal.
function arraysEqual(a: string[], b: string[]): boolean {
  if (a.length !== b.length) return false
  for (let i = 0; i < a.length; i++) {
    if (a[i] !== b[i]) return false
  }
  return true
}

// cleanPath is a basic path.Clean equivalent for local use.
function cleanPath(p: string): string {
  if (p === '' || p === '.') return '.'
  const parts: string[] = []
  const isAbs = p[0] === '/'
  const segs = p.split('/')
  for (const seg of segs) {
    if (seg === '' || seg === '.') continue
    if (seg === '..') {
      if (parts.length > 0 && parts[parts.length - 1] !== '..') {
        parts.pop()
      } else if (!isAbs) {
        parts.push('..')
      }
    } else {
      parts.push(seg)
    }
  }
  let result = parts.join('/')
  if (isAbs) {
    result = '/' + result
  }
  return result || (isAbs ? '/' : '.')
}
