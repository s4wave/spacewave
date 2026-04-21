package stream

import "time"

// OpenOpts are optional arguments when opening a stream.
type OpenOpts struct{}

// Stream is a stream-based data channel between two peers over a link.
type Stream interface {
	// Read data from the stream.
	Read(b []byte) (n int, err error)
	// Write data to the stream.
	Write(b []byte) (n int, err error)
	// SetReadDeadline sets the read deadline as defined by
	// A zero time value disables the deadline.
	SetReadDeadline(t time.Time) error
	// SetWriteDeadline sets the write deadline as defined by
	// A zero time value disables the deadline.
	SetWriteDeadline(t time.Time) error
	// SetDeadline sets both read and write deadlines as defined by
	// A zero time value disables the deadlines.
	SetDeadline(t time.Time) error
	// Close closes the stream.
	Close() error
}
