package unixfs

import (
	"context"
	"io"
	"sync/atomic"
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
	// flag contains file open flags
	flag int
	// t is a constant write timestamp
	t atomic.Pointer[time.Time]
	// idx is the current file index
	// note: concurrent read() and write() calls have undefined behavior.
	// while not expected, the atomic integer will protect against concurrent access.
	// note: concurrent ReadAt calls will work correctly (even during a Write()).
	idx atomic.Int64
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

// Write writes data to the file node.
func (f *BillyFSFile) Write(p []byte) (n int, err error) {
	if f.GetReadOnly() {
		return 0, billy.ErrReadOnly
	}

	startIdx := f.idx.Load()
	err = f.h.Write(f.ctx, startIdx, p, f.timestamp())
	if err != nil {
		return 0, err
	}
	n = len(p)
	if n != 0 {
		f.idx.Add(int64(n))
	}

	return n, nil
}

// WriteAt writes data to the file node at an offset.
func (f *BillyFSFile) WriteAt(p []byte, off int64) (n int, err error) {
	if f.GetReadOnly() {
		return 0, billy.ErrReadOnly
	}

	err = f.h.Write(f.ctx, off, p, f.timestamp())
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

// Read reads data from the file node, advancing the file handle offset.
func (f *BillyFSFile) Read(p []byte) (n int, err error) {
	idx := f.idx.Load()
	rn, err := f.h.Read(f.ctx, idx, p)
	if rn != 0 {
		f.idx.Add(rn)
	}
	return int(rn), err
}

// ReadAt attempts to read data at a location in the file.
func (f *BillyFSFile) ReadAt(p []byte, off int64) (n int, err error) {
	rn, err := f.h.Read(f.ctx, off, p)
	return int(rn), err
}

// Seek attempts to move the file handle to a location in a file.
func (f *BillyFSFile) Seek(offset int64, whence int) (int64, error) {
	var out int64
	switch whence {
	case io.SeekCurrent:
		out = f.idx.Add(offset)
	case io.SeekStart:
		f.idx.Store(offset)
		out = offset
	case io.SeekEnd:
		size, err := f.h.GetSize(f.ctx)
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

// SetOpTimestamp sets the timestamp for FS write operations.
func (f *BillyFSFile) SetOpTimestamp(t time.Time) {
	f.t.Store(&t)
}

// GetOpTimestamp returns the current timestamp set to use for writes.
func (f *BillyFSFile) GetOpTimestamp() time.Time {
	ts := f.t.Load()
	if ts == nil {
		return time.Time{}
	}
	return *ts
}

// timestamp returns the timestamp to use for writes..
func (f *BillyFSFile) timestamp() time.Time {
	t := f.GetOpTimestamp()
	if t.IsZero() {
		return time.Now()
	}
	return t
}

// _ is a type assertion
var (
	_ billy.File  = ((*BillyFSFile)(nil))
	_ io.WriterAt = ((*BillyFSFile)(nil))
)
