package stream

import (
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"github.com/pion/transport/v5/deadline"
)

// messagePipeInboxSize bounds the messages a pipe end holds for Read. A full
// inbox drops new messages, as a message stream on a link may.
const messagePipeInboxSize = 64

// ErrMessageTooLarge is returned by a message pipe Write longer than the
// pipe's maximum message size.
var ErrMessageTooLarge = errors.New("message exceeds the maximum message size")

// NewMessagePipe returns the two connected ends of an in-memory
// MessageStream. Messages longer than maxSize fail to Write, as messages
// larger than one packet fail on a link. The control streams are a net.Pipe.
func NewMessagePipe(maxSize int) (MessageStream, MessageStream) {
	left, right := net.Pipe()
	a, b := newMessagePipe(left, maxSize), newMessagePipe(right, maxSize)
	a.peer, b.peer = b, a
	return a, b
}

// messagePipe is one end of a message pipe.
type messagePipe struct {
	control net.Conn
	maxSize int
	peer    *messagePipe

	inbox         chan []byte
	closed        chan struct{}
	closeOnce     sync.Once
	readDeadline  *deadline.Deadline
	writeDeadline *deadline.Deadline
}

// newMessagePipe constructs one end of a message pipe.
func newMessagePipe(control net.Conn, maxSize int) *messagePipe {
	return &messagePipe{
		control:       control,
		maxSize:       maxSize,
		inbox:         make(chan []byte, messagePipeInboxSize),
		closed:        make(chan struct{}),
		readDeadline:  deadline.New(),
		writeDeadline: deadline.New(),
	}
}

// Control returns the reliable stream of this message stream.
func (p *messagePipe) Control() Stream {
	return p.control
}

// Read returns the next received message.
func (p *messagePipe) Read(b []byte) (int, error) {
	select {
	case msg := <-p.inbox:
		if len(msg) > len(b) {
			return 0, io.ErrShortBuffer
		}
		return copy(b, msg), nil
	case <-p.closed:
		return 0, io.EOF
	case <-p.peer.closed:
		return 0, io.EOF
	case <-p.readDeadline.Done():
		return 0, p.readDeadline.Err()
	}
}

// Write sends b as one message. A message the peer has no room for is dropped.
func (p *messagePipe) Write(b []byte) (int, error) {
	select {
	case <-p.closed:
		return 0, io.ErrClosedPipe
	case <-p.peer.closed:
		return 0, io.ErrClosedPipe
	case <-p.writeDeadline.Done():
		return 0, p.writeDeadline.Err()
	default:
	}
	if len(b) > p.maxSize {
		return 0, ErrMessageTooLarge
	}
	select {
	case p.peer.inbox <- append([]byte(nil), b...):
	default:
	}
	return len(b), nil
}

// SetReadDeadline sets the deadline of pending and future Reads.
func (p *messagePipe) SetReadDeadline(t time.Time) error {
	p.readDeadline.Set(t)
	return nil
}

// SetWriteDeadline sets the deadline of future Writes. A Write never blocks.
func (p *messagePipe) SetWriteDeadline(t time.Time) error {
	p.writeDeadline.Set(t)
	return nil
}

// SetDeadline sets both deadlines.
func (p *messagePipe) SetDeadline(t time.Time) error {
	p.readDeadline.Set(t)
	p.writeDeadline.Set(t)
	return nil
}

// Close stops this end's message plane and closes its control stream.
func (p *messagePipe) Close() error {
	p.closeOnce.Do(func() {
		close(p.closed)
		_ = p.control.Close()
	})
	return nil
}

// _ is a type assertion
var _ MessageStream = (*messagePipe)(nil)
