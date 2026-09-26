package transport_quic

import (
	"io"
	"math"
	"sync"
	"time"

	"github.com/pion/transport/v5/deadline"
	"github.com/pkg/errors"
	quic "github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/quicvarint"
	"github.com/s4wave/spacewave/net/stream"
)

// messageInboxSize bounds the received messages a message stream holds for
// Read. A full inbox drops new messages, as the network may.
const messageInboxSize = 64

// ErrDatagramsUnsupported is returned when opening a message stream on a link
// whose peers did not both enable QUIC datagrams.
var ErrDatagramsUnsupported = errors.New("link does not support unreliable streams: quic datagrams are disabled")

// messageStream is a MessageStream on a QUIC link. Its control stream is a
// bidirectional QUIC stream; each message is one QUIC datagram carrying the
// control stream's ID as a varint prefix, as in RFC 9297.
type messageStream struct {
	// link carries the datagrams and registers this stream.
	link *Link
	// control is the reliable stream paired with the message plane.
	control *quic.Stream
	// prefix encodes the control stream ID at the start of each datagram.
	prefix []byte

	// inbox holds received messages until Read consumes them.
	inbox chan []byte
	// closed wakes readers and rejects writes after Close.
	closed chan struct{}
	// closeOnce serializes closing the message plane and control stream.
	closeOnce sync.Once
	// readDeadline bounds waiting for the next message.
	readDeadline *deadline.Deadline
	// writeDeadline rejects writes after the configured time.
	writeDeadline *deadline.Deadline
}

// newMessageStream binds the message plane of control and registers it to
// receive the link's datagrams.
func (l *Link) newMessageStream(control *quic.Stream) (*messageStream, error) {
	state := l.sess.ConnectionState()
	if !state.SupportsDatagrams.Local || !state.SupportsDatagrams.Remote {
		return nil, ErrDatagramsUnsupported
	}
	s := &messageStream{
		link:          l,
		control:       control,
		prefix:        quicvarint.Append(nil, uint64(control.StreamID())), //nolint:gosec // stream IDs are varints below 2^62
		inbox:         make(chan []byte, messageInboxSize),
		closed:        make(chan struct{}),
		readDeadline:  deadline.New(),
		writeDeadline: deadline.New(),
	}
	l.messagesMtx.Lock()
	l.messages[string(s.prefix)] = s
	l.messagesMtx.Unlock()
	l.receiveOnce.Do(func() { go l.receiveDatagrams() })
	return s, nil
}

// receiveDatagrams delivers each datagram to the message stream it names
// until the link closes. Datagrams for unknown streams are dropped: a message
// may arrive before its stream is accepted or after it closes.
func (l *Link) receiveDatagrams() {
	for {
		datagram, err := l.sess.ReceiveDatagram(l.ctx)
		if err != nil {
			return
		}
		_, n, err := quicvarint.Parse(datagram)
		if err != nil {
			continue
		}
		l.messagesMtx.Lock()
		s := l.messages[string(datagram[:n])]
		l.messagesMtx.Unlock()
		if s == nil {
			continue
		}
		select {
		case s.inbox <- datagram[n:]:
		default:
		}
	}
}

// AcceptMessageStream binds the message plane of an accepted stream whose
// opener set OpenOpts.Unreliable.
func (l *Link) AcceptMessageStream(control stream.Stream) (stream.MessageStream, error) {
	qstream, ok := control.(reliableStream)
	if !ok {
		return nil, errors.Errorf("message stream control must be a quic stream, got %T", control)
	}
	return l.newMessageStream(qstream.Stream)
}

// Control returns the reliable stream of this message stream.
func (s *messageStream) Control() stream.Stream {
	return reliableStream{s.control}
}

// Read returns the next received message.
func (s *messageStream) Read(b []byte) (int, error) {
	select {
	case msg := <-s.inbox:
		if len(msg) > len(b) {
			return 0, io.ErrShortBuffer
		}
		return copy(b, msg), nil
	case <-s.closed:
		return 0, io.EOF
	case <-s.link.ctx.Done():
		return 0, io.EOF
	case <-s.readDeadline.Done():
		return 0, s.readDeadline.Err()
	}
}

// Write sends b as one message.
func (s *messageStream) Write(b []byte) (int, error) {
	// Reject writes after closure or expiry before assembling a datagram.
	select {
	case <-s.closed:
		return 0, io.ErrClosedPipe
	case <-s.writeDeadline.Done():
		return 0, s.writeDeadline.Err()
	default:
	}

	// Check the combined length before allocating the prefixed datagram.
	if len(b) > math.MaxInt-len(s.prefix) {
		return 0, &quic.DatagramTooLargeError{MaxDatagramPayloadSize: int64(math.MaxInt)}
	}
	datagram := make([]byte, 0, len(s.prefix)+len(b))
	datagram = append(append(datagram, s.prefix...), b...)
	if err := s.link.sess.SendDatagram(datagram); err != nil {
		return 0, err
	}
	return len(b), nil
}

// SetReadDeadline sets the deadline of pending and future Reads.
func (s *messageStream) SetReadDeadline(t time.Time) error {
	s.readDeadline.Set(t)
	return nil
}

// SetWriteDeadline sets the deadline of future Writes. A Write never blocks.
func (s *messageStream) SetWriteDeadline(t time.Time) error {
	s.writeDeadline.Set(t)
	return nil
}

// SetDeadline sets both deadlines.
func (s *messageStream) SetDeadline(t time.Time) error {
	s.readDeadline.Set(t)
	s.writeDeadline.Set(t)
	return nil
}

// Close stops the message plane and closes the control stream.
func (s *messageStream) Close() error {
	s.closeOnce.Do(func() {
		close(s.closed)
		s.link.messagesMtx.Lock()
		delete(s.link.messages, string(s.prefix))
		s.link.messagesMtx.Unlock()
		s.control.CancelRead(0)
		_ = s.control.Close()
	})
	return nil
}

// _ is a type assertion
var _ stream.MessageStream = (*messageStream)(nil)
