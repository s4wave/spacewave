package fstree

import "errors"

var (
	// ErrExist is returned if the file or directory already exists.
	ErrExist = errors.New("file already exists")
	// ErrNotExist is returned if the file does not exist.
	ErrNotExist = errors.New("file does not exist")
	// ErrClosed is returned if read on a file that is already closed.
	// Note: many functions return context.Canceled instead.
	ErrClosed = errors.New("file already closed")
	// ErrNotDirectory is returned if mkdir on an inode that is not a directory.
	ErrNotDirectory = errors.New("not a directory")
	// ErrNotFile is returned if write or read on an inode that is not a file.
	ErrNotFile = errors.New("not a file")
	// ErrOutOfBounds indicates a directory index was out of bounds.
	ErrOutOfBounds = errors.New("dirent out of bounds")
)
