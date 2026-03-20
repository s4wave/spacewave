package resource

import (
	"context"
	"strconv"
	"strings"
	"sync"

	"github.com/aperturerobotics/starpc/srpc"
)

// routedClient wraps an SRPC client and prefixes service IDs with a resource
// ID for routing. Used by the server to target specific attached resources
// over a shared yamux session.
type routedClient struct {
	inner  srpc.Client
	prefix string
}

// NewRoutedClient wraps an SRPC client so all calls are prefixed with the
// resource ID for routing to the correct attached resource mux.
func NewRoutedClient(inner srpc.Client, resourceID uint32) srpc.Client {
	return &routedClient{
		inner:  inner,
		prefix: strconv.FormatUint(uint64(resourceID), 10) + "/",
	}
}

// ExecCall executes a request/reply RPC with the resource ID prefix.
func (c *routedClient) ExecCall(ctx context.Context, service, method string, in, out srpc.Message) error {
	return c.inner.ExecCall(ctx, c.prefix+service, method, in, out)
}

// NewStream opens a streaming RPC with the resource ID prefix.
func (c *routedClient) NewStream(ctx context.Context, service, method string, firstMsg srpc.Message) (srpc.Stream, error) {
	return c.inner.NewStream(ctx, c.prefix+service, method, firstMsg)
}

// RoutedInvoker dispatches SRPC calls to the correct mux by parsing a
// resource ID prefix from the service ID. Used by the client to route
// incoming yamux sub-stream RPCs to the right attached resource.
type RoutedInvoker struct {
	mu    sync.Mutex
	muxes map[uint32]srpc.Invoker
}

// NewRoutedInvoker creates a new RoutedInvoker.
func NewRoutedInvoker() *RoutedInvoker {
	return &RoutedInvoker{muxes: make(map[uint32]srpc.Invoker)}
}

// SetMux registers or replaces a mux for a resource ID.
func (r *RoutedInvoker) SetMux(resourceID uint32, mux srpc.Invoker) {
	r.mu.Lock()
	r.muxes[resourceID] = mux
	r.mu.Unlock()
}

// RemoveMux removes a mux for a resource ID.
func (r *RoutedInvoker) RemoveMux(resourceID uint32) {
	r.mu.Lock()
	delete(r.muxes, resourceID)
	r.mu.Unlock()
}

// InvokeMethod parses the resource ID prefix from serviceID and dispatches.
func (r *RoutedInvoker) InvokeMethod(serviceID, methodID string, strm srpc.Stream) (bool, error) {
	prefix, rest, ok := strings.Cut(serviceID, "/")
	if !ok {
		return false, nil
	}
	id, err := strconv.ParseUint(prefix, 10, 32)
	if err != nil {
		return false, nil
	}
	r.mu.Lock()
	mux := r.muxes[uint32(id)]
	r.mu.Unlock()
	if mux == nil {
		return false, ErrResourceNotFound
	}
	return mux.InvokeMethod(rest, methodID, strm)
}

// _ is a type assertion
var _ srpc.Client = (*routedClient)(nil)

// _ is a type assertion
var _ srpc.Invoker = (*RoutedInvoker)(nil)
