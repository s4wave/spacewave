// Package unixfs_errors contains common error definitions.
package unixfs_errors

import (
	"context"
	io "io"
	"io/fs"
	"strings"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
)

var (
	// ErrFsNotFound is returned if the unixfs was not found by id.
	ErrFsNotFound = errors.New("fs not found")
	// ErrExist is returned if the file or directory already exists.
	ErrExist = fs.ErrExist
	// ErrNotExist is returned if the file does not exist.
	ErrNotExist = fs.ErrNotExist
	// ErrClosed is returned if read on a file that is already closed.
	// Note: many functions return context.Canceled instead.
	ErrClosed = fs.ErrClosed

	// ErrReadOnly is returned if the FSCursor is read-only.
	ErrReadOnly = errors.New("read-only fs")
	// ErrReleased is returned if a FSCursor or Inode are released.
	ErrReleased = errors.New("cursor or inode released")
	// ErrNotDirectory is returned if mkdir on an inode that is not a directory.
	ErrNotDirectory = errors.New("not a directory")
	// ErrNotFile is returned if write or read on an inode that is not a file.
	ErrNotFile = errors.New("not a file")
	// ErrOutOfBounds indicates a directory index was out of bounds.
	ErrOutOfBounds = errors.New("dirent out of bounds")
	// ErrEmptyPath is returned if the FSPath was empty.
	ErrEmptyPath = errors.New("empty path")
	// ErrAbsolutePath is returned if the FSPath cannot be absolute (but was).
	ErrAbsolutePath = errors.New("absolute path not allowed")
	// ErrInodeUnresolvable is returned if the inode could not be resolved in time.
	ErrInodeUnresolvable = errors.New("inode unable to be resolved")
	// ErrNotSymlink is returned if readlink is called on a non-symlink entry.
	ErrNotSymlink = errors.New("not a symlink")
	// ErrEmptyTimestamp is returned if a timestamp cannot be empty but was empty.
	ErrEmptyTimestamp = timestamppb.ErrEmptyTimestamp
	// ErrMoveToSelf is returned if attempting to move or copy a path to itself.
	ErrMoveToSelf = errors.New("cannot copy/move a path into itself")
	// ErrInvalidWrite means that a write returned an impossible count.
	ErrInvalidWrite = errors.New("invalid write result")
	// ErrEmptyUnixFsId is returned if the filesystem id is empty.
	ErrEmptyUnixFsId = errors.New("empty unixfs id")
	// ErrCrossFsRename is retruned if we try to rename across two unrelated FS.
	ErrCrossFsRename = errors.New("cross-fs rename unimplemented")
	// ErrUnknown is returned if a remote service returned an unknown error.
	ErrUnknown = errors.New("unknown unixfs error")
)

// NewUnixFSError converts a Go error into a UnixFS error.
func NewUnixFSError(err error) *UnixFSError {
	if err == nil {
		return nil
	}

	// Unwrap the error to find the base error.
	uErr := &UnixFSError{}
	baseErr := err
	for {
		nerr := errors.Unwrap(err)
		if nerr == nil || nerr == baseErr {
			break
		}
		baseErr = nerr
	}

	// If we unwrapped the error, set ErrorType to the prefix, trimming the : suffix.
	if baseErr != err {
		errStr := err.Error()
		baseErrStr := baseErr.Error()
		if errStr == "" {
			errStr = ErrUnknown.Error()
		}
		uErr.ErrorBody = strings.TrimSuffix(
			strings.TrimSpace(strings.TrimSuffix(errStr, baseErrStr)),
			":",
		)
	}

	// Determine the UnixFSErrorType.
	switch baseErr {
	case ErrFsNotFound:
		uErr.ErrorType = UnixFSErrorType_FS_NOT_FOUND
	case ErrExist:
		uErr.ErrorType = UnixFSErrorType_EXIST
	case ErrNotExist:
		uErr.ErrorType = UnixFSErrorType_NOT_EXIST
	case ErrClosed:
		uErr.ErrorType = UnixFSErrorType_CLOSED
	case ErrReadOnly:
		uErr.ErrorType = UnixFSErrorType_READ_ONLY
	case ErrReleased:
		uErr.ErrorType = UnixFSErrorType_RELEASED
	case ErrNotDirectory:
		uErr.ErrorType = UnixFSErrorType_NOT_DIRECTORY
	case ErrNotFile:
		uErr.ErrorType = UnixFSErrorType_NOT_FILE
	case ErrOutOfBounds:
		uErr.ErrorType = UnixFSErrorType_OUT_OF_BOUNDS
	case ErrEmptyPath:
		uErr.ErrorType = UnixFSErrorType_EMPTY_PATH
	case ErrAbsolutePath:
		uErr.ErrorType = UnixFSErrorType_ABSOLUTE_PATH
	case ErrInodeUnresolvable:
		uErr.ErrorType = UnixFSErrorType_INODE_UNRESOLVABLE
	case ErrNotSymlink:
		uErr.ErrorType = UnixFSErrorType_NOT_SYMLINK
	case ErrEmptyTimestamp:
		uErr.ErrorType = UnixFSErrorType_EMPTY_TIMESTAMP
	case ErrMoveToSelf:
		uErr.ErrorType = UnixFSErrorType_MOVE_TO_SELF
	case ErrInvalidWrite:
		uErr.ErrorType = UnixFSErrorType_INVALID_WRITE
	case ErrEmptyUnixFsId:
		uErr.ErrorType = UnixFSErrorType_EMPTY_UNIXFS_ID
	case ErrCrossFsRename:
		uErr.ErrorType = UnixFSErrorType_CROSS_FS_RENAME
	case context.Canceled:
		uErr.ErrorType = UnixFSErrorType_CONTEXT_CANCELED
	case io.EOF:
		uErr.ErrorType = UnixFSErrorType_EOF
	default:
		uErr.ErrorType = UnixFSErrorType_OTHER
		if uErr.ErrorBody == "" {
			uErr.ErrorBody = baseErr.Error()
		}
	}

	return uErr
}

// ToGoError converts a UnixFSError into the corresponding Go error from unixfs_errors.
func (e *UnixFSError) ToGoError() error {
	if e == nil {
		return nil
	}

	var err error
	switch e.ErrorType {
	case UnixFSErrorType_NONE:
		return nil
	case UnixFSErrorType_FS_NOT_FOUND:
		err = ErrFsNotFound
	case UnixFSErrorType_EXIST:
		err = ErrExist
	case UnixFSErrorType_NOT_EXIST:
		err = ErrNotExist
	case UnixFSErrorType_CLOSED:
		err = ErrClosed
	case UnixFSErrorType_READ_ONLY:
		err = ErrReadOnly
	case UnixFSErrorType_RELEASED:
		err = ErrReleased
	case UnixFSErrorType_NOT_DIRECTORY:
		err = ErrNotDirectory
	case UnixFSErrorType_NOT_FILE:
		err = ErrNotFile
	case UnixFSErrorType_OUT_OF_BOUNDS:
		err = ErrOutOfBounds
	case UnixFSErrorType_EMPTY_PATH:
		err = ErrEmptyPath
	case UnixFSErrorType_ABSOLUTE_PATH:
		err = ErrAbsolutePath
	case UnixFSErrorType_INODE_UNRESOLVABLE:
		err = ErrInodeUnresolvable
	case UnixFSErrorType_NOT_SYMLINK:
		err = ErrNotSymlink
	case UnixFSErrorType_EMPTY_TIMESTAMP:
		err = ErrEmptyTimestamp
	case UnixFSErrorType_MOVE_TO_SELF:
		err = ErrMoveToSelf
	case UnixFSErrorType_INVALID_WRITE:
		err = ErrInvalidWrite
	case UnixFSErrorType_EMPTY_UNIXFS_ID:
		err = ErrEmptyUnixFsId
	case UnixFSErrorType_CROSS_FS_RENAME:
		err = ErrCrossFsRename
	case UnixFSErrorType_CONTEXT_CANCELED:
		err = context.Canceled
	case UnixFSErrorType_EOF:
		err = io.EOF
	case UnixFSErrorType_OTHER:
		if e.ErrorBody != "" {
			return errors.New(e.ErrorBody)
		}
		// if type is OTHER and ErrorBody is empty, return unknown unixfs error
		fallthrough
	default:
		return ErrUnknown
	}

	if e.ErrorBody != "" {
		return errors.Wrap(err, e.ErrorBody)
	}

	return err
}
