import type {
  FSCursor,
  FSCursorDirent,
  FSCursorNodeType,
  FSCursorOps,
} from '../../fs-cursor.js'
import {
  ErrHandleIDEmpty,
  ErrReadOnly,
  ErrReleased,
  UnixFSError,
} from '../../errors/errors.js'
import { UnixFSErrorType } from '../../errors/errors.pb.js'
import type { UnixFSError as ProtoUnixFSError } from '../../errors/errors.pb.js'
import { NodeType } from '../../block/fstree.pb.js'
import type { RemoteFSCursor } from './fs-cursor-remote.js'

// RemoteFSCursorOps implements FSCursorOps backed by a remote ops handle ID.
export class RemoteFSCursorOps implements FSCursorOps {
  released = false
  readonly c: RemoteFSCursor
  readonly handleId: bigint
  readonly nodeType: NodeType
  readonly name: string
  private optimalWriteSize: bigint | null = null

  constructor(
    c: RemoteFSCursor,
    handleId: bigint,
    nodeType: NodeType,
    name: string,
  ) {
    this.c = c
    this.handleId = handleId
    this.nodeType = nodeType
    this.name = name
  }

  // getIsDirectory returns true if this is a directory node.
  getIsDirectory(): boolean {
    return this.nodeType === NodeType.NodeType_DIRECTORY
  }

  // getIsFile returns true if this is a file node.
  getIsFile(): boolean {
    return this.nodeType === NodeType.NodeType_FILE
  }

  // getIsSymlink returns true if this is a symlink node.
  getIsSymlink(): boolean {
    return this.nodeType === NodeType.NodeType_SYMLINK
  }

  // checkReleased checks if this ops or its parent cursor is released.
  checkReleased(): boolean {
    return this.released || this.c.released
  }

  // getName returns the name of the inode.
  getName(): string {
    return this.name
  }

  // getPermissions returns the permissions bits of the file mode.
  async getPermissions(signal?: AbortSignal): Promise<number> {
    if (this.checkReleased()) {
      throw ErrReleased
    }
    const resp = await this.c.c.client.OpsGetPermissions(
      { opsHandleId: this.handleId },
      signal,
    )
    this.handleRpcError(resp.unixfsError)
    return resp.fileMode ?? 0
  }

  // setPermissions updates the permissions bits of the file mode.
  async setPermissions(
    permissions: number,
    ts: Date,
    signal?: AbortSignal,
  ): Promise<void> {
    if (this.checkReleased()) {
      throw ErrReleased
    }
    const resp = await this.c.c.client.OpsSetPermissions(
      {
        opsHandleId: this.handleId,
        fileMode: permissions,
        timestamp: ts,
      },
      signal,
    )
    this.handleRpcError(resp.unixfsError)
  }

  // getSize returns the size of the inode in bytes.
  async getSize(signal?: AbortSignal): Promise<bigint> {
    if (this.checkReleased()) {
      throw ErrReleased
    }
    const resp = await this.c.c.client.OpsGetSize(
      { opsHandleId: this.handleId },
      signal,
    )
    this.handleRpcError(resp.unixfsError)
    return resp.size ?? 0n
  }

  // getModTimestamp returns the modification timestamp.
  async getModTimestamp(signal?: AbortSignal): Promise<Date> {
    if (this.checkReleased()) {
      throw ErrReleased
    }
    const resp = await this.c.c.client.OpsGetModTimestamp(
      { opsHandleId: this.handleId },
      signal,
    )
    this.handleRpcError(resp.unixfsError)
    return resp.modTimestamp ?? new Date(0)
  }

  // setModTimestamp updates the modification timestamp of the node.
  async setModTimestamp(mtime: Date, signal?: AbortSignal): Promise<void> {
    if (this.checkReleased()) {
      throw ErrReleased
    }
    const resp = await this.c.c.client.OpsSetModTimestamp(
      {
        opsHandleId: this.handleId,
        modTimestamp: mtime,
      },
      signal,
    )
    this.handleRpcError(resp.unixfsError)
  }

  // readAt reads from a location in a file node.
  // Overload: pass Uint8Array to fill buffer, returns bytes read as bigint.
  // Overload: pass bigint size to allocate and return {data, n}.
  // Throws ErrEOF if the offset is past the end of the file (data may still be
  // partially returned in the buffer overload).
  async readAt(
    offset: bigint,
    dataOrSize: Uint8Array | bigint,
    signal?: AbortSignal,
  ): Promise<bigint | { data: Uint8Array; n: bigint }> {
    const isBuf = dataOrSize instanceof Uint8Array
    if (isBuf && dataOrSize.length === 0) {
      throw new Error('short buffer')
    }
    if (this.checkReleased()) {
      throw ErrReleased
    }
    const size = isBuf ? BigInt(dataOrSize.length) : (dataOrSize as bigint)
    const resp = await this.c.c.client.OpsReadAt(
      {
        opsHandleId: this.handleId,
        offset,
        size,
      },
      signal,
    )

    const ufErr = resp.unixfsError
      ? UnixFSError.fromProto(resp.unixfsError)
      : null
    if (ufErr && !ufErr.isEOF) {
      this.handleRpcError(resp.unixfsError)
    }

    const retData = resp.data ?? new Uint8Array(0)
    if (isBuf) {
      const copyLen = Math.min(retData.length, dataOrSize.length)
      dataOrSize.set(retData.subarray(0, copyLen))
      return BigInt(copyLen)
    }
    return { data: retData, n: BigInt(retData.length) }
  }

  // getOptimalWriteSize returns the best write size to use for the write call.
  async getOptimalWriteSize(signal?: AbortSignal): Promise<bigint> {
    if (this.optimalWriteSize !== null && this.optimalWriteSize > 0n) {
      return this.optimalWriteSize
    }
    if (this.optimalWriteSize === -1n) {
      return 0n
    }
    if (this.checkReleased()) {
      throw ErrReleased
    }
    const resp = await this.c.c.client.OpsGetOptimalWriteSize(
      { opsHandleId: this.handleId },
      signal,
    )
    this.handleRpcError(resp.unixfsError)
    const val = resp.optimalWriteSize ?? 0n
    if (val > 0n) {
      if (this.optimalWriteSize === null) {
        this.optimalWriteSize = val
      }
    } else {
      if (this.optimalWriteSize === null) {
        this.optimalWriteSize = -1n
      }
    }
    return val
  }

  // writeAt writes to a location within a file node synchronously.
  async writeAt(
    offset: bigint,
    data: Uint8Array,
    ts: Date,
    signal?: AbortSignal,
  ): Promise<void> {
    if (this.checkReleased()) {
      throw ErrReleased
    }
    const resp = await this.c.c.client.OpsWriteAt(
      {
        opsHandleId: this.handleId,
        offset,
        data,
        timestamp: ts,
      },
      signal,
    )
    this.handleRpcError(resp.unixfsError)
  }

  // truncate shrinks or extends a file to the specified size.
  async truncate(
    nsize: bigint,
    ts: Date,
    signal?: AbortSignal,
  ): Promise<void> {
    if (this.checkReleased()) {
      throw ErrReleased
    }
    const resp = await this.c.c.client.OpsTruncate(
      {
        opsHandleId: this.handleId,
        nsize,
        timestamp: ts,
      },
      signal,
    )
    this.handleRpcError(resp.unixfsError)
  }

  // lookup looks up a child entry in a directory.
  async lookup(name: string, signal?: AbortSignal): Promise<FSCursor> {
    if (this.checkReleased()) {
      throw ErrReleased
    }
    const resp = await this.c.c.client.OpsLookup(
      {
        opsHandleId: this.handleId,
        clientHandleId: this.c.c.clientHandleId,
        cursorHandleId: this.c.cursorHandleId,
        name,
      },
      signal,
    )

    const ufErr = resp.unixfsError
      ? UnixFSError.fromProto(resp.unixfsError)
      : null

    if (this.c.c.released) {
      throw ErrReleased
    }

    if (ufErr) {
      this.markReleasedIfError(resp.unixfsError)
      throw ufErr
    }

    const handleId = resp.cursorHandleId
    if (!handleId) {
      throw ErrHandleIDEmpty
    }
    return this.c.c.ingestCursor(handleId)
  }

  // readdirAll reads all directory entries.
  async readdirAll(
    skip: bigint,
    cb: (ent: FSCursorDirent) => void,
    signal?: AbortSignal,
  ): Promise<void> {
    if (this.checkReleased()) {
      throw ErrReleased
    }
    const stream = this.c.c.client.OpsReaddirAll(
      {
        opsHandleId: this.handleId,
        skip,
      },
      signal,
    )
    for await (const msg of stream) {
      const body = msg.body
      if (body?.case === 'unixfsError') {
        this.handleRpcError(body.value)
        return
      }
      if (body?.case === 'done') {
        return
      }
      if (body?.case === 'dirent') {
        const dirent = body.value
        if (dirent.nodeType) {
          cb({
            getName(): string {
              return dirent.name ?? ''
            },
            getIsDirectory(): boolean {
              return dirent.nodeType === NodeType.NodeType_DIRECTORY
            },
            getIsFile(): boolean {
              return dirent.nodeType === NodeType.NodeType_FILE
            },
            getIsSymlink(): boolean {
              return dirent.nodeType === NodeType.NodeType_SYMLINK
            },
          })
        }
      }
    }
  }

  // mknod creates child entries in a directory.
  async mknod(
    checkExist: boolean,
    names: string[],
    nodeType: FSCursorNodeType,
    permissions: number,
    ts: Date,
    signal?: AbortSignal,
  ): Promise<void> {
    if (this.checkReleased()) {
      throw ErrReleased
    }
    let nt = NodeType.NodeType_UNKNOWN
    if (nodeType.getIsDirectory()) {
      nt = NodeType.NodeType_DIRECTORY
    } else if (nodeType.getIsFile()) {
      nt = NodeType.NodeType_FILE
    } else if (nodeType.getIsSymlink()) {
      nt = NodeType.NodeType_SYMLINK
    }
    const resp = await this.c.c.client.OpsMknod(
      {
        opsHandleId: this.handleId,
        checkExist,
        names,
        nodeType: nt,
        permissions,
        timestamp: ts,
      },
      signal,
    )
    this.handleRpcError(resp.unixfsError)
  }

  // symlink creates a symbolic link from a location to a path.
  async symlink(
    checkExist: boolean,
    name: string,
    target: string[],
    targetIsAbsolute: boolean,
    ts: Date,
    signal?: AbortSignal,
  ): Promise<void> {
    if (this.checkReleased()) {
      throw ErrReleased
    }
    const resp = await this.c.c.client.OpsSymlink(
      {
        opsHandleId: this.handleId,
        checkExist,
        name,
        symlink: {
          targetPath: {
            nodes: target,
            absolute: targetIsAbsolute,
          },
        },
        timestamp: ts,
      },
      signal,
    )
    this.handleRpcError(resp.unixfsError)
  }

  // readlink reads a symbolic link contents.
  async readlink(
    name: string,
    signal?: AbortSignal,
  ): Promise<{ path: string[]; isAbsolute: boolean }> {
    if (this.checkReleased()) {
      throw ErrReleased
    }
    const resp = await this.c.c.client.OpsReadlink(
      {
        opsHandleId: this.handleId,
        name,
      },
      signal,
    )
    this.handleRpcError(resp.unixfsError)
    const tgtPath = resp.symlink?.targetPath
    return {
      path: tgtPath?.nodes ?? [],
      isAbsolute: tgtPath?.absolute ?? false,
    }
  }

  // copyTo performs an optimized copy of an inode to another inode.
  async copyTo(
    tgtDir: FSCursorOps,
    tgtName: string,
    ts: Date,
    signal?: AbortSignal,
  ): Promise<boolean> {
    if (this.checkReleased()) {
      throw ErrReleased
    }
    if (!(tgtDir instanceof RemoteFSCursorOps)) {
      return false
    }
    if (tgtDir.c.c !== this.c.c) {
      return false
    }
    if (tgtDir.checkReleased()) {
      return false
    }
    const resp = await this.c.c.client.OpsCopyTo(
      {
        opsHandleId: this.handleId,
        targetDirOpsHandleId: tgtDir.handleId,
        targetName: tgtName,
        timestamp: ts,
      },
      signal,
    )
    const ufErr = resp.unixfsError
      ? UnixFSError.fromProto(resp.unixfsError)
      : null
    if (ufErr) {
      tgtDir.markReleasedIfError(resp.unixfsError)
      this.markReleasedIfError(resp.unixfsError)
      throw ufErr
    }
    return resp.done ?? false
  }

  // copyFrom performs an optimized copy from another inode.
  async copyFrom(
    name: string,
    srcCursorOps: FSCursorOps,
    ts: Date,
    signal?: AbortSignal,
  ): Promise<boolean> {
    if (this.checkReleased()) {
      throw ErrReleased
    }
    if (!(srcCursorOps instanceof RemoteFSCursorOps)) {
      return false
    }
    if (srcCursorOps.c.c !== this.c.c) {
      return false
    }
    if (srcCursorOps.checkReleased()) {
      return false
    }
    const resp = await this.c.c.client.OpsCopyFrom(
      {
        opsHandleId: this.handleId,
        name,
        srcCursorOpsHandleId: srcCursorOps.handleId,
        timestamp: ts,
      },
      signal,
    )
    const ufErr = resp.unixfsError
      ? UnixFSError.fromProto(resp.unixfsError)
      : null
    if (ufErr) {
      srcCursorOps.markReleasedIfError(resp.unixfsError)
      this.markReleasedIfError(resp.unixfsError)
      throw ufErr
    }
    return resp.done ?? false
  }

  // moveTo performs an atomic and optimized move to another inode.
  async moveTo(
    tgtCursorOps: FSCursorOps,
    tgtName: string,
    ts: Date,
    signal?: AbortSignal,
  ): Promise<boolean> {
    if (this.checkReleased()) {
      throw ErrReleased
    }
    if (!(tgtCursorOps instanceof RemoteFSCursorOps)) {
      return false
    }
    if (tgtCursorOps.c.c !== this.c.c) {
      return false
    }
    if (tgtCursorOps.checkReleased()) {
      return false
    }
    const resp = await this.c.c.client.OpsMoveTo(
      {
        opsHandleId: this.handleId,
        targetDirOpsHandleId: tgtCursorOps.handleId,
        targetName: tgtName,
        timestamp: ts,
      },
      signal,
    )
    const ufErr = resp.unixfsError
      ? UnixFSError.fromProto(resp.unixfsError)
      : null
    if (ufErr) {
      tgtCursorOps.markReleasedIfError(resp.unixfsError)
      this.markReleasedIfError(resp.unixfsError)
      throw ufErr
    }
    return resp.done ?? false
  }

  // moveFrom performs an atomic and optimized move from another inode.
  async moveFrom(
    name: string,
    srcCursorOps: FSCursorOps,
    ts: Date,
    signal?: AbortSignal,
  ): Promise<boolean> {
    if (this.checkReleased()) {
      throw ErrReleased
    }
    if (!(srcCursorOps instanceof RemoteFSCursorOps)) {
      return false
    }
    if (srcCursorOps.c.c !== this.c.c) {
      return false
    }
    if (srcCursorOps.checkReleased()) {
      return false
    }
    const resp = await this.c.c.client.OpsMoveFrom(
      {
        opsHandleId: this.handleId,
        name,
        srcOpsHandleId: srcCursorOps.handleId,
        timestamp: ts,
      },
      signal,
    )
    const ufErr = resp.unixfsError
      ? UnixFSError.fromProto(resp.unixfsError)
      : null
    if (ufErr) {
      srcCursorOps.markReleasedIfError(resp.unixfsError)
      this.markReleasedIfError(resp.unixfsError)
      throw ufErr
    }
    return resp.done ?? false
  }

  // remove deletes entries from a directory.
  async remove(
    names: string[],
    ts: Date,
    signal?: AbortSignal,
  ): Promise<void> {
    if (this.checkReleased()) {
      throw ErrReleased
    }
    const resp = await this.c.c.client.OpsRemove(
      {
        opsHandleId: this.handleId,
        names,
        timestamp: ts,
      },
      signal,
    )
    this.handleRpcError(resp.unixfsError)
  }

  // mknodWithContent returns ErrReadOnly (RPC support not yet implemented).
  async mknodWithContent(
    _name: string,
    _nodeType: FSCursorNodeType,
    _dataLen: bigint,
    _data: Uint8Array,
    _permissions: number,
    _ts: Date,
    _signal?: AbortSignal,
  ): Promise<void> {
    if (this.checkReleased()) {
      throw ErrReleased
    }
    throw ErrReadOnly
  }

  // markReleasedIfError marks this ops as released if the error is RELEASED.
  // Does not throw. Used when multiple ops need marking before throwing.
  private markReleasedIfError(err?: ProtoUnixFSError): void {
    if (!err || err.errorType === UnixFSErrorType.NONE) {
      return
    }
    const e = UnixFSError.fromProto(err)
    if (e?.isReleased && !this.released) {
      this.released = true
      this.c.c.removeOps(this.handleId)
    }
  }

  // handleRpcError checks a proto error response and handles released state.
  private handleRpcError(err?: ProtoUnixFSError): void {
    if (!err || err.errorType === UnixFSErrorType.NONE) {
      return
    }
    this.markReleasedIfError(err)
    const e = UnixFSError.fromProto(err)
    if (e) {
      throw e
    }
  }
}
