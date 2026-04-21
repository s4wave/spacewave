import {
  UnixFSError as ProtoUnixFSError,
  UnixFSErrorType,
} from './errors.pb.js'

// defaultMessage returns the default error message for a given error type.
function defaultMessage(type: UnixFSErrorType): string {
  switch (type) {
    case UnixFSErrorType.FS_NOT_FOUND:
      return 'fs not found'
    case UnixFSErrorType.EXIST:
      return 'file already exists'
    case UnixFSErrorType.NOT_EXIST:
      return 'file does not exist'
    case UnixFSErrorType.CLOSED:
      return 'file already closed'
    case UnixFSErrorType.READ_ONLY:
      return 'read-only fs'
    case UnixFSErrorType.RELEASED:
      return 'cursor or inode released'
    case UnixFSErrorType.NOT_DIRECTORY:
      return 'not a directory'
    case UnixFSErrorType.NOT_FILE:
      return 'not a file'
    case UnixFSErrorType.OUT_OF_BOUNDS:
      return 'dirent out of bounds'
    case UnixFSErrorType.EMPTY_PATH:
      return 'empty path'
    case UnixFSErrorType.ABSOLUTE_PATH:
      return 'absolute path not allowed'
    case UnixFSErrorType.INODE_UNRESOLVABLE:
      return 'inode unable to be resolved'
    case UnixFSErrorType.NOT_SYMLINK:
      return 'not a symlink'
    case UnixFSErrorType.EMPTY_TIMESTAMP:
      return 'empty timestamp'
    case UnixFSErrorType.MOVE_TO_SELF:
      return 'cannot copy/move a path into itself'
    case UnixFSErrorType.INVALID_WRITE:
      return 'invalid write result'
    case UnixFSErrorType.EMPTY_UNIXFS_ID:
      return 'empty unixfs id'
    case UnixFSErrorType.CONTEXT_CANCELED:
      return 'context canceled'
    case UnixFSErrorType.EOF:
      return 'EOF'
    case UnixFSErrorType.CROSS_FS_RENAME:
      return 'cross-fs rename unimplemented'
    case UnixFSErrorType.OTHER:
      return 'unknown unixfs error'
    default:
      return 'unknown unixfs error'
  }
}

// UnixFSError is an error type representing a UnixFS error with a typed category.
export class UnixFSError extends Error {
  readonly type: UnixFSErrorType

  constructor(type: UnixFSErrorType, message?: string) {
    super(message ?? defaultMessage(type))
    this.type = type
    this.name = 'UnixFSError'
  }

  // fromProto converts a proto UnixFSError to a UnixFSError instance.
  // Returns null if the error type is NONE.
  static fromProto(proto: ProtoUnixFSError): UnixFSError | null {
    if (!proto || proto.errorType === UnixFSErrorType.NONE) {
      return null
    }

    const errType = proto.errorType ?? UnixFSErrorType.OTHER
    const body = proto.errorBody ?? ''

    if (errType === UnixFSErrorType.OTHER) {
      if (body) {
        return new UnixFSError(errType, body)
      }
      return new UnixFSError(errType)
    }

    if (body) {
      return new UnixFSError(errType, body + ': ' + defaultMessage(errType))
    }
    return new UnixFSError(errType)
  }

  // toProto converts this UnixFSError to a proto UnixFSError.
  toProto(): ProtoUnixFSError {
    return {
      errorType: this.type,
      errorBody: this.message !== defaultMessage(this.type) ? this.message : '',
    }
  }

  // isReleased returns true if this is a RELEASED error.
  get isReleased(): boolean {
    return this.type === UnixFSErrorType.RELEASED
  }

  // isNotExist returns true if this is a NOT_EXIST error.
  get isNotExist(): boolean {
    return this.type === UnixFSErrorType.NOT_EXIST
  }

  // isEOF returns true if this is an EOF error.
  get isEOF(): boolean {
    return this.type === UnixFSErrorType.EOF
  }
}

// Sentinel error instances for identity checks.
export const ErrFsNotFound = new UnixFSError(UnixFSErrorType.FS_NOT_FOUND)
export const ErrExist = new UnixFSError(UnixFSErrorType.EXIST)
export const ErrNotExist = new UnixFSError(UnixFSErrorType.NOT_EXIST)
export const ErrClosed = new UnixFSError(UnixFSErrorType.CLOSED)
export const ErrReadOnly = new UnixFSError(UnixFSErrorType.READ_ONLY)
export const ErrReleased = new UnixFSError(UnixFSErrorType.RELEASED)
export const ErrNotDirectory = new UnixFSError(UnixFSErrorType.NOT_DIRECTORY)
export const ErrNotFile = new UnixFSError(UnixFSErrorType.NOT_FILE)
export const ErrOutOfBounds = new UnixFSError(UnixFSErrorType.OUT_OF_BOUNDS)
export const ErrEmptyPath = new UnixFSError(UnixFSErrorType.EMPTY_PATH)
export const ErrAbsolutePath = new UnixFSError(UnixFSErrorType.ABSOLUTE_PATH)
export const ErrInodeUnresolvable = new UnixFSError(
  UnixFSErrorType.INODE_UNRESOLVABLE,
)
export const ErrNotSymlink = new UnixFSError(UnixFSErrorType.NOT_SYMLINK)
export const ErrEmptyTimestamp = new UnixFSError(
  UnixFSErrorType.EMPTY_TIMESTAMP,
)
export const ErrMoveToSelf = new UnixFSError(UnixFSErrorType.MOVE_TO_SELF)
export const ErrInvalidWrite = new UnixFSError(UnixFSErrorType.INVALID_WRITE)
export const ErrEmptyUnixFsId = new UnixFSError(UnixFSErrorType.EMPTY_UNIXFS_ID)
export const ErrContextCanceled = new UnixFSError(
  UnixFSErrorType.CONTEXT_CANCELED,
)
export const ErrEOF = new UnixFSError(UnixFSErrorType.EOF)
export const ErrCrossFsRename = new UnixFSError(UnixFSErrorType.CROSS_FS_RENAME)
export const ErrUnknown = new UnixFSError(UnixFSErrorType.OTHER)

// ErrHandleIDEmpty is returned if the handle id was empty.
export const ErrHandleIDEmpty = new Error('handle id cannot be zero')

// isUnixFSError checks if an error is a UnixFSError, optionally of a specific type.
export function isUnixFSError(
  err: unknown,
  type?: UnixFSErrorType,
): err is UnixFSError {
  if (!(err instanceof UnixFSError)) {
    return false
  }
  if (type !== undefined) {
    return err.type === type
  }
  return true
}
