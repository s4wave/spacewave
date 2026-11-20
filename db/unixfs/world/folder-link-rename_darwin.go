//go:build darwin

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
// newpath exists.
func renameNoReplace(oldpath, newpath string) error {
	if err := unix.RenamexNp(oldpath, newpath, unix.RENAME_EXCL); err != nil {
		return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: err}
	}
	return nil
}

// exchange atomically swaps the files at a and b.
func exchange(a, b string) error {
	err := unix.RenamexNp(a, b, unix.RENAME_SWAP)
	if errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EINVAL) {
		return errExchangeUnsupported
	}
	if err != nil {
		return &os.LinkError{Op: "exchange", Old: a, New: b, Err: err}
	}
	return nil
}
