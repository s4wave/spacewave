package framedstream

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"math"
	"sync"

	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
)

// MaxMessageSize is the max message size in bytes.
var MaxMessageSize uint32 = 10 * 1024 * 1024 // 10MB

// Stream wraps an io.ReadWriteCloser to implement rpcstream.RpcStream.
// Uses LittleEndian uint32 length-prefix framing for messages.
type Stream struct {
	ctx context.Context
	rwc io.ReadWriteCloser

	readMtx sync.Mutex
	readBuf bytes.Buffer

	writeMtx sync.Mutex
}

// New creates a new framed Stream.
func New(ctx context.Context, rwc io.ReadWriteCloser) *Stream {
	return &Stream{
		ctx: ctx,
		rwc: rwc,
	}
}

// Context returns the stream context.
func (s *Stream) Context() context.Context {
	return s.ctx
}

// MsgSend sends a protobuf message to the remote.
func (s *Stream) MsgSend(msg srpc.Message) error {
	data, err := msg.MarshalVT()
	if err != nil {
		return err
	}
	return s.writeFramedData(data)
}

// MsgRecv receives a protobuf message from the remote.
func (s *Stream) MsgRecv(msg srpc.Message) error {
	data, err := s.readFramedData()
	if err != nil {
		return err
	}
	return msg.UnmarshalVT(data)
}

// CloseSend signals to the remote that we will no longer send any messages.
func (s *Stream) CloseSend() error {
	return nil
}

// Close closes the stream.
func (s *Stream) Close() error {
	return s.rwc.Close()
}

// Send sends an RpcStreamPacket.
func (s *Stream) Send(pkt *rpcstream.RpcStreamPacket) error {
	return s.MsgSend(pkt)
}

// Recv receives an RpcStreamPacket.
func (s *Stream) Recv() (*rpcstream.RpcStreamPacket, error) {
	pkt := &rpcstream.RpcStreamPacket{}
	if err := s.MsgRecv(pkt); err != nil {
		return nil, err
	}
	return pkt, nil
}

// SendRaw writes raw bytes with length-prefix framing.
func (s *Stream) SendRaw(data []byte) error {
	return s.writeFramedData(data)
}

// RecvRaw reads a length-prefixed frame and returns the raw bytes.
func (s *Stream) RecvRaw() ([]byte, error) {
	return s.readFramedData()
}

// writeFramedData writes data with a LittleEndian uint32 length prefix.
func (s *Stream) writeFramedData(data []byte) error {
	s.writeMtx.Lock()
	defer s.writeMtx.Unlock()

	if len(data) > math.MaxUint32 {
		return errors.New("data size exceeds maximum uint32 value")
	}

	lenBuf := make([]byte, 4)
	binary.LittleEndian.PutUint32(lenBuf, uint32(len(data))) //nolint:gosec
	if _, err := s.rwc.Write(lenBuf); err != nil {
		return err
	}

	if _, err := s.rwc.Write(data); err != nil {
		return err
	}

	return nil
}

// readFramedData reads a length-prefixed frame.
func (s *Stream) readFramedData() ([]byte, error) {
	s.readMtx.Lock()
	defer s.readMtx.Unlock()

	if err := s.readUntil(4); err != nil {
		return nil, err
	}

	lenBuf := s.readBuf.Next(4)
	msgLen := binary.LittleEndian.Uint32(lenBuf)
	if msgLen > MaxMessageSize {
		return nil, io.ErrShortBuffer
	}

	if err := s.readUntil(int(msgLen)); err != nil {
		return nil, err
	}

	return s.readBuf.Next(int(msgLen)), nil
}

// readUntil reads from the underlying connection until the buffer has at least n bytes.
func (s *Stream) readUntil(n int) error {
	buf := make([]byte, 4096)
	for s.readBuf.Len() < n {
		nr, err := s.rwc.Read(buf)
		if nr > 0 {
			s.readBuf.Write(buf[:nr])
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// _ is a type assertion
var _ rpcstream.RpcStream = (*Stream)(nil)
