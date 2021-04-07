package closer

import (
	"io"
	"sync"
)

// ReadCloser wraps a writer to make a ReadCloser.
type ReadCloser struct {
	closeMtx sync.Mutex
	rd       io.Reader
	close    func() error
}

// NewReadCloser builds a new write closer
func NewReadCloser(rd io.Reader, close func() error) *ReadCloser {
	return &ReadCloser{
		rd:    rd,
		close: close,
	}
}

// Read writes data to the io.Readr.
func (w *ReadCloser) Read(p []byte) (n int, err error) {
	w.closeMtx.Lock()
	defer w.closeMtx.Unlock()
	if w.rd == nil {
		// Close() already called
		return 0, io.EOF
	}
	return w.rd.Read(p)
}

// Close closes the ReadCloser.
func (w *ReadCloser) Close() error {
	w.closeMtx.Lock()
	closeFn := w.close
	w.rd = nil
	w.close = nil
	w.closeMtx.Unlock()
	if closeFn != nil {
		return closeFn()
	}
	return nil
}

// _ is a type assertion
var _ io.ReadCloser = ((*ReadCloser)(nil))
