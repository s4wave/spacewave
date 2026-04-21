package stream_api

import (
	stream_api_rpc "github.com/s4wave/spacewave/net/stream/api/rpc"
)

// AcceptStreamClientRPC fulfills the RPC on the client side.
type AcceptStreamClientRPC struct {
	SRPCStreamService_AcceptStreamClient
}

// NewAcceptStreamClientRPC builds a new AcceptStreamClient.
func NewAcceptStreamClientRPC(
	client SRPCStreamService_AcceptStreamClient,
) stream_api_rpc.RPC {
	return &AcceptStreamClientRPC{
		SRPCStreamService_AcceptStreamClient: client,
	}
}

// Send sends a packet.
func (r *AcceptStreamClientRPC) Send(resp *stream_api_rpc.Data) error {
	return r.SRPCStreamService_AcceptStreamClient.Send(
		&AcceptStreamRequest{
			Data: resp,
		},
	)
}

// Recv receives a packet.
func (r *AcceptStreamClientRPC) Recv() (*stream_api_rpc.Data, error) {
	msg, err := r.SRPCStreamService_AcceptStreamClient.Recv()
	return msg.GetData(), err
}

// _ is a type assertion
var _ stream_api_rpc.RPC = ((*AcceptStreamClientRPC)(nil))
