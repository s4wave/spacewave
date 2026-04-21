package iofs

import "io/fs"

// WritableFileInfo wraps a FileInfo to set writable permissions.
type WritableFileInfo struct {
	fs.FileInfo
}

// NewWritableFileInfo constructs a new FileInfo.
func NewWritableFileInfo(info fs.FileInfo) *WritableFileInfo {
	return &WritableFileInfo{FileInfo: info}
}

// Mode returns the file info mode.
func (w *WritableFileInfo) Mode() fs.FileMode {
	return w.FileInfo.Mode() | 0o222
}
