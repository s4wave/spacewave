package billyhttp

import (
	"net/http"
	"path"

	"github.com/go-git/go-billy/v5"
)

// BillyFs is the set of required billy filesystem interfaces.
type BillyFs interface {
	billy.Basic
	billy.Dir
}

// FileSystem implements the HTTP filesystem.
type FileSystem struct {
	// fs is the billy filesystem
	fs BillyFs
}

// NewFileSystem constructs the FileSystem from a Billy FileSystem.
func NewFileSystem(fs BillyFs) *FileSystem {
	return &FileSystem{fs: fs}
}

// Open opens the file at the given path.
func (f *FileSystem) Open(name string) (http.File, error) {
	// Determine if file or dir.
	name = path.Clean(name)
	fi, err := f.fs.Stat(name)
	if err != nil {
		return nil, err
	}
	if fi.IsDir() {
		return NewDir(f.fs, name), nil
	}
	return NewFile(f.fs, name)
}

// _ is a type assertion
var _ http.FileSystem = ((*FileSystem)(nil))
