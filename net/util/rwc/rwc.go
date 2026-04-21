package rwc

import (
	"io"
)

// ReadWriteCloser implements ReadWriteCloser with a Reader and Writer.
type ReadWriteCloser struct {
	reader io.ReadCloser
	writer io.WriteCloser
}

// NewReadWriteCloser builds a new ReadWriteCloser.
func NewReadWriteCloser(
	reader io.ReadCloser,
	writer io.WriteCloser,
) io.ReadWriteCloser {
	return &ReadWriteCloser{reader: reader, writer: writer}
}

// Read implements io.Reader
func (r *ReadWriteCloser) Read(p []byte) (n int, err error) {
	return r.reader.Read(p)
}

// Write implements io.Writer
func (r *ReadWriteCloser) Write(p []byte) (n int, err error) {
	return r.writer.Write(p)
}

// Close closes both streams.
func (r *ReadWriteCloser) Close() error {
	er1 := r.reader.Close()
	er2 := r.writer.Close()
	if er1 != nil {
		return er1
	}
	if er2 != nil {
		return er2
	}

	return nil
}
