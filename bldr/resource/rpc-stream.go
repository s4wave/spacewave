package resource

import (
	"context"
	"errors"

	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
)

// ResourceRpcStream carries one Resource handshake followed by SRPC data frames.
type ResourceRpcStream interface {
	srpc.Stream
	Send(*ResourceRpcPacket) error
	Recv() (*ResourceRpcPacket, error)
}

// NewResourceRpcClient opens a typed route for each invocation on resourceID.
func NewResourceRpcClient[T ResourceRpcStream](caller func(context.Context) (T, error), resourceID uint32) srpc.Client {
	return srpc.NewClient(func(ctx context.Context, handler srpc.PacketDataHandler, closed srpc.CloseHandler) (srpc.PacketWriter, error) {
		// Negotiate exactly once before exposing the data transport.
		stream, err := caller(ctx)
		if err != nil {
			return nil, err
		}
		err = stream.Send(&ResourceRpcPacket{Body: &ResourceRpcPacket_Init{Init: &ResourceRpcInit{ResourceId: resourceID}}})
		if err == nil {
			var response *ResourceRpcPacket
			response, err = stream.Recv()
			if err == nil {
				if ack := response.GetAck(); ack == nil {
					err = errors.New("expected ResourceRpc acknowledgement")
				} else if failure := ack.GetFailure(); failure != nil {
					err = failure
				}
			}
		}
		if err != nil {
			_ = stream.Close()
			return nil, err
		}

		// Reuse SRPC framing and termination after the Resource acknowledgement.
		data := &resourceRpcDataStream{stream}
		go rpcstream.ReadPump(data, handler, closed)
		return rpcstream.NewRpcStreamWriter(data), nil
	})
}

// HandleResourceRpc acknowledges a route before serving its SRPC data frames.
// The handler waits for active methods before releasing their resource context.
func HandleResourceRpc(stream ResourceRpcStream, lookup func(context.Context, uint32) (srpc.Invoker, error)) error {
	request, err := stream.Recv()
	if err != nil {
		return err
	}
	init := request.GetInit()
	if init == nil {
		return errors.New("expected ResourceRpc init")
	}

	// Publish a typed refusal without starting a second handshake.
	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()
	mux, routeErr := lookup(ctx, init.GetResourceId())
	if routeErr == nil && mux == nil {
		routeErr = ErrResourceNotFound
	}
	if err := stream.Send(&ResourceRpcPacket{Body: &ResourceRpcPacket_Ack{Ack: &ResourceRpcAck{Failure: FailureFromError(routeErr)}}}); err != nil {
		return err
	}
	if routeErr != nil {
		return nil
	}

	// The SRPC owner drains active methods before the route returns.
	data := &resourceRpcDataStream{stream}
	server := srpc.NewServerRPC(ctx, mux, rpcstream.NewRpcStreamWriter(data))
	go rpcstream.ReadPump(data, server.HandlePacketData, server.HandleStreamClose)
	err = server.Wait(context.WithoutCancel(stream.Context()))
	if errors.Is(err, context.Canceled) && ctx.Err() == nil {
		return nil
	}
	return err
}

// resourceRpcDataStream adapts negotiated Resource data to the SRPC codec.
type resourceRpcDataStream struct {
	ResourceRpcStream
}

// Send forwards only data; the Resource owner already negotiated the route.
func (s *resourceRpcDataStream) Send(packet *rpcstream.RpcStreamPacket) error {
	data, ok := packet.GetBody().(*rpcstream.RpcStreamPacket_Data)
	if !ok {
		return rpcstream.ErrUnexpectedPacket
	}
	return s.ResourceRpcStream.Send(&ResourceRpcPacket{Body: &ResourceRpcPacket_Data{Data: data.Data}})
}

// Recv rejects a repeated handshake after the route has been acknowledged.
func (s *resourceRpcDataStream) Recv() (*rpcstream.RpcStreamPacket, error) {
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
