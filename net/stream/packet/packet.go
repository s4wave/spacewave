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
	data, err := msg.MarshalVT()
	if err != nil {
		return err
	}

	pktBuf := make([]byte, len(data)+4)
	copy(pktBuf[4:], data)

	dataLen := len(data)
	if dataLen > math.MaxInt32 {
		return errors.New("message too large: exceeds maximum uint32 value")
	}

	msgLen := uint32(dataLen)
	binary.LittleEndian.PutUint32(pktBuf, msgLen)

	s.sendMtx.Lock()
	defer s.sendMtx.Unlock()

	if _, err := s.Write(pktBuf); err != nil {
		return err
	}
	return nil
}

// RecvMsg tries to receive a message on the wire.
func (s *Session) RecvMsg(msg protobuf_go_lite.Message) error {
	data := make([]byte, 4)
	s.readMtx.Lock()
	defer s.readMtx.Unlock()

	if _, err := io.ReadFull(s.ReadWriteCloser, data); err != nil {
		return err
	}

	messageLen := binary.LittleEndian.Uint32(data)
	if messageLen > 0 {
		if messageLen > s.maxMessageSize {
			return errors.Errorf("invalid message len: %d", messageLen)
		}

		data = make([]byte, messageLen)
		if _, err := io.ReadFull(s.ReadWriteCloser, data); err != nil {
			return err
		}

		return msg.UnmarshalVT(data)
	}

	msg.Reset()
	return nil
}
