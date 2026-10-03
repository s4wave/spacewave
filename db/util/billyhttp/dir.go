package billyhttp

import (
	"errors"
	"io"
	"io/fs"
	"net/http"
)

// Dir implements the HTTP directory.
type Dir struct {
	// fs is the base filesystem
	fs BillyFs
	// path is the path to this dir
	path string
	// ents holds the listed entries not yet returned by Readdir.
	ents []fs.DirEntry
	// listed is set once Readdir has listed the directory.
	listed bool
}

// NewDir constructs the Dir from a Billy Dir.
func NewDir(fs BillyFs, path string) *Dir {
	return &Dir{fs: fs, path: path}
}

// Stat returns the directory metadata.
func (f *Dir) Stat() (fs.FileInfo, error) {
	return f.fs.Stat(f.path)
}

// Readdir reads the directory contents following the os.File contract.
//
// With count > 0 it returns at most count entries, continuing after the
// entries returned by earlier calls, and io.EOF once none remain. With count
// <= 0 it returns all remaining entries and a nil error.
func (f *Dir) Readdir(count int) ([]fs.FileInfo, error) {
	// List the directory once on the first call.
	if !f.listed {
		ents, err := f.fs.ReadDir(f.path)
		if err != nil {
			return nil, err
		}
		f.ents, f.listed = ents, true
	}

	// Select the next entries to return.
	ents := f.ents
	if count > 0 {
		if len(ents) == 0 {
			return nil, io.EOF
		}
		ents = ents[:min(count, len(ents))]
	}
	f.ents = f.ents[len(ents):]

	// Resolve the selected directory entries to HTTP file metadata.
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

// Read returns an error because a directory has no contents to read.
func (f *Dir) Read(p []byte) (n int, err error) {
	return 0, errors.New("not a file")
}

// Seek returns an error because a directory has no contents to seek.
func (f *Dir) Seek(offset int64, whence int) (int64, error) {
	return 0, errors.New("not a file")
}

// Close releases nothing because Dir holds no open handles.
func (f *Dir) Close() error {
	return nil
}

// _ is a type assertion
var _ http.File = (*Dir)(nil)
