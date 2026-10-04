package resource_server

import (
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/bldr/resource"
)

// resourceServerClientInvoker scopes registrations to their publishing call.
type resourceServerClientInvoker struct {
	// mux serves the resource's methods.
	mux srpc.Invoker
	// client owns the resource generation, or is nil for attached resources.
	client *RemoteResourceClient
	// parentResourceID identifies the resource receiving the call.
	parentResourceID uint32
}

// InvokeMethod releases unpublished pending registrations when the handler exits.
func (c *resourceServerClientInvoker) InvokeMethod(serviceID, methodID string, strm srpc.Stream) (bool, error) {
	// Forward attached-resource calls without adding server registration state.
	if c.client == nil {
		return c.mux.InvokeMethod(serviceID, methodID, strm)
	}

	// Bind registration and response publication to the invocation lifetime.
	resourceCtx := newResourceRPCContext(c.client, c.parentResourceID)
	defer resourceCtx.releaseUnpublished()
	resource.OnResourceRpcAbandon(strm.Context(), resourceCtx.abandon)
	childCtx := WithResourceClientContext(strm.Context(), resourceCtx)
	return c.mux.InvokeMethod(serviceID, methodID, &resourceRPCStream{
		Stream:   srpc.NewStreamWithContext(strm, childCtx),
		resource: resourceCtx,
	})
}

// _ is a type assertion
var _ srpc.Invoker = (*resourceServerClientInvoker)(nil)
