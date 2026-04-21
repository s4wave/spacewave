import type { FSCursorNodeType } from './fs-cursor.js'

// FSWriter coordinates writes to a filesystem tree.
// Methods should not return until the updated state has been synced to the FS.
// Updated state can be synced by setting the new root hash.
// Writers should be constructed one per FS object. Do not reuse.
export interface FSWriter {
  // filesystemError is called when an internal error is encountered.
  filesystemError(err: Error): void

  // mknod creates one or more inodes at the given paths.
  // An error may be thrown if one or more parent directories do not exist.
  // ErrExist should be thrown if one of the path entries exists with a different type.
  // Mkdir is implemented with mknod.
  // Paths must be relative.
  mknod(
    paths: string[][],
    nodeType: FSCursorNodeType,
    permissions: number,
    ts: Date,
    signal?: AbortSignal,
  ): Promise<void>

  // symlink creates a symbolic link from one location to another.
  // An error may be thrown if one or more parent directories do not exist.
  // Supports absolute paths with targetIsAbsolute flag.
  symlink(
    path: string[],
    target: string[],
    targetIsAbsolute: boolean,
    ts: Date,
    signal?: AbortSignal,
  ): Promise<void>

  // setPermissions sets the permissions bits of the nodes at the paths.
  // The file mode portion of the value is ignored.
  // Paths must be relative.
  setPermissions(
    paths: string[][],
    fm: number,
    ts: Date,
    signal?: AbortSignal,
  ): Promise<void>

  // setModTimestamp sets the modification timestamp of the nodes at the paths.
  // mtime is the modification timestamp to set.
  // Paths must be relative.
  setModTimestamp(
    paths: string[][],
    mtime: Date,
    signal?: AbortSignal,
  ): Promise<void>

  // writeAt writes data to an offset in an inode (usually a file).
  // Must not retain data after returning.
  // Paths must be relative.
  writeAt(
    path: string[],
    offset: bigint,
    data: Uint8Array,
    ts: Date,
    signal?: AbortSignal,
  ): Promise<void>

  // truncate shrinks or extends a file to the specified size.
  // The extended part will be a sparse range (hole) reading as zeros.
  // Paths must be relative.
  // The file must already exist.
  truncate(
    path: string[],
    nsize: bigint,
    ts: Date,
    signal?: AbortSignal,
  ): Promise<void>

  // copy recursively copies a source path to a destination, overwriting destination.
  // Performs the copy in a single operation.
  // Paths must be relative.
  copy(
    srcPath: string[],
    tgtPath: string[],
    ts: Date,
    signal?: AbortSignal,
  ): Promise<void>

  // rename recursively moves a source path to a destination, overwriting destination.
  // Performs the move in a single operation.
  // Paths must be relative.
  rename(
    srcPath: string[],
    tgtPath: string[],
    ts: Date,
    signal?: AbortSignal,
  ): Promise<void>

  // remove removes one or more paths from the tree.
  // Parents must be directories.
  // Non-existent paths may not throw.
  // Paths must be relative.
  remove(paths: string[][], ts: Date, signal?: AbortSignal): Promise<void>

  // mknodWithContent creates a file and writes its content atomically.
  // The file appears fully formed in a single operation.
  // path is the full relative path to the new file.
  // dataLen is the total file size in bytes.
  // data provides the file content.
  // Path must be relative.
  mknodWithContent(
    path: string[],
    nodeType: FSCursorNodeType,
    dataLen: bigint,
    data: Uint8Array,
    permissions: number,
    ts: Date,
    signal?: AbortSignal,
  ): Promise<void>
}
