package unixfs

import (
	"context"
	"io"
	"time"
)

// FSHandleReadWriter wraps a FSHandle to implement io reader and writer.
type FSHandleReadWriter struct {
	ctx context.Context
	h   *FSHandle
	ts  func() time.Time

	idx  int64
	size uint64
}

// NewFSHandleReadWriter constructs a new ReadWriter from a FSHandle.
// if ts is nil, uses now()
func NewFSHandleReadWriter(ctx context.Context, h *FSHandle, ts func() time.Time) *FSHandleReadWriter {
	if ts == nil {
		ts = time.Now
	}
	return &FSHandleReadWriter{ctx: ctx, h: h, ts: ts}
}

// Read reads data from the file at the index.
func (w *FSHandleReadWriter) Read(p []byte) (n int, err error) {
	nr, err := w.h.ReadAt(w.ctx, w.idx, p)
	if nr > 0 {
		if err == io.EOF {
			err = nil
		} else if err == nil {
			w.idx += nr
		}
	}
	return int(nr), err
}

// Write writes data to the file at the index.
func (w *FSHandleReadWriter) Write(p []byte) (n int, err error) {
	wts := w.ts()
	err = w.h.WriteAt(w.ctx, w.idx, p, wts)
	if err != nil {
		return 0, err
	}
	w.idx += int64(len(p))
	return len(p), nil
}

// Seek moves the read/writer to a location in the file.
func (w *FSHandleReadWriter) Seek(offset int64, whence int) (int64, error) {
	if whence == io.SeekCurrent {
		w.idx += offset
		return w.idx, nil
	}
	if whence == io.SeekStart {
		w.idx = offset
		return w.idx, nil
	}
	if whence == io.SeekEnd {
		size, err := w.getSize()
		if err != nil {
			return 0, err
		}
		w.idx = int64(size) + offset
	}
	return w.idx, nil
}

// getSize determines the size from the cached data or by calling GetSize.
func (w *FSHandleReadWriter) getSize() (uint64, error) {
	if w.size != 0 {
		return w.size, nil
	}
	var err error
	w.size, err = w.h.GetSize(w.ctx)
	return w.size, err
}

// _ is a type assertion
var _ io.ReadWriteSeeker = ((*FSHandleReadWriter)(nil))
