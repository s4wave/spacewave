package billyhttp

import (
	"net/http"
	"path"
	"strings"

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
	// prefix is the filesystem prefix for HTTP
	prefix string
}

// NewFileSystem constructs the FileSystem from a Billy FileSystem.
//
// Prefix is a path prefix to prepend to file paths for HTTP.
// The prefix is trimmed from the paths when opening files.
func NewFileSystem(fs BillyFs, prefix string) *FileSystem {
	if len(prefix) != 0 {
		prefix = path.Clean(prefix)
	}
	return &FileSystem{fs: fs, prefix: prefix}
}

// Open opens the file at the given path.
func (f *FileSystem) Open(name string) (http.File, error) {
	name = path.Clean(name)
	if strings.HasPrefix(name, "/") {
		name = name[1:]
	}
	if len(f.prefix) != 0 {
		name = strings.TrimPrefix(name, f.prefix)
		name = path.Clean(name)
	}
	if strings.HasPrefix(name, "/") {
		name = name[1:]
	}

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
