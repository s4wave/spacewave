import { describe, it, expect } from 'vitest'
import {
  UnixFSError,
  ErrFsNotFound,
  ErrExist,
  ErrNotExist,
  ErrClosed,
  ErrReadOnly,
  ErrReleased,
  ErrNotDirectory,
  ErrNotFile,
  ErrOutOfBounds,
  ErrEmptyPath,
  ErrAbsolutePath,
  ErrInodeUnresolvable,
  ErrNotSymlink,
  ErrEmptyTimestamp,
  ErrMoveToSelf,
  ErrInvalidWrite,
  ErrEmptyUnixFsId,
  ErrContextCanceled,
  ErrEOF,
  ErrCrossFsRename,
  ErrUnknown,
  isUnixFSError,
} from './errors.js'
import { UnixFSErrorType } from './errors.pb.js'

describe('UnixFSError', () => {
  describe('constructor', () => {
    it('uses provided message', () => {
      const err = new UnixFSError(UnixFSErrorType.NOT_EXIST, 'custom msg')
      expect(err.message).toBe('custom msg')
      expect(err.type).toBe(UnixFSErrorType.NOT_EXIST)
      expect(err.name).toBe('UnixFSError')
    })

    it('falls back to default message when none provided', () => {
      const err = new UnixFSError(UnixFSErrorType.EOF)
      expect(err.message).toBe('EOF')
      expect(err.type).toBe(UnixFSErrorType.EOF)
    })
  })

  describe('fromProto', () => {
    it('returns null for NONE error type', () => {
      expect(
        UnixFSError.fromProto({ errorType: UnixFSErrorType.NONE }),
      ).toBeNull()
    })

    it('returns OTHER error for empty proto (undefined errorType defaults to OTHER)', () => {
      const err = UnixFSError.fromProto({} as { errorType: UnixFSErrorType })
      expect(err).toBeInstanceOf(UnixFSError)
      expect(err!.type).toBe(UnixFSErrorType.OTHER)
    })

    it('creates error for NOT_EXIST type', () => {
      const err = UnixFSError.fromProto({
        errorType: UnixFSErrorType.NOT_EXIST,
      })
      expect(err).toBeInstanceOf(UnixFSError)
      expect(err!.type).toBe(UnixFSErrorType.NOT_EXIST)
      expect(err!.message).toBe('file does not exist')
    })

    it('creates error for RELEASED type', () => {
      const err = UnixFSError.fromProto({
        errorType: UnixFSErrorType.RELEASED,
      })
      expect(err).toBeInstanceOf(UnixFSError)
      expect(err!.type).toBe(UnixFSErrorType.RELEASED)
      expect(err!.message).toBe('cursor or inode released')
    })

    it('creates error for EOF type', () => {
      const err = UnixFSError.fromProto({ errorType: UnixFSErrorType.EOF })
      expect(err).toBeInstanceOf(UnixFSError)
      expect(err!.type).toBe(UnixFSErrorType.EOF)
      expect(err!.isEOF).toBe(true)
    })

    it('creates error for OTHER type with body', () => {
      const err = UnixFSError.fromProto({
        errorType: UnixFSErrorType.OTHER,
        errorBody: 'something went wrong',
      })
      expect(err).toBeInstanceOf(UnixFSError)
      expect(err!.type).toBe(UnixFSErrorType.OTHER)
      expect(err!.message).toBe('something went wrong')
    })

    it('creates error for OTHER type without body', () => {
      const err = UnixFSError.fromProto({ errorType: UnixFSErrorType.OTHER })
      expect(err).toBeInstanceOf(UnixFSError)
      expect(err!.type).toBe(UnixFSErrorType.OTHER)
      expect(err!.message).toBe('unknown unixfs error')
    })

    it('prepends body to default message for typed errors', () => {
      const err = UnixFSError.fromProto({
        errorType: UnixFSErrorType.NOT_EXIST,
        errorBody: 'path /foo',
      })
      expect(err).toBeInstanceOf(UnixFSError)
      expect(err!.type).toBe(UnixFSErrorType.NOT_EXIST)
      expect(err!.message).toBe('path /foo: file does not exist')
    })

    it('handles all error types', () => {
      const types = [
        UnixFSErrorType.FS_NOT_FOUND,
        UnixFSErrorType.EXIST,
        UnixFSErrorType.NOT_EXIST,
        UnixFSErrorType.CLOSED,
        UnixFSErrorType.READ_ONLY,
        UnixFSErrorType.RELEASED,
        UnixFSErrorType.NOT_DIRECTORY,
        UnixFSErrorType.NOT_FILE,
        UnixFSErrorType.OUT_OF_BOUNDS,
        UnixFSErrorType.EMPTY_PATH,
        UnixFSErrorType.ABSOLUTE_PATH,
        UnixFSErrorType.INODE_UNRESOLVABLE,
        UnixFSErrorType.NOT_SYMLINK,
        UnixFSErrorType.EMPTY_TIMESTAMP,
        UnixFSErrorType.MOVE_TO_SELF,
        UnixFSErrorType.INVALID_WRITE,
        UnixFSErrorType.EMPTY_UNIXFS_ID,
        UnixFSErrorType.CONTEXT_CANCELED,
        UnixFSErrorType.EOF,
        UnixFSErrorType.CROSS_FS_RENAME,
      ]
      for (const t of types) {
        const err = UnixFSError.fromProto({ errorType: t })
        expect(err).toBeInstanceOf(UnixFSError)
        expect(err!.type).toBe(t)
        expect(err!.message.length).toBeGreaterThan(0)
      }
    })
  })

  describe('toProto', () => {
    it('round-trips a default-message error', () => {
      const original = new UnixFSError(UnixFSErrorType.NOT_EXIST)
      const proto = original.toProto()
      expect(proto.errorType).toBe(UnixFSErrorType.NOT_EXIST)
      expect(proto.errorBody).toBe('')

      const restored = UnixFSError.fromProto(proto)
      expect(restored!.type).toBe(original.type)
      expect(restored!.message).toBe(original.message)
    })

    it('round-trips a custom-message error', () => {
      const original = new UnixFSError(UnixFSErrorType.OTHER, 'custom problem')
      const proto = original.toProto()
      expect(proto.errorType).toBe(UnixFSErrorType.OTHER)
      expect(proto.errorBody).toBe('custom problem')

      const restored = UnixFSError.fromProto(proto)
      expect(restored!.type).toBe(original.type)
      expect(restored!.message).toBe(original.message)
    })
  })

  describe('convenience getters', () => {
    it('isReleased returns true for RELEASED type', () => {
      const err = new UnixFSError(UnixFSErrorType.RELEASED)
      expect(err.isReleased).toBe(true)
      expect(err.isNotExist).toBe(false)
      expect(err.isEOF).toBe(false)
    })

    it('isNotExist returns true for NOT_EXIST type', () => {
      const err = new UnixFSError(UnixFSErrorType.NOT_EXIST)
      expect(err.isNotExist).toBe(true)
      expect(err.isReleased).toBe(false)
    })

    it('isEOF returns true for EOF type', () => {
      const err = new UnixFSError(UnixFSErrorType.EOF)
      expect(err.isEOF).toBe(true)
      expect(err.isReleased).toBe(false)
    })
  })

  describe('sentinel errors', () => {
    it('each sentinel has the correct type', () => {
      expect(ErrFsNotFound.type).toBe(UnixFSErrorType.FS_NOT_FOUND)
      expect(ErrExist.type).toBe(UnixFSErrorType.EXIST)
      expect(ErrNotExist.type).toBe(UnixFSErrorType.NOT_EXIST)
      expect(ErrClosed.type).toBe(UnixFSErrorType.CLOSED)
      expect(ErrReadOnly.type).toBe(UnixFSErrorType.READ_ONLY)
      expect(ErrReleased.type).toBe(UnixFSErrorType.RELEASED)
      expect(ErrNotDirectory.type).toBe(UnixFSErrorType.NOT_DIRECTORY)
      expect(ErrNotFile.type).toBe(UnixFSErrorType.NOT_FILE)
      expect(ErrOutOfBounds.type).toBe(UnixFSErrorType.OUT_OF_BOUNDS)
      expect(ErrEmptyPath.type).toBe(UnixFSErrorType.EMPTY_PATH)
      expect(ErrAbsolutePath.type).toBe(UnixFSErrorType.ABSOLUTE_PATH)
      expect(ErrInodeUnresolvable.type).toBe(UnixFSErrorType.INODE_UNRESOLVABLE)
      expect(ErrNotSymlink.type).toBe(UnixFSErrorType.NOT_SYMLINK)
      expect(ErrEmptyTimestamp.type).toBe(UnixFSErrorType.EMPTY_TIMESTAMP)
      expect(ErrMoveToSelf.type).toBe(UnixFSErrorType.MOVE_TO_SELF)
      expect(ErrInvalidWrite.type).toBe(UnixFSErrorType.INVALID_WRITE)
      expect(ErrEmptyUnixFsId.type).toBe(UnixFSErrorType.EMPTY_UNIXFS_ID)
      expect(ErrContextCanceled.type).toBe(UnixFSErrorType.CONTEXT_CANCELED)
      expect(ErrEOF.type).toBe(UnixFSErrorType.EOF)
      expect(ErrCrossFsRename.type).toBe(UnixFSErrorType.CROSS_FS_RENAME)
      expect(ErrUnknown.type).toBe(UnixFSErrorType.OTHER)
    })

    it('sentinels are UnixFSError instances', () => {
      expect(ErrReleased).toBeInstanceOf(UnixFSError)
      expect(ErrNotExist).toBeInstanceOf(UnixFSError)
      expect(ErrEOF).toBeInstanceOf(UnixFSError)
    })
  })
})

describe('isUnixFSError', () => {
  it('returns true for UnixFSError without type filter', () => {
    expect(isUnixFSError(ErrReleased)).toBe(true)
  })

  it('returns true when type matches', () => {
    expect(isUnixFSError(ErrReleased, UnixFSErrorType.RELEASED)).toBe(true)
  })

  it('returns false when type does not match', () => {
    expect(isUnixFSError(ErrReleased, UnixFSErrorType.NOT_EXIST)).toBe(false)
  })

  it('returns false for plain Error', () => {
    expect(isUnixFSError(new Error('nope'))).toBe(false)
  })

  it('returns false for non-error values', () => {
    expect(isUnixFSError(null)).toBe(false)
    expect(isUnixFSError(undefined)).toBe(false)
    expect(isUnixFSError('string')).toBe(false)
    expect(isUnixFSError(42)).toBe(false)
  })
})
