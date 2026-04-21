package billyhttp

import (
	"errors"
	"io/fs"
	"net/http"

	"github.com/go-git/go-billy/v6"
)

// File implements the HTTP file.
type File struct {
	// File is the billy file
	billy.File
	// path is the path to File
	path string
	// fs is the filesystem
	fs BillyFs
}

// NewFile constructs the File from a Billy File.
func NewFile(fs BillyFs, path string) (*File, error) {
	f, err := fs.Open(path)
	if err != nil {
		return nil, err
	}
	return &File{File: f, path: path, fs: fs}, nil
}

func (f *File) Readdir(count int) ([]fs.FileInfo, error) {
	// ENOTDIR
	return nil, errors.New("not a directory")
}

func (f *File) Stat() (fs.FileInfo, error) {
	return f.fs.Stat(f.path)
}

// _ is a type assertion
var _ http.File = ((*File)(nil))
