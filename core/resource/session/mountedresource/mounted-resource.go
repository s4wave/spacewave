package mountedresource

import "github.com/aperturerobotics/starpc/srpc"

// Context is the resource-client surface needed to register a mounted value.
type Context interface {
	AddResourceValue(mux srpc.Invoker, value any, releaseFn func()) (uint32, error)
}

// Add registers a child resource mux together with its typed resource value.
func Add(resourceCtx Context, mux srpc.Invoker, value any, releaseFn func()) (uint32, error) {
	return resourceCtx.AddResourceValue(mux, value, releaseFn)
}
