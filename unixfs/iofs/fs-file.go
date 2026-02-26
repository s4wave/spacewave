package unixfs_iofs

import (
	"context"
	"io"
	"io/fs"
	"runtime"
	"sync/atomic"

	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
)

// IoFSFile is the set of interfaces FSFile implements.
type IoFSFile interface {
	fs.File
	fs.ReadDirFile
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

// ReadDir reads the contents of the directory and returns
// a slice of up to n DirEntry values in directory order.
// Subsequent calls on the same file will yield further DirEntry values.
//
// If n > 0, ReadDir returns at most n DirEntry structures. In this case, if
// ReadDir returns an empty slice, it will return a non-nil error explaining
// why. At the end of a directory, the error is io.EOF. (ReadDir must return
// io.EOF itself, not an error wrapping io.EOF.)
//
// If n <= 0, ReadDir returns all the DirEntry values from the directory
// in a single slice. In this case, if ReadDir succeeds (reads all the way
// to the end of the directory), it returns the slice and a nil error.
// If it encounters an error before the end of the directory,
// ReadDir returns the DirEntry list read until that point and a non-nil error.
func (f *FSFile) ReadDir(count int) ([]fs.DirEntry, error) {
	nodeType, err := f.handle.GetNodeType(f.ctx)
	if err != nil {
		return nil, err
	}
	if !nodeType.GetIsDirectory() {
		return nil, unixfs_errors.ErrNotDirectory
	}

	var idx uint64
	idxBefore := f.idx.Load()
	if idxBefore > 0 {
		idx = uint64(idxBefore)
	}
	if count < 0 {
		count = 0
	}

	ents, err := unixfs.ReaddirAllToDirEntries(f.ctx, idx, uint64(count), f.handle) //nolint:gosec
	if err == nil {
		nents := len(ents)
		if nents == 0 {
			if count > 0 {
				err = io.EOF
			}
		} else {
			f.idx.Add(int64(len(ents)))
		}
	}

	return ents, err
}

// Read reads data from the file.
func (f *FSFile) Read(data []byte) (int, error) {
	idx := f.idx.Load()
	rn, err := f.handle.ReadAt(f.ctx, idx, data)
	if rn != 0 {
		f.idx.Add(rn)
	}
	return int(rn), err
}

// ReadAt reads data from a location in the file.
func (f *FSFile) ReadAt(p []byte, off int64) (n int, err error) {
	rn, err := f.handle.ReadAt(f.ctx, off, p)
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
		out = int64(size) + offset //nolint:gosec
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
