/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'unixfs.errors'

/** UnixFSErrorType contains all potential UnixFS errors. */
export enum UnixFSErrorType {
  /** NONE - NONE indicates no error. */
  NONE = 0,
  /** FS_NOT_FOUND - FS_NOT_FOUND corresponds to unixfs_errors.ErrFsNotFound. */
  FS_NOT_FOUND = 1,
  /** EXIST - EXIST corresponds to unixfs_errors.ErrExist. */
  EXIST = 2,
  /** NOT_EXIST - NOT_EXIST corresponds to unixfs_errors.ErrNotExist. */
  NOT_EXIST = 3,
  /** CLOSED - CLOSED corresponds to unixfs_errors.ErrClosed. */
  CLOSED = 4,
  /** READ_ONLY - READ_ONLY corresponds to unixfs_errors.ErrReadOnly. */
  READ_ONLY = 5,
  /** RELEASED - RELEASED corresponds to unixfs_errors.ErrReleased. */
  RELEASED = 6,
  /** NOT_DIRECTORY - NOT_DIRECTORY corresponds to unixfs_errors.ErrNotDirectory. */
  NOT_DIRECTORY = 7,
  /** NOT_FILE - NOT_FILE corresponds to unixfs_errors.ErrNotFile. */
  NOT_FILE = 8,
  /** OUT_OF_BOUNDS - OUT_OF_BOUNDS corresponds to unixfs_errors.ErrOutOfBounds. */
  OUT_OF_BOUNDS = 9,
  /** EMPTY_PATH - EMPTY_PATH corresponds to unixfs_errors.ErrEmptyPath. */
  EMPTY_PATH = 10,
  /** INODE_UNRESOLVABLE - INODE_UNRESOLVABLE corresponds to unixfs_errors.ErrInodeUnresolvable. */
  INODE_UNRESOLVABLE = 11,
  /** NOT_SYMLINK - NOT_SYMLINK corresponds to unixfs_errors.ErrNotSymlink. */
  NOT_SYMLINK = 12,
  /** EMPTY_TIMESTAMP - EMPTY_TIMESTAMP corresponds to unixfs_errors.ErrEmptyTimestamp. */
  EMPTY_TIMESTAMP = 13,
  /** MOVE_TO_SELF - MOVE_TO_SELF corresponds to unixfs_errors.ErrMoveToSelf. */
  MOVE_TO_SELF = 14,
  /** INVALID_WRITE - INVALID_WRITE corresponds to unixfs_errors.ErrInvalidWrite. */
  INVALID_WRITE = 15,
  /** EMPTY_UNIXFS_ID - EMPTY_UNIXFS_ID corresponds to unixfs_errors.ErrEmptyUnixFsId. */
  EMPTY_UNIXFS_ID = 16,
  /** OTHER - OTHER corresponds to a string error not defined in the unixfs errors list. */
  OTHER = 17,
  UNRECOGNIZED = -1,
}

export function unixFSErrorTypeFromJSON(object: any): UnixFSErrorType {
  switch (object) {
    case 0:
    case 'NONE':
      return UnixFSErrorType.NONE
    case 1:
    case 'FS_NOT_FOUND':
      return UnixFSErrorType.FS_NOT_FOUND
    case 2:
    case 'EXIST':
      return UnixFSErrorType.EXIST
    case 3:
    case 'NOT_EXIST':
      return UnixFSErrorType.NOT_EXIST
    case 4:
    case 'CLOSED':
      return UnixFSErrorType.CLOSED
    case 5:
    case 'READ_ONLY':
      return UnixFSErrorType.READ_ONLY
    case 6:
    case 'RELEASED':
      return UnixFSErrorType.RELEASED
    case 7:
    case 'NOT_DIRECTORY':
      return UnixFSErrorType.NOT_DIRECTORY
    case 8:
    case 'NOT_FILE':
      return UnixFSErrorType.NOT_FILE
    case 9:
    case 'OUT_OF_BOUNDS':
      return UnixFSErrorType.OUT_OF_BOUNDS
    case 10:
    case 'EMPTY_PATH':
      return UnixFSErrorType.EMPTY_PATH
    case 11:
    case 'INODE_UNRESOLVABLE':
      return UnixFSErrorType.INODE_UNRESOLVABLE
    case 12:
    case 'NOT_SYMLINK':
      return UnixFSErrorType.NOT_SYMLINK
    case 13:
    case 'EMPTY_TIMESTAMP':
      return UnixFSErrorType.EMPTY_TIMESTAMP
    case 14:
    case 'MOVE_TO_SELF':
      return UnixFSErrorType.MOVE_TO_SELF
    case 15:
    case 'INVALID_WRITE':
      return UnixFSErrorType.INVALID_WRITE
    case 16:
    case 'EMPTY_UNIXFS_ID':
      return UnixFSErrorType.EMPTY_UNIXFS_ID
    case 17:
    case 'OTHER':
      return UnixFSErrorType.OTHER
    case -1:
    case 'UNRECOGNIZED':
    default:
      return UnixFSErrorType.UNRECOGNIZED
  }
}

export function unixFSErrorTypeToJSON(object: UnixFSErrorType): string {
  switch (object) {
    case UnixFSErrorType.NONE:
      return 'NONE'
    case UnixFSErrorType.FS_NOT_FOUND:
      return 'FS_NOT_FOUND'
    case UnixFSErrorType.EXIST:
      return 'EXIST'
    case UnixFSErrorType.NOT_EXIST:
      return 'NOT_EXIST'
    case UnixFSErrorType.CLOSED:
      return 'CLOSED'
    case UnixFSErrorType.READ_ONLY:
      return 'READ_ONLY'
    case UnixFSErrorType.RELEASED:
      return 'RELEASED'
    case UnixFSErrorType.NOT_DIRECTORY:
      return 'NOT_DIRECTORY'
    case UnixFSErrorType.NOT_FILE:
      return 'NOT_FILE'
    case UnixFSErrorType.OUT_OF_BOUNDS:
      return 'OUT_OF_BOUNDS'
    case UnixFSErrorType.EMPTY_PATH:
      return 'EMPTY_PATH'
    case UnixFSErrorType.INODE_UNRESOLVABLE:
      return 'INODE_UNRESOLVABLE'
    case UnixFSErrorType.NOT_SYMLINK:
      return 'NOT_SYMLINK'
    case UnixFSErrorType.EMPTY_TIMESTAMP:
      return 'EMPTY_TIMESTAMP'
    case UnixFSErrorType.MOVE_TO_SELF:
      return 'MOVE_TO_SELF'
    case UnixFSErrorType.INVALID_WRITE:
      return 'INVALID_WRITE'
    case UnixFSErrorType.EMPTY_UNIXFS_ID:
      return 'EMPTY_UNIXFS_ID'
    case UnixFSErrorType.OTHER:
      return 'OTHER'
    case UnixFSErrorType.UNRECOGNIZED:
    default:
      return 'UNRECOGNIZED'
  }
}

/** UnixFSError contains an RPC error returned by a cursor, if any. */
export interface UnixFSError {
  /** ErrorType is the type of error, zero if none. */
  errorType: UnixFSErrorType
  /**
   * ErrorBody is the body of the error.
   * If this is set and the error is type OTHER, return errors.New(ErrorBody).
   * If this is set and the error is another type, return errors.Wrap(the_error, ErrorBody).
   * If this is unset and error_type is OTHER, return errors.New("unknown unixfs error").
   */
  errorBody: string
}

function createBaseUnixFSError(): UnixFSError {
  return { errorType: 0, errorBody: '' }
}

export const UnixFSError = {
  encode(
    message: UnixFSError,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.errorType !== 0) {
      writer.uint32(8).int32(message.errorType)
    }
    if (message.errorBody !== '') {
      writer.uint32(18).string(message.errorBody)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): UnixFSError {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseUnixFSError()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.errorType = reader.int32() as any
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.errorBody = reader.string()
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
  // Transform<UnixFSError, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<UnixFSError | UnixFSError[]>
      | Iterable<UnixFSError | UnixFSError[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [UnixFSError.encode(p).finish()]
        }
      } else {
        yield* [UnixFSError.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, UnixFSError>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<UnixFSError> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [UnixFSError.decode(p)]
        }
      } else {
        yield* [UnixFSError.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): UnixFSError {
    return {
      errorType: isSet(object.errorType)
        ? unixFSErrorTypeFromJSON(object.errorType)
        : 0,
      errorBody: isSet(object.errorBody) ? String(object.errorBody) : '',
    }
  },

  toJSON(message: UnixFSError): unknown {
    const obj: any = {}
    if (message.errorType !== 0) {
      obj.errorType = unixFSErrorTypeToJSON(message.errorType)
    }
    if (message.errorBody !== '') {
      obj.errorBody = message.errorBody
    }
    return obj
  },

  create<I extends Exact<DeepPartial<UnixFSError>, I>>(base?: I): UnixFSError {
    return UnixFSError.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<UnixFSError>, I>>(
    object: I,
  ): UnixFSError {
    const message = createBaseUnixFSError()
    message.errorType = object.errorType ?? 0
    message.errorBody = object.errorBody ?? ''
    return message
  },
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
  : T extends Array<infer U>
  ? Array<DeepPartial<U>>
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
