//go:build linux

package unixfs_world

import (
	"os"

	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

// twoWaySupported reports that this platform has the renames and the trash a
// two-way link needs.
const twoWaySupported = true

// errExchangeUnsupported marks a file system without an atomic exchange.
var errExchangeUnsupported = errors.New("file system cannot exchange files atomically")

// renameNoReplace renames oldpath to newpath, failing with fs.ErrExist when
// newpath exists. A file system without RENAME_NOREPLACE links the file under
// newpath, which fails the same way, then unlinks oldpath.
func renameNoReplace(oldpath, newpath string) error {
	err := unix.Renameat2(unix.AT_FDCWD, oldpath, unix.AT_FDCWD, newpath, unix.RENAME_NOREPLACE)
	if errors.Is(err, unix.EINVAL) || errors.Is(err, unix.ENOSYS) {
		if err := os.Link(oldpath, newpath); err != nil {
			return err
		}
		return os.Remove(oldpath)
	}
	if err != nil {
		return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: err}
	}
	return nil
}

// exchange atomically swaps the files at a and b.
func exchange(a, b string) error {
	err := unix.Renameat2(unix.AT_FDCWD, a, unix.AT_FDCWD, b, unix.RENAME_EXCHANGE)
	if errors.Is(err, unix.EINVAL) || errors.Is(err, unix.ENOSYS) {
		return errExchangeUnsupported
	}
	if err != nil {
		return &os.LinkError{Op: "exchange", Old: a, New: b, Err: err}
	}
	return nil
}
