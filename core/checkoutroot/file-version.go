//go:build !js

package checkoutroot

import (
	"io/fs"
	"syscall"

	"github.com/go-git/go-billy/v6"
)

// nativeFile exposes metadata on the file descriptor returned by osfs.FromRoot.
type nativeFile interface {
	// Stat reads the open file's metadata.
	Stat() (fs.FileInfo, error)
	// SyscallConn gives the version reader access to the Windows file handle.
	SyscallConn() (syscall.RawConn, error)
}

// readHostFileVersion takes the version from the same open file as the read.
func readHostFileVersion(file billy.File) ([]byte, error) {
	return openFileVersion(file.(nativeFile))
}
