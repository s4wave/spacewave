package resource

import (
	"github.com/aperturerobotics/starpc/rpcstream"
)

// resourceRpcDataStream adapts negotiated Resource data to the SRPC codec.
type resourceRpcDataStream struct {
	// ResourceRpcStream carries the negotiated route and data packets.
	ResourceRpcStream
	// retain leaves closure to ExecCall on clients and the route reader on servers.
	retain bool
	// onSendError ends responses when nested completion cannot reach the caller.
	onSendError func()
}

// CloseSend leaves unary route closure to ExecCall and forwards streaming closure.
func (s *resourceRpcDataStream) CloseSend() error {
	if s.retain {
		return nil
	}
	return s.ResourceRpcStream.CloseSend()
}

// Send forwards only data; the Resource route is already negotiated.
func (s *resourceRpcDataStream) Send(packet *rpcstream.RpcStreamPacket) error {
	// Require an SRPC data packet before forwarding it over the Resource route.
	data, ok := packet.GetBody().(*rpcstream.RpcStreamPacket_Data)
	if !ok {
		return rpcstream.ErrUnexpectedPacket
	}

	// A failed response transport cannot deliver nested completion to its caller.
	err := s.ResourceRpcStream.Send(&ResourceRpcPacket{Body: &ResourceRpcPacket_Data{Data: data.Data}})
	if err != nil && s.onSendError != nil {
		s.onSendError()
	}
	return err
}

// Recv rejects a repeated handshake after the route has been acknowledged.
func (s *resourceRpcDataStream) Recv() (*rpcstream.RpcStreamPacket, error) {
	// Read the next packet and accept only data frames.
	packet, err := s.ResourceRpcStream.Recv()
	if err != nil {
		return nil, err
	}
	data, ok := packet.GetBody().(*ResourceRpcPacket_Data)
	if !ok {
		return nil, rpcstream.ErrUnexpectedPacket
	}
	return &rpcstream.RpcStreamPacket{Body: &rpcstream.RpcStreamPacket_Data{Data: data.Data}}, nil
}

// _ is a type assertion.
var _ rpcstream.RpcStream = (*resourceRpcDataStream)(nil)
