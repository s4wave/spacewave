package wazerofs

import (
	"context"
	"io/fs"

	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
	wazero_exp_sys "github.com/tetratelabs/wazero/experimental/sys"
)

// UnixfsErrorToWazeroErrno converts a unixfs error to a wazero experimental syscall errno.
// Returns EIO if the error is not recognized.
func UnixfsErrorToWazeroErrno(err error) wazero_exp_sys.Errno {
	if _, ok := err.(*fs.PathError); ok {
		return wazero_exp_sys.EINVAL
	}

	switch err {
	case context.Canceled:
		return wazero_exp_sys.EINTR
	case unixfs_errors.ErrReadOnly:
		return wazero_exp_sys.EROFS
	case unixfs_errors.ErrExist:
		return wazero_exp_sys.EEXIST
	case unixfs_errors.ErrNotExist:
		return wazero_exp_sys.ENOENT
	case unixfs_errors.ErrReleased:
		return wazero_exp_sys.EBADF
	case unixfs_errors.ErrClosed:
		return wazero_exp_sys.EBADF
	case unixfs_errors.ErrNotFile:
		return wazero_exp_sys.ENOTDIR
	case unixfs_errors.ErrNotDirectory:
		return wazero_exp_sys.EISDIR
	case unixfs_errors.ErrOutOfBounds:
		return wazero_exp_sys.ERANGE
	case unixfs_errors.ErrEmptyPath:
		return wazero_exp_sys.EINVAL
	case unixfs_errors.ErrInodeUnresolvable:
		return wazero_exp_sys.EIO
	default:
		return wazero_exp_sys.EIO
	}
}
