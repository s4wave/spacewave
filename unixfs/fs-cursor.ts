// ReadAtBuffer is a buffer that readAtTo can write into.
// Accepts Uint8Array and any TypedArray with set() and length.
export interface ReadAtBuffer {
  readonly length: number
  set(source: ArrayLike<number>, offset?: number): void
}

// FSCursorNodeType indicates the type of node.
export interface FSCursorNodeType {
  // getIsDirectory returns if the node is a directory.
  getIsDirectory(): boolean
  // getIsFile returns if the node is a regular file.
  getIsFile(): boolean
  // getIsSymlink returns if the node is a symlink.
  getIsSymlink(): boolean
}

// FSCursorDirent is a directory entry.
export interface FSCursorDirent extends FSCursorNodeType {
  // getName returns the name of the directory entry.
  getName(): string
}

// FSCursor is a location in a filesystem tree.
// All operations should throw ErrReleased if the cursor is released.
// The cursor can release itself if a complete cursor re-build is necessary.
export interface FSCursor {
  // checkReleased checks if the fs cursor is currently released.
  checkReleased(): boolean

  // getProxyCursor returns a FSCursor to replace this one, if necessary.
  // This is used to resolve a symbolic link, mount, etc.
  // Return null if no redirection necessary (in most cases).
  // This will be called before any of the other calls.
  // Releasing a child cursor does not release the parent, and vice-versa.
  // Throw ErrReleased if this FSCursor was released.
  getProxyCursor(signal?: AbortSignal): Promise<FSCursor | null>

  // addChangeCb adds a change callback to detect when the cursor has changed.
  // This will be called only if getProxyCursor returns null.
  // cb must not block, and should be called when cursor changes / is released.
  // cb will be called immediately (same call tree) if already released.
  // The cursor may hold a lock internally while calling cb, do not call release inside the callback.
  addChangeCb(cb: FSCursorChangeCb): void

  // getCursorOps returns the FSCursorOps for the FSCursor.
  // Called after addChangeCb and only if getProxyCursor returns null.
  // Returning null will be corrected to throwing ErrNotExist.
  // Throw ErrReleased to indicate this FSCursor was released.
  getCursorOps(signal?: AbortSignal): Promise<FSCursorOps | null>

  // release releases the filesystem cursor.
  release(): void

  // Symbol.dispose enables using-based cleanup.
  [Symbol.dispose](): void
}

// FSCursorOps are operations called against a non-proxy FSCursor.
// Operations throw ErrReleased if the FSCursorOps was released.
// After release, the system will call getCursorOps again.
// If the node type changes for any reason, the ops object should be released.
// All ops must be concurrency safe and may be called concurrently.
export interface FSCursorOps extends FSCursorNodeType {
  // checkReleased checks if the fs cursor ops object is currently released.
  // This indicates if the FSCursorOps is released, not the parent FSCursor.
  checkReleased(): boolean

  // getName returns the name of the inode (if applicable).
  getName(): string

  // getPermissions returns the permissions bits of the file mode.
  // Only the permissions bits are set in the returned value.
  getPermissions(signal?: AbortSignal): Promise<number>

  // setPermissions updates the permissions bits of the file mode.
  // Only the permissions bits are used from the value.
  setPermissions(
    permissions: number,
    ts: Date,
    signal?: AbortSignal,
  ): Promise<void>

  // getSize returns the size of the inode (in bytes).
  // Usually applicable only if this is a FILE.
  getSize(signal?: AbortSignal): Promise<bigint>

  // getModTimestamp returns the modification timestamp.
  getModTimestamp(signal?: AbortSignal): Promise<Date>

  // setModTimestamp updates the modification timestamp of the node.
  setModTimestamp(mtime: Date, signal?: AbortSignal): Promise<void>

  // readAt reads from a location in a File node.
  // Allocates a buffer of the given size, reads into it, returns {data, n}.
  readAt(
    offset: bigint,
    size: bigint,
    signal?: AbortSignal,
  ): Promise<{ data: Uint8Array; n: bigint }>

  // readAtTo reads from a location in a File node into an existing buffer.
  // Returns the number of bytes read.
  readAtTo(
    offset: bigint,
    data: ReadAtBuffer,
    signal?: AbortSignal,
  ): Promise<bigint>

  // getOptimalWriteSize returns the best write size to use for the write call.
  // May return zero to indicate no known optimal size.
  getOptimalWriteSize(signal?: AbortSignal): Promise<bigint>

  // writeAt writes to a location within a File node synchronously.
  // Accepts any size for the data parameter.
  // Call getOptimalWriteSize to determine the best size of data to use.
  // The change should be fully written to the file before returning.
  writeAt(
    offset: bigint,
    data: Uint8Array,
    ts: Date,
    signal?: AbortSignal,
  ): Promise<void>

  // truncate shrinks or extends a file to the specified size.
  // The extended part will be a sparse range (hole) reading as zeros.
  truncate(nsize: bigint, ts: Date, signal?: AbortSignal): Promise<void>

  // lookup looks up a child entry in a directory.
  // Throws ErrNotExist if the child entry was not found.
  // Throws ErrReleased if the reference has been released.
  // Creates a new FSCursor at the new location.
  lookup(name: string, signal?: AbortSignal): Promise<FSCursor>

  // readdirAll reads all directory entries.
  // If skip is set, skips the first N directory entries.
  readdirAll(
    skip: bigint,
    cb: (ent: FSCursorDirent) => void,
    signal?: AbortSignal,
  ): Promise<void>

  // mknod creates child entries in a directory.
  // inode must be a directory.
  // if permissions is zero, default permissions will be set.
  // if checkExist, checks if name exists, throws ErrExist if so.
  mknod(
    checkExist: boolean,
    names: string[],
    nodeType: FSCursorNodeType,
    permissions: number,
    ts: Date,
    signal?: AbortSignal,
  ): Promise<void>

  // symlink creates a symbolic link from a location to a path.
  symlink(
    checkExist: boolean,
    name: string,
    target: string[],
    targetIsAbsolute: boolean,
    ts: Date,
    signal?: AbortSignal,
  ): Promise<void>

  // readlink reads a symbolic link contents.
  // If name is empty, reads the link at the cursor position.
  // Throws ErrNotSymlink if not a symbolic link.
  readlink(
    name: string,
    signal?: AbortSignal,
  ): Promise<{ path: string[]; isAbsolute: boolean }>

  // copyTo performs an optimized copy of a dirent inode to another inode.
  // If the src is a directory, this should be a recursive copy.
  // If the destination already exists, this should clobber the destination.
  // Returns false if optimized copy to the target is not implemented.
  copyTo(
    tgtDir: FSCursorOps,
    tgtName: string,
    ts: Date,
    signal?: AbortSignal,
  ): Promise<boolean>

  // copyFrom performs an optimized copy from another inode.
  // If the src is a directory, this should be a recursive copy.
  // If the destination already exists, this should clobber the destination.
  // Returns false if optimized copy from the target is not implemented.
  copyFrom(
    name: string,
    srcCursorOps: FSCursorOps,
    ts: Date,
    signal?: AbortSignal,
  ): Promise<boolean>

  // moveTo performs an atomic and optimized move to another inode.
  // If the src is a directory, this should be a recursive move.
  // If the destination already exists, this should clobber the destination.
  // Returns false if atomic move to the target is not implemented.
  moveTo(
    tgtCursorOps: FSCursorOps,
    tgtName: string,
    ts: Date,
    signal?: AbortSignal,
  ): Promise<boolean>

  // moveFrom performs an atomic and optimized move from another inode.
  // If the src is a directory, this should be a recursive move.
  // If the destination already exists, this should clobber the destination.
  // Returns false if atomic move from the target is not implemented.
  moveFrom(
    name: string,
    srcCursorOps: FSCursorOps,
    ts: Date,
    signal?: AbortSignal,
  ): Promise<boolean>

  // remove deletes entries from a directory.
  // Throws ErrReadOnly if read-only.
  // Does not throw if they did not exist.
  remove(names: string[], ts: Date, signal?: AbortSignal): Promise<void>

  // mknodWithContent creates a file entry and writes content atomically.
  // The inode must be a directory.
  // The new file appears fully formed with all content written.
  // dataLen is the total file size in bytes.
  // data provides the file content to write.
  mknodWithContent(
    name: string,
    nodeType: FSCursorNodeType,
    dataLen: bigint,
    data: Uint8Array,
    permissions: number,
    ts: Date,
    signal?: AbortSignal,
  ): Promise<void>
}

// FSCursorChangeCb is a callback function for a cursor change.
// Handles changes to the cursor.
// Return false to remove the callback handler.
export type FSCursorChangeCb = (ch: FSCursorChange) => boolean

// FSCursorChange is information about a change.
// If the offset and size is zero, handlers should completely flush inode cache.
export interface FSCursorChange {
  cursor: FSCursor
  released: boolean
  offset: bigint
  size: bigint
}

// cloneFSCursorChange copies a FSCursorChange.
export function cloneFSCursorChange(c: FSCursorChange): FSCursorChange {
  return {
    cursor: c.cursor,
    released: c.released,
    offset: c.offset,
    size: c.size,
  }
}

// FSCursorChangeCbSlice manages a list of change callbacks.
export class FSCursorChangeCbSlice {
  private cbs: FSCursorChangeCb[] = []

  // add appends a callback to the slice.
  add(cb: FSCursorChangeCb): void {
    this.cbs.push(cb)
  }

  // callCbs calls all callbacks and removes those returning false.
  callCbs(change: FSCursorChange): void {
    for (let i = 0; i < this.cbs.length; i++) {
      if (!this.cbs[i](change)) {
        this.cbs[i] = this.cbs[this.cbs.length - 1]
        this.cbs.length--
        i--
      }
    }
  }
}
