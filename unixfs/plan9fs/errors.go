package plan9fs

import (
	"context"
	"errors"

	"github.com/aperturerobotics/hydra/tx"
	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
)

// errShortRead is returned when a buffer read runs out of data.
var errShortRead = errors.New("short read in 9p message")

// errUnsupported is returned for unsupported 9p operations.
var errUnsupported = errors.New("operation not supported")

// errTooManyNames is returned when TWALK has more than maxWalkNames.
var errTooManyNames = errors.New("too many walk names")

// toErrno converts a Go error to a Linux errno value.
// Uses errors.Is to handle wrapped errors correctly.
func toErrno(err error) uint32 {
	if err == nil {
		return 0
	}
	switch {
	case errors.Is(err, errUnsupported):
		return ENOTSUP
	case errors.Is(err, errTooManyNames):
		return EINVAL
	case errors.Is(err, errBadFid):
		return EBADF
	case errors.Is(err, errFidInUse):
		return EEXIST
	case errors.Is(err, context.Canceled):
		return EINTR
	case errors.Is(err, tx.ErrNotWrite):
		return EROFS
	case errors.Is(err, unixfs_errors.ErrReadOnly):
		return EROFS
	case errors.Is(err, unixfs_errors.ErrExist):
		return EEXIST
	case errors.Is(err, unixfs_errors.ErrNotExist):
		return ENOENT
	case errors.Is(err, unixfs_errors.ErrReleased):
		return EBADF
	case errors.Is(err, unixfs_errors.ErrClosed):
		return EBADF
	case errors.Is(err, unixfs_errors.ErrNotFile):
		return EINVAL
	case errors.Is(err, unixfs_errors.ErrNotDirectory):
		return ENOTDIR
	case errors.Is(err, unixfs_errors.ErrOutOfBounds):
		return ERANGE
	case errors.Is(err, unixfs_errors.ErrEmptyPath):
		return EINVAL
	case errors.Is(err, unixfs_errors.ErrNotSymlink):
		return EINVAL
	case errors.Is(err, unixfs_errors.ErrMoveToSelf):
		return EINVAL
	case errors.Is(err, unixfs_errors.ErrCrossFsRename):
		return EINVAL
	default:
		return EIO
	}
}
