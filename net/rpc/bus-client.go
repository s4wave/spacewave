package bifrost_rpc

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
)

// BusClient implements srpc.Client looking up the RPC client on-demand when a RPC starts.
type BusClient struct {
	b bus.Bus
}

// NewBusClient constructs a new rpc client.
func NewBusClient(b bus.Bus) *BusClient {
	return &BusClient{b: b}
}

// ExecCall executes a request/reply RPC with the remote.
func (c *BusClient) ExecCall(
	ctx context.Context,
	service,
	method string,
	in,
	out srpc.Message,
) error {
	clientSet, _, ref, err := ExLookupRpcClientSet(ctx, c.b, service, method, true, nil)
	if err != nil {
		return err
	}
	defer ref.Release()

	return clientSet.ExecCall(ctx, service, method, in, out)
}

// NewStream starts a streaming RPC with the remote & returns the stream.
// firstMsg is optional.
func (c *BusClient) NewStream(
	ctx context.Context,
	service,
	method string,
	firstMsg srpc.Message,
) (srpc.Stream, error) {
	clientSet, _, ref, err := ExLookupRpcClientSet(ctx, c.b, service, method, true, nil)
	if err != nil {
		return nil, err
	}

	strm, err := clientSet.NewStream(ctx, service, method, firstMsg)
	if err != nil {
		ref.Release()
		return nil, err
	}

	return srpc.NewStreamWithClose(strm, func() error {
		ref.Release()
		return nil
	}), nil
}

// _ is a type assertion
var _ srpc.Client = (*BusClient)(nil)
