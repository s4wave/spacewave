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

	// parentResourceID orders generation teardown, without controlling ownership.
	parentResourceID uint32
	// offer owns the unaccepted handle, independently of its invoking parent.
	offer *resourceRPCContext
}

// acceptOffer consumes the invocation's obligation under the generation lock.
func (r *trackedResource) acceptOffer(id uint32) {
	if r.offer != nil {
		delete(r.offer.unpublished, id)
		delete(r.offer.published, id)
		r.offer = nil
	}
}
