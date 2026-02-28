package billyhttp

import (
	"errors"
	"io/fs"
	"net/http"
)

// Dir implements the HTTP directory.
type Dir struct {
	// fs is the base filesysetm
	fs BillyFs
	// path is the path to this dir
	path string
}

// NewDir constructs the Dir from a Billy Dir.
func NewDir(fs BillyFs, path string) *Dir {
	return &Dir{fs: fs, path: path}
}

func (f *Dir) Stat() (fs.FileInfo, error) {
	return f.fs.Stat(f.path)
}

// Readdir reads the directory contents.
func (f *Dir) Readdir(count int) ([]fs.FileInfo, error) {
	ents, err := f.fs.ReadDir(f.path)
	if err != nil {
		return nil, err
	}
	if count > 0 && count < len(ents) {
		ents = ents[:count]
	}
	fis := make([]fs.FileInfo, 0, len(ents))
	for _, ent := range ents {
		fi, err := ent.Info()
		if err != nil {
			return nil, err
		}
		fis = append(fis, fi)
	}
	return fis, nil
}

func (f *Dir) Read(p []byte) (n int, err error) {
	return 0, errors.New("not a file")
}

func (f *Dir) Seek(offset int64, whence int) (int64, error) {
	return 0, errors.New("not a file")
}

func (f *Dir) Close() error {
	// no-op.
	return nil
}

// _ is a type assertion
var _ http.File = ((*Dir)(nil))
