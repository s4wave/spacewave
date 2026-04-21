package iofs

import (
	"io"
	"io/fs"
)

// IoFSFile is the minimum interface that File must implement.
type IoFSFile interface {
	fs.File
	io.Seeker
}

// WritableFile wraps an fs.File to set writable permissions.
type WritableFile struct {
	IoFSFile
}

// NewWritableFile constructs a new WritableFile.
func NewWritableFile(file IoFSFile) *WritableFile {
	return &WritableFile{IoFSFile: file}
}

func (f *WritableFile) Stat() (fs.FileInfo, error) {
	fileInfo, err := f.IoFSFile.Stat()
	if err != nil {
		return nil, err
	}
	return NewWritableFileInfo(fileInfo), nil
}

// _ is a type assertion
var _ IoFSFile = ((*WritableFile)(nil))
