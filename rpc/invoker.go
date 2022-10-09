package bldr_rpc

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
)

// Invoker implements the bldr RPC invoker with a directive.
type Invoker struct {
	// b is the bus
	b bus.Bus
	// clientID is the client identifier.
	// can be empty
	clientID string
}

// NewInvoker constructs a new rpc method invoker.
// clientID can be empty, will be used for directives.
func NewInvoker(b bus.Bus, clientID string) *Invoker {
	return &Invoker{
		b:        b,
		clientID: clientID,
	}
}

// InvokeMethod invokes the method matching the service & method ID.
// Returns false, nil if not found.
// If service string is empty, ignore it.
func (i *Invoker) InvokeMethod(serviceID, methodID string, strm srpc.Stream) (bool, error) {
	ctx := strm.Context()

	invokers, invokerRef, err := ExLookupRpcService(ctx, i.b, serviceID, i.clientID)
	if err != nil || invokerRef == nil {
		return false, err
	}
	defer invokerRef.Release()

	for _, invoker := range invokers {
		found, err := invoker.InvokeMethod(serviceID, methodID, strm)
		if found || err != nil {
			return found && err == nil, err
		}
	}

	return false, nil
}

// _ is a type assertion
var _ srpc.Invoker = ((*Invoker)(nil))
