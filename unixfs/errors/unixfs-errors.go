// Package unixfs_errors contains common error definitions.
package unixfs_errors

import (
	"errors"
	"os"

	"github.com/aperturerobotics/timestamp"
)

var (
	// ErrExist is returned if the file or directory already exists.
	ErrExist = os.ErrExist
	// ErrNotExist is returned if the file does not exist.
	ErrNotExist = os.ErrNotExist
	// ErrClosed is returned if read on a file that is already closed.
	// Note: many functions return context.Canceled instead.
	ErrClosed = os.ErrClosed

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
	// ErrInodeUnresolvable is returned if the inode could not be resolved in time.
	ErrInodeUnresolvable = errors.New("inode unable to be resolved")
	// ErrNotSymlink is returned if readlink is called on a non-symlink entry.
	ErrNotSymlink = errors.New("not a symlink")
	// ErrEmptyTimestamp is returned if a timestamp cannot be empty.
	ErrEmptyTimestamp = timestamp.ErrEmptyTimestamp
	// ErrMoveToSelf is returned if attempting to move or copy a path to itself.
	ErrMoveToSelf = errors.New("cannot copy/move a path into itself")
)
