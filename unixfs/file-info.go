package unixfs

import (
	"io/fs"
	"time"
)

// FileInfo contains information about a file.
type FileInfo struct {
	name    string
	size    int64
	mode    fs.FileMode
	modTime time.Time
}

// NewFileInfo constructs a new file info with details.
func NewFileInfo(
	name string,
	size int64,
	mode fs.FileMode,
	modTime time.Time,
) *FileInfo {
	return &FileInfo{
		name:    name,
		size:    size,
		mode:    mode,
		modTime: modTime,
	}
}

// Name returns the name of the file.
func (i *FileInfo) Name() string {
	return i.name
}

// Size returns length in bytes for regular files; system-dependent for others
func (i *FileInfo) Size() int64 {
	return i.size
}

// Mode is the unixfs file mode bitset.
func (i *FileInfo) Mode() fs.FileMode {
	return i.mode
}

// ModTime is the modification time.
func (i *FileInfo) ModTime() time.Time {
	return i.modTime
}

// IsDir is an abbreviation for Mode().IsDir()
func (i *FileInfo) IsDir() bool {
	return i.mode.IsDir()
}

// Sys returns the underlying data source (nil here).
func (i *FileInfo) Sys() any {
	return nil
}

// _ is a type assertion
var _ fs.FileInfo = ((*FileInfo)(nil))
