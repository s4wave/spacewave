import {
  AsyncRWMutex,
  type RWMutexLock,
} from '@go/github.com/aperturerobotics/util/csync/rwmutex.js'

import type { FSCursor, FSCursorOps } from './fs-cursor.js'
import {
  ErrReleased,
  ErrInodeUnresolvable,
  ErrNotExist,
  ErrNotFile,
  ErrNotDirectory,
  ErrEmptyPath,
  ErrMoveToSelf,
  ErrCrossFsRename,
  isUnixFSError,
} from './errors/errors.js'
import { UnixFSErrorType } from './errors/errors.pb.js'

// INODE_TRIES is the maximum number of retry attempts for accessInode.
const INODE_TRIES = 100

// AccessInodeCb is a callback for accessInode.
export type AccessInodeCb = (
  cursor: FSCursor,
  ops: FSCursorOps,
) => Promise<void>

// abortReason coerces a signal reason into an Error.
function abortReason(signal: AbortSignal): Error {
  const reason: unknown = signal.reason
  if (reason instanceof Error) {
    return reason
  }
  return new Error(typeof reason === 'string' ? reason : 'aborted')
}

// abortPromise returns a promise that rejects when the signal aborts.
function abortPromise(signal: AbortSignal): Promise<never> {
  if (signal.aborted) {
    return Promise.reject(abortReason(signal))
  }
  return new Promise<never>((_, reject) => {
    signal.addEventListener('abort', () => reject(abortReason(signal)), {
      once: true,
    })
  })
}

// FsInode is internal tracking of a location in a FS.
// The inode will be released if:
//   - there is any error fetching/refreshing the parent cursors
//   - the underlying FSCursor is released.
//   - there are 0 references to the node and 0 child nodes
export class FsInode {
  private isReleased = false
  // parent is the inode which created this inode (immutable).
  readonly parent: FsInode | null
  // name is the name associated with the inode (immutable).
  readonly name: string

  // relErr is the error set when releasing.
  private relErr: Error | null = null
  // refs is the list of inode ref handles.
  refs: FSHandle[] = []
  // children contains any child inodes, sorted by name.
  private children: FsInode[] = []
  // rmtx is the read/write mutex for the inode fields and children.
  private rmtx = new AsyncRWMutex()
  // fsWait is set if a routine is currently resolving fsCursors or fsOps.
  fsWait: { promise: Promise<void>; resolve: () => void } | null = null
  // fsCursors contains the current fs cursor instances.
  // Multiple can be set if one cursor proxies to another.
  // The last element in the list is the cursor used for fsOps.
  fsCursors: FSCursor[] = []
  // fsOps contains the current fs ops instance.
  fsOps: FSCursorOps | null = null
  // relCbs is an array of callbacks called on release.
  private relCbs: Array<() => void> = []

  constructor(parent: FsInode | null, name: string, cursors: FSCursor[]) {
    this.parent = parent
    this.name = name
    this.fsCursors = cursors
  }

  // checkReleasedFlag checks if released without locking anything.
  checkReleasedFlag(): boolean {
    return this.isReleased
  }

  // checkReleasedWithErr checks if the node was released.
  // If released, returns the release error or ErrReleased.
  checkReleasedWithErr(): Error | null {
    if (!this.isReleased) {
      return null
    }
    return this.relErr ?? ErrReleased
  }

  // addReferenceLocked adds a new FSHandle pointing to this location.
  addReferenceLocked(checkReleased: boolean): FSHandle {
    if (checkReleased) {
      const relErr = this.checkReleasedWithErr()
      if (relErr) {
        throw relErr
      }
    }
    const ref = new FSHandle(this)
    this.refs.push(ref)
    return ref
  }

  // addReference adds a new reference, acquiring the write lock.
  async addReference(signal?: AbortSignal): Promise<FSHandle> {
    const lock = await this.rmtx.lock(true, signal)
    try {
      return this.addReferenceLocked(true)
    } finally {
      lock.release()
    }
  }

  // mergeReferencesLocked merges a list of refs into this inode,
  // skipping any released refs.
  private mergeReferencesLocked(refs: FSHandle[]): void {
    if (refs.length === 0) {
      return
    }

    // If this inode is released, release all refs instead.
    if (this.isReleased) {
      for (const ref of refs) {
        ref._setReleased()
        ref._fireRelCbs()
      }
      return
    }

    for (const ref of refs) {
      if (!ref.checkReleased()) {
        ref._setInode(this)
        this.refs.push(ref)
      }
    }
  }

  // mergeWithNodeLocked merges the given inode into this, releasing the given
  // inode and its children. Merges references and children recursively.
  mergeWithNodeLocked(node: FsInode, err: Error | null): void {
    const toRelease: FsInode[] = []
    const nodStk: FsInode[] = [this]
    const srcStk: FsInode[] = [node]

    while (nodStk.length > 0) {
      const next = nodStk.pop()!
      const src = srcStk.pop()!

      next.mergeReferencesLocked(src.refs)
      src.refs = []

      for (const srcChild of src.children) {
        if (srcChild.isReleased || srcChild.releaseIfNecessaryLocked()) {
          continue
        }

        const { child: childLoc, index: childLocIdx } =
          next.findChildInodeInternal(srcChild.name, true)
        if (childLoc) {
          nodStk.push(childLoc)
          srcStk.push(srcChild)
        } else {
          const newChild = new FsInode(next, srcChild.name, [])
          next.children.splice(childLocIdx, 0, newChild)
          nodStk.push(newChild)
          srcStk.push(srcChild)
        }
      }

      src.children = []
      toRelease.push(src)
    }

    // Release in bottom-up order.
    for (let i = toRelease.length - 1; i >= 0; i--) {
      toRelease[i].releaseLocked(err)
    }
  }

  // clearCursorsWithChildrenLocked clears all fscursor and ops on the inode
  // and children. Does not release the inodes, just releases the cursors.
  clearCursorsWithChildrenLocked(): void {
    const toRelease: FsInode[] = []
    const nodStk: FsInode[] = [this]

    while (nodStk.length > 0) {
      const src = nodStk.pop()!

      for (const srcChild of src.children) {
        if (srcChild.isReleased || srcChild.releaseIfNecessaryLocked()) {
          continue
        }
        nodStk.push(srcChild)
      }

      toRelease.push(src)
    }

    // Release cursors in bottom-up order.
    for (let i = toRelease.length - 1; i >= 0; i--) {
      const next = toRelease[i]
      next.fsOps = null
      for (let j = next.fsCursors.length - 1; j >= 0; j--) {
        next.fsCursors[j].release()
      }
      next.fsCursors = []
    }
  }

  // accessInode resolves ops and calls cb with the resolved cursor and ops.
  // If cb throws ErrReleased and ops was actually released, retries.
  // cb may be null to just resolve without calling.
  async accessInode(
    signal: AbortSignal,
    cb: AccessInodeCb | null,
  ): Promise<void> {
    let lastErr: Error | null = null

    const handleErr = (err: unknown): Error | null => {
      if (!isUnixFSError(err, UnixFSErrorType.RELEASED)) {
        return err as Error
      }

      if (signal.aborted) {
        return new Error('context canceled')
      }
      if (this.isReleased && this.relErr) {
        return this.relErr
      }
      if (this.parent && this.parent.isReleased && this.parent.relErr) {
        return this.parent.relErr
      }

      if (!isUnixFSError(err, UnixFSErrorType.RELEASED)) {
        return err
      }

      // Return null to indicate retry.
      lastErr = err
      return null
    }

    for (let i = 0; i < INODE_TRIES; i++) {
      signal.throwIfAborted()

      const relErr = this.checkReleasedWithErr()
      if (relErr) {
        throw relErr
      }

      let resolved: { cursor: FSCursor; ops: FSCursorOps } | null
      try {
        resolved = await this.resolveOps(signal)
      } catch (err) {
        const herr = handleErr(err)
        if (herr) {
          throw herr
        }
        continue
      }

      if (!resolved) {
        // try-again case (resolution in-flight or just completed)
        continue
      }

      const { cursor, ops } = resolved
      if (cb === null) {
        return
      }

      try {
        await cb(cursor, ops)
        return
      } catch (err) {
        if (
          isUnixFSError(err, UnixFSErrorType.RELEASED) &&
          !ops.checkReleased()
        ) {
          throw err
        }
        const herr = handleErr(err)
        if (herr) {
          throw herr
        }
        continue
      }
    }

    if (
      lastErr !== null &&
      !isUnixFSError(lastErr, UnixFSErrorType.RELEASED)
    ) {
      // lastErr is mutated inside handleErr's closure, so eslint cannot
      // narrow its type past null even after the !== null guard above.
      // eslint-disable-next-line @typescript-eslint/only-throw-error
      throw lastErr
    }
    throw ErrInodeUnresolvable
  }

  // resolveOps resolves the inode operations.
  // Low-level op used by accessInode, use accessInode instead.
  private async resolveOps(
    signal: AbortSignal,
  ): Promise<{ cursor: FSCursor; ops: FSCursorOps } | null> {
    const lock = await this.rmtx.lock(true, signal)

    // Fast path: cached and valid.
    if (this.fsOps && !this.fsOps.checkReleased()) {
      const cursor = this.fsCursors[this.fsCursors.length - 1]
      lock.release()
      return { cursor, ops: this.fsOps }
    }
    if (this.fsOps) {
      this.fsOps = null
    }

    // If resolution in-flight, wait for it.
    if (this.fsWait) {
      const waitPromise = this.fsWait.promise
      lock.release()
      await Promise.race([waitPromise, abortPromise(signal)])
      return null
    }

    // Start resolution.
    let resolve!: () => void
    const promise = new Promise<void>((r) => {
      resolve = r
    })
    this.fsWait = { promise, resolve }
    // resolveOpsRoutineLocked expects lock to be held; it releases it.
    this.resolveOpsRoutineLocked(signal, lock)

    // Return context.Canceled if signal was canceled.
    if (signal.aborted) {
      throw new Error('context canceled')
    }
    // Otherwise return try-again signal.
    return null
  }

  // resolveOpsRoutineLocked resolves the fsOps.
  // rmtx must be held by caller. Returns with rmtx unlocked.
  private resolveOpsRoutineLocked(
    signal: AbortSignal,
    lock: RWMutexLock,
  ): void {
    const iparent = this.parent
    const iname = this.name
    const fsWait = this.fsWait!

    // Check if fsOps already set.
    if (this.fsOps) {
      if (this.fsOps.checkReleased()) {
        this.fsOps = null
      } else {
        // Already resolved.
        fsWait.resolve()
        this.fsWait = null
        lock.release()
        return
      }
    }

    // Remove any released cursors from last to first.
    const cursorStackInner = this.fsCursors.filter((c) => !c.checkReleased())
    this.fsCursors = cursorStackInner
    const cursorStack = [...cursorStackInner]

    // Unlock rmtx.
    lock.release()

    // Async portion.
    void this.resolveOpsRoutineAsync(
      signal,
      cursorStack,
      iparent,
      iname,
    ).finally(() => {
      if (this.fsWait === fsWait) {
        fsWait.resolve()
        this.fsWait = null
      } else {
        fsWait.resolve()
      }
    })
  }

  // resolveOpsRoutineAsync is the async resolution logic extracted for clarity.
  private async resolveOpsRoutineAsync(
    signal: AbortSignal,
    cursorStack: FSCursor[],
    iparent: FsInode | null,
    iname: string,
  ): Promise<void> {
    let fsOps: FSCursorOps | null = null
    let err: Error | null = null

    outer: for (;;) {
      // If there are 0 cursors remaining, use lookup from parent to get one.
      if (cursorStack.length === 0) {
        if (iparent) {
          try {
            await iparent.accessInode(
              signal,
              async (_cursor: FSCursor, ops: FSCursorOps) => {
                const iCursor = await ops.lookup(iname, signal)
                cursorStack.push(iCursor)
              },
            )
          } catch (e) {
            err = e as Error
          }
        } else {
          err = ErrInodeUnresolvable
        }
        if (err) {
          // Error fetching parent cursors. Lock and release this + children.
          try {
            const lock = await this.rmtx.lock(true, signal)
            this.releaseWithChildrenLocked(err)
            this.fsWait = null
            lock.release()
          } catch {
            // context canceled
          }
          return
        }
      }

      // Resolve the proxies as needed.
      while (cursorStack.length > 0) {
        if (this.isReleased) {
          return
        }

        const next = cursorStack[cursorStack.length - 1]
        let pcursor: FSCursor | null
        try {
          pcursor = await next.getProxyCursor(signal)
        } catch (e) {
          if (isUnixFSError(e, UnixFSErrorType.RELEASED)) {
            cursorStack.pop()
            continue
          }
          err = e as Error
          break
        }
        if (pcursor) {
          cursorStack.push(pcursor)
          continue
        }

        // No more proxies: get the ops.
        try {
          fsOps = await next.getCursorOps(signal)
        } catch (e) {
          if (isUnixFSError(e, UnixFSErrorType.RELEASED)) {
            cursorStack.pop()
            continue
          }
          err = e as Error
          break
        }
        if (!fsOps) {
          err = ErrNotExist
          break
        }

        break
      }

      if (err) {
        break
      }

      if (fsOps && !fsOps.checkReleased()) {
        break outer
      }

      fsOps = null
    }

    // Acquire lock to store results.
    let lock: RWMutexLock
    try {
      lock = await this.rmtx.lock(true, signal)
    } catch {
      // Context canceled. Don't leak cursors.
      for (let i = cursorStack.length - 1; i >= 0; i--) {
        cursorStack[i].release()
      }
      return
    }
    this.fsCursors = cursorStack
    this.fsOps = fsOps
    this.fsWait = null
    if (err) {
      this.releaseWithChildrenLocked(err)
    }
    lock.release()
  }

  // lookup attempts to lookup a directory entry returning a new FSHandle.
  async lookup(signal: AbortSignal, name: string): Promise<FSHandle> {
    const lock = await this.rmtx.lock(true, signal)

    let nref: FSHandle | null = null
    let childReady = false
    let childInode: FsInode | null

    const { child, index: insertIdx } = this.findChildInodeInternal(name, false)
    childInode = child

    let wasReleased = false
    if (childInode && childInode.isReleased) {
      wasReleased = true
      childInode = null
    }

    if (childInode) {
      // Lock the child inode.
      let childLock: RWMutexLock
      try {
        childLock = await childInode.rmtx.lock(true, signal)
      } catch (e) {
        lock.release()
        throw e
      }

      // Add reference to child inode, checking if released.
      try {
        nref = childInode.addReferenceLocked(true)
        // Check if the child is already resolved.
        childReady =
          childInode.fsOps !== null && !childInode.fsOps.checkReleased()
      } catch {
        // addReferenceLocked threw: child was released.
        wasReleased = true
        childInode = null
      }

      childLock.release()
    }

    // Create new child inode if necessary.
    if (wasReleased || !childInode) {
      childInode = new FsInode(this, name, [])
      nref = childInode.addReferenceLocked(false)
      if (wasReleased) {
        // Replace at the old released index.
        this.children[insertIdx] = childInode
      } else {
        // Insert at the sorted position.
        this.children.splice(insertIdx, 0, childInode)
      }
    }

    lock.release()

    // If the child was already resolved, return now.
    if (childReady) {
      return nref!
    }

    // Wait until the child inode ops are resolved.
    try {
      await childInode.accessInode(signal, null)
    } catch (e) {
      nref!.release()
      throw e
    }

    return nref!
  }

  // findChildInodeInternal looks for an existing child by name using binary search.
  // Returns the child and its index (or the insertion index if not found).
  private findChildInodeInternal(
    name: string,
    checkReleased: boolean,
  ): { child: FsInode | null; index: number } {
    let lo = 0
    let hi = this.children.length
    while (lo < hi) {
      const mid = (lo + hi) >>> 1
      if (this.children[mid].name < name) {
        lo = mid + 1
      } else {
        hi = mid
      }
    }
    if (lo >= this.children.length || this.children[lo].name !== name) {
      return { child: null, index: lo }
    }
    const child = this.children[lo]
    if (checkReleased && child.isReleased) {
      this.children.splice(lo, 1)
      return { child: null, index: lo }
    }
    return { child, index: lo }
  }

  // findChildInode looks for an existing child by name.
  // Public accessor used by FSHandle.rename.
  findChildInode(
    name: string,
    checkReleased: boolean,
  ): { child: FsInode | null; index: number } {
    return this.findChildInodeInternal(name, checkReleased)
  }

  // removeChildInodeAtIdx removes a child from the children array at an index.
  removeChildInodeAtIdx(idx: number): void {
    this.children.splice(idx, 1)
  }

  // removeRefLocked removes a ref from the refs list.
  // If the refs list is now empty, calls releaseIfNecessaryLocked.
  removeRefLocked(h: FSHandle): void {
    if (this.refs.length === 0) {
      return
    }
    const idx = this.refs.indexOf(h)
    if (idx !== -1) {
      // Swap-remove for efficiency.
      this.refs[idx] = this.refs[this.refs.length - 1]
      this.refs.length--
    }
    if (this.refs.length === 0) {
      this.releaseIfNecessaryLocked()
    }
  }

  // releaseIfNecessaryLocked releases this inode if it has no refs and no children.
  // Returns true if the node was released.
  releaseIfNecessaryLocked(): boolean {
    if (this.isReleased) {
      return true
    }
    if (this.refs.length > 0 || this.children.length > 0) {
      return false
    }
    this.releaseLocked(null)
    return true
  }

  // releaseLocked marks the fsInode as released.
  // Caller must hold the lock.
  // Caller must ensure all children are released first.
  releaseLocked(err: Error | null): void {
    if (this.isReleased) {
      return
    }
    this.isReleased = true
    if (err) {
      this.relErr = err
    }
    this.refs = []
    this.children = []
    this.fsOps = null
    this.fsWait = null

    // Release all fs cursors.
    const cursors = this.fsCursors
    this.fsCursors = []
    for (let i = cursors.length - 1; i >= 0; i--) {
      cursors[i].release()
    }

    // Call release callbacks asynchronously.
    const cbs = this.relCbs
    this.relCbs = []
    for (const cb of cbs) {
      queueMicrotask(cb)
    }
  }

  // releaseWithChildrenLocked releases this inode and all child inodes.
  releaseWithChildrenLocked(err: Error | null): void {
    const toRelease: FsInode[] = []
    const nodStk: FsInode[] = [this]

    while (nodStk.length > 0) {
      const next = nodStk.pop()!
      for (const child of next.children) {
        toRelease.push(child)
        nodStk.push(child)
      }
    }

    // Release in bottom-up order.
    for (let i = toRelease.length - 1; i >= 0; i--) {
      toRelease[i].releaseLocked(err)
    }

    // Finally release this node.
    this.releaseLocked(err)
  }

  // addReleaseCb adds a callback that will be called when the inode is released.
  addReleaseCb(cb: () => void): void {
    this.relCbs.push(cb)
  }

  // getRmtx returns the rwmutex for external locking (used by FSHandle.rename).
  getRmtx(): AsyncRWMutex {
    return this.rmtx
  }

  // getChildren returns the children array for external access (used by FSHandle.rename).
  getChildren(): FsInode[] {
    return this.children
  }

  // insertChildAt inserts a child inode at a specific index.
  insertChildAt(idx: number, child: FsInode): void {
    this.children.splice(idx, 0, child)
  }
}

// FSHandle is imported circularly. We use a late-binding approach.
// The actual FSHandle class is defined in fs-handle.ts and calls into FsInode.
// We forward-declare the shape here to avoid import cycles.
// This file re-exports from fs-handle.ts, which imports FsInode.

// FSHandle is an open handle to a location in a FSTree.
// The handle may be released if the location ceases to exist.
export class FSHandle {
  private _released = false
  private _inode: FsInode
  private _relCbs: Array<() => void> = []

  constructor(inode: FsInode) {
    this._inode = inode
  }

  // i returns the underlying inode.
  _getInode(): FsInode {
    return this._inode
  }

  // _setInode sets the underlying inode (used by mergeReferencesLocked).
  _setInode(inode: FsInode): void {
    this._inode = inode
  }

  // _setReleased marks the handle as released (used by mergeReferencesLocked).
  _setReleased(): void {
    this._released = true
  }

  // _fireRelCbs fires all release callbacks.
  _fireRelCbs(): void {
    const cbs = this._relCbs
    this._relCbs = []
    for (const cb of cbs) {
      cb()
    }
  }

  // checkReleased checks if released without locking anything.
  checkReleased(): boolean {
    return this._released
  }

  // getName returns the name of the inode.
  getName(): string {
    return this._inode.name
  }

  // addReleaseCallback adds a callback that will be called when the FSHandle
  // is released. May be called immediately if already released.
  addReleaseCallback(rcb: () => void): void {
    if (!rcb) {
      return
    }

    let called = false
    const cb = () => {
      if (called) return
      called = true
      rcb()
    }

    this._relCbs.push(cb)

    if (this._released) {
      cb()
      return
    }

    const inode = this._inode
    if (inode.checkReleasedFlag()) {
      cb()
      return
    }

    inode.addReleaseCb(cb)
    // If inode was released or changed, fire immediately.
    if (inode.checkReleasedFlag() || this._inode !== inode) {
      cb()
    }
  }

  // accessOps accesses the FSCursor and FSCursorOps handles at the inode.
  async accessOps(
    signal: AbortSignal,
    cb: (cursor: FSCursor, ops: FSCursorOps) => Promise<void>,
  ): Promise<void> {
    return this._inode.accessInode(signal, cb)
  }

  // getOps resolves and returns the FSCursor and FSCursorOps once.
  async getOps(
    signal: AbortSignal,
  ): Promise<{ cursor: FSCursor; ops: FSCursorOps }> {
    let cursor: FSCursor | null = null
    let ops: FSCursorOps | null = null
    await this.accessOps(signal, (c, o) => {
      if (o.checkReleased() || c.checkReleased()) {
        return Promise.reject(ErrReleased)
      }
      cursor = c
      ops = o
      return Promise.resolve()
    })
    return { cursor: cursor!, ops: ops! }
  }

  // getFileInfo constructs a FileInfo for the inode at this handle.
  async getFileInfo(signal: AbortSignal): Promise<{
    name: string
    size: bigint
    mode: number
    modTime: Date
    isDir: boolean
  }> {
    let result!: {
      name: string
      size: bigint
      mode: number
      modTime: Date
      isDir: boolean
    }
    await this._inode.accessInode(signal, async (_cursor, ops) => {
      const permissions = await ops.getPermissions(signal)
      const mode = nodeTypeToMode(ops, permissions)
      const size = await ops.getSize(signal)
      const modTime = await ops.getModTimestamp(signal)
      result = {
        name: ops.getName(),
        size,
        mode,
        modTime,
        isDir: ops.getIsDirectory(),
      }
    })
    return result
  }

  // getNodeType returns the FSCursor node type.
  async getNodeType(signal: AbortSignal): Promise<{
    getIsDirectory(): boolean
    getIsFile(): boolean
    getIsSymlink(): boolean
  }> {
    let nt!: {
      getIsDirectory(): boolean
      getIsFile(): boolean
      getIsSymlink(): boolean
    }
    await this._inode.accessInode(signal, (_cursor, ops) => {
      nt = {
        getIsDirectory: () => ops.getIsDirectory(),
        getIsFile: () => ops.getIsFile(),
        getIsSymlink: () => ops.getIsSymlink(),
      }
      return Promise.resolve()
    })
    return nt
  }

  // getSize returns the size of the inode (in bytes).
  async getSize(signal: AbortSignal): Promise<bigint> {
    let size = 0n
    await this._inode.accessInode(signal, async (_cursor, ops) => {
      size = await ops.getSize(signal)
    })
    return size
  }

  // getOptimalWriteSize returns the optimal write size for the node.
  async getOptimalWriteSize(signal: AbortSignal): Promise<bigint> {
    let size = 0n
    await this._inode.accessInode(signal, async (_cursor, ops) => {
      size = await ops.getOptimalWriteSize(signal)
    })
    return size
  }

  // getModTimestamp returns the modification time.
  async getModTimestamp(signal: AbortSignal): Promise<Date> {
    let mtime!: Date
    await this._inode.accessInode(signal, async (_cursor, ops) => {
      mtime = await ops.getModTimestamp(signal)
    })
    return mtime
  }

  // getPermissions returns the permissions bits of the file mode.
  async getPermissions(signal: AbortSignal): Promise<number> {
    let perms = 0
    await this._inode.accessInode(signal, async (_cursor, ops) => {
      perms = await ops.getPermissions(signal)
    })
    return perms
  }

  // setPermissions updates the permissions bits of the file mode.
  async setPermissions(
    signal: AbortSignal,
    permissions: number,
    ts: Date,
  ): Promise<void> {
    await this._inode.accessInode(signal, async (_cursor, ops) => {
      await ops.setPermissions(permissions, ts, signal)
    })
  }

  // setModTimestamp updates the modification timestamp of the node.
  async setModTimestamp(signal: AbortSignal, mtime: Date): Promise<void> {
    await this._inode.accessInode(signal, async (_cursor, ops) => {
      await ops.setModTimestamp(mtime, signal)
    })
  }

  // readAt reads from a location in a File node, allocating a buffer.
  async readAt(
    signal: AbortSignal,
    offset: bigint,
    size: bigint,
  ): Promise<{ data: Uint8Array; n: bigint }> {
    let result: { data: Uint8Array; n: bigint } = {
      data: new Uint8Array(0),
      n: 0n,
    }
    await this._inode.accessInode(signal, async (_cursor, ops) => {
      if (!ops.getIsFile()) {
        throw ErrNotFile
      }
      result = await ops.readAt(offset, size, signal)
    })
    return result
  }

  // readAtTo reads from a location in a File node into an existing buffer.
  async readAtTo(
    signal: AbortSignal,
    offset: bigint,
    data: Uint8Array,
  ): Promise<bigint> {
    let read = 0n
    await this._inode.accessInode(signal, async (_cursor, ops) => {
      if (!ops.getIsFile()) {
        throw ErrNotFile
      }
      read = await ops.readAtTo(offset, data, signal)
    })
    return read
  }

  // writeAt writes to an offset in a file node synchronously.
  async writeAt(
    signal: AbortSignal,
    offset: bigint,
    data: Uint8Array,
    ts: Date,
  ): Promise<void> {
    await this._inode.accessInode(signal, async (_cursor, ops) => {
      if (!ops.getIsFile()) {
        throw ErrNotFile
      }
      await ops.writeAt(offset, data, ts, signal)
    })
  }

  // truncate shrinks or extends a file to the specified size.
  async truncate(signal: AbortSignal, nsize: bigint, ts: Date): Promise<void> {
    await this._inode.accessInode(signal, async (_cursor, ops) => {
      if (!ops.getIsFile()) {
        throw ErrNotFile
      }
      await ops.truncate(nsize, ts, signal)
    })
  }

  // readdirAll reads all directory entries.
  async readdirAll(
    signal: AbortSignal,
    skip: bigint,
    cb: (ent: FSCursorDirent) => void,
  ): Promise<void> {
    await this._inode.accessInode(signal, async (_cursor, ops) => {
      await ops.readdirAll(skip, cb, signal)
    })
  }

  // lookup looks up a child entry in a directory.
  async lookup(signal: AbortSignal, name: string): Promise<FSHandle> {
    return this._inode.lookup(signal, name)
  }

  // lookupPath looks up a path and returns the handle and traversed parts.
  // Check if handle is not null and release it even if an error is returned.
  async lookupPath(
    signal: AbortSignal,
    filePath: string,
  ): Promise<{ handle: FSHandle; traversed: string[]; error?: Error }> {
    const pathParts = cleanSplitValidateRelativePath(filePath)
    return this.lookupPathPts(signal, pathParts)
  }

  // lookupPathPts looks up path parts and returns the handle and traversed parts.
  // Matches Go semantics: returns a handle even on error (for partial traversal).
  // The caller must release the returned handle even if error is set.
  async lookupPathPts(
    signal: AbortSignal,
    pathParts: string[],
  ): Promise<{ handle: FSHandle; traversed: string[]; error?: Error }> {
    let outHandle = await this.clone(signal)
    for (let i = 0; i < pathParts.length; i++) {
      const part = pathParts[i]
      if (part === '.' || part === '') {
        continue
      }

      try {
        const nh = await outHandle.lookup(signal, part)
        outHandle.release()
        outHandle = nh
      } catch (err) {
        return {
          handle: outHandle,
          traversed: pathParts.slice(0, i),
          error: err as Error,
        }
      }
    }
    return { handle: outHandle, traversed: pathParts }
  }

  // mknod creates child entries in a directory.
  async mknod(
    signal: AbortSignal,
    checkExist: boolean,
    names: string[],
    nodeType: FSCursorNodeType,
    permissions: number,
    ts: Date,
  ): Promise<void> {
    await this._inode.accessInode(signal, async (_cursor, ops) => {
      await ops.mknod(checkExist, names, nodeType, permissions, ts, signal)
    })
  }

  // mkdirAll creates a directory named path, along with any necessary parents.
  async mkdirAll(
    signal: AbortSignal,
    dirPath: string[],
    perm: number,
    ts: Date,
  ): Promise<void> {
    let dirHandle = await this.clone(signal)
    for (const pname of dirPath) {
      if (pname === '.') {
        continue
      }
      let dh: FSHandle | null
      try {
        dh = await dirHandle.lookup(signal, pname)
      } catch (e) {
        if (isUnixFSError(e, UnixFSErrorType.NOT_EXIST)) {
          await dirHandle.mknod(
            signal,
            false,
            [pname],
            newFSCursorNodeType_Dir(),
            perm,
            ts,
          )
          dh = await dirHandle.lookup(signal, pname)
        } else {
          dirHandle.release()
          throw e
        }
      }
      dirHandle.release()
      dirHandle = dh!
      // Check it is a dir.
      const nt = await dirHandle.getNodeType(signal)
      if (!nt.getIsDirectory()) {
        dirHandle.release()
        throw ErrNotDirectory
      }
    }
    dirHandle.release()
  }

  // mkdirAllPath creates directories along a string path.
  async mkdirAllPath(
    signal: AbortSignal,
    filepath: string,
    perm: number,
    ts: Date,
  ): Promise<void> {
    if (filepath === '' || filepath === '.') {
      return
    }
    const { parts } = splitPath(filepath)
    await this.mkdirAll(signal, parts, perm, ts)
  }

  // mkdirLookup looks up or creates a directory, then returns a handle.
  async mkdirLookup(
    signal: AbortSignal,
    name: string,
    perm: number,
    ts: Date,
  ): Promise<FSHandle> {
    let dir: FSHandle
    try {
      dir = await this.lookup(signal, name)
    } catch (e) {
      if (!isUnixFSError(e, UnixFSErrorType.NOT_EXIST)) {
        throw e
      }
      await this.mknod(
        signal,
        false,
        [name],
        newFSCursorNodeType_Dir(),
        perm,
        ts,
      )
      dir = await this.lookup(signal, name)
    }

    const nt = await dir.getNodeType(signal)
    if (!nt.getIsDirectory()) {
      dir.release()
      throw ErrNotDirectory
    }
    return dir
  }

  // mkdirAllLookup creates directories and returns a handle to the last one.
  async mkdirAllLookup(
    signal: AbortSignal,
    dirPath: string[],
    perm: number,
    ts: Date,
  ): Promise<FSHandle> {
    let currHandle = await this.clone(signal)
    if (dirPath.length === 0) {
      return currHandle
    }
    for (const pname of dirPath) {
      if (pname === '.') {
        continue
      }
      let newHandle: FSHandle
      try {
        newHandle = await currHandle.mkdirLookup(signal, pname, perm, ts)
      } catch (e) {
        currHandle.release()
        throw e
      }
      currHandle.release()
      currHandle = newHandle
    }
    return currHandle
  }

  // mkdirAllPathLookup is like mkdirAllLookup but takes a string path.
  async mkdirAllPathLookup(
    signal: AbortSignal,
    filepath: string,
    perm: number,
    ts: Date,
  ): Promise<FSHandle> {
    if (filepath === '' || filepath === '.') {
      return this.clone(signal)
    }
    const { parts } = splitPath(filepath)
    return this.mkdirAllLookup(signal, parts, perm, ts)
  }

  // symlink creates a symbolic link from a location to a path.
  async symlink(
    signal: AbortSignal,
    checkExist: boolean,
    name: string,
    target: string[],
    targetIsAbsolute: boolean,
    ts: Date,
  ): Promise<void> {
    if (!name || target.length === 0) {
      throw ErrEmptyPath
    }
    await this._inode.accessInode(signal, async (_cursor, ops) => {
      await ops.symlink(checkExist, name, target, targetIsAbsolute, ts, signal)
    })
  }

  // readlink reads a symbolic link contents.
  async readlink(
    signal: AbortSignal,
    name: string,
  ): Promise<{ path: string[]; isAbsolute: boolean }> {
    if (!name) {
      let result!: { path: string[]; isAbsolute: boolean }
      await this._inode.accessInode(signal, async (_cursor, ops) => {
        result = await ops.readlink(name, signal)
      })
      return result
    }

    const handle = await this.lookup(signal, name)
    try {
      let result!: { path: string[]; isAbsolute: boolean }
      await handle._inode.accessInode(signal, async (_cursor, ops) => {
        result = await ops.readlink(name, signal)
      })
      return result
    } finally {
      handle.release()
    }
  }

  // copy recursively copies a location to a destination.
  async copy(
    signal: AbortSignal,
    dest: FSHandle,
    destName: string,
    ts: Date,
  ): Promise<void> {
    if (!dest || dest.checkReleased() || this.checkReleased()) {
      throw ErrReleased
    }
    if (this === dest) {
      return
    }

    await this._inode.accessInode(signal, async (_cursor, srcOps) => {
      await dest._inode.accessInode(signal, async (_cursor2, destOps) => {
        // Attempt optimized copy from src -> dest.
        const doneTo = await srcOps.copyTo(destOps, destName, ts, signal)
        if (doneTo) return

        // Attempt optimized copy from dest <- src.
        const doneFrom = await destOps.copyFrom(destName, srcOps, ts, signal)
        if (doneFrom) return

        // No optimized path exists.
        throw new Error('unable to copy between these locations')
      })
    })
  }

  // rename recursively moves a source path to a destination.
  async rename(
    signal: AbortSignal,
    dest: FSHandle,
    destName: string,
    ts: Date,
  ): Promise<void> {
    if (!dest || dest.checkReleased() || this.checkReleased()) {
      throw ErrReleased
    }

    const lockedNodes = new Map<FsInode, RWMutexLock>()
    const relLockedNodes = () => {
      for (const [, lock] of lockedNodes) {
        lock.release()
      }
      lockedNodes.clear()
    }

    try {
      for (;;) {
        signal.throwIfAborted()

        const srcLoc = this._inode
        const destParent = dest._inode

        // Check if srcLoc is destParent.
        if (srcLoc === destParent) {
          throw ErrMoveToSelf
        }
        // Check if srcLoc is a parent of destParent.
        for (let nn: FsInode | null = destParent.parent; nn; nn = nn.parent) {
          if (nn === srcLoc) {
            throw ErrMoveToSelf
          }
        }

        // Lock srcLoc first, then try destParent.
        const relSrcLoc = await srcLoc.getRmtx().lock(true, signal)

        const relDestParent = destParent.getRmtx().tryLock(true)
        if (!relDestParent) {
          relSrcLoc.release()
          const relDest2 = await destParent.getRmtx().lock(true, signal)
          const relSrc2 = srcLoc.getRmtx().tryLock(true)
          if (!relSrc2) {
            relDest2.release()
            continue
          }
          lockedNodes.set(srcLoc, relSrc2)
          lockedNodes.set(destParent, relDest2)
        } else {
          lockedNodes.set(srcLoc, relSrcLoc)
          lockedNodes.set(destParent, relDestParent)
        }

        // Check both are not released.
        const srcErr = srcLoc.checkReleasedWithErr()
        if (srcErr) {
          relLockedNodes()
          throw srcErr
        }
        const destErr = destParent.checkReleasedWithErr()
        if (destErr) {
          relLockedNodes()
          throw destErr
        }

        // If either is currently resolving fsOps, wait.
        if (srcLoc.fsWait) {
          const w = srcLoc.fsWait.promise
          relLockedNodes()
          await Promise.race([w, abortPromise(signal)])
          continue
        }
        if (destParent.fsWait) {
          const w = destParent.fsWait.promise
          relLockedNodes()
          await Promise.race([w, abortPromise(signal)])
          continue
        }

        // Resolve fsOps for src.
        const fsOpsSrc = srcLoc.fsOps
        if (!fsOpsSrc || fsOpsSrc.checkReleased()) {
          // Need to resolve src first.
          relLockedNodes()
          await srcLoc.accessInode(signal, null)
          continue
        }

        // Resolve fsOps for dest.
        const fsOpsDest = destParent.fsOps
        if (!fsOpsDest || fsOpsDest.checkReleased()) {
          relLockedNodes()
          await destParent.accessInode(signal, null)
          continue
        }

        // Lock children of both.
        const nodStk: FsInode[] = [srcLoc, destParent]
        let lockErr: Error | null = null
        while (nodStk.length > 0) {
          const nod = nodStk.pop()!
          for (const nodChild of nod.getChildren()) {
            if (lockedNodes.has(nodChild)) {
              continue
            }
            let childLock: RWMutexLock
            try {
              childLock = await nodChild.getRmtx().lock(true, signal)
            } catch (e) {
              lockErr = e as Error
              break
            }
            if (
              nodChild.checkReleasedFlag() ||
              nodChild.releaseIfNecessaryLocked()
            ) {
              childLock.release()
              continue
            }
            lockedNodes.set(nodChild, childLock)
            nodStk.push(nodChild)
          }
          if (lockErr) break
        }
        if (lockErr) {
          relLockedNodes()
          throw lockErr
        }

        // If ops were released while locking, retry.
        if (fsOpsSrc.checkReleased() || fsOpsDest.checkReleased()) {
          relLockedNodes()
          continue
        }

        // Attempt optimized move src -> dest.
        const doneTo = await fsOpsSrc.moveTo(fsOpsDest, destName, ts, signal)
        if (doneTo) {
          // Success.
          this.finalizeRename(srcLoc, destParent, destName)
          return
        }

        // Attempt optimized move dest <- src.
        const doneFrom = await fsOpsDest.moveFrom(
          destName,
          fsOpsSrc,
          ts,
          signal,
        )
        if (doneFrom) {
          this.finalizeRename(srcLoc, destParent, destName)
          return
        }

        // No optimized path.
        relLockedNodes()
        throw ErrCrossFsRename
      }
    } finally {
      relLockedNodes()
    }
  }

  // finalizeRename cleans up inode tree after a successful rename.
  private finalizeRename(
    srcLoc: FsInode,
    destParent: FsInode,
    destName: string,
  ): void {
    // Remove source inode from parent.
    if (srcLoc.parent) {
      const { child: oldChild, index: oldChildIdx } =
        srcLoc.parent.findChildInode(srcLoc.name, false)
      if (oldChild) {
        srcLoc.parent.removeChildInodeAtIdx(oldChildIdx)
        if (oldChild !== srcLoc) {
          oldChild.releaseWithChildrenLocked(null)
        }
      }
    }

    // If dest parent released, bail.
    if (destParent.checkReleasedFlag()) {
      destParent.releaseLocked(ErrReleased)
      throw ErrReleased
    }

    // Lookup or create destination inode location.
    const { child: destLoc, index: destLocIdx } = destParent.findChildInode(
      destName,
      true,
    )
    let dest: FsInode
    if (destLoc) {
      dest = destLoc
    } else {
      dest = new FsInode(destParent, destName, [])
      destParent.insertChildAt(destLocIdx, dest)
    }

    // Merge srcLoc -> destLoc.
    dest.mergeWithNodeLocked(srcLoc, ErrReleased)

    // Recursively clear all fs cursors and ops for destLoc.
    dest.clearCursorsWithChildrenLocked()
  }

  // mknodWithContent creates a file entry with content written atomically.
  async mknodWithContent(
    signal: AbortSignal,
    name: string,
    nodeType: FSCursorNodeType,
    dataLen: bigint,
    data: Uint8Array,
    permissions: number,
    ts: Date,
  ): Promise<void> {
    await this._inode.accessInode(signal, async (_cursor, ops) => {
      await ops.mknodWithContent(
        name,
        nodeType,
        dataLen,
        data,
        permissions,
        ts,
        signal,
      )
    })
  }

  // remove removes entries from a directory.
  async remove(signal: AbortSignal, names: string[], ts: Date): Promise<void> {
    if (names.length === 0) {
      return
    }
    await this._inode.accessInode(signal, async (_cursor, ops) => {
      await ops.remove(names, ts, signal)
    })
  }

  // clone makes a copy of the FSHandle.
  async clone(signal?: AbortSignal): Promise<FSHandle> {
    return this._inode.addReference(signal)
  }

  // release releases the FSHandle.
  release(): void {
    if (this._released) {
      return
    }
    this._released = true

    const inode = this._inode
    // In TS single-threaded context, try to lock synchronously.
    const lock = inode.getRmtx().tryLock(true)
    if (lock) {
      inode.removeRefLocked(this)
      lock.release()
    }

    this._fireRelCbs()
  }

  // Symbol.dispose enables using-based cleanup.
  [Symbol.dispose](): void {
    this.release()
  }

  // create constructs a new FSHandle with a FSCursor.
  static create(cursor: FSCursor): FSHandle {
    const inode = new FsInode(null, '', [cursor])
    return inode.addReferenceLocked(false)
  }

  // createWithPrefix constructs a new FSHandle with a FSCursor and follows a prefix.
  static async createWithPrefix(
    signal: AbortSignal,
    cursor: FSCursor,
    prefixPath: string[],
    mkdirPath: boolean,
    ts: Date,
  ): Promise<FSHandle> {
    const rootHandle = FSHandle.create(cursor)
    if (prefixPath.length === 0) {
      return rootHandle
    }

    if (mkdirPath) {
      try {
        await rootHandle.mkdirAll(
          signal,
          prefixPath,
          defaultPermissions(newFSCursorNodeType_Dir()),
          ts,
        )
      } catch (e) {
        rootHandle.release()
        throw e
      }
    }

    const result = await rootHandle.lookupPathPts(signal, prefixPath)
    rootHandle.release()
    if (result.error) {
      result.handle.release()
      throw result.error
    }
    return result.handle
  }
}

// Inline utility functions to avoid circular imports.
// These mirror the Go implementations.

import type { FSCursorDirent, FSCursorNodeType } from './fs-cursor.js'
import { splitPath, cleanSplitValidateRelativePath } from './path.js'
import {
  newFSCursorNodeType_Dir,
  defaultPermissions,
  nodeTypeToMode,
} from './fs-node-type.js'
