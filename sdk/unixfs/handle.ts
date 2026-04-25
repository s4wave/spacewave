import { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import {
  Resource,
  createWatchableDebugInfo,
  type ResourceDebugInfo,
  type WatchableDebugInfo,
} from '@aptre/bldr-sdk/resource/resource.js'
import { FSHandleResourceServiceClient } from './handle_srpc.pb.js'
import type {
  FileInfo,
  NodeType,
  HandleReaddirResponse,
  HandleWatchReaddirResponse,
  MknodType,
  DirEntry,
  HandleUploadFileRequest,
  HandleUploadTreeRequest,
} from './handle.pb.js'

// FSHandleMeta contains metadata for constructing an FSHandle.
interface FSHandleMeta {
  info?: FileInfo
  path?: string
}

// TreeUploadDirectory is one explicit directory in a tree upload.
export interface TreeUploadDirectory {
  kind: 'directory'
  path: string
  mode?: number
}

// TreeUploadFile is one streamed file in a tree upload.
export interface TreeUploadFile {
  kind: 'file'
  path: string
  totalSize: bigint
  stream: ReadableStream<Uint8Array>
  mode?: number
  onProgress?: (bytesWritten: bigint) => void
}

// TreeUploadEntry is one tree upload entry.
export type TreeUploadEntry = TreeUploadDirectory | TreeUploadFile

// IFSHandle contains the FSHandle interface.
export interface IFSHandle {
  // getInfo returns the cached file info.
  getInfo(): FileInfo

  // isDirectory returns true if this handle points to a directory.
  isDirectory(): boolean

  // isFile returns true if this handle points to a regular file.
  isFile(): boolean

  // lookup looks up a child by name and returns a new handle.
  lookup(name: string, abortSignal?: AbortSignal): Promise<FSHandle>

  // lookupPath looks up a path (relative to this handle) and returns a new handle.
  lookupPath(
    path: string,
    abortSignal?: AbortSignal,
  ): Promise<{ handle: FSHandle; traversedPath: string[] }>

  // readAt reads bytes at the given offset.
  readAt(
    offset: bigint,
    length: bigint,
    abortSignal?: AbortSignal,
  ): Promise<{ data: Uint8Array; bytesRead: bigint; eof: boolean }>

  // writeAt writes bytes at the given offset.
  writeAt(
    offset: bigint,
    data: Uint8Array,
    abortSignal?: AbortSignal,
  ): Promise<bigint>

  // truncate truncates the file to the given size.
  truncate(size: bigint, abortSignal?: AbortSignal): Promise<void>

  // getSize returns the current size of the file.
  getSize(abortSignal?: AbortSignal): Promise<bigint>

  // getFileInfo fetches fresh file info from the server.
  getFileInfo(abortSignal?: AbortSignal): Promise<FileInfo>

  // getNodeType returns the node type (file, directory, symlink).
  getNodeType(abortSignal?: AbortSignal): Promise<NodeType>

  // readdir reads directory entries as an async iterable.
  readdir(skip?: bigint, abortSignal?: AbortSignal): AsyncIterable<DirEntry>

  // readdirAll reads all directory entries into an array.
  readdirAll(skip?: bigint, abortSignal?: AbortSignal): Promise<DirEntry[]>

  // watchReaddir watches directory entries as an async iterable.
  // Each yielded value is the complete current directory listing.
  watchReaddir(abortSignal?: AbortSignal): AsyncIterable<DirEntry[]>

  // mknod creates new files or directories.
  mknod(
    names: string[],
    nodeType: MknodType,
    mode?: number,
    checkExist?: boolean,
    abortSignal?: AbortSignal,
  ): Promise<void>

  // remove removes files or directories by name.
  remove(names: string[], abortSignal?: AbortSignal): Promise<void>

  // mkdirAll creates a directory and all parent directories.
  mkdirAll(
    pathParts: string[],
    mode?: number,
    abortSignal?: AbortSignal,
  ): Promise<void>

  // rename renames an entry within this directory.
  // sourceName is the name of the entry to rename.
  // destName is the new name for the entry.
  // destParentResourceId is the resource ID of the destination parent (0 for same directory).
  rename(
    sourceName: string,
    destName: string,
    destParentResourceId?: number,
    abortSignal?: AbortSignal,
  ): Promise<void>

  // uploadFile uploads a file via client-streaming.
  uploadFile(
    name: string,
    totalSize: bigint,
    stream: ReadableStream<Uint8Array>,
    mode?: number,
    onProgress?: (bytesWritten: bigint) => void,
    abortSignal?: AbortSignal,
  ): Promise<bigint>

  // uploadTree uploads a directory tree relative to this handle.
  uploadTree(
    entries: TreeUploadEntry[],
    onProgress?: (bytesWritten: bigint) => void,
    abortSignal?: AbortSignal,
  ): Promise<{
    bytesWritten: bigint
    filesWritten: bigint
    directoriesWritten: bigint
  }>

  // clone creates a copy of this handle pointing to the same location.
  clone(abortSignal?: AbortSignal): Promise<FSHandle>

  // release releases the resource.
  release(): void

  // Symbol.dispose for using with 'using' statement.
  [Symbol.dispose](): void
}

// FSHandle represents a handle to a filesystem location.
// Each instance maps 1:1 to a Go FSHandle on the backend.
// This mirrors the Hydra FSHandle pattern for full performance via stateful handles.
export class FSHandle extends Resource implements IFSHandle {
  private service: FSHandleResourceServiceClient
  private _info: FileInfo
  private _path: string
  private _debugInfo: WatchableDebugInfo

  constructor(resourceRef: ClientResourceRef, meta?: FSHandleMeta) {
    super(resourceRef)
    this.service = new FSHandleResourceServiceClient(resourceRef.client)
    this._info = meta?.info ?? {}
    this._path = meta?.path ?? ''
    this._debugInfo = createWatchableDebugInfo(this.buildDebugInfo())
  }

  // buildDebugInfo builds the debug info from current state.
  private buildDebugInfo(): ResourceDebugInfo {
    const name = this._info.name
    const isRoot = !this._path && !name
    const details: Record<string, string | number | boolean | null> = {}

    // Always include path if available
    if (this._path) {
      details.path = this._path
    }

    // Include file info when available
    if (name) {
      details.name = name
    }
    if (this._info.isDir !== undefined) {
      details.isDir = this._info.isDir
    }
    if (this._info.size !== undefined) {
      details.size = Number(this._info.size)
    }

    // Build label with type suffix
    let label = isRoot ? '(root)' : name || this._path.split('/').pop()
    if (this._info.isDir !== undefined) {
      label = `${label} (${this._info.isDir ? 'dir' : 'file'})`
    }

    return {
      label,
      details: Object.keys(details).length > 0 ? details : undefined,
    }
  }

  // getDebugInfo returns debug information for devtools.
  public getDebugInfo(): ResourceDebugInfo {
    return this._debugInfo.get()
  }

  // watchDebugInfo returns an async iterable of debug info updates.
  public watchDebugInfo(
    _abortSignal?: AbortSignal,
  ): AsyncIterable<ResourceDebugInfo> {
    return this._debugInfo.watch()
  }

  // getInfo returns the cached file info.
  public getInfo(): FileInfo {
    return this._info
  }

  // getPath returns the logical path tracked for this handle.
  public getPath(): string {
    return this._path
  }

  // isDirectory returns true if this handle points to a directory.
  public isDirectory(): boolean {
    return this._info.isDir ?? false
  }

  // isFile returns true if this handle points to a regular file.
  public isFile(): boolean {
    return !this._info.isDir
  }

  // lookup looks up a child by name and returns a new handle.
  public async lookup(
    name: string,
    abortSignal?: AbortSignal,
  ): Promise<FSHandle> {
    const resp = await this.service.Lookup({ name }, abortSignal)
    const childPath = this._path ? `${this._path}/${name}` : name
    return this.resourceRef.createResource(resp.resourceId ?? 0, FSHandle, {
      info: resp.info,
      path: childPath,
    })
  }

  // lookupPath looks up a path (relative to this handle) and returns a new handle.
  public async lookupPath(
    path: string,
    abortSignal?: AbortSignal,
  ): Promise<{ handle: FSHandle; traversedPath: string[] }> {
    const resp = await this.service.LookupPath({ path }, abortSignal)
    const childPath = this._path ? `${this._path}/${path}` : path
    const handle = this.resourceRef.createResource(
      resp.resourceId ?? 0,
      FSHandle,
      {
        info: resp.info,
        path: childPath,
      },
    )
    return {
      handle,
      traversedPath: resp.traversedPath ?? [],
    }
  }

  // readAt reads bytes at the given offset.
  public async readAt(
    offset: bigint,
    length: bigint,
    abortSignal?: AbortSignal,
  ): Promise<{ data: Uint8Array; bytesRead: bigint; eof: boolean }> {
    const resp = await this.service.ReadAt({ offset, length }, abortSignal)
    return {
      data: resp.data ?? new Uint8Array(),
      bytesRead: resp.bytesRead ?? 0n,
      eof: resp.eof ?? false,
    }
  }

  // writeAt writes bytes at the given offset.
  public async writeAt(
    offset: bigint,
    data: Uint8Array,
    abortSignal?: AbortSignal,
  ): Promise<bigint> {
    const resp = await this.service.WriteAt({ offset, data }, abortSignal)
    return resp.bytesWritten ?? 0n
  }

  // truncate truncates the file to the given size.
  public async truncate(
    size: bigint,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    await this.service.Truncate({ size }, abortSignal)
  }

  // getSize returns the current size of the file.
  public async getSize(abortSignal?: AbortSignal): Promise<bigint> {
    const resp = await this.service.GetSize({}, abortSignal)
    return resp.size ?? 0n
  }

  // getFileInfo fetches fresh file info from the server.
  public async getFileInfo(abortSignal?: AbortSignal): Promise<FileInfo> {
    const resp = await this.service.GetFileInfo({}, abortSignal)
    const info = resp.info ?? {}
    // Update cached info and push debug info update
    this._info = info
    this._debugInfo.set(this.buildDebugInfo())
    return info
  }

  // getNodeType returns the node type (file, directory, symlink).
  public async getNodeType(abortSignal?: AbortSignal): Promise<NodeType> {
    const resp = await this.service.GetNodeType({}, abortSignal)
    return resp.nodeType ?? {}
  }

  // readlink reads the target of a symbolic link at this handle.
  public async readlink(abortSignal?: AbortSignal): Promise<string> {
    const resp = await this.service.Readlink({}, abortSignal)
    return resp.target ?? ''
  }

  // readdir reads directory entries as an async iterable.
  public async *readdir(
    skip?: bigint,
    abortSignal?: AbortSignal,
  ): AsyncIterable<DirEntry> {
    const stream = this.service.Readdir({ skip }, abortSignal)
    for await (const resp of stream as AsyncIterable<HandleReaddirResponse>) {
      if (resp.done) {
        break
      }
      if (resp.entry) {
        yield resp.entry
      }
    }
  }

  // readdirAll reads all directory entries into an array.
  public async readdirAll(
    skip?: bigint,
    abortSignal?: AbortSignal,
  ): Promise<DirEntry[]> {
    const entries: DirEntry[] = []
    for await (const entry of this.readdir(skip, abortSignal)) {
      entries.push(entry)
    }
    return entries
  }

  // watchReaddir watches directory entries as an async iterable.
  // Each yielded value is the complete current directory listing.
  public async *watchReaddir(
    abortSignal?: AbortSignal,
  ): AsyncIterable<DirEntry[]> {
    const stream = this.service.WatchReaddir({}, abortSignal)
    for await (const resp of stream as AsyncIterable<HandleWatchReaddirResponse>) {
      yield resp.entries ?? []
    }
  }

  // mknod creates new files or directories.
  public async mknod(
    names: string[],
    nodeType: MknodType,
    mode?: number,
    checkExist?: boolean,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    await this.service.Mknod({ names, nodeType, mode, checkExist }, abortSignal)
  }

  // remove removes files or directories by name.
  public async remove(
    names: string[],
    abortSignal?: AbortSignal,
  ): Promise<void> {
    await this.service.Remove({ names }, abortSignal)
  }

  // mkdirAll creates a directory and all parent directories.
  public async mkdirAll(
    pathParts: string[],
    mode?: number,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    await this.service.MkdirAll({ pathParts, mode }, abortSignal)
  }

  // rename renames an entry within this directory.
  // sourceName is the name of the entry to rename.
  // destName is the new name for the entry.
  // destParentResourceId is the resource ID of the destination parent (0 for same directory).
  public async rename(
    sourceName: string,
    destName: string,
    destParentResourceId?: number,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    await this.service.Rename(
      { sourceName, destName, destParentResourceId: destParentResourceId ?? 0 },
      abortSignal,
    )
  }

  // uploadFile uploads a file via client-streaming.
  public async uploadFile(
    name: string,
    totalSize: bigint,
    stream: ReadableStream<Uint8Array>,
    mode?: number,
    onProgress?: (bytesWritten: bigint) => void,
    abortSignal?: AbortSignal,
  ): Promise<bigint> {
    const reader = stream.getReader()
    let bytesWritten = 0n
    let first = true

    async function* generateMessages(): AsyncIterable<HandleUploadFileRequest> {
      try {
        while (true) {
          const { done, value } = await reader.read()
          if (done) break

          const msg: HandleUploadFileRequest = { data: value }
          if (first) {
            msg.name = name
            msg.totalSize = totalSize
            msg.mode = mode ?? 0
            first = false
          }
          yield msg

          bytesWritten += BigInt(value.byteLength)
          onProgress?.(bytesWritten)
        }
      } finally {
        reader.releaseLock()
      }
    }

    const response = await this.service.UploadFile(
      generateMessages(),
      abortSignal,
    )
    return response.bytesWritten ?? 0n
  }

  // uploadTree uploads a directory tree via client-streaming.
  public async uploadTree(
    entries: TreeUploadEntry[],
    onProgress?: (bytesWritten: bigint) => void,
    abortSignal?: AbortSignal,
  ): Promise<{
    bytesWritten: bigint
    filesWritten: bigint
    directoriesWritten: bigint
  }> {
    let totalBytesWritten = 0n

    async function* generateMessages(): AsyncIterable<HandleUploadTreeRequest> {
      for (const entry of entries) {
        if (entry.kind === 'directory') {
          yield {
            body: {
              case: 'directory',
              value: {
                path: entry.path,
                mode: entry.mode ?? 0o755,
              },
            },
          }
          continue
        }

        yield {
          body: {
            case: 'fileStart',
            value: {
              path: entry.path,
              totalSize: entry.totalSize,
              mode: entry.mode ?? 0,
            },
          },
        }

        const reader = entry.stream.getReader()
        let fileBytesWritten = 0n
        try {
          while (true) {
            const { done, value } = await reader.read()
            if (done) break
            yield { body: { case: 'data', value } }
            fileBytesWritten += BigInt(value.byteLength)
            totalBytesWritten += BigInt(value.byteLength)
            entry.onProgress?.(fileBytesWritten)
            onProgress?.(totalBytesWritten)
          }
        } finally {
          reader.releaseLock()
        }
      }
    }

    const response = await this.service.UploadTree(
      generateMessages(),
      abortSignal,
    )
    return {
      bytesWritten: response.bytesWritten ?? 0n,
      filesWritten: response.filesWritten ?? 0n,
      directoriesWritten: response.directoriesWritten ?? 0n,
    }
  }

  // clone creates a copy of this handle pointing to the same location.
  public async clone(abortSignal?: AbortSignal): Promise<FSHandle> {
    const resp = await this.service.Clone({}, abortSignal)
    return this.resourceRef.createResource(resp.resourceId ?? 0, FSHandle, {
      info: this._info,
      path: this._path,
    })
  }
}
