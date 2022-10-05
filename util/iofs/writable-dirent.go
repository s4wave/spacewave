package iofs

import "io/fs"

// WritableDirEntry wraps a DirEntry to be writable.
type WritableDirEntry struct {
	fs.DirEntry
}

// NewWritableDirEntry constructs a new writable DirEntry.
func NewWritableDirEntry(ent fs.DirEntry) *WritableDirEntry {
	return &WritableDirEntry{DirEntry: ent}
}

func (e *WritableDirEntry) Info() (fs.FileInfo, error) {
	i, err := e.DirEntry.Info()
	if err != nil {
		return nil, err
	}
	return NewWritableFileInfo(i), nil
}

// _ is a type assertion
var _ fs.DirEntry = ((*WritableDirEntry)(nil))
