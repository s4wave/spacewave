package unixfs_iofs

import (
	"context"
	"io"
	"io/fs"
	"runtime"

	"github.com/aperturerobotics/hydra/unixfs"
	"go.uber.org/atomic"
)

// IoFSFile is the set of interfaces FSFile implements.
type IoFSFile interface {
	fs.File
	io.ReaderAt
	io.Seeker
}

// FSFile implements the fs.File interface.
type FSFile struct {
	// ctx is the context for ops
	ctx context.Context
	// handle is the fs handle
	handle *unixfs.FSHandle
	// idx is the current file index
	// note: concurrent read() calls have undefined behavior.
	// while not expected, the atomic integer will protect against concurrent access.
	// note: concurrent ReadAt calls will work correctly (even during a Write()).
	idx atomic.Int64
}

// NewFS constructs a new fs.FS from a FSHandle.
//
// Returns nil if handle == nil.
func NewFSFile(ctx context.Context, handle *unixfs.FSHandle) *FSFile {
	if handle == nil {
		return nil
	}
	fsFile := &FSFile{ctx: ctx, handle: handle}
	// set a finalizer in case the caller forgets to call Close.
	runtime.SetFinalizer(fsFile, func(f *FSFile) {
		_ = f.Close()
	})
	return fsFile
}

// Stat returns file information about the file.
func (f *FSFile) Stat() (fs.FileInfo, error) {
	return f.handle.GetFileInfo(f.ctx)
}

// Read reads data from the file.
func (f *FSFile) Read(data []byte) (int, error) {
	idx := f.idx.Load()
	rn, err := f.handle.Read(f.ctx, idx, data)
	if rn != 0 {
		f.idx.Add(rn)
	}
	return int(rn), err
}

// ReadAt reads data from a location in the file.
func (f *FSFile) ReadAt(p []byte, off int64) (n int, err error) {
	rn, err := f.handle.Read(f.ctx, off, p)
	return int(rn), err
}

// Seek moves to a location in the file.
func (f *FSFile) Seek(offset int64, whence int) (int64, error) {
	var out int64
	switch whence {
	case io.SeekCurrent:
		out = f.idx.Add(offset)
	case io.SeekStart:
		f.idx.Store(offset)
		out = offset
	case io.SeekEnd:
		size, err := f.handle.GetSize(f.ctx)
		if err != nil {
			return 0, err
		}
		out = int64(size) - offset
		f.idx.Store(out)
	}
	if out < 0 {
		return out, io.EOF
	}
	return out, nil
}

// Close closes the file handle.
func (f *FSFile) Close() error {
	f.handle.Release()
	return nil
}

// _ is a type assertion
var _ IoFSFile = ((*FSFile)(nil))
