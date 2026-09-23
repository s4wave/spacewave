package bifrost_rpc

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
)

// BusClient implements srpc.Client looking up the RPC client on-demand when a RPC starts.
type BusClient struct {
	// b resolves the requested service when each call starts.
	b bus.Bus
	// wait holds calls until a client becomes available.
	wait bool
}

// NewBusClient constructs a new rpc client.
func NewBusClient(b bus.Bus) *BusClient {
	return NewBusClientWithWait(b, true)
}

// NewBusClientWithWait controls whether an unavailable service waits for startup.
// With wait false, calls settle with ErrServiceClientUnavailable after lookup.
func NewBusClientWithWait(b bus.Bus, wait bool) *BusClient {
	return &BusClient{b: b, wait: wait}
}

// ExecCall executes a request/reply RPC with the remote.
func (c *BusClient) ExecCall(
	ctx context.Context,
	service,
	method string,
	in,
	out srpc.Message,
) error {
	clientSet, _, ref, err := ExLookupRpcClientSet(ctx, c.b, service, method, c.wait, nil)
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
	clientSet, _, ref, err := ExLookupRpcClientSet(ctx, c.b, service, method, c.wait, nil)
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
