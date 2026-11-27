package dex_solicit

import (
	"io"
	"sync/atomic"
)

// countedStream counts each read's bytes in the session's received count,
// which the session's delivery rate and the progress of each read pending on
// it measure.
type countedStream struct {
	io.ReadWriteCloser
	// received is the session's count.
	received *atomic.Uint64
}

// Read reads from the stream and counts the bytes read.
func (s countedStream) Read(p []byte) (int, error) {
	n, err := s.ReadWriteCloser.Read(p)
	s.received.Add(uint64(n)) //nolint:gosec // Read never returns a negative count.
	return n, err
}
