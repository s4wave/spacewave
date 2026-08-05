package stream_packet

import (
	"encoding/binary"
	"io"
	"math"
	"sync"

	protobuf_go_lite "github.com/aperturerobotics/protobuf-go-lite"
	"github.com/pkg/errors"
)

// Session wraps a stream in a session.
type Session struct {
	io.ReadWriteCloser
	sendMtx        sync.Mutex
	readMtx        sync.Mutex
	maxMessageSize uint32
}

// NewSession builds a new session.
func NewSession(
	stream io.ReadWriteCloser,
	maxMessageSize uint32,
) *Session {
	return &Session{
		ReadWriteCloser: stream,
		maxMessageSize:  maxMessageSize,
	}
}

// SendMsg tries to send a message on the wire.
func (s *Session) SendMsg(msg protobuf_go_lite.Message) error {
	// Measure and bound the serialized message.
	size := msg.SizeVT()
	if size > math.MaxInt32 {
		return errors.New("message too large: exceeds maximum uint32 value")
	}

	// Allocate the length-prefixed packet and marshal the payload.
	pktBuf := make([]byte, size+4)
	binary.LittleEndian.PutUint32(pktBuf[:4], uint32(size))
	if _, err := msg.MarshalToSizedBufferVT(pktBuf[4:]); err != nil {
		return err
	}

	// Serialize writes under the session send lock.
	s.sendMtx.Lock()
	defer s.sendMtx.Unlock()

	// Write the complete packet to the underlying stream.
	if _, err := s.Write(pktBuf); err != nil {
		return err
	}
	return nil
}

// RecvMsg tries to receive a message on the wire.
func (s *Session) RecvMsg(msg protobuf_go_lite.Message) error {
	// Serialize reads under the session receive lock.
	var hdr [4]byte
	s.readMtx.Lock()
	defer s.readMtx.Unlock()

	// Read the fixed-size message length prefix.
	if _, err := io.ReadFull(s.ReadWriteCloser, hdr[:]); err != nil {
		return err
	}

	// Decode and validate the payload length.
	messageLen := binary.LittleEndian.Uint32(hdr[:])
	if messageLen > 0 {
		if messageLen > s.maxMessageSize {
			return errors.Errorf("invalid message len: %d", messageLen)
		}

		// Read and decode the payload bytes.
		data := make([]byte, messageLen)
		if _, err := io.ReadFull(s.ReadWriteCloser, data); err != nil {
			return err
		}

		return msg.UnmarshalVT(data)
	}

	// Reset the destination for an empty message.
	msg.Reset()
	return nil
}
