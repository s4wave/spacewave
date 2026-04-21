package iofs

import (
	"errors"
	"io/fs"

	unixfs_iofs "github.com/s4wave/spacewave/db/unixfs/iofs"
)

// WritableFS wraps an embed.FS to change default permissions to writable.
type WritableFS struct {
	fs.FS
}

// NewWritableFS wraps a io/fs.FS to make files writable.
func NewWritableFS(fs fs.FS) *WritableFS {
	return &WritableFS{FS: fs}
}

// Open opens the named file.
//
// When Open returns an error, it should be of type *PathError
// with the Op field set to "open", the Path field set to name,
// and the Err field describing the problem.
//
// Open should reject attempts to open names that do not satisfy
// ValidPath(name), returning a *PathError with Err set to
// ErrInvalid or ErrNotExist.
func (w *WritableFS) Open(name string) (fs.File, error) {
	f, err := w.FS.Open(name)
	if err != nil {
		return nil, err
	}
	ff, ffOk := f.(IoFSFile)
	if !ffOk {
		return nil, errors.New("io/fs: file must implement io.Seeker")
	}
	return NewWritableFile(ff), nil
}

// Stat returns a FileInfo describing the file.
// If there is an error, it should be of type *PathError.
func (w *WritableFS) Stat(name string) (fs.FileInfo, error) {
	fi, err := fs.Stat(w.FS, name)
	if err != nil {
		return nil, err
	}
	return NewWritableFileInfo(fi), nil
}

// ReadDir reads the named directory and returns a list of directory entries
// sorted by filename.
func (w *WritableFS) ReadDir(name string) ([]fs.DirEntry, error) {
	dirents, err := fs.ReadDir(w.FS, name)
	if err != nil {
		return nil, err
	}
	writableEnts := make([]fs.DirEntry, len(dirents))
	for i, ent := range dirents {
		writableEnts[i] = NewWritableDirEntry(ent)
	}
	return writableEnts, nil
}

// ReadFile reads the named file and returns its contents.
// A successful call returns a nil error, not io.EOF.
// (Because ReadFile reads the whole file, the expected EOF
// from the final Read is not treated as an error to be reported.)
//
// The caller is permitted to modify the returned byte slice.
// This method should return a copy of the underlying data.
func (w *WritableFS) ReadFile(name string) ([]byte, error) {
	return fs.ReadFile(w.FS, name)
}

// _ is a type assertion
var _ unixfs_iofs.IoFS = ((*WritableFS)(nil))
