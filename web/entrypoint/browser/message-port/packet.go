//go:build js
// +build js

package message_port

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
)

// MessagePortPacketStream wraps a MessagePort into a PacketWriter and PacketStream.
type MessagePortPacketStream struct {
	port *MessagePort
}

// NewMessagePortPacketStream builds a new MessagePortPacketStream.
func NewMessagePortPacketStream(port *MessagePort) *MessagePortPacketStream {
	return &MessagePortPacketStream{port: port}
}

// WritePacket writes a packet to the remote.
func (w *MessagePortPacketStream) WritePacket(p *srpc.Packet) error {
	data, err := p.MarshalVT()
	if err != nil {
		return err
	}

	w.port.WriteMessage(data)
	return nil
}

// ReadPump is a goroutine that reads packets to a packet handler.
func (w *MessagePortPacketStream) ReadPump(ctx context.Context, cb srpc.PacketDataHandler, closed srpc.CloseHandler) {
	err := w.ReadToHandler(ctx, cb)
	// signal that the stream is now closed.
	if closed != nil {
		closed(err)
	}
}

// ReadToHandler reads data to the given handler.
// Does not handle closing the stream, use ReadPump instead.
func (w *MessagePortPacketStream) ReadToHandler(ctx context.Context, cb srpc.PacketDataHandler) error {
	for {
		data, err := w.port.ReadMessage(ctx)
		if err != nil {
			return err
		}

		if err := cb(data); err != nil {
			return err
		}
	}
}

// Close closes the writer.
func (w *MessagePortPacketStream) Close() error {
	return w.port.Close()
}

var _ srpc.PacketWriter = ((*MessagePortPacketStream)(nil))
