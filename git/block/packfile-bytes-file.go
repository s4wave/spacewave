package git_block

import (
	"io"
	"io/fs"
	"time"

	"github.com/pkg/errors"
)

type packfileBytesFile struct {
	name string
	rdr  *bytesReaderAt
}

func newPackfileBytesFile(name string, data []byte) *packfileBytesFile {
	return &packfileBytesFile{name: name, rdr: &bytesReaderAt{data: data}}
}

func (f *packfileBytesFile) Name() string {
	return f.name
}

func (f *packfileBytesFile) Stat() (fs.FileInfo, error) {
	return packfileBytesInfo{name: f.name, size: int64(len(f.rdr.data))}, nil
}

func (f *packfileBytesFile) Read(p []byte) (int, error) {
	return f.rdr.Read(p)
}

func (f *packfileBytesFile) ReadAt(p []byte, off int64) (int, error) {
	return f.rdr.ReadAt(p, off)
}

func (f *packfileBytesFile) Seek(offset int64, whence int) (int64, error) {
	return f.rdr.Seek(offset, whence)
}

func (f *packfileBytesFile) Write([]byte) (int, error) {
	return 0, errors.New("packfile bytes file is read-only")
}

func (f *packfileBytesFile) WriteAt([]byte, int64) (int, error) {
	return 0, errors.New("packfile bytes file is read-only")
}

func (f *packfileBytesFile) Truncate(int64) error {
	return errors.New("packfile bytes file is read-only")
}

func (f *packfileBytesFile) Close() error {
	return nil
}

type packfileBytesInfo struct {
	name string
	size int64
}

func (i packfileBytesInfo) Name() string {
	return i.name
}

func (i packfileBytesInfo) Size() int64 {
	return i.size
}

func (i packfileBytesInfo) Mode() fs.FileMode {
	return 0o444
}

func (i packfileBytesInfo) ModTime() time.Time {
	return time.Time{}
}

func (i packfileBytesInfo) IsDir() bool {
	return false
}

func (i packfileBytesInfo) Sys() any {
	return nil
}

type bytesReaderAt struct {
	data []byte
	pos  int64
}

func (r *bytesReaderAt) Read(p []byte) (int, error) {
	n, err := r.ReadAt(p, r.pos)
	r.pos += int64(n)
	return n, err
}

func (r *bytesReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, errors.New("negative offset")
	}
	if off >= int64(len(r.data)) {
		return 0, io.EOF
	}
	n := copy(p, r.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (r *bytesReaderAt) Seek(offset int64, whence int) (int64, error) {
	next := offset
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		next += r.pos
	case io.SeekEnd:
		next += int64(len(r.data))
	default:
		return 0, errors.New("invalid seek whence")
	}
	if next < 0 {
		return 0, errors.New("negative seek position")
	}
	r.pos = next
	return next, nil
}
