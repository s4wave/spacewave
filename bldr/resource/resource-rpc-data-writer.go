package resource

import (
	"errors"

	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
)

// resourceRpcPacketDataField is the field number of the data frame body in
// ResourceRpcPacket.
const resourceRpcPacketDataField = 3

// resourceRpcDataWriter writes SRPC packets as data frames of a negotiated
// Resource route.
//
// A packet is not marshaled on its own. WritePacketMessage nests it inside the
// ResourceRpcPacket data field and the stream encodes the whole frame in one
// pass, so a reply is copied once on its way to the transport.
type resourceRpcDataWriter struct {
	// stream carries the negotiated route.
	stream ResourceRpcStream
	// retain leaves closure to ExecCall on clients and the route reader on servers.
	retain bool
	// onSendError ends responses when a failed transport cannot carry nested
	// completion to the caller.
	onSendError func()
}

// WritePacket writes a decoded packet as a data frame.
func (w *resourceRpcDataWriter) WritePacket(p *srpc.Packet) error {
	return w.WritePacketMessage(p)
}

// WritePacketMessage writes a packet as a data frame, encoding it directly
// into the frame the stream sends.
func (w *resourceRpcDataWriter) WritePacketMessage(p srpc.SizedMessage) error {
	// A failed response transport cannot deliver nested completion to its
	// caller. An oversized packet leaves the transport intact, so the handler's
	// error completion still reaches the caller.
	err := w.stream.MsgSend(srpc.NewNestedMessage(resourceRpcPacketDataField, p))
	if err != nil && !errors.Is(err, srpc.ErrMessageTooLarge) && w.onSendError != nil {
		w.onSendError()
	}
	return err
}

// Close leaves unary route closure to ExecCall and forwards streaming closure.
func (w *resourceRpcDataWriter) Close() error {
	if w.retain {
		return nil
	}
	return w.stream.CloseSend()
}

// readResourceRpcData passes each data frame of a negotiated route to the
// handler until the stream ends, then reports the result to closed.
func readResourceRpcData(stream ResourceRpcStream, handler srpc.PacketDataHandler, closed srpc.CloseHandler) {
	err := func() error {
		for {
			// Read the next frame and accept only data after the handshake.
			packet, err := stream.Recv()
			if err != nil {
				return err
			}
			data, ok := packet.GetBody().(*ResourceRpcPacket_Data)
			if !ok {
				return rpcstream.ErrUnexpectedPacket
			}

			// Skip an empty frame and pass the rest to the SRPC codec.
			if len(data.Data) == 0 {
				continue
			}
			if err := handler(data.Data); err != nil {
				return err
			}
		}
	}()

	// Signal that the route is closed.
	if closed != nil {
		closed(err)
	}
}

// _ is a type assertion.
var _ srpc.PacketMessageWriter = (*resourceRpcDataWriter)(nil)
