package unixfs_sync

import (
	"os"
	"time"

	"github.com/go-git/go-billy/v6"
)

// rootChtimesFS adds file times and permissions to a Billy disk filesystem
// without leaving its root directory.
type rootChtimesFS struct {
	// Filesystem is the disk filesystem all other operations go to.
	billy.Filesystem
	// root is the disk directory the filesystem is rooted at.
	root *os.Root
}

// Chtimes changes the access and modification times of the named file.
func (f *rootChtimesFS) Chtimes(name string, atime, mtime time.Time) error {
	return f.root.Chtimes(name, atime, mtime)
}

// Chmod changes permissions within the same output root as file writes.
func (f *rootChtimesFS) Chmod(name string, mode os.FileMode) error {
	return f.root.Chmod(name, mode)
}

// _ is a type assertion
var (
	_ modTimeSetter = (*rootChtimesFS)(nil)
	_ billy.Chmod   = (*rootChtimesFS)(nil)
)
