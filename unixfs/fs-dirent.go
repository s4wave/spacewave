package unixfs

import (
	"io/fs"
	"os"

	"github.com/pkg/errors"
)

// FSDirEntry implements fs.DirEntry with a FSCursorDirent and associated FileInfo.
type FSDirEntry struct {
	// ent is the directory entry
	ent FSCursorDirent
	// fileInfo is the file information
	fileInfo fs.FileInfo
}

// NewFSDirEntry constructs a new directory entry from a unixfs.FSCursorDirent.
//
// fileInfo can be nil but will return ErrInfoUnavailable for
// if fileInfo is nil permissions will be set to defaults.
func NewFSDirEntry(ent FSCursorDirent, fileInfo fs.FileInfo) fs.DirEntry {
	return &FSDirEntry{
		ent:      ent,
		fileInfo: fileInfo,
	}
}

// Name returns the name of the file (or subdirectory) described by the entry.
// This name is only the final element of the path (the base name), not the entire path.
// For example, Name would return "hello.go" not "home/gopher/hello.go".
func (e *FSDirEntry) Name() string {
	return e.ent.GetName()
}

// IsDir reports whether the entry describes a directory.
func (e *FSDirEntry) IsDir() bool {
	return e.ent.GetIsDirectory()
}

// Type returns the type bits for the entry.
// The type bits are a subset of the usual FileMode bits, those returned by the FileMode.Type method.
func (e *FSDirEntry) Type() fs.FileMode {
	var defaultMode fs.FileMode
	if e.fileInfo != nil {
		defaultMode = e.fileInfo.Mode()
	} else {
		if e.ent.GetIsDirectory() {
			defaultMode = fs.ModeDir | 0555
		} else {
			defaultMode = 0444
		}
	}

	typ := NodeTypeToMode(e.ent, defaultMode.Perm())
	if typ == os.ModeIrregular {
		return defaultMode
	}
	return typ
}

// Info returns the FileInfo for the file or subdirectory described by the entry.
// The returned FileInfo may be from the time of the original directory read
// or from the time of the call to Info. If the file has been removed or renamed
// since the directory read, Info may return an error satisfying errors.Is(err, ErrNotExist).
// If the entry denotes a symbolic link, Info reports the information about the link itself,
// not the link's target.
func (e *FSDirEntry) Info() (fs.FileInfo, error) {
	if e.fileInfo == nil {
		return nil, errors.New("file info unavailable")
	}
	return e.fileInfo, nil
}

// _ is a type assertion
var _ fs.DirEntry = (*FSDirEntry)(nil)
