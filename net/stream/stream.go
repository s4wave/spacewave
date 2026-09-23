package stream

import "time"

// OpenOpts are optional arguments when opening a stream.
type OpenOpts struct {
	// Unreliable opens a MessageStream: its messages may be lost or arrive
	// out of order, so a lost message never delays the ones after it.
	Unreliable bool
}

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

// MessageStream is a stream opened with OpenOpts.Unreliable.
//
// Each Write sends one message and each Read returns one whole message.
// Messages are delivered at most once, possibly out of order, and a lost
// message is never retransmitted. Write fails when a message exceeds the size
// the link can send in one packet; Read fails with io.ErrShortBuffer when b is
// too small for the next message, which is then dropped.
//
// Control is the reliable stream the message stream was negotiated on. It
// stays open for the stream's lifetime and carries whatever the protocol
// needs delivered reliably. Close closes both.
type MessageStream interface {
	Stream

	// Control returns the reliable stream of this message stream.
	Control() Stream
}
