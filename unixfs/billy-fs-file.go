package unixfs

import (
	"context"
	"io"
	"time"

	"github.com/go-git/go-billy/v5"
)

// BillyFSFile implements the Billy filesystem File interface with a FSHandle.
type BillyFSFile struct {
	// ctx is the context
	ctx context.Context
	// name is the filename as passed to open
	name string
	// h is the filesystem handle
	h *FSHandle
	// idx is the current file index
	idx int64
	// flag is any file flags
	flag int
	// ts is the reference timestamp
	ts time.Time
}

// NewBillyFSFile constructs a new Billy FS file handle.
// The handle will be released when the file is closed.
// If ts is zero, uses time.Now.
func NewBillyFSFile(ctx context.Context, name string, h *FSHandle, flag int, ts time.Time) *BillyFSFile {
	return &BillyFSFile{ctx: ctx, name: name, h: h, flag: flag}
}

// GetReadOnly checks if the readonly flag is set.
func (f *BillyFSFile) GetReadOnly() bool {
	return isReadOnly(f.flag)
}

// Name returns the name of the file as presented to Open.
func (f *BillyFSFile) Name() string {
	return f.name
}

// Write attempts to write data to the file node.
func (f *BillyFSFile) Write(p []byte) (n int, err error) {
	if f.GetReadOnly() {
		return 0, billy.ErrReadOnly
	}

	err = f.h.Write(f.ctx, f.idx, p, f.timestamp())
	if err != nil {
		return 0, err
	}
	n = len(p)
	f.idx += int64(n)
	return n, nil
}

// Read attempts to read data from the file node.
func (f *BillyFSFile) Read(p []byte) (n int, err error) {
	idx := f.idx
	rn, err := f.h.Read(f.ctx, idx, p)
	f.idx += rn
	return int(rn), err
}

// ReadAt attempts to read data at a location in the file.
func (f *BillyFSFile) ReadAt(p []byte, off int64) (n int, err error) {
	rn, err := f.h.Read(f.ctx, off, p)
	return int(rn), err
}

// Seek attempts to move the file handle to a location in a file.
func (f *BillyFSFile) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekCurrent:
		f.idx += offset
	case io.SeekStart:
		f.idx = offset
	case io.SeekEnd:
		size, err := f.h.GetSize(f.ctx)
		if err != nil {
			return 0, err
		}
		f.idx = int64(size) - offset
	}
	if f.idx < 0 {
		f.idx = 0
		return 0, io.EOF
	}
	return f.idx, nil
}

// Truncate the file.
func (f *BillyFSFile) Truncate(size int64) error {
	if f.GetReadOnly() {
		return billy.ErrReadOnly
	}
	if size < 0 {
		size = 0
	}
	return f.h.Truncate(f.ctx, uint64(size), f.timestamp())
}

// Close closes the file handle.
func (f *BillyFSFile) Close() error {
	f.h.Release()
	return nil
}

// Lock locks the file like e.g. flock. It protects against access from
// other processes.
func (f *BillyFSFile) Lock() error {
	// XXX: we do not yet implement flock.
	return nil
}

// Unlock unlocks the file.
func (f *BillyFSFile) Unlock() error {
	// XXX: we do not yet implement flock.
	return nil
}

// timestamp returns the current timestamp.
func (f *BillyFSFile) timestamp() time.Time {
	if f.ts.IsZero() {
		return time.Now()
	}
	return f.ts
}

// _ is a type assertion
var _ billy.File = ((*BillyFSFile)(nil))
