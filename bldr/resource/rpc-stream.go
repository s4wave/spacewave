package resource

import (
	"context"

	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
)

// ResourceRpcStream carries one Resource handshake followed by SRPC data frames.
type ResourceRpcStream interface {
	srpc.Stream
	// Send sends a route control or nested SRPC packet.
	Send(*ResourceRpcPacket) error
	// Recv receives a route control or nested SRPC packet.
	Recv() (*ResourceRpcPacket, error)
}

// NewResourceRpcClient opens a typed route for each invocation on resourceID.
func NewResourceRpcClient[T ResourceRpcStream](caller func(context.Context) (T, error), resourceID uint32) srpc.Client {
	inner := srpc.NewClient(func(ctx context.Context, handler srpc.PacketDataHandler, closed srpc.CloseHandler) (srpc.PacketWriter, error) {
		// Keep a unary transport alive long enough to send its final abandonment.
		call, _ := ctx.Value(resourceRpcCallKey{}).(*resourceRpcCall)
		transportCtx := ctx
		stop := func() bool { return true }
		if call != nil {
			transportCtx, call.cancel = context.WithCancel(context.WithoutCancel(ctx))
			stop = context.AfterFunc(ctx, call.cancel)
		}
		defer stop()

		// Negotiate exactly once before exposing the data transport.
		stream, err := caller(transportCtx)
		if err != nil {
			return nil, err
		}
		if call != nil {
			call.stream = stream
		}
		err = stream.Send(&ResourceRpcPacket{Body: &ResourceRpcPacket_Init{Init: &ResourceRpcInit{ResourceId: resourceID}}})
		if err == nil {
			var response *ResourceRpcPacket
			response, err = stream.Recv()
			if err == nil {
				ack := response.GetAck()
				if ack == nil {
					err = errors.New("expected ResourceRpc acknowledgement")
				}
				if ack != nil && ack.GetFailure() != nil {
					err = ack.GetFailure()
				}
			}
		}
		if err != nil {
			// Release the failed stream before reporting the negotiation error.
			_ = stream.Close()
			return nil, err
		}

		// Reuse SRPC framing and termination after the Resource acknowledgement.
		data := &resourceRpcDataStream{ResourceRpcStream: stream, retain: call != nil}
		go rpcstream.ReadPump(data, handler, closed)
		return rpcstream.NewRpcStreamWriter(data), nil
	})
	return &resourceRpcClient{inner: inner}
}

// HandleResourceRpc acknowledges a route before serving its SRPC data frames.
// The handler waits for active methods before releasing their resource context.
func HandleResourceRpc(stream ResourceRpcStream, lookup func(context.Context, uint32) (srpc.Invoker, error)) error {
	// Read the init packet and require a valid resource handshake.
	request, err := stream.Recv()
	if err != nil {
		return err
	}
	init := request.GetInit()
	if init == nil {
		return errors.New("expected ResourceRpc init")
	}

	// Resolve the route for the requested resource and acknowledge the result.
	// Publish a typed refusal without starting a second handshake.
	route := &resourceRpcRoute{}
	ctx, cancel := context.WithCancel(context.WithValue(stream.Context(), resourceRpcRouteKey{}, route))
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

	// Keep reading after handler completion so late abandonment reaches its children.
	data := &resourceRpcDataStream{
		ResourceRpcStream: stream,
		retain:            true,
		onSendError: func() {
			// Report response EOF while retaining the reader for abandonment.
			_ = stream.CloseSend()
		},
	}
	server := srpc.NewServerRPC(ctx, mux, rpcstream.NewRpcStreamWriter(data))

	// The nested completion packet settles calls while the outer route stays open.
	for {
		// Accept route controls until the client closes its sending direction.
		packet, readErr := stream.Recv()
		if readErr != nil {
			err = readErr
			break
		}
		switch body := packet.GetBody().(type) {
		case *ResourceRpcPacket_Data:
			err = server.HandlePacketData(body.Data)
		case *ResourceRpcPacket_Abandon:
			route.abandon()
			cancel()
		default:
			err = rpcstream.ErrUnexpectedPacket
		}
		if err != nil {
			break
		}
	}

	// A route close cancels active methods; Wait drains them before returning.
	server.HandleStreamClose(err)
	err = server.Wait(context.WithoutCancel(stream.Context()))
	if errors.Is(err, context.Canceled) && ctx.Err() == nil {
		return nil
	}
	return err
}
