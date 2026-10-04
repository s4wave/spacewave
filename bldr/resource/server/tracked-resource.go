package resource_server

import "github.com/aperturerobotics/starpc/srpc"

// trackedResource retains one resource until its generation releases it.
type trackedResource struct {
	// mux serves the resource's RPC methods.
	mux srpc.Invoker
	// value carries the optional in-process resource value.
	value any
	// ownerClientID identifies the owning client generation.
	ownerClientID uint32
	// releaseFn tears down the resource after removal from the generation.
	releaseFn func()

	// parentResourceID links invocation resources for pending-child cleanup.
	parentResourceID uint32
	// pending remains true until the client adopts the resource.
	pending bool
}
