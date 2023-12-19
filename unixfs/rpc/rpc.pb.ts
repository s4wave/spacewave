/* eslint-disable */
import { Timestamp } from '@go/github.com/aperturerobotics/timestamp/timestamp.pb.js'
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'
import {
  FSSymlink,
  NodeType,
  nodeTypeFromJSON,
  nodeTypeToJSON,
} from '../block/fstree.pb.js'
import { UnixFSError } from '../errors/errors.pb.js'

export const protobufPackage = 'unixfs.rpc'

/** GetProxyCursorRequest is the request body for GetProxyCursor. */
export interface GetProxyCursorRequest {
  /** CursorHandleId is the handle identifier for the cursor. */
  cursorHandleId: Long
  /**
   * ClientHandleId is the handle identifier for the client.
   * Call FSCursorClient to get a client handle ID.
   */
  clientHandleId: Long
}

/** GetProxyCursorResponse is the response body for GetProxyCursor. */
export interface GetProxyCursorResponse {
  /** UnixfsError contains the error returned by the call, if any. */
  unixfsError: UnixFSError | undefined
  /**
   * CursorHandleId is the handle identifier returned by get proxy cursor.
   * if zero, the FSCursor returned is nil (no proxy cursor needed).
   */
  cursorHandleId: Long
}

/** FSCursorChange represents the FSCursorChange struct from unixfs. */
export interface FSCursorChange {
  /** CursorHandleId is the handle identifier for the cursor. */
  cursorHandleId: Long
  /** Released indicates the cursor was released. */
  released: boolean
  /** Offset is the location to flush from. */
  offset: Long
  /** Size is the amount of data to flush. */
  size: Long
}

/** FSCursorDirent represents the FSCursorDirent interface from unixfs. */
export interface FSCursorDirent {
  /** Name is the name of the directory entry. */
  name: string
  /** NodeType is the type of node at the dirent. */
  nodeType: NodeType
}

/** FSCursorClientRequest is the request body for FSCursorClient. */
export interface FSCursorClientRequest {}

/** FSCursorClientResponse contains an event for an FSCursor change. */
export interface FSCursorClientResponse {
  body?:
    | { $case: 'init'; init: FSClientInit }
    | { $case: 'cursorChange'; cursorChange: FSCursorChange }
    | {
        $case: 'unixfsError'
        unixfsError: UnixFSError
      }
    | undefined
}

/** FSClientInit is the initialization response to FSCursorClient. */
export interface FSClientInit {
  /**
   * ClientHandleId is the handle identifier for the client.
   * The client should use this ID going forward for requests.
   */
  clientHandleId: Long
  /**
   * CursorHandleId is the handle identifier for the root cursor.
   * Usually ID #1.
   */
  cursorHandleId: Long
}

/** GetCursorOpsRequest requests that we resolve the FSCursorOps for an FSCursor. */
export interface GetCursorOpsRequest {
  /** CursorHandleId is the handle identifier for the cursor. */
  cursorHandleId: Long
}

/** GetCursorOpsResponse is the response body for GetCursorOps. */
export interface GetCursorOpsResponse {
  /** UnixfsError contains the error returned by the call, if any. */
  unixfsError: UnixFSError | undefined
  /**
   * OpsHandleId is the handle identifier for the ops.
   * If zero, the FSCursorOps will be nil.
   */
  opsHandleId: Long
  /**
   * Name is the name of the inode at the ops (if applicable).
   * Empty if ops_handle_id is zero.
   */
  name: string
  /** NodeType is the type of node at the ops object. */
  nodeType: NodeType
}

/** ReleaseFSCursorRequest is the body of the ReleaseFSCursor RPC request. */
export interface ReleaseFSCursorRequest {
  /** CursorHandleId is the handle identifier for the cursor. */
  cursorHandleId: Long
  /**
   * ClientHandleId is the handle identifier for the client.
   * Call FSCursorClient to get a client handle ID.
   */
  clientHandleId: Long
}

/** ReleaseFSCursorResponse is the body of the ReleaseFSCursor RPC response. */
export interface ReleaseFSCursorResponse {}

/** OpsGetPermissionsRequest is the body of the ops GetPermissions request. */
export interface OpsGetPermissionsRequest {
  /** OpsHandleId uniquely identifies the open ops handle. */
  opsHandleId: Long
}

/** OpsGetPermissionsResponse is the body of the ops GetPermissions response. */
export interface OpsGetPermissionsResponse {
  /** UnixfsError contains the error returned by the call, if any. */
  unixfsError: UnixFSError | undefined
  /** FileMode corresponds to fs.FileMode containing the permissions bits. */
  fileMode: number
}

/** OpsSetPermissionsRequest is the body of the ops SetPermissions request. */
export interface OpsSetPermissionsRequest {
  /** OpsHandleId uniquely identifies the open ops handle. */
  opsHandleId: Long
  /** FileMode corresponds to fs.FileMode containing the permissions bits. */
  fileMode: number
  /** Timestamp is the desired timestamp for the operation. */
  timestamp: Timestamp | undefined
}

/** OpsSetPermissionsResponse is the body of the ops SetPermissions response. */
export interface OpsSetPermissionsResponse {
  /** UnixfsError contains the error returned by the call, if any. */
  unixfsError: UnixFSError | undefined
}

/** OpsGetSizeRequest is the body of the ops GetSize request. */
export interface OpsGetSizeRequest {
  /** OpsHandleId uniquely identifies the open ops handle. */
  opsHandleId: Long
}

/** OpsGetSizeResponse is the body of the ops GetSize response. */
export interface OpsGetSizeResponse {
  /** UnixfsError contains the error returned by the call, if any. */
  unixfsError: UnixFSError | undefined
  /** Size contains the size of the inode in bytes. */
  size: Long
}

/** OpsGetModTimestampRequest is the body of the ops GetModTimestamp request. */
export interface OpsGetModTimestampRequest {
  /** OpsHandleId uniquely identifies the open ops handle. */
  opsHandleId: Long
}

/** OpsGetModTimestampResponse is the body of the ops GetModTimestamp response. */
export interface OpsGetModTimestampResponse {
  /** UnixfsError contains the error returned by the call, if any. */
  unixfsError: UnixFSError | undefined
  /** ModTimestamp contains the modification timestamp. */
  modTimestamp: Timestamp | undefined
}

/** OpsSetModTimestampRequest is the body of the ops SetModTimestamp request. */
export interface OpsSetModTimestampRequest {
  /** OpsHandleId uniquely identifies the open ops handle. */
  opsHandleId: Long
  /** ModTimestamp is the desired modification timestamp to set. */
  modTimestamp: Timestamp | undefined
}

/** OpsSetModTimestampResponse is the body of the ops SetModTimestamp response. */
export interface OpsSetModTimestampResponse {
  /** UnixfsError contains the error returned by the call, if any. */
  unixfsError: UnixFSError | undefined
}

/** OpsReadAtRequest is the body of the ops ReadAt request. */
export interface OpsReadAtRequest {
  /** OpsHandleId uniquely identifies the open ops handle. */
  opsHandleId: Long
  /** Offset is the location in the file from which to read. */
  offset: Long
  /**
   * Size is the size of data to read (buffer max size).
   * This will automatically be capped by the hardcoded limit of 256e7 (256MB).
   */
  size: Long
}

/** OpsReadAtResponse is the body of the ops ReadAt response. */
export interface OpsReadAtResponse {
  /** UnixfsError contains the error returned by the call, if any. */
  unixfsError: UnixFSError | undefined
  /** Data contains the data read, if any. */
  data: Uint8Array
}

/** OpsGetOptimalWriteSizeRequest is the body of the ops GetOptimalWriteSize request. */
export interface OpsGetOptimalWriteSizeRequest {
  /** OpsHandleId uniquely identifies the open ops handle. */
  opsHandleId: Long
}

/** OpsGetOptimalWriteSizeResponse is the body of the ops GetOptimalWriteSize response. */
export interface OpsGetOptimalWriteSizeResponse {
  /** UnixfsError contains the error returned by the call, if any. */
  unixfsError: UnixFSError | undefined
  /** OptimalWriteSize contains the optimal write size in bytes. */
  optimalWriteSize: Long
}

/** OpsWriteAtRequest is the body of the ops WriteAt request. */
export interface OpsWriteAtRequest {
  /** OpsHandleId uniquely identifies the open ops handle. */
  opsHandleId: Long
  /** Offset is the location in the file at which to write. */
  offset: Long
  /** Data is the chunk of data to write. */
  data: Uint8Array
  /** Timestamp is the desired timestamp for the operation. */
  timestamp: Timestamp | undefined
}

/** OpsWriteAtResponse is the body of the ops WriteAt response. */
export interface OpsWriteAtResponse {
  /** UnixfsError contains the error returned by the call, if any. */
  unixfsError: UnixFSError | undefined
}

/** OpsTruncateRequest is the body of the ops Truncate request. */
export interface OpsTruncateRequest {
  /** OpsHandleId uniquely identifies the open ops handle. */
  opsHandleId: Long
  /** NewSize is the desired new size for the file. */
  nsize: Long
  /** Timestamp is the desired timestamp for the operation. */
  timestamp: Timestamp | undefined
}

/** OpsTruncateResponse is the body of the ops Truncate response. */
export interface OpsTruncateResponse {
  /** UnixfsError contains the error returned by the call, if any. */
  unixfsError: UnixFSError | undefined
}

/** OpsLookupRequest is the body of the ops Lookup request. */
export interface OpsLookupRequest {
  /**
   * CursorHandleId is the identifier for the cursor at the parent location.
   * Must match ops_handle_id or ErrReleased will be returned.
   */
  cursorHandleId: Long
  /** OpsHandleId uniquely identifies the open ops handle. */
  opsHandleId: Long
  /**
   * ClientHandleId is the handle identifier for the client.
   * Call FSCursorClient to get a client handle ID.
   */
  clientHandleId: Long
  /** Name is the name of the child entry to look up. */
  name: string
}

/** OpsLookupResponse is the body of the ops Lookup response. */
export interface OpsLookupResponse {
  /** CursorHandleId is the identifier for the cursor at the new location. */
  cursorHandleId: Long
  /** UnixfsError contains the error returned by the call, if any. */
  unixfsError: UnixFSError | undefined
}

/** OpsReaddirAllRequest is the body of the ops ReaddirAll request. */
export interface OpsReaddirAllRequest {
  /** OpsHandleId uniquely identifies the open ops handle. */
  opsHandleId: Long
  /** Skip is the number of directory entries to skip. */
  skip: Long
}

/** OpsReaddirAllResponse is the body of the ops ReaddirAll response. */
export interface OpsReaddirAllResponse {
  body?:
    | { $case: 'unixfsError'; unixfsError: UnixFSError }
    | { $case: 'done'; done: boolean }
    | {
        $case: 'dirent'
        dirent: FSCursorDirent
      }
    | undefined
}

/** OpsMknodRequest is the body of the ops Mknod request. */
export interface OpsMknodRequest {
  /** OpsHandleId uniquely identifies the open ops handle. */
  opsHandleId: Long
  /** CheckExist indicates if the existence of the node should be checked. */
  checkExist: boolean
  /** Names contains the names of the nodes to be created. */
  names: string[]
  /** NodeType is the type of node to create. */
  nodeType: NodeType
  /** Permissions corresponds to fs.FileMode containing the permissions bits. */
  permissions: number
  /** Timestamp is the desired timestamp for the operation. */
  timestamp: Timestamp | undefined
}

/** OpsMknodResponse is the body of the ops Mknod response. */
export interface OpsMknodResponse {
  /** UnixfsError contains the error returned by the call, if any. */
  unixfsError: UnixFSError | undefined
}

/** OpsSymlinkRequest is the body of the ops Symlink request. */
export interface OpsSymlinkRequest {
  /** OpsHandleId uniquely identifies the open ops handle. */
  opsHandleId: Long
  /** CheckExist indicates if the existence of the node should be checked. */
  checkExist: boolean
  /** Name is the name of the link to be created. */
  name: string
  /** Symlink is the symlink to create. */
  symlink: FSSymlink | undefined
  /** Timestamp is the desired timestamp for the operation. */
  timestamp: Timestamp | undefined
}

/** OpsSymlinkResponse is the body of the ops Symlink response. */
export interface OpsSymlinkResponse {
  /** UnixfsError contains the error returned by the call, if any. */
  unixfsError: UnixFSError | undefined
}

/** OpsReadlinkRequest is the body of the ops Readlink request. */
export interface OpsReadlinkRequest {
  /** OpsHandleId uniquely identifies the open ops handle. */
  opsHandleId: Long
  /** Name is the name of the symbolic link to be read. */
  name: string
}

/** OpsReadlinkResponse is the body of the ops Readlink response. */
export interface OpsReadlinkResponse {
  /** UnixfsError contains the error returned by the call, if any. */
  unixfsError: UnixFSError | undefined
  /** Symlink contains the read symlink. */
  symlink: FSSymlink | undefined
}

/** OpsCopyToRequest is the body of the ops CopyTo request. */
export interface OpsCopyToRequest {
  /** OpsHandleId uniquely identifies the open ops handle. */
  opsHandleId: Long
  /** TargetDirOpsHandleId is the target directory FSCursorOps handle ID for the copy operation. */
  targetDirOpsHandleId: Long
  /** Target name of the inode for the copy operation. */
  targetName: string
  /** Timestamp is the desired timestamp for the operation. */
  timestamp: Timestamp | undefined
}

/** OpsCopyToResponse is the body of the ops CopyTo response. */
export interface OpsCopyToResponse {
  /** UnixfsError contains the error returned by the call, if any. */
  unixfsError: UnixFSError | undefined
  /** Done indicates whether the copy operation is complete. */
  done: boolean
}

/** OpsCopyFromRequest is the body of the ops CopyFrom request. */
export interface OpsCopyFromRequest {
  /** OpsHandleId uniquely identifies the open ops handle. */
  opsHandleId: Long
  /** Name of the inode for the copy operation. */
  name: string
  /** SrcCursorOpsHandleId is the handle identifier for the source cursor ops object for the copy operation. */
  srcCursorOpsHandleId: Long
  /** Timestamp is the desired timestamp for the operation. */
  timestamp: Timestamp | undefined
}

/** OpsCopyFromResponse is the body of the ops CopyFrom response. */
export interface OpsCopyFromResponse {
  /** UnixfsError contains the error returned by the call, if any. */
  unixfsError: UnixFSError | undefined
  /** Done indicates whether the copy operation is complete. */
  done: boolean
}

/** OpsMoveToRequest is the body of the ops MoveTo request. */
export interface OpsMoveToRequest {
  /** OpsHandleId uniquely identifies the open ops handle. */
  opsHandleId: Long
  /** TargetDirOpsHandleId is the handle id for the FSCursorOps for the directory for the move operation. */
  targetDirOpsHandleId: Long
  /** Target name of the inode for the move operation. */
  targetName: string
  /** Timestamp is the desired timestamp for the operation. */
  timestamp: Timestamp | undefined
}

/** OpsMoveToResponse is the body of the ops MoveTo response. */
export interface OpsMoveToResponse {
  /** UnixfsError contains the error returned by the call, if any. */
  unixfsError: UnixFSError | undefined
  /** Done indicates whether the move operation is complete. */
  done: boolean
}

/** OpsMoveFromRequest is the body of the ops MoveFrom request. */
export interface OpsMoveFromRequest {
  /** OpsHandleId uniquely identifies the open ops handle. */
  opsHandleId: Long
  /** Name of the inode for the move operation. */
  name: string
  /** SrcOpsHandleId is the handle id for the FSCursorOps for the source of the move operation. */
  srcOpsHandleId: Long
  /** Timestamp is the desired timestamp for the operation. */
  timestamp: Timestamp | undefined
}

/** OpsMoveFromResponse is the body of the ops MoveFrom response. */
export interface OpsMoveFromResponse {
  /** UnixfsError contains the error returned by the call, if any. */
  unixfsError: UnixFSError | undefined
  /** Done indicates whether the move operation is complete. */
  done: boolean
}

/** OpsRemoveRequest is the body of the ops Remove request. */
export interface OpsRemoveRequest {
  /** OpsHandleId uniquely identifies the open ops handle. */
  opsHandleId: Long
  /** Names is the list of entry names to be removed. */
  names: string[]
  /** Timestamp is the desired timestamp for the operation. */
  timestamp: Timestamp | undefined
}

/** OpsRemoveResponse is the body of the ops Remove response. */
export interface OpsRemoveResponse {
  /** UnixfsError contains the error returned by the call, if any. */
  unixfsError: UnixFSError | undefined
}

function createBaseGetProxyCursorRequest(): GetProxyCursorRequest {
  return { cursorHandleId: Long.UZERO, clientHandleId: Long.UZERO }
}

export const GetProxyCursorRequest = {
  encode(
    message: GetProxyCursorRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (!message.cursorHandleId.isZero()) {
      writer.uint32(8).uint64(message.cursorHandleId)
    }
    if (!message.clientHandleId.isZero()) {
      writer.uint32(16).uint64(message.clientHandleId)
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): GetProxyCursorRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseGetProxyCursorRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.cursorHandleId = reader.uint64() as Long
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.clientHandleId = reader.uint64() as Long
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<GetProxyCursorRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<GetProxyCursorRequest | GetProxyCursorRequest[]>
      | Iterable<GetProxyCursorRequest | GetProxyCursorRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [GetProxyCursorRequest.encode(p).finish()]
        }
      } else {
        yield* [GetProxyCursorRequest.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, GetProxyCursorRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<GetProxyCursorRequest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [GetProxyCursorRequest.decode(p)]
        }
      } else {
        yield* [GetProxyCursorRequest.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): GetProxyCursorRequest {
    return {
      cursorHandleId: isSet(object.cursorHandleId)
        ? Long.fromValue(object.cursorHandleId)
        : Long.UZERO,
      clientHandleId: isSet(object.clientHandleId)
        ? Long.fromValue(object.clientHandleId)
        : Long.UZERO,
    }
  },

  toJSON(message: GetProxyCursorRequest): unknown {
    const obj: any = {}
    if (!message.cursorHandleId.isZero()) {
      obj.cursorHandleId = (message.cursorHandleId || Long.UZERO).toString()
    }
    if (!message.clientHandleId.isZero()) {
      obj.clientHandleId = (message.clientHandleId || Long.UZERO).toString()
    }
    return obj
  },

  create<I extends Exact<DeepPartial<GetProxyCursorRequest>, I>>(
    base?: I,
  ): GetProxyCursorRequest {
    return GetProxyCursorRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<GetProxyCursorRequest>, I>>(
    object: I,
  ): GetProxyCursorRequest {
    const message = createBaseGetProxyCursorRequest()
    message.cursorHandleId =
      object.cursorHandleId !== undefined && object.cursorHandleId !== null
        ? Long.fromValue(object.cursorHandleId)
        : Long.UZERO
    message.clientHandleId =
      object.clientHandleId !== undefined && object.clientHandleId !== null
        ? Long.fromValue(object.clientHandleId)
        : Long.UZERO
    return message
  },
}

function createBaseGetProxyCursorResponse(): GetProxyCursorResponse {
  return { unixfsError: undefined, cursorHandleId: Long.UZERO }
}

export const GetProxyCursorResponse = {
  encode(
    message: GetProxyCursorResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.unixfsError !== undefined) {
      UnixFSError.encode(message.unixfsError, writer.uint32(10).fork()).ldelim()
    }
    if (!message.cursorHandleId.isZero()) {
      writer.uint32(16).uint64(message.cursorHandleId)
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): GetProxyCursorResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseGetProxyCursorResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.unixfsError = UnixFSError.decode(reader, reader.uint32())
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.cursorHandleId = reader.uint64() as Long
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<GetProxyCursorResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<GetProxyCursorResponse | GetProxyCursorResponse[]>
      | Iterable<GetProxyCursorResponse | GetProxyCursorResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [GetProxyCursorResponse.encode(p).finish()]
        }
      } else {
        yield* [GetProxyCursorResponse.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, GetProxyCursorResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<GetProxyCursorResponse> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [GetProxyCursorResponse.decode(p)]
        }
      } else {
        yield* [GetProxyCursorResponse.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): GetProxyCursorResponse {
    return {
      unixfsError: isSet(object.unixfsError)
        ? UnixFSError.fromJSON(object.unixfsError)
        : undefined,
      cursorHandleId: isSet(object.cursorHandleId)
        ? Long.fromValue(object.cursorHandleId)
        : Long.UZERO,
    }
  },

  toJSON(message: GetProxyCursorResponse): unknown {
    const obj: any = {}
    if (message.unixfsError !== undefined) {
      obj.unixfsError = UnixFSError.toJSON(message.unixfsError)
    }
    if (!message.cursorHandleId.isZero()) {
      obj.cursorHandleId = (message.cursorHandleId || Long.UZERO).toString()
    }
    return obj
  },

  create<I extends Exact<DeepPartial<GetProxyCursorResponse>, I>>(
    base?: I,
  ): GetProxyCursorResponse {
    return GetProxyCursorResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<GetProxyCursorResponse>, I>>(
    object: I,
  ): GetProxyCursorResponse {
    const message = createBaseGetProxyCursorResponse()
    message.unixfsError =
      object.unixfsError !== undefined && object.unixfsError !== null
        ? UnixFSError.fromPartial(object.unixfsError)
        : undefined
    message.cursorHandleId =
      object.cursorHandleId !== undefined && object.cursorHandleId !== null
        ? Long.fromValue(object.cursorHandleId)
        : Long.UZERO
    return message
  },
}

function createBaseFSCursorChange(): FSCursorChange {
  return {
    cursorHandleId: Long.UZERO,
    released: false,
    offset: Long.UZERO,
    size: Long.UZERO,
  }
}

export const FSCursorChange = {
  encode(
    message: FSCursorChange,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (!message.cursorHandleId.isZero()) {
      writer.uint32(8).uint64(message.cursorHandleId)
    }
    if (message.released === true) {
      writer.uint32(16).bool(message.released)
    }
    if (!message.offset.isZero()) {
      writer.uint32(24).uint64(message.offset)
    }
    if (!message.size.isZero()) {
      writer.uint32(32).uint64(message.size)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): FSCursorChange {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseFSCursorChange()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.cursorHandleId = reader.uint64() as Long
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.released = reader.bool()
          continue
        case 3:
          if (tag !== 24) {
            break
          }

          message.offset = reader.uint64() as Long
          continue
        case 4:
          if (tag !== 32) {
            break
          }

          message.size = reader.uint64() as Long
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<FSCursorChange, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<FSCursorChange | FSCursorChange[]>
      | Iterable<FSCursorChange | FSCursorChange[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [FSCursorChange.encode(p).finish()]
        }
      } else {
        yield* [FSCursorChange.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, FSCursorChange>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<FSCursorChange> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [FSCursorChange.decode(p)]
        }
      } else {
        yield* [FSCursorChange.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): FSCursorChange {
    return {
      cursorHandleId: isSet(object.cursorHandleId)
        ? Long.fromValue(object.cursorHandleId)
        : Long.UZERO,
      released: isSet(object.released)
        ? globalThis.Boolean(object.released)
        : false,
      offset: isSet(object.offset) ? Long.fromValue(object.offset) : Long.UZERO,
      size: isSet(object.size) ? Long.fromValue(object.size) : Long.UZERO,
    }
  },

  toJSON(message: FSCursorChange): unknown {
    const obj: any = {}
    if (!message.cursorHandleId.isZero()) {
      obj.cursorHandleId = (message.cursorHandleId || Long.UZERO).toString()
    }
    if (message.released === true) {
      obj.released = message.released
    }
    if (!message.offset.isZero()) {
      obj.offset = (message.offset || Long.UZERO).toString()
    }
    if (!message.size.isZero()) {
      obj.size = (message.size || Long.UZERO).toString()
    }
    return obj
  },

  create<I extends Exact<DeepPartial<FSCursorChange>, I>>(
    base?: I,
  ): FSCursorChange {
    return FSCursorChange.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<FSCursorChange>, I>>(
    object: I,
  ): FSCursorChange {
    const message = createBaseFSCursorChange()
    message.cursorHandleId =
      object.cursorHandleId !== undefined && object.cursorHandleId !== null
        ? Long.fromValue(object.cursorHandleId)
        : Long.UZERO
    message.released = object.released ?? false
    message.offset =
      object.offset !== undefined && object.offset !== null
        ? Long.fromValue(object.offset)
        : Long.UZERO
    message.size =
      object.size !== undefined && object.size !== null
        ? Long.fromValue(object.size)
        : Long.UZERO
    return message
  },
}

function createBaseFSCursorDirent(): FSCursorDirent {
  return { name: '', nodeType: 0 }
}

export const FSCursorDirent = {
  encode(
    message: FSCursorDirent,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.name !== '') {
      writer.uint32(10).string(message.name)
    }
    if (message.nodeType !== 0) {
      writer.uint32(16).int32(message.nodeType)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): FSCursorDirent {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseFSCursorDirent()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.name = reader.string()
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.nodeType = reader.int32() as any
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<FSCursorDirent, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<FSCursorDirent | FSCursorDirent[]>
      | Iterable<FSCursorDirent | FSCursorDirent[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [FSCursorDirent.encode(p).finish()]
        }
      } else {
        yield* [FSCursorDirent.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, FSCursorDirent>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<FSCursorDirent> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [FSCursorDirent.decode(p)]
        }
      } else {
        yield* [FSCursorDirent.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): FSCursorDirent {
    return {
      name: isSet(object.name) ? globalThis.String(object.name) : '',
      nodeType: isSet(object.nodeType) ? nodeTypeFromJSON(object.nodeType) : 0,
    }
  },

  toJSON(message: FSCursorDirent): unknown {
    const obj: any = {}
    if (message.name !== '') {
      obj.name = message.name
    }
    if (message.nodeType !== 0) {
      obj.nodeType = nodeTypeToJSON(message.nodeType)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<FSCursorDirent>, I>>(
    base?: I,
  ): FSCursorDirent {
    return FSCursorDirent.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<FSCursorDirent>, I>>(
    object: I,
  ): FSCursorDirent {
    const message = createBaseFSCursorDirent()
    message.name = object.name ?? ''
    message.nodeType = object.nodeType ?? 0
    return message
  },
}

function createBaseFSCursorClientRequest(): FSCursorClientRequest {
  return {}
}

export const FSCursorClientRequest = {
  encode(
    _: FSCursorClientRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): FSCursorClientRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseFSCursorClientRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<FSCursorClientRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<FSCursorClientRequest | FSCursorClientRequest[]>
      | Iterable<FSCursorClientRequest | FSCursorClientRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [FSCursorClientRequest.encode(p).finish()]
        }
      } else {
        yield* [FSCursorClientRequest.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, FSCursorClientRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<FSCursorClientRequest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [FSCursorClientRequest.decode(p)]
        }
      } else {
        yield* [FSCursorClientRequest.decode(pkt as any)]
      }
    }
  },

  fromJSON(_: any): FSCursorClientRequest {
    return {}
  },

  toJSON(_: FSCursorClientRequest): unknown {
    const obj: any = {}
    return obj
  },

  create<I extends Exact<DeepPartial<FSCursorClientRequest>, I>>(
    base?: I,
  ): FSCursorClientRequest {
    return FSCursorClientRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<FSCursorClientRequest>, I>>(
    _: I,
  ): FSCursorClientRequest {
    const message = createBaseFSCursorClientRequest()
    return message
  },
}

function createBaseFSCursorClientResponse(): FSCursorClientResponse {
  return { body: undefined }
}

export const FSCursorClientResponse = {
  encode(
    message: FSCursorClientResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    switch (message.body?.$case) {
      case 'init':
        FSClientInit.encode(
          message.body.init,
          writer.uint32(10).fork(),
        ).ldelim()
        break
      case 'cursorChange':
        FSCursorChange.encode(
          message.body.cursorChange,
          writer.uint32(18).fork(),
        ).ldelim()
        break
      case 'unixfsError':
        UnixFSError.encode(
          message.body.unixfsError,
          writer.uint32(26).fork(),
        ).ldelim()
        break
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): FSCursorClientResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseFSCursorClientResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.body = {
            $case: 'init',
            init: FSClientInit.decode(reader, reader.uint32()),
          }
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.body = {
            $case: 'cursorChange',
            cursorChange: FSCursorChange.decode(reader, reader.uint32()),
          }
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.body = {
            $case: 'unixfsError',
            unixfsError: UnixFSError.decode(reader, reader.uint32()),
          }
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<FSCursorClientResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<FSCursorClientResponse | FSCursorClientResponse[]>
      | Iterable<FSCursorClientResponse | FSCursorClientResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [FSCursorClientResponse.encode(p).finish()]
        }
      } else {
        yield* [FSCursorClientResponse.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, FSCursorClientResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<FSCursorClientResponse> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [FSCursorClientResponse.decode(p)]
        }
      } else {
        yield* [FSCursorClientResponse.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): FSCursorClientResponse {
    return {
      body: isSet(object.init)
        ? { $case: 'init', init: FSClientInit.fromJSON(object.init) }
        : isSet(object.cursorChange)
          ? {
              $case: 'cursorChange',
              cursorChange: FSCursorChange.fromJSON(object.cursorChange),
            }
          : isSet(object.unixfsError)
            ? {
                $case: 'unixfsError',
                unixfsError: UnixFSError.fromJSON(object.unixfsError),
              }
            : undefined,
    }
  },

  toJSON(message: FSCursorClientResponse): unknown {
    const obj: any = {}
    if (message.body?.$case === 'init') {
      obj.init = FSClientInit.toJSON(message.body.init)
    }
    if (message.body?.$case === 'cursorChange') {
      obj.cursorChange = FSCursorChange.toJSON(message.body.cursorChange)
    }
    if (message.body?.$case === 'unixfsError') {
      obj.unixfsError = UnixFSError.toJSON(message.body.unixfsError)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<FSCursorClientResponse>, I>>(
    base?: I,
  ): FSCursorClientResponse {
    return FSCursorClientResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<FSCursorClientResponse>, I>>(
    object: I,
  ): FSCursorClientResponse {
    const message = createBaseFSCursorClientResponse()
    if (
      object.body?.$case === 'init' &&
      object.body?.init !== undefined &&
      object.body?.init !== null
    ) {
      message.body = {
        $case: 'init',
        init: FSClientInit.fromPartial(object.body.init),
      }
    }
    if (
      object.body?.$case === 'cursorChange' &&
      object.body?.cursorChange !== undefined &&
      object.body?.cursorChange !== null
    ) {
      message.body = {
        $case: 'cursorChange',
        cursorChange: FSCursorChange.fromPartial(object.body.cursorChange),
      }
    }
    if (
      object.body?.$case === 'unixfsError' &&
      object.body?.unixfsError !== undefined &&
      object.body?.unixfsError !== null
    ) {
      message.body = {
        $case: 'unixfsError',
        unixfsError: UnixFSError.fromPartial(object.body.unixfsError),
      }
    }
    return message
  },
}

function createBaseFSClientInit(): FSClientInit {
  return { clientHandleId: Long.UZERO, cursorHandleId: Long.UZERO }
}

export const FSClientInit = {
  encode(
    message: FSClientInit,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (!message.clientHandleId.isZero()) {
      writer.uint32(8).uint64(message.clientHandleId)
    }
    if (!message.cursorHandleId.isZero()) {
      writer.uint32(16).uint64(message.cursorHandleId)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): FSClientInit {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseFSClientInit()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.clientHandleId = reader.uint64() as Long
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.cursorHandleId = reader.uint64() as Long
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<FSClientInit, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<FSClientInit | FSClientInit[]>
      | Iterable<FSClientInit | FSClientInit[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [FSClientInit.encode(p).finish()]
        }
      } else {
        yield* [FSClientInit.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, FSClientInit>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<FSClientInit> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [FSClientInit.decode(p)]
        }
      } else {
        yield* [FSClientInit.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): FSClientInit {
    return {
      clientHandleId: isSet(object.clientHandleId)
        ? Long.fromValue(object.clientHandleId)
        : Long.UZERO,
      cursorHandleId: isSet(object.cursorHandleId)
        ? Long.fromValue(object.cursorHandleId)
        : Long.UZERO,
    }
  },

  toJSON(message: FSClientInit): unknown {
    const obj: any = {}
    if (!message.clientHandleId.isZero()) {
      obj.clientHandleId = (message.clientHandleId || Long.UZERO).toString()
    }
    if (!message.cursorHandleId.isZero()) {
      obj.cursorHandleId = (message.cursorHandleId || Long.UZERO).toString()
    }
    return obj
  },

  create<I extends Exact<DeepPartial<FSClientInit>, I>>(
    base?: I,
  ): FSClientInit {
    return FSClientInit.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<FSClientInit>, I>>(
    object: I,
  ): FSClientInit {
    const message = createBaseFSClientInit()
    message.clientHandleId =
      object.clientHandleId !== undefined && object.clientHandleId !== null
        ? Long.fromValue(object.clientHandleId)
        : Long.UZERO
    message.cursorHandleId =
      object.cursorHandleId !== undefined && object.cursorHandleId !== null
        ? Long.fromValue(object.cursorHandleId)
        : Long.UZERO
    return message
  },
}

function createBaseGetCursorOpsRequest(): GetCursorOpsRequest {
  return { cursorHandleId: Long.UZERO }
}

export const GetCursorOpsRequest = {
  encode(
    message: GetCursorOpsRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (!message.cursorHandleId.isZero()) {
      writer.uint32(8).uint64(message.cursorHandleId)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): GetCursorOpsRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseGetCursorOpsRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.cursorHandleId = reader.uint64() as Long
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<GetCursorOpsRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<GetCursorOpsRequest | GetCursorOpsRequest[]>
      | Iterable<GetCursorOpsRequest | GetCursorOpsRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [GetCursorOpsRequest.encode(p).finish()]
        }
      } else {
        yield* [GetCursorOpsRequest.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, GetCursorOpsRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<GetCursorOpsRequest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [GetCursorOpsRequest.decode(p)]
        }
      } else {
        yield* [GetCursorOpsRequest.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): GetCursorOpsRequest {
    return {
      cursorHandleId: isSet(object.cursorHandleId)
        ? Long.fromValue(object.cursorHandleId)
        : Long.UZERO,
    }
  },

  toJSON(message: GetCursorOpsRequest): unknown {
    const obj: any = {}
    if (!message.cursorHandleId.isZero()) {
      obj.cursorHandleId = (message.cursorHandleId || Long.UZERO).toString()
    }
    return obj
  },

  create<I extends Exact<DeepPartial<GetCursorOpsRequest>, I>>(
    base?: I,
  ): GetCursorOpsRequest {
    return GetCursorOpsRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<GetCursorOpsRequest>, I>>(
    object: I,
  ): GetCursorOpsRequest {
    const message = createBaseGetCursorOpsRequest()
    message.cursorHandleId =
      object.cursorHandleId !== undefined && object.cursorHandleId !== null
        ? Long.fromValue(object.cursorHandleId)
        : Long.UZERO
    return message
  },
}

function createBaseGetCursorOpsResponse(): GetCursorOpsResponse {
  return {
    unixfsError: undefined,
    opsHandleId: Long.UZERO,
    name: '',
    nodeType: 0,
  }
}

export const GetCursorOpsResponse = {
  encode(
    message: GetCursorOpsResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.unixfsError !== undefined) {
      UnixFSError.encode(message.unixfsError, writer.uint32(10).fork()).ldelim()
    }
    if (!message.opsHandleId.isZero()) {
      writer.uint32(16).uint64(message.opsHandleId)
    }
    if (message.name !== '') {
      writer.uint32(26).string(message.name)
    }
    if (message.nodeType !== 0) {
      writer.uint32(32).int32(message.nodeType)
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): GetCursorOpsResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseGetCursorOpsResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.unixfsError = UnixFSError.decode(reader, reader.uint32())
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.opsHandleId = reader.uint64() as Long
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.name = reader.string()
          continue
        case 4:
          if (tag !== 32) {
            break
          }

          message.nodeType = reader.int32() as any
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<GetCursorOpsResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<GetCursorOpsResponse | GetCursorOpsResponse[]>
      | Iterable<GetCursorOpsResponse | GetCursorOpsResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [GetCursorOpsResponse.encode(p).finish()]
        }
      } else {
        yield* [GetCursorOpsResponse.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, GetCursorOpsResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<GetCursorOpsResponse> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [GetCursorOpsResponse.decode(p)]
        }
      } else {
        yield* [GetCursorOpsResponse.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): GetCursorOpsResponse {
    return {
      unixfsError: isSet(object.unixfsError)
        ? UnixFSError.fromJSON(object.unixfsError)
        : undefined,
      opsHandleId: isSet(object.opsHandleId)
        ? Long.fromValue(object.opsHandleId)
        : Long.UZERO,
      name: isSet(object.name) ? globalThis.String(object.name) : '',
      nodeType: isSet(object.nodeType) ? nodeTypeFromJSON(object.nodeType) : 0,
    }
  },

  toJSON(message: GetCursorOpsResponse): unknown {
    const obj: any = {}
    if (message.unixfsError !== undefined) {
      obj.unixfsError = UnixFSError.toJSON(message.unixfsError)
    }
    if (!message.opsHandleId.isZero()) {
      obj.opsHandleId = (message.opsHandleId || Long.UZERO).toString()
    }
    if (message.name !== '') {
      obj.name = message.name
    }
    if (message.nodeType !== 0) {
      obj.nodeType = nodeTypeToJSON(message.nodeType)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<GetCursorOpsResponse>, I>>(
    base?: I,
  ): GetCursorOpsResponse {
    return GetCursorOpsResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<GetCursorOpsResponse>, I>>(
    object: I,
  ): GetCursorOpsResponse {
    const message = createBaseGetCursorOpsResponse()
    message.unixfsError =
      object.unixfsError !== undefined && object.unixfsError !== null
        ? UnixFSError.fromPartial(object.unixfsError)
        : undefined
    message.opsHandleId =
      object.opsHandleId !== undefined && object.opsHandleId !== null
        ? Long.fromValue(object.opsHandleId)
        : Long.UZERO
    message.name = object.name ?? ''
    message.nodeType = object.nodeType ?? 0
    return message
  },
}

function createBaseReleaseFSCursorRequest(): ReleaseFSCursorRequest {
  return { cursorHandleId: Long.UZERO, clientHandleId: Long.UZERO }
}

export const ReleaseFSCursorRequest = {
  encode(
    message: ReleaseFSCursorRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (!message.cursorHandleId.isZero()) {
      writer.uint32(8).uint64(message.cursorHandleId)
    }
    if (!message.clientHandleId.isZero()) {
      writer.uint32(16).uint64(message.clientHandleId)
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): ReleaseFSCursorRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseReleaseFSCursorRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.cursorHandleId = reader.uint64() as Long
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.clientHandleId = reader.uint64() as Long
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<ReleaseFSCursorRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ReleaseFSCursorRequest | ReleaseFSCursorRequest[]>
      | Iterable<ReleaseFSCursorRequest | ReleaseFSCursorRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [ReleaseFSCursorRequest.encode(p).finish()]
        }
      } else {
        yield* [ReleaseFSCursorRequest.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ReleaseFSCursorRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ReleaseFSCursorRequest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [ReleaseFSCursorRequest.decode(p)]
        }
      } else {
        yield* [ReleaseFSCursorRequest.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): ReleaseFSCursorRequest {
    return {
      cursorHandleId: isSet(object.cursorHandleId)
        ? Long.fromValue(object.cursorHandleId)
        : Long.UZERO,
      clientHandleId: isSet(object.clientHandleId)
        ? Long.fromValue(object.clientHandleId)
        : Long.UZERO,
    }
  },

  toJSON(message: ReleaseFSCursorRequest): unknown {
    const obj: any = {}
    if (!message.cursorHandleId.isZero()) {
      obj.cursorHandleId = (message.cursorHandleId || Long.UZERO).toString()
    }
    if (!message.clientHandleId.isZero()) {
      obj.clientHandleId = (message.clientHandleId || Long.UZERO).toString()
    }
    return obj
  },

  create<I extends Exact<DeepPartial<ReleaseFSCursorRequest>, I>>(
    base?: I,
  ): ReleaseFSCursorRequest {
    return ReleaseFSCursorRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<ReleaseFSCursorRequest>, I>>(
    object: I,
  ): ReleaseFSCursorRequest {
    const message = createBaseReleaseFSCursorRequest()
    message.cursorHandleId =
      object.cursorHandleId !== undefined && object.cursorHandleId !== null
        ? Long.fromValue(object.cursorHandleId)
        : Long.UZERO
    message.clientHandleId =
      object.clientHandleId !== undefined && object.clientHandleId !== null
        ? Long.fromValue(object.clientHandleId)
        : Long.UZERO
    return message
  },
}

function createBaseReleaseFSCursorResponse(): ReleaseFSCursorResponse {
  return {}
}

export const ReleaseFSCursorResponse = {
  encode(
    _: ReleaseFSCursorResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): ReleaseFSCursorResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseReleaseFSCursorResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<ReleaseFSCursorResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ReleaseFSCursorResponse | ReleaseFSCursorResponse[]>
      | Iterable<ReleaseFSCursorResponse | ReleaseFSCursorResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [ReleaseFSCursorResponse.encode(p).finish()]
        }
      } else {
        yield* [ReleaseFSCursorResponse.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ReleaseFSCursorResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<ReleaseFSCursorResponse> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [ReleaseFSCursorResponse.decode(p)]
        }
      } else {
        yield* [ReleaseFSCursorResponse.decode(pkt as any)]
      }
    }
  },

  fromJSON(_: any): ReleaseFSCursorResponse {
    return {}
  },

  toJSON(_: ReleaseFSCursorResponse): unknown {
    const obj: any = {}
    return obj
  },

  create<I extends Exact<DeepPartial<ReleaseFSCursorResponse>, I>>(
    base?: I,
  ): ReleaseFSCursorResponse {
    return ReleaseFSCursorResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<ReleaseFSCursorResponse>, I>>(
    _: I,
  ): ReleaseFSCursorResponse {
    const message = createBaseReleaseFSCursorResponse()
    return message
  },
}

function createBaseOpsGetPermissionsRequest(): OpsGetPermissionsRequest {
  return { opsHandleId: Long.UZERO }
}

export const OpsGetPermissionsRequest = {
  encode(
    message: OpsGetPermissionsRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (!message.opsHandleId.isZero()) {
      writer.uint32(8).uint64(message.opsHandleId)
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): OpsGetPermissionsRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseOpsGetPermissionsRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.opsHandleId = reader.uint64() as Long
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<OpsGetPermissionsRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<OpsGetPermissionsRequest | OpsGetPermissionsRequest[]>
      | Iterable<OpsGetPermissionsRequest | OpsGetPermissionsRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsGetPermissionsRequest.encode(p).finish()]
        }
      } else {
        yield* [OpsGetPermissionsRequest.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, OpsGetPermissionsRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<OpsGetPermissionsRequest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsGetPermissionsRequest.decode(p)]
        }
      } else {
        yield* [OpsGetPermissionsRequest.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): OpsGetPermissionsRequest {
    return {
      opsHandleId: isSet(object.opsHandleId)
        ? Long.fromValue(object.opsHandleId)
        : Long.UZERO,
    }
  },

  toJSON(message: OpsGetPermissionsRequest): unknown {
    const obj: any = {}
    if (!message.opsHandleId.isZero()) {
      obj.opsHandleId = (message.opsHandleId || Long.UZERO).toString()
    }
    return obj
  },

  create<I extends Exact<DeepPartial<OpsGetPermissionsRequest>, I>>(
    base?: I,
  ): OpsGetPermissionsRequest {
    return OpsGetPermissionsRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<OpsGetPermissionsRequest>, I>>(
    object: I,
  ): OpsGetPermissionsRequest {
    const message = createBaseOpsGetPermissionsRequest()
    message.opsHandleId =
      object.opsHandleId !== undefined && object.opsHandleId !== null
        ? Long.fromValue(object.opsHandleId)
        : Long.UZERO
    return message
  },
}

function createBaseOpsGetPermissionsResponse(): OpsGetPermissionsResponse {
  return { unixfsError: undefined, fileMode: 0 }
}

export const OpsGetPermissionsResponse = {
  encode(
    message: OpsGetPermissionsResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.unixfsError !== undefined) {
      UnixFSError.encode(message.unixfsError, writer.uint32(10).fork()).ldelim()
    }
    if (message.fileMode !== 0) {
      writer.uint32(16).uint32(message.fileMode)
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): OpsGetPermissionsResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseOpsGetPermissionsResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.unixfsError = UnixFSError.decode(reader, reader.uint32())
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.fileMode = reader.uint32()
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<OpsGetPermissionsResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<OpsGetPermissionsResponse | OpsGetPermissionsResponse[]>
      | Iterable<OpsGetPermissionsResponse | OpsGetPermissionsResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsGetPermissionsResponse.encode(p).finish()]
        }
      } else {
        yield* [OpsGetPermissionsResponse.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, OpsGetPermissionsResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<OpsGetPermissionsResponse> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsGetPermissionsResponse.decode(p)]
        }
      } else {
        yield* [OpsGetPermissionsResponse.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): OpsGetPermissionsResponse {
    return {
      unixfsError: isSet(object.unixfsError)
        ? UnixFSError.fromJSON(object.unixfsError)
        : undefined,
      fileMode: isSet(object.fileMode) ? globalThis.Number(object.fileMode) : 0,
    }
  },

  toJSON(message: OpsGetPermissionsResponse): unknown {
    const obj: any = {}
    if (message.unixfsError !== undefined) {
      obj.unixfsError = UnixFSError.toJSON(message.unixfsError)
    }
    if (message.fileMode !== 0) {
      obj.fileMode = Math.round(message.fileMode)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<OpsGetPermissionsResponse>, I>>(
    base?: I,
  ): OpsGetPermissionsResponse {
    return OpsGetPermissionsResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<OpsGetPermissionsResponse>, I>>(
    object: I,
  ): OpsGetPermissionsResponse {
    const message = createBaseOpsGetPermissionsResponse()
    message.unixfsError =
      object.unixfsError !== undefined && object.unixfsError !== null
        ? UnixFSError.fromPartial(object.unixfsError)
        : undefined
    message.fileMode = object.fileMode ?? 0
    return message
  },
}

function createBaseOpsSetPermissionsRequest(): OpsSetPermissionsRequest {
  return { opsHandleId: Long.UZERO, fileMode: 0, timestamp: undefined }
}

export const OpsSetPermissionsRequest = {
  encode(
    message: OpsSetPermissionsRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (!message.opsHandleId.isZero()) {
      writer.uint32(8).uint64(message.opsHandleId)
    }
    if (message.fileMode !== 0) {
      writer.uint32(16).uint32(message.fileMode)
    }
    if (message.timestamp !== undefined) {
      Timestamp.encode(message.timestamp, writer.uint32(26).fork()).ldelim()
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): OpsSetPermissionsRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseOpsSetPermissionsRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.opsHandleId = reader.uint64() as Long
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.fileMode = reader.uint32()
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.timestamp = Timestamp.decode(reader, reader.uint32())
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<OpsSetPermissionsRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<OpsSetPermissionsRequest | OpsSetPermissionsRequest[]>
      | Iterable<OpsSetPermissionsRequest | OpsSetPermissionsRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsSetPermissionsRequest.encode(p).finish()]
        }
      } else {
        yield* [OpsSetPermissionsRequest.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, OpsSetPermissionsRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<OpsSetPermissionsRequest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsSetPermissionsRequest.decode(p)]
        }
      } else {
        yield* [OpsSetPermissionsRequest.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): OpsSetPermissionsRequest {
    return {
      opsHandleId: isSet(object.opsHandleId)
        ? Long.fromValue(object.opsHandleId)
        : Long.UZERO,
      fileMode: isSet(object.fileMode) ? globalThis.Number(object.fileMode) : 0,
      timestamp: isSet(object.timestamp)
        ? Timestamp.fromJSON(object.timestamp)
        : undefined,
    }
  },

  toJSON(message: OpsSetPermissionsRequest): unknown {
    const obj: any = {}
    if (!message.opsHandleId.isZero()) {
      obj.opsHandleId = (message.opsHandleId || Long.UZERO).toString()
    }
    if (message.fileMode !== 0) {
      obj.fileMode = Math.round(message.fileMode)
    }
    if (message.timestamp !== undefined) {
      obj.timestamp = Timestamp.toJSON(message.timestamp)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<OpsSetPermissionsRequest>, I>>(
    base?: I,
  ): OpsSetPermissionsRequest {
    return OpsSetPermissionsRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<OpsSetPermissionsRequest>, I>>(
    object: I,
  ): OpsSetPermissionsRequest {
    const message = createBaseOpsSetPermissionsRequest()
    message.opsHandleId =
      object.opsHandleId !== undefined && object.opsHandleId !== null
        ? Long.fromValue(object.opsHandleId)
        : Long.UZERO
    message.fileMode = object.fileMode ?? 0
    message.timestamp =
      object.timestamp !== undefined && object.timestamp !== null
        ? Timestamp.fromPartial(object.timestamp)
        : undefined
    return message
  },
}

function createBaseOpsSetPermissionsResponse(): OpsSetPermissionsResponse {
  return { unixfsError: undefined }
}

export const OpsSetPermissionsResponse = {
  encode(
    message: OpsSetPermissionsResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.unixfsError !== undefined) {
      UnixFSError.encode(message.unixfsError, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): OpsSetPermissionsResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseOpsSetPermissionsResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.unixfsError = UnixFSError.decode(reader, reader.uint32())
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<OpsSetPermissionsResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<OpsSetPermissionsResponse | OpsSetPermissionsResponse[]>
      | Iterable<OpsSetPermissionsResponse | OpsSetPermissionsResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsSetPermissionsResponse.encode(p).finish()]
        }
      } else {
        yield* [OpsSetPermissionsResponse.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, OpsSetPermissionsResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<OpsSetPermissionsResponse> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsSetPermissionsResponse.decode(p)]
        }
      } else {
        yield* [OpsSetPermissionsResponse.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): OpsSetPermissionsResponse {
    return {
      unixfsError: isSet(object.unixfsError)
        ? UnixFSError.fromJSON(object.unixfsError)
        : undefined,
    }
  },

  toJSON(message: OpsSetPermissionsResponse): unknown {
    const obj: any = {}
    if (message.unixfsError !== undefined) {
      obj.unixfsError = UnixFSError.toJSON(message.unixfsError)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<OpsSetPermissionsResponse>, I>>(
    base?: I,
  ): OpsSetPermissionsResponse {
    return OpsSetPermissionsResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<OpsSetPermissionsResponse>, I>>(
    object: I,
  ): OpsSetPermissionsResponse {
    const message = createBaseOpsSetPermissionsResponse()
    message.unixfsError =
      object.unixfsError !== undefined && object.unixfsError !== null
        ? UnixFSError.fromPartial(object.unixfsError)
        : undefined
    return message
  },
}

function createBaseOpsGetSizeRequest(): OpsGetSizeRequest {
  return { opsHandleId: Long.UZERO }
}

export const OpsGetSizeRequest = {
  encode(
    message: OpsGetSizeRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (!message.opsHandleId.isZero()) {
      writer.uint32(8).uint64(message.opsHandleId)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): OpsGetSizeRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseOpsGetSizeRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.opsHandleId = reader.uint64() as Long
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<OpsGetSizeRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<OpsGetSizeRequest | OpsGetSizeRequest[]>
      | Iterable<OpsGetSizeRequest | OpsGetSizeRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsGetSizeRequest.encode(p).finish()]
        }
      } else {
        yield* [OpsGetSizeRequest.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, OpsGetSizeRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<OpsGetSizeRequest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsGetSizeRequest.decode(p)]
        }
      } else {
        yield* [OpsGetSizeRequest.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): OpsGetSizeRequest {
    return {
      opsHandleId: isSet(object.opsHandleId)
        ? Long.fromValue(object.opsHandleId)
        : Long.UZERO,
    }
  },

  toJSON(message: OpsGetSizeRequest): unknown {
    const obj: any = {}
    if (!message.opsHandleId.isZero()) {
      obj.opsHandleId = (message.opsHandleId || Long.UZERO).toString()
    }
    return obj
  },

  create<I extends Exact<DeepPartial<OpsGetSizeRequest>, I>>(
    base?: I,
  ): OpsGetSizeRequest {
    return OpsGetSizeRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<OpsGetSizeRequest>, I>>(
    object: I,
  ): OpsGetSizeRequest {
    const message = createBaseOpsGetSizeRequest()
    message.opsHandleId =
      object.opsHandleId !== undefined && object.opsHandleId !== null
        ? Long.fromValue(object.opsHandleId)
        : Long.UZERO
    return message
  },
}

function createBaseOpsGetSizeResponse(): OpsGetSizeResponse {
  return { unixfsError: undefined, size: Long.UZERO }
}

export const OpsGetSizeResponse = {
  encode(
    message: OpsGetSizeResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.unixfsError !== undefined) {
      UnixFSError.encode(message.unixfsError, writer.uint32(10).fork()).ldelim()
    }
    if (!message.size.isZero()) {
      writer.uint32(16).uint64(message.size)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): OpsGetSizeResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseOpsGetSizeResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.unixfsError = UnixFSError.decode(reader, reader.uint32())
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.size = reader.uint64() as Long
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<OpsGetSizeResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<OpsGetSizeResponse | OpsGetSizeResponse[]>
      | Iterable<OpsGetSizeResponse | OpsGetSizeResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsGetSizeResponse.encode(p).finish()]
        }
      } else {
        yield* [OpsGetSizeResponse.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, OpsGetSizeResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<OpsGetSizeResponse> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsGetSizeResponse.decode(p)]
        }
      } else {
        yield* [OpsGetSizeResponse.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): OpsGetSizeResponse {
    return {
      unixfsError: isSet(object.unixfsError)
        ? UnixFSError.fromJSON(object.unixfsError)
        : undefined,
      size: isSet(object.size) ? Long.fromValue(object.size) : Long.UZERO,
    }
  },

  toJSON(message: OpsGetSizeResponse): unknown {
    const obj: any = {}
    if (message.unixfsError !== undefined) {
      obj.unixfsError = UnixFSError.toJSON(message.unixfsError)
    }
    if (!message.size.isZero()) {
      obj.size = (message.size || Long.UZERO).toString()
    }
    return obj
  },

  create<I extends Exact<DeepPartial<OpsGetSizeResponse>, I>>(
    base?: I,
  ): OpsGetSizeResponse {
    return OpsGetSizeResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<OpsGetSizeResponse>, I>>(
    object: I,
  ): OpsGetSizeResponse {
    const message = createBaseOpsGetSizeResponse()
    message.unixfsError =
      object.unixfsError !== undefined && object.unixfsError !== null
        ? UnixFSError.fromPartial(object.unixfsError)
        : undefined
    message.size =
      object.size !== undefined && object.size !== null
        ? Long.fromValue(object.size)
        : Long.UZERO
    return message
  },
}

function createBaseOpsGetModTimestampRequest(): OpsGetModTimestampRequest {
  return { opsHandleId: Long.UZERO }
}

export const OpsGetModTimestampRequest = {
  encode(
    message: OpsGetModTimestampRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (!message.opsHandleId.isZero()) {
      writer.uint32(8).uint64(message.opsHandleId)
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): OpsGetModTimestampRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseOpsGetModTimestampRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.opsHandleId = reader.uint64() as Long
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<OpsGetModTimestampRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<OpsGetModTimestampRequest | OpsGetModTimestampRequest[]>
      | Iterable<OpsGetModTimestampRequest | OpsGetModTimestampRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsGetModTimestampRequest.encode(p).finish()]
        }
      } else {
        yield* [OpsGetModTimestampRequest.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, OpsGetModTimestampRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<OpsGetModTimestampRequest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsGetModTimestampRequest.decode(p)]
        }
      } else {
        yield* [OpsGetModTimestampRequest.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): OpsGetModTimestampRequest {
    return {
      opsHandleId: isSet(object.opsHandleId)
        ? Long.fromValue(object.opsHandleId)
        : Long.UZERO,
    }
  },

  toJSON(message: OpsGetModTimestampRequest): unknown {
    const obj: any = {}
    if (!message.opsHandleId.isZero()) {
      obj.opsHandleId = (message.opsHandleId || Long.UZERO).toString()
    }
    return obj
  },

  create<I extends Exact<DeepPartial<OpsGetModTimestampRequest>, I>>(
    base?: I,
  ): OpsGetModTimestampRequest {
    return OpsGetModTimestampRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<OpsGetModTimestampRequest>, I>>(
    object: I,
  ): OpsGetModTimestampRequest {
    const message = createBaseOpsGetModTimestampRequest()
    message.opsHandleId =
      object.opsHandleId !== undefined && object.opsHandleId !== null
        ? Long.fromValue(object.opsHandleId)
        : Long.UZERO
    return message
  },
}

function createBaseOpsGetModTimestampResponse(): OpsGetModTimestampResponse {
  return { unixfsError: undefined, modTimestamp: undefined }
}

export const OpsGetModTimestampResponse = {
  encode(
    message: OpsGetModTimestampResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.unixfsError !== undefined) {
      UnixFSError.encode(message.unixfsError, writer.uint32(10).fork()).ldelim()
    }
    if (message.modTimestamp !== undefined) {
      Timestamp.encode(message.modTimestamp, writer.uint32(18).fork()).ldelim()
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): OpsGetModTimestampResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseOpsGetModTimestampResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.unixfsError = UnixFSError.decode(reader, reader.uint32())
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.modTimestamp = Timestamp.decode(reader, reader.uint32())
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<OpsGetModTimestampResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<OpsGetModTimestampResponse | OpsGetModTimestampResponse[]>
      | Iterable<OpsGetModTimestampResponse | OpsGetModTimestampResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsGetModTimestampResponse.encode(p).finish()]
        }
      } else {
        yield* [OpsGetModTimestampResponse.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, OpsGetModTimestampResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<OpsGetModTimestampResponse> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsGetModTimestampResponse.decode(p)]
        }
      } else {
        yield* [OpsGetModTimestampResponse.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): OpsGetModTimestampResponse {
    return {
      unixfsError: isSet(object.unixfsError)
        ? UnixFSError.fromJSON(object.unixfsError)
        : undefined,
      modTimestamp: isSet(object.modTimestamp)
        ? Timestamp.fromJSON(object.modTimestamp)
        : undefined,
    }
  },

  toJSON(message: OpsGetModTimestampResponse): unknown {
    const obj: any = {}
    if (message.unixfsError !== undefined) {
      obj.unixfsError = UnixFSError.toJSON(message.unixfsError)
    }
    if (message.modTimestamp !== undefined) {
      obj.modTimestamp = Timestamp.toJSON(message.modTimestamp)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<OpsGetModTimestampResponse>, I>>(
    base?: I,
  ): OpsGetModTimestampResponse {
    return OpsGetModTimestampResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<OpsGetModTimestampResponse>, I>>(
    object: I,
  ): OpsGetModTimestampResponse {
    const message = createBaseOpsGetModTimestampResponse()
    message.unixfsError =
      object.unixfsError !== undefined && object.unixfsError !== null
        ? UnixFSError.fromPartial(object.unixfsError)
        : undefined
    message.modTimestamp =
      object.modTimestamp !== undefined && object.modTimestamp !== null
        ? Timestamp.fromPartial(object.modTimestamp)
        : undefined
    return message
  },
}

function createBaseOpsSetModTimestampRequest(): OpsSetModTimestampRequest {
  return { opsHandleId: Long.UZERO, modTimestamp: undefined }
}

export const OpsSetModTimestampRequest = {
  encode(
    message: OpsSetModTimestampRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (!message.opsHandleId.isZero()) {
      writer.uint32(8).uint64(message.opsHandleId)
    }
    if (message.modTimestamp !== undefined) {
      Timestamp.encode(message.modTimestamp, writer.uint32(18).fork()).ldelim()
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): OpsSetModTimestampRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseOpsSetModTimestampRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.opsHandleId = reader.uint64() as Long
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.modTimestamp = Timestamp.decode(reader, reader.uint32())
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<OpsSetModTimestampRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<OpsSetModTimestampRequest | OpsSetModTimestampRequest[]>
      | Iterable<OpsSetModTimestampRequest | OpsSetModTimestampRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsSetModTimestampRequest.encode(p).finish()]
        }
      } else {
        yield* [OpsSetModTimestampRequest.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, OpsSetModTimestampRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<OpsSetModTimestampRequest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsSetModTimestampRequest.decode(p)]
        }
      } else {
        yield* [OpsSetModTimestampRequest.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): OpsSetModTimestampRequest {
    return {
      opsHandleId: isSet(object.opsHandleId)
        ? Long.fromValue(object.opsHandleId)
        : Long.UZERO,
      modTimestamp: isSet(object.modTimestamp)
        ? Timestamp.fromJSON(object.modTimestamp)
        : undefined,
    }
  },

  toJSON(message: OpsSetModTimestampRequest): unknown {
    const obj: any = {}
    if (!message.opsHandleId.isZero()) {
      obj.opsHandleId = (message.opsHandleId || Long.UZERO).toString()
    }
    if (message.modTimestamp !== undefined) {
      obj.modTimestamp = Timestamp.toJSON(message.modTimestamp)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<OpsSetModTimestampRequest>, I>>(
    base?: I,
  ): OpsSetModTimestampRequest {
    return OpsSetModTimestampRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<OpsSetModTimestampRequest>, I>>(
    object: I,
  ): OpsSetModTimestampRequest {
    const message = createBaseOpsSetModTimestampRequest()
    message.opsHandleId =
      object.opsHandleId !== undefined && object.opsHandleId !== null
        ? Long.fromValue(object.opsHandleId)
        : Long.UZERO
    message.modTimestamp =
      object.modTimestamp !== undefined && object.modTimestamp !== null
        ? Timestamp.fromPartial(object.modTimestamp)
        : undefined
    return message
  },
}

function createBaseOpsSetModTimestampResponse(): OpsSetModTimestampResponse {
  return { unixfsError: undefined }
}

export const OpsSetModTimestampResponse = {
  encode(
    message: OpsSetModTimestampResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.unixfsError !== undefined) {
      UnixFSError.encode(message.unixfsError, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): OpsSetModTimestampResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseOpsSetModTimestampResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.unixfsError = UnixFSError.decode(reader, reader.uint32())
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<OpsSetModTimestampResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<OpsSetModTimestampResponse | OpsSetModTimestampResponse[]>
      | Iterable<OpsSetModTimestampResponse | OpsSetModTimestampResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsSetModTimestampResponse.encode(p).finish()]
        }
      } else {
        yield* [OpsSetModTimestampResponse.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, OpsSetModTimestampResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<OpsSetModTimestampResponse> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsSetModTimestampResponse.decode(p)]
        }
      } else {
        yield* [OpsSetModTimestampResponse.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): OpsSetModTimestampResponse {
    return {
      unixfsError: isSet(object.unixfsError)
        ? UnixFSError.fromJSON(object.unixfsError)
        : undefined,
    }
  },

  toJSON(message: OpsSetModTimestampResponse): unknown {
    const obj: any = {}
    if (message.unixfsError !== undefined) {
      obj.unixfsError = UnixFSError.toJSON(message.unixfsError)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<OpsSetModTimestampResponse>, I>>(
    base?: I,
  ): OpsSetModTimestampResponse {
    return OpsSetModTimestampResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<OpsSetModTimestampResponse>, I>>(
    object: I,
  ): OpsSetModTimestampResponse {
    const message = createBaseOpsSetModTimestampResponse()
    message.unixfsError =
      object.unixfsError !== undefined && object.unixfsError !== null
        ? UnixFSError.fromPartial(object.unixfsError)
        : undefined
    return message
  },
}

function createBaseOpsReadAtRequest(): OpsReadAtRequest {
  return { opsHandleId: Long.UZERO, offset: Long.ZERO, size: Long.ZERO }
}

export const OpsReadAtRequest = {
  encode(
    message: OpsReadAtRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (!message.opsHandleId.isZero()) {
      writer.uint32(8).uint64(message.opsHandleId)
    }
    if (!message.offset.isZero()) {
      writer.uint32(16).int64(message.offset)
    }
    if (!message.size.isZero()) {
      writer.uint32(24).int64(message.size)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): OpsReadAtRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseOpsReadAtRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.opsHandleId = reader.uint64() as Long
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.offset = reader.int64() as Long
          continue
        case 3:
          if (tag !== 24) {
            break
          }

          message.size = reader.int64() as Long
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<OpsReadAtRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<OpsReadAtRequest | OpsReadAtRequest[]>
      | Iterable<OpsReadAtRequest | OpsReadAtRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsReadAtRequest.encode(p).finish()]
        }
      } else {
        yield* [OpsReadAtRequest.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, OpsReadAtRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<OpsReadAtRequest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsReadAtRequest.decode(p)]
        }
      } else {
        yield* [OpsReadAtRequest.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): OpsReadAtRequest {
    return {
      opsHandleId: isSet(object.opsHandleId)
        ? Long.fromValue(object.opsHandleId)
        : Long.UZERO,
      offset: isSet(object.offset) ? Long.fromValue(object.offset) : Long.ZERO,
      size: isSet(object.size) ? Long.fromValue(object.size) : Long.ZERO,
    }
  },

  toJSON(message: OpsReadAtRequest): unknown {
    const obj: any = {}
    if (!message.opsHandleId.isZero()) {
      obj.opsHandleId = (message.opsHandleId || Long.UZERO).toString()
    }
    if (!message.offset.isZero()) {
      obj.offset = (message.offset || Long.ZERO).toString()
    }
    if (!message.size.isZero()) {
      obj.size = (message.size || Long.ZERO).toString()
    }
    return obj
  },

  create<I extends Exact<DeepPartial<OpsReadAtRequest>, I>>(
    base?: I,
  ): OpsReadAtRequest {
    return OpsReadAtRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<OpsReadAtRequest>, I>>(
    object: I,
  ): OpsReadAtRequest {
    const message = createBaseOpsReadAtRequest()
    message.opsHandleId =
      object.opsHandleId !== undefined && object.opsHandleId !== null
        ? Long.fromValue(object.opsHandleId)
        : Long.UZERO
    message.offset =
      object.offset !== undefined && object.offset !== null
        ? Long.fromValue(object.offset)
        : Long.ZERO
    message.size =
      object.size !== undefined && object.size !== null
        ? Long.fromValue(object.size)
        : Long.ZERO
    return message
  },
}

function createBaseOpsReadAtResponse(): OpsReadAtResponse {
  return { unixfsError: undefined, data: new Uint8Array(0) }
}

export const OpsReadAtResponse = {
  encode(
    message: OpsReadAtResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.unixfsError !== undefined) {
      UnixFSError.encode(message.unixfsError, writer.uint32(10).fork()).ldelim()
    }
    if (message.data.length !== 0) {
      writer.uint32(18).bytes(message.data)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): OpsReadAtResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseOpsReadAtResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.unixfsError = UnixFSError.decode(reader, reader.uint32())
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.data = reader.bytes()
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<OpsReadAtResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<OpsReadAtResponse | OpsReadAtResponse[]>
      | Iterable<OpsReadAtResponse | OpsReadAtResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsReadAtResponse.encode(p).finish()]
        }
      } else {
        yield* [OpsReadAtResponse.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, OpsReadAtResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<OpsReadAtResponse> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsReadAtResponse.decode(p)]
        }
      } else {
        yield* [OpsReadAtResponse.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): OpsReadAtResponse {
    return {
      unixfsError: isSet(object.unixfsError)
        ? UnixFSError.fromJSON(object.unixfsError)
        : undefined,
      data: isSet(object.data)
        ? bytesFromBase64(object.data)
        : new Uint8Array(0),
    }
  },

  toJSON(message: OpsReadAtResponse): unknown {
    const obj: any = {}
    if (message.unixfsError !== undefined) {
      obj.unixfsError = UnixFSError.toJSON(message.unixfsError)
    }
    if (message.data.length !== 0) {
      obj.data = base64FromBytes(message.data)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<OpsReadAtResponse>, I>>(
    base?: I,
  ): OpsReadAtResponse {
    return OpsReadAtResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<OpsReadAtResponse>, I>>(
    object: I,
  ): OpsReadAtResponse {
    const message = createBaseOpsReadAtResponse()
    message.unixfsError =
      object.unixfsError !== undefined && object.unixfsError !== null
        ? UnixFSError.fromPartial(object.unixfsError)
        : undefined
    message.data = object.data ?? new Uint8Array(0)
    return message
  },
}

function createBaseOpsGetOptimalWriteSizeRequest(): OpsGetOptimalWriteSizeRequest {
  return { opsHandleId: Long.UZERO }
}

export const OpsGetOptimalWriteSizeRequest = {
  encode(
    message: OpsGetOptimalWriteSizeRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (!message.opsHandleId.isZero()) {
      writer.uint32(8).uint64(message.opsHandleId)
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): OpsGetOptimalWriteSizeRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseOpsGetOptimalWriteSizeRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.opsHandleId = reader.uint64() as Long
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<OpsGetOptimalWriteSizeRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<
          OpsGetOptimalWriteSizeRequest | OpsGetOptimalWriteSizeRequest[]
        >
      | Iterable<
          OpsGetOptimalWriteSizeRequest | OpsGetOptimalWriteSizeRequest[]
        >,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsGetOptimalWriteSizeRequest.encode(p).finish()]
        }
      } else {
        yield* [OpsGetOptimalWriteSizeRequest.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, OpsGetOptimalWriteSizeRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<OpsGetOptimalWriteSizeRequest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsGetOptimalWriteSizeRequest.decode(p)]
        }
      } else {
        yield* [OpsGetOptimalWriteSizeRequest.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): OpsGetOptimalWriteSizeRequest {
    return {
      opsHandleId: isSet(object.opsHandleId)
        ? Long.fromValue(object.opsHandleId)
        : Long.UZERO,
    }
  },

  toJSON(message: OpsGetOptimalWriteSizeRequest): unknown {
    const obj: any = {}
    if (!message.opsHandleId.isZero()) {
      obj.opsHandleId = (message.opsHandleId || Long.UZERO).toString()
    }
    return obj
  },

  create<I extends Exact<DeepPartial<OpsGetOptimalWriteSizeRequest>, I>>(
    base?: I,
  ): OpsGetOptimalWriteSizeRequest {
    return OpsGetOptimalWriteSizeRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<OpsGetOptimalWriteSizeRequest>, I>>(
    object: I,
  ): OpsGetOptimalWriteSizeRequest {
    const message = createBaseOpsGetOptimalWriteSizeRequest()
    message.opsHandleId =
      object.opsHandleId !== undefined && object.opsHandleId !== null
        ? Long.fromValue(object.opsHandleId)
        : Long.UZERO
    return message
  },
}

function createBaseOpsGetOptimalWriteSizeResponse(): OpsGetOptimalWriteSizeResponse {
  return { unixfsError: undefined, optimalWriteSize: Long.ZERO }
}

export const OpsGetOptimalWriteSizeResponse = {
  encode(
    message: OpsGetOptimalWriteSizeResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.unixfsError !== undefined) {
      UnixFSError.encode(message.unixfsError, writer.uint32(10).fork()).ldelim()
    }
    if (!message.optimalWriteSize.isZero()) {
      writer.uint32(16).int64(message.optimalWriteSize)
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): OpsGetOptimalWriteSizeResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseOpsGetOptimalWriteSizeResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.unixfsError = UnixFSError.decode(reader, reader.uint32())
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.optimalWriteSize = reader.int64() as Long
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<OpsGetOptimalWriteSizeResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<
          OpsGetOptimalWriteSizeResponse | OpsGetOptimalWriteSizeResponse[]
        >
      | Iterable<
          OpsGetOptimalWriteSizeResponse | OpsGetOptimalWriteSizeResponse[]
        >,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsGetOptimalWriteSizeResponse.encode(p).finish()]
        }
      } else {
        yield* [OpsGetOptimalWriteSizeResponse.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, OpsGetOptimalWriteSizeResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<OpsGetOptimalWriteSizeResponse> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsGetOptimalWriteSizeResponse.decode(p)]
        }
      } else {
        yield* [OpsGetOptimalWriteSizeResponse.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): OpsGetOptimalWriteSizeResponse {
    return {
      unixfsError: isSet(object.unixfsError)
        ? UnixFSError.fromJSON(object.unixfsError)
        : undefined,
      optimalWriteSize: isSet(object.optimalWriteSize)
        ? Long.fromValue(object.optimalWriteSize)
        : Long.ZERO,
    }
  },

  toJSON(message: OpsGetOptimalWriteSizeResponse): unknown {
    const obj: any = {}
    if (message.unixfsError !== undefined) {
      obj.unixfsError = UnixFSError.toJSON(message.unixfsError)
    }
    if (!message.optimalWriteSize.isZero()) {
      obj.optimalWriteSize = (message.optimalWriteSize || Long.ZERO).toString()
    }
    return obj
  },

  create<I extends Exact<DeepPartial<OpsGetOptimalWriteSizeResponse>, I>>(
    base?: I,
  ): OpsGetOptimalWriteSizeResponse {
    return OpsGetOptimalWriteSizeResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<OpsGetOptimalWriteSizeResponse>, I>>(
    object: I,
  ): OpsGetOptimalWriteSizeResponse {
    const message = createBaseOpsGetOptimalWriteSizeResponse()
    message.unixfsError =
      object.unixfsError !== undefined && object.unixfsError !== null
        ? UnixFSError.fromPartial(object.unixfsError)
        : undefined
    message.optimalWriteSize =
      object.optimalWriteSize !== undefined && object.optimalWriteSize !== null
        ? Long.fromValue(object.optimalWriteSize)
        : Long.ZERO
    return message
  },
}

function createBaseOpsWriteAtRequest(): OpsWriteAtRequest {
  return {
    opsHandleId: Long.UZERO,
    offset: Long.ZERO,
    data: new Uint8Array(0),
    timestamp: undefined,
  }
}

export const OpsWriteAtRequest = {
  encode(
    message: OpsWriteAtRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (!message.opsHandleId.isZero()) {
      writer.uint32(8).uint64(message.opsHandleId)
    }
    if (!message.offset.isZero()) {
      writer.uint32(16).int64(message.offset)
    }
    if (message.data.length !== 0) {
      writer.uint32(26).bytes(message.data)
    }
    if (message.timestamp !== undefined) {
      Timestamp.encode(message.timestamp, writer.uint32(34).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): OpsWriteAtRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseOpsWriteAtRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.opsHandleId = reader.uint64() as Long
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.offset = reader.int64() as Long
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.data = reader.bytes()
          continue
        case 4:
          if (tag !== 34) {
            break
          }

          message.timestamp = Timestamp.decode(reader, reader.uint32())
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<OpsWriteAtRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<OpsWriteAtRequest | OpsWriteAtRequest[]>
      | Iterable<OpsWriteAtRequest | OpsWriteAtRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsWriteAtRequest.encode(p).finish()]
        }
      } else {
        yield* [OpsWriteAtRequest.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, OpsWriteAtRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<OpsWriteAtRequest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsWriteAtRequest.decode(p)]
        }
      } else {
        yield* [OpsWriteAtRequest.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): OpsWriteAtRequest {
    return {
      opsHandleId: isSet(object.opsHandleId)
        ? Long.fromValue(object.opsHandleId)
        : Long.UZERO,
      offset: isSet(object.offset) ? Long.fromValue(object.offset) : Long.ZERO,
      data: isSet(object.data)
        ? bytesFromBase64(object.data)
        : new Uint8Array(0),
      timestamp: isSet(object.timestamp)
        ? Timestamp.fromJSON(object.timestamp)
        : undefined,
    }
  },

  toJSON(message: OpsWriteAtRequest): unknown {
    const obj: any = {}
    if (!message.opsHandleId.isZero()) {
      obj.opsHandleId = (message.opsHandleId || Long.UZERO).toString()
    }
    if (!message.offset.isZero()) {
      obj.offset = (message.offset || Long.ZERO).toString()
    }
    if (message.data.length !== 0) {
      obj.data = base64FromBytes(message.data)
    }
    if (message.timestamp !== undefined) {
      obj.timestamp = Timestamp.toJSON(message.timestamp)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<OpsWriteAtRequest>, I>>(
    base?: I,
  ): OpsWriteAtRequest {
    return OpsWriteAtRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<OpsWriteAtRequest>, I>>(
    object: I,
  ): OpsWriteAtRequest {
    const message = createBaseOpsWriteAtRequest()
    message.opsHandleId =
      object.opsHandleId !== undefined && object.opsHandleId !== null
        ? Long.fromValue(object.opsHandleId)
        : Long.UZERO
    message.offset =
      object.offset !== undefined && object.offset !== null
        ? Long.fromValue(object.offset)
        : Long.ZERO
    message.data = object.data ?? new Uint8Array(0)
    message.timestamp =
      object.timestamp !== undefined && object.timestamp !== null
        ? Timestamp.fromPartial(object.timestamp)
        : undefined
    return message
  },
}

function createBaseOpsWriteAtResponse(): OpsWriteAtResponse {
  return { unixfsError: undefined }
}

export const OpsWriteAtResponse = {
  encode(
    message: OpsWriteAtResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.unixfsError !== undefined) {
      UnixFSError.encode(message.unixfsError, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): OpsWriteAtResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseOpsWriteAtResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.unixfsError = UnixFSError.decode(reader, reader.uint32())
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<OpsWriteAtResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<OpsWriteAtResponse | OpsWriteAtResponse[]>
      | Iterable<OpsWriteAtResponse | OpsWriteAtResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsWriteAtResponse.encode(p).finish()]
        }
      } else {
        yield* [OpsWriteAtResponse.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, OpsWriteAtResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<OpsWriteAtResponse> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsWriteAtResponse.decode(p)]
        }
      } else {
        yield* [OpsWriteAtResponse.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): OpsWriteAtResponse {
    return {
      unixfsError: isSet(object.unixfsError)
        ? UnixFSError.fromJSON(object.unixfsError)
        : undefined,
    }
  },

  toJSON(message: OpsWriteAtResponse): unknown {
    const obj: any = {}
    if (message.unixfsError !== undefined) {
      obj.unixfsError = UnixFSError.toJSON(message.unixfsError)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<OpsWriteAtResponse>, I>>(
    base?: I,
  ): OpsWriteAtResponse {
    return OpsWriteAtResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<OpsWriteAtResponse>, I>>(
    object: I,
  ): OpsWriteAtResponse {
    const message = createBaseOpsWriteAtResponse()
    message.unixfsError =
      object.unixfsError !== undefined && object.unixfsError !== null
        ? UnixFSError.fromPartial(object.unixfsError)
        : undefined
    return message
  },
}

function createBaseOpsTruncateRequest(): OpsTruncateRequest {
  return { opsHandleId: Long.UZERO, nsize: Long.UZERO, timestamp: undefined }
}

export const OpsTruncateRequest = {
  encode(
    message: OpsTruncateRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (!message.opsHandleId.isZero()) {
      writer.uint32(8).uint64(message.opsHandleId)
    }
    if (!message.nsize.isZero()) {
      writer.uint32(16).uint64(message.nsize)
    }
    if (message.timestamp !== undefined) {
      Timestamp.encode(message.timestamp, writer.uint32(26).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): OpsTruncateRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseOpsTruncateRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.opsHandleId = reader.uint64() as Long
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.nsize = reader.uint64() as Long
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.timestamp = Timestamp.decode(reader, reader.uint32())
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<OpsTruncateRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<OpsTruncateRequest | OpsTruncateRequest[]>
      | Iterable<OpsTruncateRequest | OpsTruncateRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsTruncateRequest.encode(p).finish()]
        }
      } else {
        yield* [OpsTruncateRequest.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, OpsTruncateRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<OpsTruncateRequest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsTruncateRequest.decode(p)]
        }
      } else {
        yield* [OpsTruncateRequest.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): OpsTruncateRequest {
    return {
      opsHandleId: isSet(object.opsHandleId)
        ? Long.fromValue(object.opsHandleId)
        : Long.UZERO,
      nsize: isSet(object.nsize) ? Long.fromValue(object.nsize) : Long.UZERO,
      timestamp: isSet(object.timestamp)
        ? Timestamp.fromJSON(object.timestamp)
        : undefined,
    }
  },

  toJSON(message: OpsTruncateRequest): unknown {
    const obj: any = {}
    if (!message.opsHandleId.isZero()) {
      obj.opsHandleId = (message.opsHandleId || Long.UZERO).toString()
    }
    if (!message.nsize.isZero()) {
      obj.nsize = (message.nsize || Long.UZERO).toString()
    }
    if (message.timestamp !== undefined) {
      obj.timestamp = Timestamp.toJSON(message.timestamp)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<OpsTruncateRequest>, I>>(
    base?: I,
  ): OpsTruncateRequest {
    return OpsTruncateRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<OpsTruncateRequest>, I>>(
    object: I,
  ): OpsTruncateRequest {
    const message = createBaseOpsTruncateRequest()
    message.opsHandleId =
      object.opsHandleId !== undefined && object.opsHandleId !== null
        ? Long.fromValue(object.opsHandleId)
        : Long.UZERO
    message.nsize =
      object.nsize !== undefined && object.nsize !== null
        ? Long.fromValue(object.nsize)
        : Long.UZERO
    message.timestamp =
      object.timestamp !== undefined && object.timestamp !== null
        ? Timestamp.fromPartial(object.timestamp)
        : undefined
    return message
  },
}

function createBaseOpsTruncateResponse(): OpsTruncateResponse {
  return { unixfsError: undefined }
}

export const OpsTruncateResponse = {
  encode(
    message: OpsTruncateResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.unixfsError !== undefined) {
      UnixFSError.encode(message.unixfsError, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): OpsTruncateResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseOpsTruncateResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.unixfsError = UnixFSError.decode(reader, reader.uint32())
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<OpsTruncateResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<OpsTruncateResponse | OpsTruncateResponse[]>
      | Iterable<OpsTruncateResponse | OpsTruncateResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsTruncateResponse.encode(p).finish()]
        }
      } else {
        yield* [OpsTruncateResponse.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, OpsTruncateResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<OpsTruncateResponse> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsTruncateResponse.decode(p)]
        }
      } else {
        yield* [OpsTruncateResponse.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): OpsTruncateResponse {
    return {
      unixfsError: isSet(object.unixfsError)
        ? UnixFSError.fromJSON(object.unixfsError)
        : undefined,
    }
  },

  toJSON(message: OpsTruncateResponse): unknown {
    const obj: any = {}
    if (message.unixfsError !== undefined) {
      obj.unixfsError = UnixFSError.toJSON(message.unixfsError)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<OpsTruncateResponse>, I>>(
    base?: I,
  ): OpsTruncateResponse {
    return OpsTruncateResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<OpsTruncateResponse>, I>>(
    object: I,
  ): OpsTruncateResponse {
    const message = createBaseOpsTruncateResponse()
    message.unixfsError =
      object.unixfsError !== undefined && object.unixfsError !== null
        ? UnixFSError.fromPartial(object.unixfsError)
        : undefined
    return message
  },
}

function createBaseOpsLookupRequest(): OpsLookupRequest {
  return {
    cursorHandleId: Long.UZERO,
    opsHandleId: Long.UZERO,
    clientHandleId: Long.UZERO,
    name: '',
  }
}

export const OpsLookupRequest = {
  encode(
    message: OpsLookupRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (!message.cursorHandleId.isZero()) {
      writer.uint32(8).uint64(message.cursorHandleId)
    }
    if (!message.opsHandleId.isZero()) {
      writer.uint32(16).uint64(message.opsHandleId)
    }
    if (!message.clientHandleId.isZero()) {
      writer.uint32(24).uint64(message.clientHandleId)
    }
    if (message.name !== '') {
      writer.uint32(34).string(message.name)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): OpsLookupRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseOpsLookupRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.cursorHandleId = reader.uint64() as Long
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.opsHandleId = reader.uint64() as Long
          continue
        case 3:
          if (tag !== 24) {
            break
          }

          message.clientHandleId = reader.uint64() as Long
          continue
        case 4:
          if (tag !== 34) {
            break
          }

          message.name = reader.string()
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<OpsLookupRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<OpsLookupRequest | OpsLookupRequest[]>
      | Iterable<OpsLookupRequest | OpsLookupRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsLookupRequest.encode(p).finish()]
        }
      } else {
        yield* [OpsLookupRequest.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, OpsLookupRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<OpsLookupRequest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsLookupRequest.decode(p)]
        }
      } else {
        yield* [OpsLookupRequest.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): OpsLookupRequest {
    return {
      cursorHandleId: isSet(object.cursorHandleId)
        ? Long.fromValue(object.cursorHandleId)
        : Long.UZERO,
      opsHandleId: isSet(object.opsHandleId)
        ? Long.fromValue(object.opsHandleId)
        : Long.UZERO,
      clientHandleId: isSet(object.clientHandleId)
        ? Long.fromValue(object.clientHandleId)
        : Long.UZERO,
      name: isSet(object.name) ? globalThis.String(object.name) : '',
    }
  },

  toJSON(message: OpsLookupRequest): unknown {
    const obj: any = {}
    if (!message.cursorHandleId.isZero()) {
      obj.cursorHandleId = (message.cursorHandleId || Long.UZERO).toString()
    }
    if (!message.opsHandleId.isZero()) {
      obj.opsHandleId = (message.opsHandleId || Long.UZERO).toString()
    }
    if (!message.clientHandleId.isZero()) {
      obj.clientHandleId = (message.clientHandleId || Long.UZERO).toString()
    }
    if (message.name !== '') {
      obj.name = message.name
    }
    return obj
  },

  create<I extends Exact<DeepPartial<OpsLookupRequest>, I>>(
    base?: I,
  ): OpsLookupRequest {
    return OpsLookupRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<OpsLookupRequest>, I>>(
    object: I,
  ): OpsLookupRequest {
    const message = createBaseOpsLookupRequest()
    message.cursorHandleId =
      object.cursorHandleId !== undefined && object.cursorHandleId !== null
        ? Long.fromValue(object.cursorHandleId)
        : Long.UZERO
    message.opsHandleId =
      object.opsHandleId !== undefined && object.opsHandleId !== null
        ? Long.fromValue(object.opsHandleId)
        : Long.UZERO
    message.clientHandleId =
      object.clientHandleId !== undefined && object.clientHandleId !== null
        ? Long.fromValue(object.clientHandleId)
        : Long.UZERO
    message.name = object.name ?? ''
    return message
  },
}

function createBaseOpsLookupResponse(): OpsLookupResponse {
  return { cursorHandleId: Long.UZERO, unixfsError: undefined }
}

export const OpsLookupResponse = {
  encode(
    message: OpsLookupResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (!message.cursorHandleId.isZero()) {
      writer.uint32(8).uint64(message.cursorHandleId)
    }
    if (message.unixfsError !== undefined) {
      UnixFSError.encode(message.unixfsError, writer.uint32(18).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): OpsLookupResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseOpsLookupResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.cursorHandleId = reader.uint64() as Long
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.unixfsError = UnixFSError.decode(reader, reader.uint32())
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<OpsLookupResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<OpsLookupResponse | OpsLookupResponse[]>
      | Iterable<OpsLookupResponse | OpsLookupResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsLookupResponse.encode(p).finish()]
        }
      } else {
        yield* [OpsLookupResponse.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, OpsLookupResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<OpsLookupResponse> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsLookupResponse.decode(p)]
        }
      } else {
        yield* [OpsLookupResponse.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): OpsLookupResponse {
    return {
      cursorHandleId: isSet(object.cursorHandleId)
        ? Long.fromValue(object.cursorHandleId)
        : Long.UZERO,
      unixfsError: isSet(object.unixfsError)
        ? UnixFSError.fromJSON(object.unixfsError)
        : undefined,
    }
  },

  toJSON(message: OpsLookupResponse): unknown {
    const obj: any = {}
    if (!message.cursorHandleId.isZero()) {
      obj.cursorHandleId = (message.cursorHandleId || Long.UZERO).toString()
    }
    if (message.unixfsError !== undefined) {
      obj.unixfsError = UnixFSError.toJSON(message.unixfsError)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<OpsLookupResponse>, I>>(
    base?: I,
  ): OpsLookupResponse {
    return OpsLookupResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<OpsLookupResponse>, I>>(
    object: I,
  ): OpsLookupResponse {
    const message = createBaseOpsLookupResponse()
    message.cursorHandleId =
      object.cursorHandleId !== undefined && object.cursorHandleId !== null
        ? Long.fromValue(object.cursorHandleId)
        : Long.UZERO
    message.unixfsError =
      object.unixfsError !== undefined && object.unixfsError !== null
        ? UnixFSError.fromPartial(object.unixfsError)
        : undefined
    return message
  },
}

function createBaseOpsReaddirAllRequest(): OpsReaddirAllRequest {
  return { opsHandleId: Long.UZERO, skip: Long.UZERO }
}

export const OpsReaddirAllRequest = {
  encode(
    message: OpsReaddirAllRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (!message.opsHandleId.isZero()) {
      writer.uint32(8).uint64(message.opsHandleId)
    }
    if (!message.skip.isZero()) {
      writer.uint32(16).uint64(message.skip)
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): OpsReaddirAllRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseOpsReaddirAllRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.opsHandleId = reader.uint64() as Long
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.skip = reader.uint64() as Long
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<OpsReaddirAllRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<OpsReaddirAllRequest | OpsReaddirAllRequest[]>
      | Iterable<OpsReaddirAllRequest | OpsReaddirAllRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsReaddirAllRequest.encode(p).finish()]
        }
      } else {
        yield* [OpsReaddirAllRequest.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, OpsReaddirAllRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<OpsReaddirAllRequest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsReaddirAllRequest.decode(p)]
        }
      } else {
        yield* [OpsReaddirAllRequest.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): OpsReaddirAllRequest {
    return {
      opsHandleId: isSet(object.opsHandleId)
        ? Long.fromValue(object.opsHandleId)
        : Long.UZERO,
      skip: isSet(object.skip) ? Long.fromValue(object.skip) : Long.UZERO,
    }
  },

  toJSON(message: OpsReaddirAllRequest): unknown {
    const obj: any = {}
    if (!message.opsHandleId.isZero()) {
      obj.opsHandleId = (message.opsHandleId || Long.UZERO).toString()
    }
    if (!message.skip.isZero()) {
      obj.skip = (message.skip || Long.UZERO).toString()
    }
    return obj
  },

  create<I extends Exact<DeepPartial<OpsReaddirAllRequest>, I>>(
    base?: I,
  ): OpsReaddirAllRequest {
    return OpsReaddirAllRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<OpsReaddirAllRequest>, I>>(
    object: I,
  ): OpsReaddirAllRequest {
    const message = createBaseOpsReaddirAllRequest()
    message.opsHandleId =
      object.opsHandleId !== undefined && object.opsHandleId !== null
        ? Long.fromValue(object.opsHandleId)
        : Long.UZERO
    message.skip =
      object.skip !== undefined && object.skip !== null
        ? Long.fromValue(object.skip)
        : Long.UZERO
    return message
  },
}

function createBaseOpsReaddirAllResponse(): OpsReaddirAllResponse {
  return { body: undefined }
}

export const OpsReaddirAllResponse = {
  encode(
    message: OpsReaddirAllResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    switch (message.body?.$case) {
      case 'unixfsError':
        UnixFSError.encode(
          message.body.unixfsError,
          writer.uint32(10).fork(),
        ).ldelim()
        break
      case 'done':
        writer.uint32(16).bool(message.body.done)
        break
      case 'dirent':
        FSCursorDirent.encode(
          message.body.dirent,
          writer.uint32(26).fork(),
        ).ldelim()
        break
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): OpsReaddirAllResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseOpsReaddirAllResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.body = {
            $case: 'unixfsError',
            unixfsError: UnixFSError.decode(reader, reader.uint32()),
          }
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.body = { $case: 'done', done: reader.bool() }
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.body = {
            $case: 'dirent',
            dirent: FSCursorDirent.decode(reader, reader.uint32()),
          }
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<OpsReaddirAllResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<OpsReaddirAllResponse | OpsReaddirAllResponse[]>
      | Iterable<OpsReaddirAllResponse | OpsReaddirAllResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsReaddirAllResponse.encode(p).finish()]
        }
      } else {
        yield* [OpsReaddirAllResponse.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, OpsReaddirAllResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<OpsReaddirAllResponse> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsReaddirAllResponse.decode(p)]
        }
      } else {
        yield* [OpsReaddirAllResponse.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): OpsReaddirAllResponse {
    return {
      body: isSet(object.unixfsError)
        ? {
            $case: 'unixfsError',
            unixfsError: UnixFSError.fromJSON(object.unixfsError),
          }
        : isSet(object.done)
          ? { $case: 'done', done: globalThis.Boolean(object.done) }
          : isSet(object.dirent)
            ? {
                $case: 'dirent',
                dirent: FSCursorDirent.fromJSON(object.dirent),
              }
            : undefined,
    }
  },

  toJSON(message: OpsReaddirAllResponse): unknown {
    const obj: any = {}
    if (message.body?.$case === 'unixfsError') {
      obj.unixfsError = UnixFSError.toJSON(message.body.unixfsError)
    }
    if (message.body?.$case === 'done') {
      obj.done = message.body.done
    }
    if (message.body?.$case === 'dirent') {
      obj.dirent = FSCursorDirent.toJSON(message.body.dirent)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<OpsReaddirAllResponse>, I>>(
    base?: I,
  ): OpsReaddirAllResponse {
    return OpsReaddirAllResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<OpsReaddirAllResponse>, I>>(
    object: I,
  ): OpsReaddirAllResponse {
    const message = createBaseOpsReaddirAllResponse()
    if (
      object.body?.$case === 'unixfsError' &&
      object.body?.unixfsError !== undefined &&
      object.body?.unixfsError !== null
    ) {
      message.body = {
        $case: 'unixfsError',
        unixfsError: UnixFSError.fromPartial(object.body.unixfsError),
      }
    }
    if (
      object.body?.$case === 'done' &&
      object.body?.done !== undefined &&
      object.body?.done !== null
    ) {
      message.body = { $case: 'done', done: object.body.done }
    }
    if (
      object.body?.$case === 'dirent' &&
      object.body?.dirent !== undefined &&
      object.body?.dirent !== null
    ) {
      message.body = {
        $case: 'dirent',
        dirent: FSCursorDirent.fromPartial(object.body.dirent),
      }
    }
    return message
  },
}

function createBaseOpsMknodRequest(): OpsMknodRequest {
  return {
    opsHandleId: Long.UZERO,
    checkExist: false,
    names: [],
    nodeType: 0,
    permissions: 0,
    timestamp: undefined,
  }
}

export const OpsMknodRequest = {
  encode(
    message: OpsMknodRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (!message.opsHandleId.isZero()) {
      writer.uint32(8).uint64(message.opsHandleId)
    }
    if (message.checkExist === true) {
      writer.uint32(16).bool(message.checkExist)
    }
    for (const v of message.names) {
      writer.uint32(26).string(v!)
    }
    if (message.nodeType !== 0) {
      writer.uint32(32).int32(message.nodeType)
    }
    if (message.permissions !== 0) {
      writer.uint32(40).uint32(message.permissions)
    }
    if (message.timestamp !== undefined) {
      Timestamp.encode(message.timestamp, writer.uint32(50).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): OpsMknodRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseOpsMknodRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.opsHandleId = reader.uint64() as Long
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.checkExist = reader.bool()
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.names.push(reader.string())
          continue
        case 4:
          if (tag !== 32) {
            break
          }

          message.nodeType = reader.int32() as any
          continue
        case 5:
          if (tag !== 40) {
            break
          }

          message.permissions = reader.uint32()
          continue
        case 6:
          if (tag !== 50) {
            break
          }

          message.timestamp = Timestamp.decode(reader, reader.uint32())
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<OpsMknodRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<OpsMknodRequest | OpsMknodRequest[]>
      | Iterable<OpsMknodRequest | OpsMknodRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsMknodRequest.encode(p).finish()]
        }
      } else {
        yield* [OpsMknodRequest.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, OpsMknodRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<OpsMknodRequest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsMknodRequest.decode(p)]
        }
      } else {
        yield* [OpsMknodRequest.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): OpsMknodRequest {
    return {
      opsHandleId: isSet(object.opsHandleId)
        ? Long.fromValue(object.opsHandleId)
        : Long.UZERO,
      checkExist: isSet(object.checkExist)
        ? globalThis.Boolean(object.checkExist)
        : false,
      names: globalThis.Array.isArray(object?.names)
        ? object.names.map((e: any) => globalThis.String(e))
        : [],
      nodeType: isSet(object.nodeType) ? nodeTypeFromJSON(object.nodeType) : 0,
      permissions: isSet(object.permissions)
        ? globalThis.Number(object.permissions)
        : 0,
      timestamp: isSet(object.timestamp)
        ? Timestamp.fromJSON(object.timestamp)
        : undefined,
    }
  },

  toJSON(message: OpsMknodRequest): unknown {
    const obj: any = {}
    if (!message.opsHandleId.isZero()) {
      obj.opsHandleId = (message.opsHandleId || Long.UZERO).toString()
    }
    if (message.checkExist === true) {
      obj.checkExist = message.checkExist
    }
    if (message.names?.length) {
      obj.names = message.names
    }
    if (message.nodeType !== 0) {
      obj.nodeType = nodeTypeToJSON(message.nodeType)
    }
    if (message.permissions !== 0) {
      obj.permissions = Math.round(message.permissions)
    }
    if (message.timestamp !== undefined) {
      obj.timestamp = Timestamp.toJSON(message.timestamp)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<OpsMknodRequest>, I>>(
    base?: I,
  ): OpsMknodRequest {
    return OpsMknodRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<OpsMknodRequest>, I>>(
    object: I,
  ): OpsMknodRequest {
    const message = createBaseOpsMknodRequest()
    message.opsHandleId =
      object.opsHandleId !== undefined && object.opsHandleId !== null
        ? Long.fromValue(object.opsHandleId)
        : Long.UZERO
    message.checkExist = object.checkExist ?? false
    message.names = object.names?.map((e) => e) || []
    message.nodeType = object.nodeType ?? 0
    message.permissions = object.permissions ?? 0
    message.timestamp =
      object.timestamp !== undefined && object.timestamp !== null
        ? Timestamp.fromPartial(object.timestamp)
        : undefined
    return message
  },
}

function createBaseOpsMknodResponse(): OpsMknodResponse {
  return { unixfsError: undefined }
}

export const OpsMknodResponse = {
  encode(
    message: OpsMknodResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.unixfsError !== undefined) {
      UnixFSError.encode(message.unixfsError, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): OpsMknodResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseOpsMknodResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.unixfsError = UnixFSError.decode(reader, reader.uint32())
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<OpsMknodResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<OpsMknodResponse | OpsMknodResponse[]>
      | Iterable<OpsMknodResponse | OpsMknodResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsMknodResponse.encode(p).finish()]
        }
      } else {
        yield* [OpsMknodResponse.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, OpsMknodResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<OpsMknodResponse> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsMknodResponse.decode(p)]
        }
      } else {
        yield* [OpsMknodResponse.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): OpsMknodResponse {
    return {
      unixfsError: isSet(object.unixfsError)
        ? UnixFSError.fromJSON(object.unixfsError)
        : undefined,
    }
  },

  toJSON(message: OpsMknodResponse): unknown {
    const obj: any = {}
    if (message.unixfsError !== undefined) {
      obj.unixfsError = UnixFSError.toJSON(message.unixfsError)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<OpsMknodResponse>, I>>(
    base?: I,
  ): OpsMknodResponse {
    return OpsMknodResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<OpsMknodResponse>, I>>(
    object: I,
  ): OpsMknodResponse {
    const message = createBaseOpsMknodResponse()
    message.unixfsError =
      object.unixfsError !== undefined && object.unixfsError !== null
        ? UnixFSError.fromPartial(object.unixfsError)
        : undefined
    return message
  },
}

function createBaseOpsSymlinkRequest(): OpsSymlinkRequest {
  return {
    opsHandleId: Long.UZERO,
    checkExist: false,
    name: '',
    symlink: undefined,
    timestamp: undefined,
  }
}

export const OpsSymlinkRequest = {
  encode(
    message: OpsSymlinkRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (!message.opsHandleId.isZero()) {
      writer.uint32(8).uint64(message.opsHandleId)
    }
    if (message.checkExist === true) {
      writer.uint32(16).bool(message.checkExist)
    }
    if (message.name !== '') {
      writer.uint32(26).string(message.name)
    }
    if (message.symlink !== undefined) {
      FSSymlink.encode(message.symlink, writer.uint32(34).fork()).ldelim()
    }
    if (message.timestamp !== undefined) {
      Timestamp.encode(message.timestamp, writer.uint32(42).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): OpsSymlinkRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseOpsSymlinkRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.opsHandleId = reader.uint64() as Long
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.checkExist = reader.bool()
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.name = reader.string()
          continue
        case 4:
          if (tag !== 34) {
            break
          }

          message.symlink = FSSymlink.decode(reader, reader.uint32())
          continue
        case 5:
          if (tag !== 42) {
            break
          }

          message.timestamp = Timestamp.decode(reader, reader.uint32())
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<OpsSymlinkRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<OpsSymlinkRequest | OpsSymlinkRequest[]>
      | Iterable<OpsSymlinkRequest | OpsSymlinkRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsSymlinkRequest.encode(p).finish()]
        }
      } else {
        yield* [OpsSymlinkRequest.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, OpsSymlinkRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<OpsSymlinkRequest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsSymlinkRequest.decode(p)]
        }
      } else {
        yield* [OpsSymlinkRequest.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): OpsSymlinkRequest {
    return {
      opsHandleId: isSet(object.opsHandleId)
        ? Long.fromValue(object.opsHandleId)
        : Long.UZERO,
      checkExist: isSet(object.checkExist)
        ? globalThis.Boolean(object.checkExist)
        : false,
      name: isSet(object.name) ? globalThis.String(object.name) : '',
      symlink: isSet(object.symlink)
        ? FSSymlink.fromJSON(object.symlink)
        : undefined,
      timestamp: isSet(object.timestamp)
        ? Timestamp.fromJSON(object.timestamp)
        : undefined,
    }
  },

  toJSON(message: OpsSymlinkRequest): unknown {
    const obj: any = {}
    if (!message.opsHandleId.isZero()) {
      obj.opsHandleId = (message.opsHandleId || Long.UZERO).toString()
    }
    if (message.checkExist === true) {
      obj.checkExist = message.checkExist
    }
    if (message.name !== '') {
      obj.name = message.name
    }
    if (message.symlink !== undefined) {
      obj.symlink = FSSymlink.toJSON(message.symlink)
    }
    if (message.timestamp !== undefined) {
      obj.timestamp = Timestamp.toJSON(message.timestamp)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<OpsSymlinkRequest>, I>>(
    base?: I,
  ): OpsSymlinkRequest {
    return OpsSymlinkRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<OpsSymlinkRequest>, I>>(
    object: I,
  ): OpsSymlinkRequest {
    const message = createBaseOpsSymlinkRequest()
    message.opsHandleId =
      object.opsHandleId !== undefined && object.opsHandleId !== null
        ? Long.fromValue(object.opsHandleId)
        : Long.UZERO
    message.checkExist = object.checkExist ?? false
    message.name = object.name ?? ''
    message.symlink =
      object.symlink !== undefined && object.symlink !== null
        ? FSSymlink.fromPartial(object.symlink)
        : undefined
    message.timestamp =
      object.timestamp !== undefined && object.timestamp !== null
        ? Timestamp.fromPartial(object.timestamp)
        : undefined
    return message
  },
}

function createBaseOpsSymlinkResponse(): OpsSymlinkResponse {
  return { unixfsError: undefined }
}

export const OpsSymlinkResponse = {
  encode(
    message: OpsSymlinkResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.unixfsError !== undefined) {
      UnixFSError.encode(message.unixfsError, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): OpsSymlinkResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseOpsSymlinkResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.unixfsError = UnixFSError.decode(reader, reader.uint32())
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<OpsSymlinkResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<OpsSymlinkResponse | OpsSymlinkResponse[]>
      | Iterable<OpsSymlinkResponse | OpsSymlinkResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsSymlinkResponse.encode(p).finish()]
        }
      } else {
        yield* [OpsSymlinkResponse.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, OpsSymlinkResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<OpsSymlinkResponse> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsSymlinkResponse.decode(p)]
        }
      } else {
        yield* [OpsSymlinkResponse.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): OpsSymlinkResponse {
    return {
      unixfsError: isSet(object.unixfsError)
        ? UnixFSError.fromJSON(object.unixfsError)
        : undefined,
    }
  },

  toJSON(message: OpsSymlinkResponse): unknown {
    const obj: any = {}
    if (message.unixfsError !== undefined) {
      obj.unixfsError = UnixFSError.toJSON(message.unixfsError)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<OpsSymlinkResponse>, I>>(
    base?: I,
  ): OpsSymlinkResponse {
    return OpsSymlinkResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<OpsSymlinkResponse>, I>>(
    object: I,
  ): OpsSymlinkResponse {
    const message = createBaseOpsSymlinkResponse()
    message.unixfsError =
      object.unixfsError !== undefined && object.unixfsError !== null
        ? UnixFSError.fromPartial(object.unixfsError)
        : undefined
    return message
  },
}

function createBaseOpsReadlinkRequest(): OpsReadlinkRequest {
  return { opsHandleId: Long.UZERO, name: '' }
}

export const OpsReadlinkRequest = {
  encode(
    message: OpsReadlinkRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (!message.opsHandleId.isZero()) {
      writer.uint32(8).uint64(message.opsHandleId)
    }
    if (message.name !== '') {
      writer.uint32(18).string(message.name)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): OpsReadlinkRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseOpsReadlinkRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.opsHandleId = reader.uint64() as Long
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.name = reader.string()
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<OpsReadlinkRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<OpsReadlinkRequest | OpsReadlinkRequest[]>
      | Iterable<OpsReadlinkRequest | OpsReadlinkRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsReadlinkRequest.encode(p).finish()]
        }
      } else {
        yield* [OpsReadlinkRequest.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, OpsReadlinkRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<OpsReadlinkRequest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsReadlinkRequest.decode(p)]
        }
      } else {
        yield* [OpsReadlinkRequest.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): OpsReadlinkRequest {
    return {
      opsHandleId: isSet(object.opsHandleId)
        ? Long.fromValue(object.opsHandleId)
        : Long.UZERO,
      name: isSet(object.name) ? globalThis.String(object.name) : '',
    }
  },

  toJSON(message: OpsReadlinkRequest): unknown {
    const obj: any = {}
    if (!message.opsHandleId.isZero()) {
      obj.opsHandleId = (message.opsHandleId || Long.UZERO).toString()
    }
    if (message.name !== '') {
      obj.name = message.name
    }
    return obj
  },

  create<I extends Exact<DeepPartial<OpsReadlinkRequest>, I>>(
    base?: I,
  ): OpsReadlinkRequest {
    return OpsReadlinkRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<OpsReadlinkRequest>, I>>(
    object: I,
  ): OpsReadlinkRequest {
    const message = createBaseOpsReadlinkRequest()
    message.opsHandleId =
      object.opsHandleId !== undefined && object.opsHandleId !== null
        ? Long.fromValue(object.opsHandleId)
        : Long.UZERO
    message.name = object.name ?? ''
    return message
  },
}

function createBaseOpsReadlinkResponse(): OpsReadlinkResponse {
  return { unixfsError: undefined, symlink: undefined }
}

export const OpsReadlinkResponse = {
  encode(
    message: OpsReadlinkResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.unixfsError !== undefined) {
      UnixFSError.encode(message.unixfsError, writer.uint32(10).fork()).ldelim()
    }
    if (message.symlink !== undefined) {
      FSSymlink.encode(message.symlink, writer.uint32(18).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): OpsReadlinkResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseOpsReadlinkResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.unixfsError = UnixFSError.decode(reader, reader.uint32())
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.symlink = FSSymlink.decode(reader, reader.uint32())
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<OpsReadlinkResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<OpsReadlinkResponse | OpsReadlinkResponse[]>
      | Iterable<OpsReadlinkResponse | OpsReadlinkResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsReadlinkResponse.encode(p).finish()]
        }
      } else {
        yield* [OpsReadlinkResponse.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, OpsReadlinkResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<OpsReadlinkResponse> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsReadlinkResponse.decode(p)]
        }
      } else {
        yield* [OpsReadlinkResponse.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): OpsReadlinkResponse {
    return {
      unixfsError: isSet(object.unixfsError)
        ? UnixFSError.fromJSON(object.unixfsError)
        : undefined,
      symlink: isSet(object.symlink)
        ? FSSymlink.fromJSON(object.symlink)
        : undefined,
    }
  },

  toJSON(message: OpsReadlinkResponse): unknown {
    const obj: any = {}
    if (message.unixfsError !== undefined) {
      obj.unixfsError = UnixFSError.toJSON(message.unixfsError)
    }
    if (message.symlink !== undefined) {
      obj.symlink = FSSymlink.toJSON(message.symlink)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<OpsReadlinkResponse>, I>>(
    base?: I,
  ): OpsReadlinkResponse {
    return OpsReadlinkResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<OpsReadlinkResponse>, I>>(
    object: I,
  ): OpsReadlinkResponse {
    const message = createBaseOpsReadlinkResponse()
    message.unixfsError =
      object.unixfsError !== undefined && object.unixfsError !== null
        ? UnixFSError.fromPartial(object.unixfsError)
        : undefined
    message.symlink =
      object.symlink !== undefined && object.symlink !== null
        ? FSSymlink.fromPartial(object.symlink)
        : undefined
    return message
  },
}

function createBaseOpsCopyToRequest(): OpsCopyToRequest {
  return {
    opsHandleId: Long.UZERO,
    targetDirOpsHandleId: Long.UZERO,
    targetName: '',
    timestamp: undefined,
  }
}

export const OpsCopyToRequest = {
  encode(
    message: OpsCopyToRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (!message.opsHandleId.isZero()) {
      writer.uint32(8).uint64(message.opsHandleId)
    }
    if (!message.targetDirOpsHandleId.isZero()) {
      writer.uint32(16).uint64(message.targetDirOpsHandleId)
    }
    if (message.targetName !== '') {
      writer.uint32(26).string(message.targetName)
    }
    if (message.timestamp !== undefined) {
      Timestamp.encode(message.timestamp, writer.uint32(34).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): OpsCopyToRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseOpsCopyToRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.opsHandleId = reader.uint64() as Long
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.targetDirOpsHandleId = reader.uint64() as Long
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.targetName = reader.string()
          continue
        case 4:
          if (tag !== 34) {
            break
          }

          message.timestamp = Timestamp.decode(reader, reader.uint32())
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<OpsCopyToRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<OpsCopyToRequest | OpsCopyToRequest[]>
      | Iterable<OpsCopyToRequest | OpsCopyToRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsCopyToRequest.encode(p).finish()]
        }
      } else {
        yield* [OpsCopyToRequest.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, OpsCopyToRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<OpsCopyToRequest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsCopyToRequest.decode(p)]
        }
      } else {
        yield* [OpsCopyToRequest.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): OpsCopyToRequest {
    return {
      opsHandleId: isSet(object.opsHandleId)
        ? Long.fromValue(object.opsHandleId)
        : Long.UZERO,
      targetDirOpsHandleId: isSet(object.targetDirOpsHandleId)
        ? Long.fromValue(object.targetDirOpsHandleId)
        : Long.UZERO,
      targetName: isSet(object.targetName)
        ? globalThis.String(object.targetName)
        : '',
      timestamp: isSet(object.timestamp)
        ? Timestamp.fromJSON(object.timestamp)
        : undefined,
    }
  },

  toJSON(message: OpsCopyToRequest): unknown {
    const obj: any = {}
    if (!message.opsHandleId.isZero()) {
      obj.opsHandleId = (message.opsHandleId || Long.UZERO).toString()
    }
    if (!message.targetDirOpsHandleId.isZero()) {
      obj.targetDirOpsHandleId = (
        message.targetDirOpsHandleId || Long.UZERO
      ).toString()
    }
    if (message.targetName !== '') {
      obj.targetName = message.targetName
    }
    if (message.timestamp !== undefined) {
      obj.timestamp = Timestamp.toJSON(message.timestamp)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<OpsCopyToRequest>, I>>(
    base?: I,
  ): OpsCopyToRequest {
    return OpsCopyToRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<OpsCopyToRequest>, I>>(
    object: I,
  ): OpsCopyToRequest {
    const message = createBaseOpsCopyToRequest()
    message.opsHandleId =
      object.opsHandleId !== undefined && object.opsHandleId !== null
        ? Long.fromValue(object.opsHandleId)
        : Long.UZERO
    message.targetDirOpsHandleId =
      object.targetDirOpsHandleId !== undefined &&
      object.targetDirOpsHandleId !== null
        ? Long.fromValue(object.targetDirOpsHandleId)
        : Long.UZERO
    message.targetName = object.targetName ?? ''
    message.timestamp =
      object.timestamp !== undefined && object.timestamp !== null
        ? Timestamp.fromPartial(object.timestamp)
        : undefined
    return message
  },
}

function createBaseOpsCopyToResponse(): OpsCopyToResponse {
  return { unixfsError: undefined, done: false }
}

export const OpsCopyToResponse = {
  encode(
    message: OpsCopyToResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.unixfsError !== undefined) {
      UnixFSError.encode(message.unixfsError, writer.uint32(10).fork()).ldelim()
    }
    if (message.done === true) {
      writer.uint32(16).bool(message.done)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): OpsCopyToResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseOpsCopyToResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.unixfsError = UnixFSError.decode(reader, reader.uint32())
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.done = reader.bool()
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<OpsCopyToResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<OpsCopyToResponse | OpsCopyToResponse[]>
      | Iterable<OpsCopyToResponse | OpsCopyToResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsCopyToResponse.encode(p).finish()]
        }
      } else {
        yield* [OpsCopyToResponse.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, OpsCopyToResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<OpsCopyToResponse> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsCopyToResponse.decode(p)]
        }
      } else {
        yield* [OpsCopyToResponse.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): OpsCopyToResponse {
    return {
      unixfsError: isSet(object.unixfsError)
        ? UnixFSError.fromJSON(object.unixfsError)
        : undefined,
      done: isSet(object.done) ? globalThis.Boolean(object.done) : false,
    }
  },

  toJSON(message: OpsCopyToResponse): unknown {
    const obj: any = {}
    if (message.unixfsError !== undefined) {
      obj.unixfsError = UnixFSError.toJSON(message.unixfsError)
    }
    if (message.done === true) {
      obj.done = message.done
    }
    return obj
  },

  create<I extends Exact<DeepPartial<OpsCopyToResponse>, I>>(
    base?: I,
  ): OpsCopyToResponse {
    return OpsCopyToResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<OpsCopyToResponse>, I>>(
    object: I,
  ): OpsCopyToResponse {
    const message = createBaseOpsCopyToResponse()
    message.unixfsError =
      object.unixfsError !== undefined && object.unixfsError !== null
        ? UnixFSError.fromPartial(object.unixfsError)
        : undefined
    message.done = object.done ?? false
    return message
  },
}

function createBaseOpsCopyFromRequest(): OpsCopyFromRequest {
  return {
    opsHandleId: Long.UZERO,
    name: '',
    srcCursorOpsHandleId: Long.UZERO,
    timestamp: undefined,
  }
}

export const OpsCopyFromRequest = {
  encode(
    message: OpsCopyFromRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (!message.opsHandleId.isZero()) {
      writer.uint32(8).uint64(message.opsHandleId)
    }
    if (message.name !== '') {
      writer.uint32(18).string(message.name)
    }
    if (!message.srcCursorOpsHandleId.isZero()) {
      writer.uint32(24).uint64(message.srcCursorOpsHandleId)
    }
    if (message.timestamp !== undefined) {
      Timestamp.encode(message.timestamp, writer.uint32(34).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): OpsCopyFromRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseOpsCopyFromRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.opsHandleId = reader.uint64() as Long
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.name = reader.string()
          continue
        case 3:
          if (tag !== 24) {
            break
          }

          message.srcCursorOpsHandleId = reader.uint64() as Long
          continue
        case 4:
          if (tag !== 34) {
            break
          }

          message.timestamp = Timestamp.decode(reader, reader.uint32())
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<OpsCopyFromRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<OpsCopyFromRequest | OpsCopyFromRequest[]>
      | Iterable<OpsCopyFromRequest | OpsCopyFromRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsCopyFromRequest.encode(p).finish()]
        }
      } else {
        yield* [OpsCopyFromRequest.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, OpsCopyFromRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<OpsCopyFromRequest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsCopyFromRequest.decode(p)]
        }
      } else {
        yield* [OpsCopyFromRequest.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): OpsCopyFromRequest {
    return {
      opsHandleId: isSet(object.opsHandleId)
        ? Long.fromValue(object.opsHandleId)
        : Long.UZERO,
      name: isSet(object.name) ? globalThis.String(object.name) : '',
      srcCursorOpsHandleId: isSet(object.srcCursorOpsHandleId)
        ? Long.fromValue(object.srcCursorOpsHandleId)
        : Long.UZERO,
      timestamp: isSet(object.timestamp)
        ? Timestamp.fromJSON(object.timestamp)
        : undefined,
    }
  },

  toJSON(message: OpsCopyFromRequest): unknown {
    const obj: any = {}
    if (!message.opsHandleId.isZero()) {
      obj.opsHandleId = (message.opsHandleId || Long.UZERO).toString()
    }
    if (message.name !== '') {
      obj.name = message.name
    }
    if (!message.srcCursorOpsHandleId.isZero()) {
      obj.srcCursorOpsHandleId = (
        message.srcCursorOpsHandleId || Long.UZERO
      ).toString()
    }
    if (message.timestamp !== undefined) {
      obj.timestamp = Timestamp.toJSON(message.timestamp)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<OpsCopyFromRequest>, I>>(
    base?: I,
  ): OpsCopyFromRequest {
    return OpsCopyFromRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<OpsCopyFromRequest>, I>>(
    object: I,
  ): OpsCopyFromRequest {
    const message = createBaseOpsCopyFromRequest()
    message.opsHandleId =
      object.opsHandleId !== undefined && object.opsHandleId !== null
        ? Long.fromValue(object.opsHandleId)
        : Long.UZERO
    message.name = object.name ?? ''
    message.srcCursorOpsHandleId =
      object.srcCursorOpsHandleId !== undefined &&
      object.srcCursorOpsHandleId !== null
        ? Long.fromValue(object.srcCursorOpsHandleId)
        : Long.UZERO
    message.timestamp =
      object.timestamp !== undefined && object.timestamp !== null
        ? Timestamp.fromPartial(object.timestamp)
        : undefined
    return message
  },
}

function createBaseOpsCopyFromResponse(): OpsCopyFromResponse {
  return { unixfsError: undefined, done: false }
}

export const OpsCopyFromResponse = {
  encode(
    message: OpsCopyFromResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.unixfsError !== undefined) {
      UnixFSError.encode(message.unixfsError, writer.uint32(10).fork()).ldelim()
    }
    if (message.done === true) {
      writer.uint32(16).bool(message.done)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): OpsCopyFromResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseOpsCopyFromResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.unixfsError = UnixFSError.decode(reader, reader.uint32())
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.done = reader.bool()
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<OpsCopyFromResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<OpsCopyFromResponse | OpsCopyFromResponse[]>
      | Iterable<OpsCopyFromResponse | OpsCopyFromResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsCopyFromResponse.encode(p).finish()]
        }
      } else {
        yield* [OpsCopyFromResponse.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, OpsCopyFromResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<OpsCopyFromResponse> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsCopyFromResponse.decode(p)]
        }
      } else {
        yield* [OpsCopyFromResponse.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): OpsCopyFromResponse {
    return {
      unixfsError: isSet(object.unixfsError)
        ? UnixFSError.fromJSON(object.unixfsError)
        : undefined,
      done: isSet(object.done) ? globalThis.Boolean(object.done) : false,
    }
  },

  toJSON(message: OpsCopyFromResponse): unknown {
    const obj: any = {}
    if (message.unixfsError !== undefined) {
      obj.unixfsError = UnixFSError.toJSON(message.unixfsError)
    }
    if (message.done === true) {
      obj.done = message.done
    }
    return obj
  },

  create<I extends Exact<DeepPartial<OpsCopyFromResponse>, I>>(
    base?: I,
  ): OpsCopyFromResponse {
    return OpsCopyFromResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<OpsCopyFromResponse>, I>>(
    object: I,
  ): OpsCopyFromResponse {
    const message = createBaseOpsCopyFromResponse()
    message.unixfsError =
      object.unixfsError !== undefined && object.unixfsError !== null
        ? UnixFSError.fromPartial(object.unixfsError)
        : undefined
    message.done = object.done ?? false
    return message
  },
}

function createBaseOpsMoveToRequest(): OpsMoveToRequest {
  return {
    opsHandleId: Long.UZERO,
    targetDirOpsHandleId: Long.UZERO,
    targetName: '',
    timestamp: undefined,
  }
}

export const OpsMoveToRequest = {
  encode(
    message: OpsMoveToRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (!message.opsHandleId.isZero()) {
      writer.uint32(8).uint64(message.opsHandleId)
    }
    if (!message.targetDirOpsHandleId.isZero()) {
      writer.uint32(16).uint64(message.targetDirOpsHandleId)
    }
    if (message.targetName !== '') {
      writer.uint32(26).string(message.targetName)
    }
    if (message.timestamp !== undefined) {
      Timestamp.encode(message.timestamp, writer.uint32(34).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): OpsMoveToRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseOpsMoveToRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.opsHandleId = reader.uint64() as Long
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.targetDirOpsHandleId = reader.uint64() as Long
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.targetName = reader.string()
          continue
        case 4:
          if (tag !== 34) {
            break
          }

          message.timestamp = Timestamp.decode(reader, reader.uint32())
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<OpsMoveToRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<OpsMoveToRequest | OpsMoveToRequest[]>
      | Iterable<OpsMoveToRequest | OpsMoveToRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsMoveToRequest.encode(p).finish()]
        }
      } else {
        yield* [OpsMoveToRequest.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, OpsMoveToRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<OpsMoveToRequest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsMoveToRequest.decode(p)]
        }
      } else {
        yield* [OpsMoveToRequest.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): OpsMoveToRequest {
    return {
      opsHandleId: isSet(object.opsHandleId)
        ? Long.fromValue(object.opsHandleId)
        : Long.UZERO,
      targetDirOpsHandleId: isSet(object.targetDirOpsHandleId)
        ? Long.fromValue(object.targetDirOpsHandleId)
        : Long.UZERO,
      targetName: isSet(object.targetName)
        ? globalThis.String(object.targetName)
        : '',
      timestamp: isSet(object.timestamp)
        ? Timestamp.fromJSON(object.timestamp)
        : undefined,
    }
  },

  toJSON(message: OpsMoveToRequest): unknown {
    const obj: any = {}
    if (!message.opsHandleId.isZero()) {
      obj.opsHandleId = (message.opsHandleId || Long.UZERO).toString()
    }
    if (!message.targetDirOpsHandleId.isZero()) {
      obj.targetDirOpsHandleId = (
        message.targetDirOpsHandleId || Long.UZERO
      ).toString()
    }
    if (message.targetName !== '') {
      obj.targetName = message.targetName
    }
    if (message.timestamp !== undefined) {
      obj.timestamp = Timestamp.toJSON(message.timestamp)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<OpsMoveToRequest>, I>>(
    base?: I,
  ): OpsMoveToRequest {
    return OpsMoveToRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<OpsMoveToRequest>, I>>(
    object: I,
  ): OpsMoveToRequest {
    const message = createBaseOpsMoveToRequest()
    message.opsHandleId =
      object.opsHandleId !== undefined && object.opsHandleId !== null
        ? Long.fromValue(object.opsHandleId)
        : Long.UZERO
    message.targetDirOpsHandleId =
      object.targetDirOpsHandleId !== undefined &&
      object.targetDirOpsHandleId !== null
        ? Long.fromValue(object.targetDirOpsHandleId)
        : Long.UZERO
    message.targetName = object.targetName ?? ''
    message.timestamp =
      object.timestamp !== undefined && object.timestamp !== null
        ? Timestamp.fromPartial(object.timestamp)
        : undefined
    return message
  },
}

function createBaseOpsMoveToResponse(): OpsMoveToResponse {
  return { unixfsError: undefined, done: false }
}

export const OpsMoveToResponse = {
  encode(
    message: OpsMoveToResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.unixfsError !== undefined) {
      UnixFSError.encode(message.unixfsError, writer.uint32(10).fork()).ldelim()
    }
    if (message.done === true) {
      writer.uint32(16).bool(message.done)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): OpsMoveToResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseOpsMoveToResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.unixfsError = UnixFSError.decode(reader, reader.uint32())
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.done = reader.bool()
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<OpsMoveToResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<OpsMoveToResponse | OpsMoveToResponse[]>
      | Iterable<OpsMoveToResponse | OpsMoveToResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsMoveToResponse.encode(p).finish()]
        }
      } else {
        yield* [OpsMoveToResponse.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, OpsMoveToResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<OpsMoveToResponse> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsMoveToResponse.decode(p)]
        }
      } else {
        yield* [OpsMoveToResponse.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): OpsMoveToResponse {
    return {
      unixfsError: isSet(object.unixfsError)
        ? UnixFSError.fromJSON(object.unixfsError)
        : undefined,
      done: isSet(object.done) ? globalThis.Boolean(object.done) : false,
    }
  },

  toJSON(message: OpsMoveToResponse): unknown {
    const obj: any = {}
    if (message.unixfsError !== undefined) {
      obj.unixfsError = UnixFSError.toJSON(message.unixfsError)
    }
    if (message.done === true) {
      obj.done = message.done
    }
    return obj
  },

  create<I extends Exact<DeepPartial<OpsMoveToResponse>, I>>(
    base?: I,
  ): OpsMoveToResponse {
    return OpsMoveToResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<OpsMoveToResponse>, I>>(
    object: I,
  ): OpsMoveToResponse {
    const message = createBaseOpsMoveToResponse()
    message.unixfsError =
      object.unixfsError !== undefined && object.unixfsError !== null
        ? UnixFSError.fromPartial(object.unixfsError)
        : undefined
    message.done = object.done ?? false
    return message
  },
}

function createBaseOpsMoveFromRequest(): OpsMoveFromRequest {
  return {
    opsHandleId: Long.UZERO,
    name: '',
    srcOpsHandleId: Long.UZERO,
    timestamp: undefined,
  }
}

export const OpsMoveFromRequest = {
  encode(
    message: OpsMoveFromRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (!message.opsHandleId.isZero()) {
      writer.uint32(8).uint64(message.opsHandleId)
    }
    if (message.name !== '') {
      writer.uint32(18).string(message.name)
    }
    if (!message.srcOpsHandleId.isZero()) {
      writer.uint32(24).uint64(message.srcOpsHandleId)
    }
    if (message.timestamp !== undefined) {
      Timestamp.encode(message.timestamp, writer.uint32(34).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): OpsMoveFromRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseOpsMoveFromRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.opsHandleId = reader.uint64() as Long
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.name = reader.string()
          continue
        case 3:
          if (tag !== 24) {
            break
          }

          message.srcOpsHandleId = reader.uint64() as Long
          continue
        case 4:
          if (tag !== 34) {
            break
          }

          message.timestamp = Timestamp.decode(reader, reader.uint32())
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<OpsMoveFromRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<OpsMoveFromRequest | OpsMoveFromRequest[]>
      | Iterable<OpsMoveFromRequest | OpsMoveFromRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsMoveFromRequest.encode(p).finish()]
        }
      } else {
        yield* [OpsMoveFromRequest.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, OpsMoveFromRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<OpsMoveFromRequest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsMoveFromRequest.decode(p)]
        }
      } else {
        yield* [OpsMoveFromRequest.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): OpsMoveFromRequest {
    return {
      opsHandleId: isSet(object.opsHandleId)
        ? Long.fromValue(object.opsHandleId)
        : Long.UZERO,
      name: isSet(object.name) ? globalThis.String(object.name) : '',
      srcOpsHandleId: isSet(object.srcOpsHandleId)
        ? Long.fromValue(object.srcOpsHandleId)
        : Long.UZERO,
      timestamp: isSet(object.timestamp)
        ? Timestamp.fromJSON(object.timestamp)
        : undefined,
    }
  },

  toJSON(message: OpsMoveFromRequest): unknown {
    const obj: any = {}
    if (!message.opsHandleId.isZero()) {
      obj.opsHandleId = (message.opsHandleId || Long.UZERO).toString()
    }
    if (message.name !== '') {
      obj.name = message.name
    }
    if (!message.srcOpsHandleId.isZero()) {
      obj.srcOpsHandleId = (message.srcOpsHandleId || Long.UZERO).toString()
    }
    if (message.timestamp !== undefined) {
      obj.timestamp = Timestamp.toJSON(message.timestamp)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<OpsMoveFromRequest>, I>>(
    base?: I,
  ): OpsMoveFromRequest {
    return OpsMoveFromRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<OpsMoveFromRequest>, I>>(
    object: I,
  ): OpsMoveFromRequest {
    const message = createBaseOpsMoveFromRequest()
    message.opsHandleId =
      object.opsHandleId !== undefined && object.opsHandleId !== null
        ? Long.fromValue(object.opsHandleId)
        : Long.UZERO
    message.name = object.name ?? ''
    message.srcOpsHandleId =
      object.srcOpsHandleId !== undefined && object.srcOpsHandleId !== null
        ? Long.fromValue(object.srcOpsHandleId)
        : Long.UZERO
    message.timestamp =
      object.timestamp !== undefined && object.timestamp !== null
        ? Timestamp.fromPartial(object.timestamp)
        : undefined
    return message
  },
}

function createBaseOpsMoveFromResponse(): OpsMoveFromResponse {
  return { unixfsError: undefined, done: false }
}

export const OpsMoveFromResponse = {
  encode(
    message: OpsMoveFromResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.unixfsError !== undefined) {
      UnixFSError.encode(message.unixfsError, writer.uint32(10).fork()).ldelim()
    }
    if (message.done === true) {
      writer.uint32(16).bool(message.done)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): OpsMoveFromResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseOpsMoveFromResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.unixfsError = UnixFSError.decode(reader, reader.uint32())
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.done = reader.bool()
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<OpsMoveFromResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<OpsMoveFromResponse | OpsMoveFromResponse[]>
      | Iterable<OpsMoveFromResponse | OpsMoveFromResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsMoveFromResponse.encode(p).finish()]
        }
      } else {
        yield* [OpsMoveFromResponse.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, OpsMoveFromResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<OpsMoveFromResponse> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsMoveFromResponse.decode(p)]
        }
      } else {
        yield* [OpsMoveFromResponse.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): OpsMoveFromResponse {
    return {
      unixfsError: isSet(object.unixfsError)
        ? UnixFSError.fromJSON(object.unixfsError)
        : undefined,
      done: isSet(object.done) ? globalThis.Boolean(object.done) : false,
    }
  },

  toJSON(message: OpsMoveFromResponse): unknown {
    const obj: any = {}
    if (message.unixfsError !== undefined) {
      obj.unixfsError = UnixFSError.toJSON(message.unixfsError)
    }
    if (message.done === true) {
      obj.done = message.done
    }
    return obj
  },

  create<I extends Exact<DeepPartial<OpsMoveFromResponse>, I>>(
    base?: I,
  ): OpsMoveFromResponse {
    return OpsMoveFromResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<OpsMoveFromResponse>, I>>(
    object: I,
  ): OpsMoveFromResponse {
    const message = createBaseOpsMoveFromResponse()
    message.unixfsError =
      object.unixfsError !== undefined && object.unixfsError !== null
        ? UnixFSError.fromPartial(object.unixfsError)
        : undefined
    message.done = object.done ?? false
    return message
  },
}

function createBaseOpsRemoveRequest(): OpsRemoveRequest {
  return { opsHandleId: Long.UZERO, names: [], timestamp: undefined }
}

export const OpsRemoveRequest = {
  encode(
    message: OpsRemoveRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (!message.opsHandleId.isZero()) {
      writer.uint32(8).uint64(message.opsHandleId)
    }
    for (const v of message.names) {
      writer.uint32(18).string(v!)
    }
    if (message.timestamp !== undefined) {
      Timestamp.encode(message.timestamp, writer.uint32(26).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): OpsRemoveRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseOpsRemoveRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.opsHandleId = reader.uint64() as Long
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.names.push(reader.string())
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.timestamp = Timestamp.decode(reader, reader.uint32())
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<OpsRemoveRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<OpsRemoveRequest | OpsRemoveRequest[]>
      | Iterable<OpsRemoveRequest | OpsRemoveRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsRemoveRequest.encode(p).finish()]
        }
      } else {
        yield* [OpsRemoveRequest.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, OpsRemoveRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<OpsRemoveRequest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsRemoveRequest.decode(p)]
        }
      } else {
        yield* [OpsRemoveRequest.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): OpsRemoveRequest {
    return {
      opsHandleId: isSet(object.opsHandleId)
        ? Long.fromValue(object.opsHandleId)
        : Long.UZERO,
      names: globalThis.Array.isArray(object?.names)
        ? object.names.map((e: any) => globalThis.String(e))
        : [],
      timestamp: isSet(object.timestamp)
        ? Timestamp.fromJSON(object.timestamp)
        : undefined,
    }
  },

  toJSON(message: OpsRemoveRequest): unknown {
    const obj: any = {}
    if (!message.opsHandleId.isZero()) {
      obj.opsHandleId = (message.opsHandleId || Long.UZERO).toString()
    }
    if (message.names?.length) {
      obj.names = message.names
    }
    if (message.timestamp !== undefined) {
      obj.timestamp = Timestamp.toJSON(message.timestamp)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<OpsRemoveRequest>, I>>(
    base?: I,
  ): OpsRemoveRequest {
    return OpsRemoveRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<OpsRemoveRequest>, I>>(
    object: I,
  ): OpsRemoveRequest {
    const message = createBaseOpsRemoveRequest()
    message.opsHandleId =
      object.opsHandleId !== undefined && object.opsHandleId !== null
        ? Long.fromValue(object.opsHandleId)
        : Long.UZERO
    message.names = object.names?.map((e) => e) || []
    message.timestamp =
      object.timestamp !== undefined && object.timestamp !== null
        ? Timestamp.fromPartial(object.timestamp)
        : undefined
    return message
  },
}

function createBaseOpsRemoveResponse(): OpsRemoveResponse {
  return { unixfsError: undefined }
}

export const OpsRemoveResponse = {
  encode(
    message: OpsRemoveResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.unixfsError !== undefined) {
      UnixFSError.encode(message.unixfsError, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): OpsRemoveResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseOpsRemoveResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.unixfsError = UnixFSError.decode(reader, reader.uint32())
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<OpsRemoveResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<OpsRemoveResponse | OpsRemoveResponse[]>
      | Iterable<OpsRemoveResponse | OpsRemoveResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsRemoveResponse.encode(p).finish()]
        }
      } else {
        yield* [OpsRemoveResponse.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, OpsRemoveResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<OpsRemoveResponse> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [OpsRemoveResponse.decode(p)]
        }
      } else {
        yield* [OpsRemoveResponse.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): OpsRemoveResponse {
    return {
      unixfsError: isSet(object.unixfsError)
        ? UnixFSError.fromJSON(object.unixfsError)
        : undefined,
    }
  },

  toJSON(message: OpsRemoveResponse): unknown {
    const obj: any = {}
    if (message.unixfsError !== undefined) {
      obj.unixfsError = UnixFSError.toJSON(message.unixfsError)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<OpsRemoveResponse>, I>>(
    base?: I,
  ): OpsRemoveResponse {
    return OpsRemoveResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<OpsRemoveResponse>, I>>(
    object: I,
  ): OpsRemoveResponse {
    const message = createBaseOpsRemoveResponse()
    message.unixfsError =
      object.unixfsError !== undefined && object.unixfsError !== null
        ? UnixFSError.fromPartial(object.unixfsError)
        : undefined
    return message
  },
}

/**
 * FSCursorService exposes an FSCursor and FSCursorOps tree via RPC.
 *
 * The server and client track FSCursor and FSCursorOps handles via integer IDs.
 * The handle IDs start at 1, a zero ID indicates nil (empty).
 * This service expects to have a single client access it at a time (calling FSCursorClient).
 * Wrap the service in FSAccessService to construct one cursor service per client session.
 */
export interface FSCursorService {
  /**
   * FSCursorClient starts an instance of a client for the FSCursorService,
   * yielding a new client ID. The client can use that ID for future RPCs
   * accessing the FSCursor tree. When the streaming RPC ends, references to
   * cursors opened by the client will be released. The server will send
   * FSCursorChange for any cursors the client has subscribed to.
   */
  FSCursorClient(
    request: FSCursorClientRequest,
    abortSignal?: AbortSignal,
  ): AsyncIterable<FSCursorClientResponse>
  /** GetProxyCursor returns an FSCursor to replace an existing one, if necessary. */
  GetProxyCursor(
    request: GetProxyCursorRequest,
    abortSignal?: AbortSignal,
  ): Promise<GetProxyCursorResponse>
  /** GetCursorOps resolves the FSCursorOps handle. */
  GetCursorOps(
    request: GetCursorOpsRequest,
    abortSignal?: AbortSignal,
  ): Promise<GetCursorOpsResponse>
  /**
   * ReleaseFSCursor releases an FSCursor or FSCursorOps handle.
   * This is a Fire and Forget RPC which will return instantly.
   */
  ReleaseFSCursor(
    request: ReleaseFSCursorRequest,
    abortSignal?: AbortSignal,
  ): Promise<ReleaseFSCursorResponse>
  /**
   * OpsGetPermissions returns the permissions bits of the file mode.
   * The file mode portion of the value is ignored.
   */
  OpsGetPermissions(
    request: OpsGetPermissionsRequest,
    abortSignal?: AbortSignal,
  ): Promise<OpsGetPermissionsResponse>
  /** OpsSetPermissions updates the permissions bits of the file mode. */
  OpsSetPermissions(
    request: OpsSetPermissionsRequest,
    abortSignal?: AbortSignal,
  ): Promise<OpsSetPermissionsResponse>
  /** OpsGetSize returns the size of the inode (in bytes). */
  OpsGetSize(
    request: OpsGetSizeRequest,
    abortSignal?: AbortSignal,
  ): Promise<OpsGetSizeResponse>
  /** OpsGetModTimestamp returns the modification timestamp. */
  OpsGetModTimestamp(
    request: OpsGetModTimestampRequest,
    abortSignal?: AbortSignal,
  ): Promise<OpsGetModTimestampResponse>
  /** OpsSetModTimestamp updates the modification timestamp of the node. */
  OpsSetModTimestamp(
    request: OpsSetModTimestampRequest,
    abortSignal?: AbortSignal,
  ): Promise<OpsSetModTimestampResponse>
  /** OpsReadAt reads from a location in a File node. */
  OpsReadAt(
    request: OpsReadAtRequest,
    abortSignal?: AbortSignal,
  ): Promise<OpsReadAtResponse>
  /** OpsGetOptimalWriteSize returns the best write size to use for the Write call. */
  OpsGetOptimalWriteSize(
    request: OpsGetOptimalWriteSizeRequest,
    abortSignal?: AbortSignal,
  ): Promise<OpsGetOptimalWriteSizeResponse>
  /** OpsWriteAt writes to a location within a File node synchronously. */
  OpsWriteAt(
    request: OpsWriteAtRequest,
    abortSignal?: AbortSignal,
  ): Promise<OpsWriteAtResponse>
  /** OpsTruncate shrinks or extends a file to the specified size. */
  OpsTruncate(
    request: OpsTruncateRequest,
    abortSignal?: AbortSignal,
  ): Promise<OpsTruncateResponse>
  /** OpsLookup looks up a child entry in a directory. */
  OpsLookup(
    request: OpsLookupRequest,
    abortSignal?: AbortSignal,
  ): Promise<OpsLookupResponse>
  /** OpsReaddirAll reads all directory entries. */
  OpsReaddirAll(
    request: OpsReaddirAllRequest,
    abortSignal?: AbortSignal,
  ): AsyncIterable<OpsReaddirAllResponse>
  /** OpsMknod creates child entries in a directory. */
  OpsMknod(
    request: OpsMknodRequest,
    abortSignal?: AbortSignal,
  ): Promise<OpsMknodResponse>
  /** OpsSymlink creates a symbolic link from a location to a path. */
  OpsSymlink(
    request: OpsSymlinkRequest,
    abortSignal?: AbortSignal,
  ): Promise<OpsSymlinkResponse>
  /** OpsReadlink reads a symbolic link contents. */
  OpsReadlink(
    request: OpsReadlinkRequest,
    abortSignal?: AbortSignal,
  ): Promise<OpsReadlinkResponse>
  /** OpsCopyTo performs an optimized copy of an dirent inode to another inode. */
  OpsCopyTo(
    request: OpsCopyToRequest,
    abortSignal?: AbortSignal,
  ): Promise<OpsCopyToResponse>
  /** OpsCopyFrom performs an optimized copy from another inode. */
  OpsCopyFrom(
    request: OpsCopyFromRequest,
    abortSignal?: AbortSignal,
  ): Promise<OpsCopyFromResponse>
  /** OpsMoveTo performs an atomic and optimized move to another inode. */
  OpsMoveTo(
    request: OpsMoveToRequest,
    abortSignal?: AbortSignal,
  ): Promise<OpsMoveToResponse>
  /** OpsMoveFrom performs an atomic and optimized move from another inode. */
  OpsMoveFrom(
    request: OpsMoveFromRequest,
    abortSignal?: AbortSignal,
  ): Promise<OpsMoveFromResponse>
  /** OpsRemove deletes entries from a directory. */
  OpsRemove(
    request: OpsRemoveRequest,
    abortSignal?: AbortSignal,
  ): Promise<OpsRemoveResponse>
}

export const FSCursorServiceServiceName = 'unixfs.rpc.FSCursorService'
export class FSCursorServiceClientImpl implements FSCursorService {
  private readonly rpc: Rpc
  private readonly service: string
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || FSCursorServiceServiceName
    this.rpc = rpc
    this.FSCursorClient = this.FSCursorClient.bind(this)
    this.GetProxyCursor = this.GetProxyCursor.bind(this)
    this.GetCursorOps = this.GetCursorOps.bind(this)
    this.ReleaseFSCursor = this.ReleaseFSCursor.bind(this)
    this.OpsGetPermissions = this.OpsGetPermissions.bind(this)
    this.OpsSetPermissions = this.OpsSetPermissions.bind(this)
    this.OpsGetSize = this.OpsGetSize.bind(this)
    this.OpsGetModTimestamp = this.OpsGetModTimestamp.bind(this)
    this.OpsSetModTimestamp = this.OpsSetModTimestamp.bind(this)
    this.OpsReadAt = this.OpsReadAt.bind(this)
    this.OpsGetOptimalWriteSize = this.OpsGetOptimalWriteSize.bind(this)
    this.OpsWriteAt = this.OpsWriteAt.bind(this)
    this.OpsTruncate = this.OpsTruncate.bind(this)
    this.OpsLookup = this.OpsLookup.bind(this)
    this.OpsReaddirAll = this.OpsReaddirAll.bind(this)
    this.OpsMknod = this.OpsMknod.bind(this)
    this.OpsSymlink = this.OpsSymlink.bind(this)
    this.OpsReadlink = this.OpsReadlink.bind(this)
    this.OpsCopyTo = this.OpsCopyTo.bind(this)
    this.OpsCopyFrom = this.OpsCopyFrom.bind(this)
    this.OpsMoveTo = this.OpsMoveTo.bind(this)
    this.OpsMoveFrom = this.OpsMoveFrom.bind(this)
    this.OpsRemove = this.OpsRemove.bind(this)
  }
  FSCursorClient(
    request: FSCursorClientRequest,
    abortSignal?: AbortSignal,
  ): AsyncIterable<FSCursorClientResponse> {
    const data = FSCursorClientRequest.encode(request).finish()
    const result = this.rpc.serverStreamingRequest(
      this.service,
      'FSCursorClient',
      data,
      abortSignal || undefined,
    )
    return FSCursorClientResponse.decodeTransform(result)
  }

  GetProxyCursor(
    request: GetProxyCursorRequest,
    abortSignal?: AbortSignal,
  ): Promise<GetProxyCursorResponse> {
    const data = GetProxyCursorRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'GetProxyCursor',
      data,
      abortSignal || undefined,
    )
    return promise.then((data) =>
      GetProxyCursorResponse.decode(_m0.Reader.create(data)),
    )
  }

  GetCursorOps(
    request: GetCursorOpsRequest,
    abortSignal?: AbortSignal,
  ): Promise<GetCursorOpsResponse> {
    const data = GetCursorOpsRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'GetCursorOps',
      data,
      abortSignal || undefined,
    )
    return promise.then((data) =>
      GetCursorOpsResponse.decode(_m0.Reader.create(data)),
    )
  }

  ReleaseFSCursor(
    request: ReleaseFSCursorRequest,
    abortSignal?: AbortSignal,
  ): Promise<ReleaseFSCursorResponse> {
    const data = ReleaseFSCursorRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'ReleaseFSCursor',
      data,
      abortSignal || undefined,
    )
    return promise.then((data) =>
      ReleaseFSCursorResponse.decode(_m0.Reader.create(data)),
    )
  }

  OpsGetPermissions(
    request: OpsGetPermissionsRequest,
    abortSignal?: AbortSignal,
  ): Promise<OpsGetPermissionsResponse> {
    const data = OpsGetPermissionsRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'OpsGetPermissions',
      data,
      abortSignal || undefined,
    )
    return promise.then((data) =>
      OpsGetPermissionsResponse.decode(_m0.Reader.create(data)),
    )
  }

  OpsSetPermissions(
    request: OpsSetPermissionsRequest,
    abortSignal?: AbortSignal,
  ): Promise<OpsSetPermissionsResponse> {
    const data = OpsSetPermissionsRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'OpsSetPermissions',
      data,
      abortSignal || undefined,
    )
    return promise.then((data) =>
      OpsSetPermissionsResponse.decode(_m0.Reader.create(data)),
    )
  }

  OpsGetSize(
    request: OpsGetSizeRequest,
    abortSignal?: AbortSignal,
  ): Promise<OpsGetSizeResponse> {
    const data = OpsGetSizeRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'OpsGetSize',
      data,
      abortSignal || undefined,
    )
    return promise.then((data) =>
      OpsGetSizeResponse.decode(_m0.Reader.create(data)),
    )
  }

  OpsGetModTimestamp(
    request: OpsGetModTimestampRequest,
    abortSignal?: AbortSignal,
  ): Promise<OpsGetModTimestampResponse> {
    const data = OpsGetModTimestampRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'OpsGetModTimestamp',
      data,
      abortSignal || undefined,
    )
    return promise.then((data) =>
      OpsGetModTimestampResponse.decode(_m0.Reader.create(data)),
    )
  }

  OpsSetModTimestamp(
    request: OpsSetModTimestampRequest,
    abortSignal?: AbortSignal,
  ): Promise<OpsSetModTimestampResponse> {
    const data = OpsSetModTimestampRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'OpsSetModTimestamp',
      data,
      abortSignal || undefined,
    )
    return promise.then((data) =>
      OpsSetModTimestampResponse.decode(_m0.Reader.create(data)),
    )
  }

  OpsReadAt(
    request: OpsReadAtRequest,
    abortSignal?: AbortSignal,
  ): Promise<OpsReadAtResponse> {
    const data = OpsReadAtRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'OpsReadAt',
      data,
      abortSignal || undefined,
    )
    return promise.then((data) =>
      OpsReadAtResponse.decode(_m0.Reader.create(data)),
    )
  }

  OpsGetOptimalWriteSize(
    request: OpsGetOptimalWriteSizeRequest,
    abortSignal?: AbortSignal,
  ): Promise<OpsGetOptimalWriteSizeResponse> {
    const data = OpsGetOptimalWriteSizeRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'OpsGetOptimalWriteSize',
      data,
      abortSignal || undefined,
    )
    return promise.then((data) =>
      OpsGetOptimalWriteSizeResponse.decode(_m0.Reader.create(data)),
    )
  }

  OpsWriteAt(
    request: OpsWriteAtRequest,
    abortSignal?: AbortSignal,
  ): Promise<OpsWriteAtResponse> {
    const data = OpsWriteAtRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'OpsWriteAt',
      data,
      abortSignal || undefined,
    )
    return promise.then((data) =>
      OpsWriteAtResponse.decode(_m0.Reader.create(data)),
    )
  }

  OpsTruncate(
    request: OpsTruncateRequest,
    abortSignal?: AbortSignal,
  ): Promise<OpsTruncateResponse> {
    const data = OpsTruncateRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'OpsTruncate',
      data,
      abortSignal || undefined,
    )
    return promise.then((data) =>
      OpsTruncateResponse.decode(_m0.Reader.create(data)),
    )
  }

  OpsLookup(
    request: OpsLookupRequest,
    abortSignal?: AbortSignal,
  ): Promise<OpsLookupResponse> {
    const data = OpsLookupRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'OpsLookup',
      data,
      abortSignal || undefined,
    )
    return promise.then((data) =>
      OpsLookupResponse.decode(_m0.Reader.create(data)),
    )
  }

  OpsReaddirAll(
    request: OpsReaddirAllRequest,
    abortSignal?: AbortSignal,
  ): AsyncIterable<OpsReaddirAllResponse> {
    const data = OpsReaddirAllRequest.encode(request).finish()
    const result = this.rpc.serverStreamingRequest(
      this.service,
      'OpsReaddirAll',
      data,
      abortSignal || undefined,
    )
    return OpsReaddirAllResponse.decodeTransform(result)
  }

  OpsMknod(
    request: OpsMknodRequest,
    abortSignal?: AbortSignal,
  ): Promise<OpsMknodResponse> {
    const data = OpsMknodRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'OpsMknod',
      data,
      abortSignal || undefined,
    )
    return promise.then((data) =>
      OpsMknodResponse.decode(_m0.Reader.create(data)),
    )
  }

  OpsSymlink(
    request: OpsSymlinkRequest,
    abortSignal?: AbortSignal,
  ): Promise<OpsSymlinkResponse> {
    const data = OpsSymlinkRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'OpsSymlink',
      data,
      abortSignal || undefined,
    )
    return promise.then((data) =>
      OpsSymlinkResponse.decode(_m0.Reader.create(data)),
    )
  }

  OpsReadlink(
    request: OpsReadlinkRequest,
    abortSignal?: AbortSignal,
  ): Promise<OpsReadlinkResponse> {
    const data = OpsReadlinkRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'OpsReadlink',
      data,
      abortSignal || undefined,
    )
    return promise.then((data) =>
      OpsReadlinkResponse.decode(_m0.Reader.create(data)),
    )
  }

  OpsCopyTo(
    request: OpsCopyToRequest,
    abortSignal?: AbortSignal,
  ): Promise<OpsCopyToResponse> {
    const data = OpsCopyToRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'OpsCopyTo',
      data,
      abortSignal || undefined,
    )
    return promise.then((data) =>
      OpsCopyToResponse.decode(_m0.Reader.create(data)),
    )
  }

  OpsCopyFrom(
    request: OpsCopyFromRequest,
    abortSignal?: AbortSignal,
  ): Promise<OpsCopyFromResponse> {
    const data = OpsCopyFromRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'OpsCopyFrom',
      data,
      abortSignal || undefined,
    )
    return promise.then((data) =>
      OpsCopyFromResponse.decode(_m0.Reader.create(data)),
    )
  }

  OpsMoveTo(
    request: OpsMoveToRequest,
    abortSignal?: AbortSignal,
  ): Promise<OpsMoveToResponse> {
    const data = OpsMoveToRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'OpsMoveTo',
      data,
      abortSignal || undefined,
    )
    return promise.then((data) =>
      OpsMoveToResponse.decode(_m0.Reader.create(data)),
    )
  }

  OpsMoveFrom(
    request: OpsMoveFromRequest,
    abortSignal?: AbortSignal,
  ): Promise<OpsMoveFromResponse> {
    const data = OpsMoveFromRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'OpsMoveFrom',
      data,
      abortSignal || undefined,
    )
    return promise.then((data) =>
      OpsMoveFromResponse.decode(_m0.Reader.create(data)),
    )
  }

  OpsRemove(
    request: OpsRemoveRequest,
    abortSignal?: AbortSignal,
  ): Promise<OpsRemoveResponse> {
    const data = OpsRemoveRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'OpsRemove',
      data,
      abortSignal || undefined,
    )
    return promise.then((data) =>
      OpsRemoveResponse.decode(_m0.Reader.create(data)),
    )
  }
}

/**
 * FSCursorService exposes an FSCursor and FSCursorOps tree via RPC.
 *
 * The server and client track FSCursor and FSCursorOps handles via integer IDs.
 * The handle IDs start at 1, a zero ID indicates nil (empty).
 * This service expects to have a single client access it at a time (calling FSCursorClient).
 * Wrap the service in FSAccessService to construct one cursor service per client session.
 */
export type FSCursorServiceDefinition = typeof FSCursorServiceDefinition
export const FSCursorServiceDefinition = {
  name: 'FSCursorService',
  fullName: 'unixfs.rpc.FSCursorService',
  methods: {
    /**
     * FSCursorClient starts an instance of a client for the FSCursorService,
     * yielding a new client ID. The client can use that ID for future RPCs
     * accessing the FSCursor tree. When the streaming RPC ends, references to
     * cursors opened by the client will be released. The server will send
     * FSCursorChange for any cursors the client has subscribed to.
     */
    fSCursorClient: {
      name: 'FSCursorClient',
      requestType: FSCursorClientRequest,
      requestStream: false,
      responseType: FSCursorClientResponse,
      responseStream: true,
      options: {},
    },
    /** GetProxyCursor returns an FSCursor to replace an existing one, if necessary. */
    getProxyCursor: {
      name: 'GetProxyCursor',
      requestType: GetProxyCursorRequest,
      requestStream: false,
      responseType: GetProxyCursorResponse,
      responseStream: false,
      options: {},
    },
    /** GetCursorOps resolves the FSCursorOps handle. */
    getCursorOps: {
      name: 'GetCursorOps',
      requestType: GetCursorOpsRequest,
      requestStream: false,
      responseType: GetCursorOpsResponse,
      responseStream: false,
      options: {},
    },
    /**
     * ReleaseFSCursor releases an FSCursor or FSCursorOps handle.
     * This is a Fire and Forget RPC which will return instantly.
     */
    releaseFSCursor: {
      name: 'ReleaseFSCursor',
      requestType: ReleaseFSCursorRequest,
      requestStream: false,
      responseType: ReleaseFSCursorResponse,
      responseStream: false,
      options: {},
    },
    /**
     * OpsGetPermissions returns the permissions bits of the file mode.
     * The file mode portion of the value is ignored.
     */
    opsGetPermissions: {
      name: 'OpsGetPermissions',
      requestType: OpsGetPermissionsRequest,
      requestStream: false,
      responseType: OpsGetPermissionsResponse,
      responseStream: false,
      options: {},
    },
    /** OpsSetPermissions updates the permissions bits of the file mode. */
    opsSetPermissions: {
      name: 'OpsSetPermissions',
      requestType: OpsSetPermissionsRequest,
      requestStream: false,
      responseType: OpsSetPermissionsResponse,
      responseStream: false,
      options: {},
    },
    /** OpsGetSize returns the size of the inode (in bytes). */
    opsGetSize: {
      name: 'OpsGetSize',
      requestType: OpsGetSizeRequest,
      requestStream: false,
      responseType: OpsGetSizeResponse,
      responseStream: false,
      options: {},
    },
    /** OpsGetModTimestamp returns the modification timestamp. */
    opsGetModTimestamp: {
      name: 'OpsGetModTimestamp',
      requestType: OpsGetModTimestampRequest,
      requestStream: false,
      responseType: OpsGetModTimestampResponse,
      responseStream: false,
      options: {},
    },
    /** OpsSetModTimestamp updates the modification timestamp of the node. */
    opsSetModTimestamp: {
      name: 'OpsSetModTimestamp',
      requestType: OpsSetModTimestampRequest,
      requestStream: false,
      responseType: OpsSetModTimestampResponse,
      responseStream: false,
      options: {},
    },
    /** OpsReadAt reads from a location in a File node. */
    opsReadAt: {
      name: 'OpsReadAt',
      requestType: OpsReadAtRequest,
      requestStream: false,
      responseType: OpsReadAtResponse,
      responseStream: false,
      options: {},
    },
    /** OpsGetOptimalWriteSize returns the best write size to use for the Write call. */
    opsGetOptimalWriteSize: {
      name: 'OpsGetOptimalWriteSize',
      requestType: OpsGetOptimalWriteSizeRequest,
      requestStream: false,
      responseType: OpsGetOptimalWriteSizeResponse,
      responseStream: false,
      options: {},
    },
    /** OpsWriteAt writes to a location within a File node synchronously. */
    opsWriteAt: {
      name: 'OpsWriteAt',
      requestType: OpsWriteAtRequest,
      requestStream: false,
      responseType: OpsWriteAtResponse,
      responseStream: false,
      options: {},
    },
    /** OpsTruncate shrinks or extends a file to the specified size. */
    opsTruncate: {
      name: 'OpsTruncate',
      requestType: OpsTruncateRequest,
      requestStream: false,
      responseType: OpsTruncateResponse,
      responseStream: false,
      options: {},
    },
    /** OpsLookup looks up a child entry in a directory. */
    opsLookup: {
      name: 'OpsLookup',
      requestType: OpsLookupRequest,
      requestStream: false,
      responseType: OpsLookupResponse,
      responseStream: false,
      options: {},
    },
    /** OpsReaddirAll reads all directory entries. */
    opsReaddirAll: {
      name: 'OpsReaddirAll',
      requestType: OpsReaddirAllRequest,
      requestStream: false,
      responseType: OpsReaddirAllResponse,
      responseStream: true,
      options: {},
    },
    /** OpsMknod creates child entries in a directory. */
    opsMknod: {
      name: 'OpsMknod',
      requestType: OpsMknodRequest,
      requestStream: false,
      responseType: OpsMknodResponse,
      responseStream: false,
      options: {},
    },
    /** OpsSymlink creates a symbolic link from a location to a path. */
    opsSymlink: {
      name: 'OpsSymlink',
      requestType: OpsSymlinkRequest,
      requestStream: false,
      responseType: OpsSymlinkResponse,
      responseStream: false,
      options: {},
    },
    /** OpsReadlink reads a symbolic link contents. */
    opsReadlink: {
      name: 'OpsReadlink',
      requestType: OpsReadlinkRequest,
      requestStream: false,
      responseType: OpsReadlinkResponse,
      responseStream: false,
      options: {},
    },
    /** OpsCopyTo performs an optimized copy of an dirent inode to another inode. */
    opsCopyTo: {
      name: 'OpsCopyTo',
      requestType: OpsCopyToRequest,
      requestStream: false,
      responseType: OpsCopyToResponse,
      responseStream: false,
      options: {},
    },
    /** OpsCopyFrom performs an optimized copy from another inode. */
    opsCopyFrom: {
      name: 'OpsCopyFrom',
      requestType: OpsCopyFromRequest,
      requestStream: false,
      responseType: OpsCopyFromResponse,
      responseStream: false,
      options: {},
    },
    /** OpsMoveTo performs an atomic and optimized move to another inode. */
    opsMoveTo: {
      name: 'OpsMoveTo',
      requestType: OpsMoveToRequest,
      requestStream: false,
      responseType: OpsMoveToResponse,
      responseStream: false,
      options: {},
    },
    /** OpsMoveFrom performs an atomic and optimized move from another inode. */
    opsMoveFrom: {
      name: 'OpsMoveFrom',
      requestType: OpsMoveFromRequest,
      requestStream: false,
      responseType: OpsMoveFromResponse,
      responseStream: false,
      options: {},
    },
    /** OpsRemove deletes entries from a directory. */
    opsRemove: {
      name: 'OpsRemove',
      requestType: OpsRemoveRequest,
      requestStream: false,
      responseType: OpsRemoveResponse,
      responseStream: false,
      options: {},
    },
  },
} as const

interface Rpc {
  request(
    service: string,
    method: string,
    data: Uint8Array,
    abortSignal?: AbortSignal,
  ): Promise<Uint8Array>
  clientStreamingRequest(
    service: string,
    method: string,
    data: AsyncIterable<Uint8Array>,
    abortSignal?: AbortSignal,
  ): Promise<Uint8Array>
  serverStreamingRequest(
    service: string,
    method: string,
    data: Uint8Array,
    abortSignal?: AbortSignal,
  ): AsyncIterable<Uint8Array>
  bidirectionalStreamingRequest(
    service: string,
    method: string,
    data: AsyncIterable<Uint8Array>,
    abortSignal?: AbortSignal,
  ): AsyncIterable<Uint8Array>
}

function bytesFromBase64(b64: string): Uint8Array {
  if (globalThis.Buffer) {
    return Uint8Array.from(globalThis.Buffer.from(b64, 'base64'))
  } else {
    const bin = globalThis.atob(b64)
    const arr = new Uint8Array(bin.length)
    for (let i = 0; i < bin.length; ++i) {
      arr[i] = bin.charCodeAt(i)
    }
    return arr
  }
}

function base64FromBytes(arr: Uint8Array): string {
  if (globalThis.Buffer) {
    return globalThis.Buffer.from(arr).toString('base64')
  } else {
    const bin: string[] = []
    arr.forEach((byte) => {
      bin.push(globalThis.String.fromCharCode(byte))
    })
    return globalThis.btoa(bin.join(''))
  }
}

type Builtin =
  | Date
  | Function
  | Uint8Array
  | string
  | number
  | boolean
  | undefined

export type DeepPartial<T> = T extends Builtin
  ? T
  : T extends Long
    ? string | number | Long
    : T extends globalThis.Array<infer U>
      ? globalThis.Array<DeepPartial<U>>
      : T extends ReadonlyArray<infer U>
        ? ReadonlyArray<DeepPartial<U>>
        : T extends { $case: string }
          ? { [K in keyof Omit<T, '$case'>]?: DeepPartial<T[K]> } & {
              $case: T['$case']
            }
          : T extends {}
            ? { [K in keyof T]?: DeepPartial<T[K]> }
            : Partial<T>

type KeysOfUnion<T> = T extends T ? keyof T : never
export type Exact<P, I extends P> = P extends Builtin
  ? P
  : P & { [K in keyof P]: Exact<P[K], I[K]> } & {
      [K in Exclude<keyof I, KeysOfUnion<P>>]: never
    }

if (_m0.util.Long !== Long) {
  _m0.util.Long = Long as any
  _m0.configure()
}

function isSet(value: any): boolean {
  return value !== null && value !== undefined
}
