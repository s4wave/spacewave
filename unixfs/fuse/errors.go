//go:build linux
// +build linux

package fuse

import (
	"context"
	"syscall"

	"github.com/aperturerobotics/hydra/tx"
	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
)

// UnixfsErrorToSyscall converts a unixfs error to a syscall error
// returns EIO if the error is not recognized
func UnixfsErrorToSyscall(err error) error {
	switch err {
	case context.Canceled:
		return syscall.EINTR
	case tx.ErrNotWrite:
		return syscall.EROFS
	case unixfs_errors.ErrReadOnly:
		return syscall.EROFS
	case unixfs_errors.ErrExist:
		return syscall.EEXIST
	case unixfs_errors.ErrNotExist:
		return syscall.ENOENT
	case unixfs_errors.ErrReleased:
		return syscall.EBADF
	case unixfs_errors.ErrClosed:
		return syscall.EBADF
	case unixfs_errors.ErrNotFile:
		return syscall.EINVAL
	case unixfs_errors.ErrNotDirectory:
		return syscall.EINVAL
	case unixfs_errors.ErrOutOfBounds:
		return syscall.ERANGE
	case unixfs_errors.ErrEmptyPath:
		return syscall.EINVAL
	case unixfs_errors.ErrInodeUnresolvable:
		fallthrough
	default:
		return syscall.EIO
	}
}
