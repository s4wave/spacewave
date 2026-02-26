package unixfs_afero

import (
	"context"
	"errors"
	"io"
	"os"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/aperturerobotics/hydra/unixfs"
	"github.com/spf13/afero"
)

// AferoFSFile implements the Afero filesystem File interface with a FSHandle.
// Note: this interface is nearly identical in behavior to os.File interfaces.
type AferoFSFile struct {
	// ctx is the context
	ctx context.Context
	// name is the filename as passed to open
	name string
	// h is the filesystem handle
	h *unixfs.FSHandle
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

// NewAferoFSFile constructs a new Afero FS file handle.
// The handle may be a file or a directory.
// The handle will be released when the file is closed.
// If ts is zero, uses time.Now.
func NewAferoFSFile(ctx context.Context, name string, h *unixfs.FSHandle, flag int, ts time.Time) *AferoFSFile {
	file := &AferoFSFile{ctx: ctx, name: name, h: h, flag: flag}
	if !ts.IsZero() {
		file.SetOpTimestamp(ts)
	}
	return file
}

// GetReadOnly checks if the readonly flag is set.
func (f *AferoFSFile) GetReadOnly() bool {
	return unixfs.FlagIsReadOnly(f.flag)
}

// Name returns the name of the file as presented to Open.
func (f *AferoFSFile) Name() string {
	return f.name
}

// Readdir reads the contents of the directory associated with file and
// returns a slice of up to n FileInfo values, as would be returned
// by Lstat, in directory order. Subsequent calls on the same file will yield
// further FileInfos.
//
// If n > 0, Readdir returns at most n FileInfo structures. In this case, if
// Readdir returns an empty slice, it will return a non-nil error
// explaining why. At the end of a directory, the error is io.EOF.
//
// If n <= 0, Readdir returns all the FileInfo from the directory in
// a single slice. In this case, if Readdir succeeds (reads all
// the way to the end of the directory), it returns the slice and a
// nil error. If it encounters an error before the end of the
// directory, Readdir returns the FileInfo read until that point
// and a non-nil error.
func (f *AferoFSFile) Readdir(count int) ([]os.FileInfo, error) {
	// note: f.idx is used as the current offset
	nodeType, err := f.h.GetNodeType(f.ctx)
	if err != nil {
		return nil, err
	}
	if !nodeType.GetIsDirectory() {
		return nil, &os.PathError{Op: "readdir", Path: f.name, Err: errors.New("not a dir")}
	}

	var idx uint64
	idxBefore := f.idx.Load()
	if idxBefore > 0 {
		idx = uint64(idxBefore)
	}
	if count < 0 {
		count = 0
	}

	fi, err := unixfs.ReaddirAllToFileInfo(f.ctx, idx, uint64(count), f.h) //nolint:gosec
	if err == nil {
		f.idx.Add(int64(len(fi)))
	}

	return fi, err
}

// Readdirnames reads filenames only.
func (f *AferoFSFile) Readdirnames(limit int) ([]string, error) {
	fi, err := f.Readdir(limit)
	names := make([]string, len(fi))
	for i, f := range fi {
		names[i] = f.Name()
	}
	return names, err
}

// Write writes data to the file node.
func (f *AferoFSFile) Write(p []byte) (n int, err error) {
	if f.GetReadOnly() {
		return 0, syscall.EPERM
	}

	startIdx := f.idx.Load()
	err = f.h.WriteAt(f.ctx, startIdx, p, f.timestamp())
	if err != nil {
		return 0, err
	}
	n = len(p)
	if n != 0 {
		f.idx.Add(int64(n))
	}

	return n, nil
}

// WriteString writes a string to the file.
func (f *AferoFSFile) WriteString(s string) (ret int, err error) {
	return f.Write([]byte(s))
}

// WriteAt writes data to the file node at an offset.
func (f *AferoFSFile) WriteAt(p []byte, off int64) (n int, err error) {
	if f.GetReadOnly() {
		return 0, syscall.EPERM
	}

	err = f.h.WriteAt(f.ctx, off, p, f.timestamp())
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

// Read reads data from the file node, advancing the file handle offset.
func (f *AferoFSFile) Read(p []byte) (n int, err error) {
	idx := f.idx.Load()
	rn, err := f.h.ReadAt(f.ctx, idx, p)
	if rn != 0 {
		if err == io.EOF {
			err = nil
		}
		if err == nil {
			f.idx.Add(rn)
		}
	}
	return int(rn), err
}

// ReadAt attempts to read data at a location in the file.
func (f *AferoFSFile) ReadAt(p []byte, off int64) (n int, err error) {
	rn, err := f.h.ReadAt(f.ctx, off, p)
	return int(rn), err
}

// Seek attempts to move the file handle to a location in a file.
func (f *AferoFSFile) Seek(offset int64, whence int) (int64, error) {
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
		out = int64(size) + offset //nolint:gosec
		f.idx.Store(out)
	}
	if out < 0 {
		return out, io.EOF
	}
	return out, nil
}

// Stat returns FileInfo for the handle.
func (f *AferoFSFile) Stat() (os.FileInfo, error) {
	return f.h.GetFileInfo(f.ctx)
}

// Sync waits for any writes to be flushed to storage.
func (f *AferoFSFile) Sync() error {
	// NOTE: the FSHandle is not write-buffered (yet).
	return nil
}

// Truncate the file.
func (f *AferoFSFile) Truncate(size int64) error {
	if f.GetReadOnly() {
		return syscall.EPERM
	}
	if size < 0 {
		size = 0
	}
	return f.h.Truncate(f.ctx, uint64(size), f.timestamp()) //nolint:gosec
}

// Close closes the file handle.
func (f *AferoFSFile) Close() error {
	f.h.Release()
	return nil
}

// SetOpTimestamp sets the timestamp for FS write operations.
func (f *AferoFSFile) SetOpTimestamp(t time.Time) {
	f.t.Store(&t)
}

// GetOpTimestamp returns the current timestamp set to use for writes.
func (f *AferoFSFile) GetOpTimestamp() time.Time {
	ts := f.t.Load()
	if ts == nil {
		return time.Time{}
	}
	return *ts
}

// timestamp returns the timestamp to use for writes..
func (f *AferoFSFile) timestamp() time.Time {
	t := f.GetOpTimestamp()
	if t.IsZero() {
		return time.Now()
	}
	return t
}

// _ is a type assertion
var _ afero.File = ((*AferoFSFile)(nil))
