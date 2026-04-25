//go:build js && wasm

package opfs

import (
	"io"
	"io/fs"

	"github.com/hack-pad/safejs"
)

// asyncFileOps implements FileOps using the async OPFS API.
// Works on all threads (main thread, workers, SharedWorkers).
// Uses getFile()+arrayBuffer() for reads and createWritable() for writes.
type asyncFileOps struct {
	fh     *FileHandle
	name   string
	buf    []byte // write buffer, flushed on Flush/Close
	cursor int64
	dirty  bool // true if buf has unflushed writes
}

func newAsyncFileOps(fh *FileHandle) (*asyncFileOps, error) {
	a := &asyncFileOps{fh: fh, name: fh.name}
	// Load initial contents so reads work before any writes.
	data, err := fh.ReadFile()
	if err != nil {
		return nil, err
	}
	a.buf = data
	return a, nil
}

// ReadAt reads bytes from the file at the given offset.
func (a *asyncFileOps) ReadAt(buf []byte, off int64) (int, error) {
	if int(off) >= len(a.buf) {
		return 0, io.EOF
	}
	n := copy(buf, a.buf[off:])
	if n < len(buf) {
		return n, io.EOF
	}
	return n, nil
}

// Read reads bytes from the file at the current cursor position.
func (a *asyncFileOps) Read(buf []byte) (int, error) {
	n, err := a.ReadAt(buf, a.cursor)
	a.cursor += int64(n)
	return n, err
}

// WriteAt writes bytes to the file at the given offset.
// Buffers in memory until Flush or Close.
func (a *asyncFileOps) WriteAt(buf []byte, off int64) (int, error) {
	end := int(off) + len(buf)
	if end > len(a.buf) {
		grown := make([]byte, end)
		copy(grown, a.buf)
		a.buf = grown
	}
	n := copy(a.buf[off:], buf)
	a.dirty = true
	return n, nil
}

// Write writes bytes at the current cursor position.
func (a *asyncFileOps) Write(buf []byte) (int, error) {
	n, err := a.WriteAt(buf, a.cursor)
	a.cursor += int64(n)
	return n, err
}

// Seek sets the cursor position.
func (a *asyncFileOps) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		a.cursor = offset
	case io.SeekCurrent:
		a.cursor += offset
	case io.SeekEnd:
		a.cursor = int64(len(a.buf)) + offset
	}
	if a.cursor < 0 {
		a.cursor = 0
	}
	return a.cursor, nil
}

// Stat returns file info.
func (a *asyncFileOps) Stat() (fs.FileInfo, error) {
	return &fileInfo{name: a.name, size: int64(len(a.buf))}, nil
}

// Truncate resizes the file buffer.
func (a *asyncFileOps) Truncate(size int64) error {
	sz := int(size)
	if sz < len(a.buf) {
		a.buf = a.buf[:sz]
	} else if sz > len(a.buf) {
		grown := make([]byte, sz)
		copy(grown, a.buf)
		a.buf = grown
	}
	a.dirty = true
	return nil
}

// Flush writes the buffer to disk via createWritable().
func (a *asyncFileOps) Flush() error {
	if !a.dirty {
		return nil
	}
	err := a.writeViaWritable(a.buf)
	if err == nil {
		a.dirty = false
	}
	return err
}

// Close flushes and releases resources.
func (a *asyncFileOps) Close() error {
	err := a.Flush()
	a.buf = nil
	a.fh = nil
	return err
}

// writeViaWritable writes data using FileSystemWritableFileStream.
func (a *asyncFileOps) writeViaWritable(data []byte) error {
	promise, err := a.fh.jsHandle.Call("createWritable")
	if err != nil {
		return err
	}
	writable, err := awaitPromise(promise)
	if err != nil {
		return err
	}

	// Truncate to 0 first to clear old content.
	truncPromise, err := writable.Call("truncate", 0)
	if err != nil {
		closeWritable(writable)
		return err
	}
	_, err = awaitPromise(truncPromise)
	if err != nil {
		closeWritable(writable)
		return err
	}

	if len(data) > 0 {
		jsBuf, err := newUint8Array(len(data))
		if err != nil {
			closeWritable(writable)
			return err
		}
		_, err = safejs.CopyBytesToJS(jsBuf, data)
		if err != nil {
			closeWritable(writable)
			return err
		}
		writePromise, err := writable.Call("write", jsBuf)
		if err != nil {
			closeWritable(writable)
			return err
		}
		_, err = awaitPromise(writePromise)
		if err != nil {
			closeWritable(writable)
			return err
		}
	}

	closePromise, err := writable.Call("close")
	if err != nil {
		return err
	}
	_, err = awaitPromise(closePromise)
	return err
}

// closeWritable attempts to close a writable stream, ignoring errors.
func closeWritable(writable safejs.Value) {
	p, err := writable.Call("close")
	if err == nil {
		_, _ = awaitPromise(p)
	}
}

// _ is a type assertion.
var _ FileOps = (*asyncFileOps)(nil)
