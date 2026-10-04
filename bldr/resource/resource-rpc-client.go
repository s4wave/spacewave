package resource

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
)

// resourceRpcCallKey carries the unary call's route into its stream opener.
type resourceRpcCallKey struct{}

// resourceRpcCall retains the transport until ExecCall settles its result.
type resourceRpcCall struct {
	// stream is set by the opener before it negotiates the route.
	stream ResourceRpcStream
	// cancel ends the transport context after the result is settled.
	cancel context.CancelFunc
}

// resourceRpcClient abandons pending children when a unary call fails.
type resourceRpcClient struct {
	// inner opens and executes the nested SRPC call.
	inner srpc.Client
}

// ExecCall retains the route until its result decides whether to abandon it.
func (c *resourceRpcClient) ExecCall(ctx context.Context, service, method string, in, out srpc.Message) error {
	// Give this call one route record without sharing state between calls.
	call := &resourceRpcCall{}
	err := c.inner.ExecCall(context.WithValue(ctx, resourceRpcCallKey{}, call), service, method, in, out)
	if call.cancel != nil {
		defer call.cancel()
	}

	// A failed result leaves every returned child ID unavailable to the caller.
	if call.stream != nil {
		if err != nil {
			// A broken transport can reject abandonment; retain the original error.
			_ = call.stream.Send(&ResourceRpcPacket{Body: &ResourceRpcPacket_Abandon{Abandon: &ResourceRpcAbandon{}}})
			_ = call.stream.Close()
			return err
		}

		// Complete the successful route before releasing its context and hooks.
		_ = call.stream.CloseSend()
		_ = call.stream.Close()
	}
	return err
}

// NewStream preserves the streaming caller's control over delivered messages.
func (c *resourceRpcClient) NewStream(ctx context.Context, service, method string, firstMsg srpc.Message) (srpc.Stream, error) {
	return c.inner.NewStream(ctx, service, method, firstMsg)
}

// _ is a type assertion.
var _ srpc.Client = (*resourceRpcClient)(nil)
