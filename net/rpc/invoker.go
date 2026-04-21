package bifrost_rpc

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
)

// Invoker implements the RPC invoker with a directive.
type Invoker struct {
	// b is the bus
	b bus.Bus
	// serverID is the server identifier.
	// can be empty
	serverID string
	// wait waits for the rpc service to become available.
	wait bool
}

// NewInvoker constructs a new rpc method invoker.
// serverID can be empty, will be used for directives.
// if wait is set, waits for the rpc service to become available.
// otherwise, returns "unimplemented" if the service is unavailable.
func NewInvoker(b bus.Bus, serverID string, wait bool) *Invoker {
	return &Invoker{
		b:        b,
		serverID: serverID,
		wait:     wait,
	}
}

// InvokeMethod invokes the method matching the service & method ID.
// Returns false, nil if not found.
// If service string is empty, ignore it.
func (i *Invoker) InvokeMethod(serviceID, methodID string, strm srpc.Stream) (bool, error) {
	ctx := strm.Context()

	invokers, _, invokerRef, err := ExLookupRpcService(ctx, i.b, serviceID, i.serverID, i.wait, nil)
	if err != nil || invokerRef == nil {
		return false, err
	}
	defer invokerRef.Release()

	var sl srpc.InvokerSlice = invokers
	return sl.InvokeMethod(serviceID, methodID, strm)
}

// _ is a type assertion
var _ srpc.Invoker = ((*Invoker)(nil))
