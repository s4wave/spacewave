package closer

import (
	"io"
	"sync"
)

// WriteCloser wraps a writer to make a WriteCloser.
type WriteCloser struct {
	closeMtx sync.Mutex
	wr       io.Writer
	close    func() error
}

// NewWriteCloser builds a new write closer
func NewWriteCloser(wr io.Writer, close func() error) *WriteCloser {
	return &WriteCloser{
		wr:    wr,
		close: close,
	}
}

// Write writes data to the io.Writer.
func (w *WriteCloser) Write(p []byte) (n int, err error) {
	w.closeMtx.Lock()
	defer w.closeMtx.Unlock()
	if w.wr == nil {
		// Close() already called
		return 0, io.EOF
	}
	return w.wr.Write(p)
}

// Close closes the WriteCloser.
func (w *WriteCloser) Close() error {
	w.closeMtx.Lock()
	closeFn := w.close
	w.wr = nil
	w.close = nil
	w.closeMtx.Unlock()
	if closeFn != nil {
		return closeFn()
	}
	return nil
}

// _ is a type assertion
var _ io.WriteCloser = ((*WriteCloser)(nil))
