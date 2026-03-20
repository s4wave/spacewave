package resource_server

import (
	"github.com/aperturerobotics/starpc/srpc"
)

// trackedResource holds state for an ongoing tracked resource.
type trackedResource struct {
	// mux is the srpc mux for the resource
	mux srpc.Invoker
	// ownerClientID is the client that owns this resource
	ownerClientID uint32
	// releaseFn is an optional callback when the resource is released
	releaseFn func()
}
