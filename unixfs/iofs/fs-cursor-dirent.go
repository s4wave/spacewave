package unixfs_iofs

import (
	"io/fs"

	"github.com/aperturerobotics/hydra/unixfs"
)

// FSCursorDirent is a directory entry wrapped to be a FSCursorDirent.
type FSCursorDirent struct {
	// dirent is the directory entry
	dirent fs.DirEntry
}

// NewFSCursorDirent constructs a FSCursorDirent from a fs.DirEntry.
func NewFSCursorDirent(dirent fs.DirEntry) *FSCursorDirent {
	return &FSCursorDirent{dirent: dirent}
}

// GetName returns the name of the directory entry.
func (e *FSCursorDirent) GetName() string {
	return e.dirent.Name()
}

// GetIsDirectory returns if the node is a directory.
func (e *FSCursorDirent) GetIsDirectory() bool {
	return e.dirent.IsDir()
}

// GetIsFile returns if the node is a regular file.
func (e *FSCursorDirent) GetIsFile() bool {
	return e.dirent.Type().IsRegular()
}

// GetIsSymlink returns if the node is a symlink.
func (e *FSCursorDirent) GetIsSymlink() bool {
	return e.dirent.Type()&fs.ModeSymlink != 0
}

// _ is a type assertion
var _ unixfs.FSCursorDirent = ((*FSCursorDirent)(nil))
