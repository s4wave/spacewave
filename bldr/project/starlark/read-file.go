//go:build !js

package bldr_project_starlark

import (
	"io"
	"io/fs"

	"github.com/pkg/errors"
)

// MaxFileSize is the largest file ReadFile returns and the largest message the
// bounded evaluation exchanges.
const MaxFileSize = 16 << 20

// ErrFileTooLarge is returned by ReadFile for a file larger than MaxFileSize.
var ErrFileTooLarge = errors.New("file is too large")

// ReadFile reads the file name from fsys for a caller that does not trust the
// repository. It stops at MaxFileSize, so a huge file cannot fill the memory of
// the calling process, and returns ErrFileTooLarge instead.
func ReadFile(fsys fs.FS, name string) ([]byte, error) {
	// Open the file and read one byte past the limit to detect an oversized one.
	f, err := fsys.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, MaxFileSize+1))
	if err != nil {
		return nil, errors.Wrapf(err, "read %s", name)
	}
	if len(data) > MaxFileSize {
		return nil, errors.Wrapf(ErrFileTooLarge, "read %s", name)
	}
	return data, nil
}
