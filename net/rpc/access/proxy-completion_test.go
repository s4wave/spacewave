package bifrost_rpc_access

import (
	"context"
	"errors"
	"testing"

	"github.com/aperturerobotics/starpc/echo"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
)

// queuedProxyResponse delivers a response queued before the request write fails.
type queuedProxyResponse struct {
	SRPCAccessRpcServiceClient
	SRPCAccessRpcService_CallRpcServiceClient

	// terminal holds the remote method's last packet.
	terminal *rpcstream.RpcStreamPacket
	// closed signals when the proxy closes the stream after the failed write.
	closed chan struct{}
}

// CallRpcService opens the stream holding the response.
func (s *queuedProxyResponse) CallRpcService(context.Context) (SRPCAccessRpcService_CallRpcServiceClient, error) {
	return s, nil
}

// Send accepts the call setup and fails when the proxy forwards request data.
func (s *queuedProxyResponse) Send(pkt *rpcstream.RpcStreamPacket) error {
	// Accept the stream and call setup packets.
	if pkt.GetInit() != nil {
		return nil
	}
	var call srpc.Packet
	if err := call.UnmarshalVT(pkt.GetData()); err != nil {
		return err
	}
	if call.GetCallStart() != nil {
		return nil
	}

	// The remote has closed its input while its response is still queued.
	return context.Canceled
}

// Recv allows the proxy to read the queued response after closing the stream.
func (s *queuedProxyResponse) Recv() (*rpcstream.RpcStreamPacket, error) {
	<-s.closed
	return s.terminal, nil
}

// Close wakes Recv without discarding the queued response.
func (s *queuedProxyResponse) Close() error {
	select {
	case s.closed <- struct{}{}:
	default:
	}
	return nil
}

// TestProxyInvokerKeepsQueuedRemoteError checks that a failed request write
// preserves the remote method's error, even when the write finishes first.
func TestProxyInvokerKeepsQueuedRemoteError(t *testing.T) {
	// Queue the remote method's completed response behind the failed write.
	want := errors.New("remote method failed")
	data, err := srpc.NewCallDataPacket(nil, false, true, want).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	client := &queuedProxyResponse{
		terminal: &rpcstream.RpcStreamPacket{Body: &rpcstream.RpcStreamPacket_Data{Data: data}},
		closed:   make(chan struct{}, 1),
	}

	// Report the terminal packet to the caller without replacing its error.
	const serviceID = echo.SRPCEchoerServiceID
	invoker := NewProxyInvoker(client, &LookupRpcServiceRequest{ServiceId: serviceID}, false)
	found, err := invoker.InvokeMethod(serviceID, "Echo", &responseFirstStream{ctx: t.Context()})
	if !found || err == nil || err.Error() != want.Error() {
		t.Fatalf("found=%v, error=%v, want %v", found, err, want)
	}
}
