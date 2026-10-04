package resource_server

import (
	"context"
	"maps"
	"slices"

	"github.com/aperturerobotics/starpc/srpc"
)

// resourceRPCContext retains invocation children until a response publishes
// them. Handler completion releases unpublished children that remain pending.
type resourceRPCContext struct {
	// client owns the resource generation and guards this state with server.bcast.
	client *RemoteResourceClient
	// parentResourceID is the resource receiving the invocation.
	parentResourceID uint32
	// unpublished holds resource IDs not yet covered by a successful response.
	unpublished map[uint32]struct{}
	// ended rejects registrations after handler completion.
	ended bool
}

// newResourceRPCContext constructs the registration scope for one invocation.
func newResourceRPCContext(
	client *RemoteResourceClient,
	parentResourceID uint32,
) *resourceRPCContext {
	return &resourceRPCContext{
		client:           client,
		parentResourceID: parentResourceID,
	}
}

// Context returns the ResourceClient generation context.
func (c *resourceRPCContext) Context() context.Context { return c.client.Context() }

// AddResource adds a pending invocation child.
func (c *resourceRPCContext) AddResource(mux srpc.Invoker, releaseFn func()) (uint32, error) {
	return c.AddResourceValue(mux, nil, releaseFn)
}

// AddResourceValue adds a pending invocation child with an in-process value.
func (c *resourceRPCContext) AddResourceValue(mux srpc.Invoker, value any, releaseFn func()) (uint32, error) {
	return c.client.addResource(c.parentResourceID, mux, value, releaseFn, c)
}

// send publishes only registrations that precede this response's send.
func (c *resourceRPCContext) send(strm srpc.Stream, msg srpc.Message) error {
	// Snapshot the registrations covered by this response before transport I/O.
	var ids []uint32
	c.client.server.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		ids = slices.Collect(maps.Keys(c.unpublished))
	})

	// Leave failed response registrations for completion cleanup.
	if err := strm.MsgSend(msg); err != nil {
		return err
	}

	// Transfer published registrations to the generation's Adopt/Release controls.
	c.client.server.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for _, id := range ids {
			delete(c.unpublished, id)
		}
	})
	return nil
}

// releaseUnpublished releases pending registrations after the handler exits.
func (c *resourceRPCContext) releaseUnpublished() {
	// Remove only unpublished, unadopted resources under the adoption lock.
	var releaseFns []func()
	var releasedIDs []uint32
	c.client.server.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Release invocation children in ID order using the generation's tree cleanup.
		c.ended = true
		ids := slices.Sorted(maps.Keys(c.unpublished))
		for _, id := range ids {
			if res := c.client.resources[id]; res != nil && res.pending {
				c.client.releaseLocked(id, true, false, &releaseFns, &releasedIDs)
			}
		}
		clear(c.unpublished)
		c.client.finishRelease(releasedIDs, broadcast)
	})

	// Run cleanup outside the lifecycle lock after the resources disappear.
	for _, releaseFn := range releaseFns {
		releaseFn()
	}
}

// ReleaseResource releases a resource from the server side.
func (c *resourceRPCContext) ReleaseResource(resourceID uint32) bool {
	return c.client.ReleaseResource(resourceID)
}

// GetResourceValue returns an in-process resource value.
func (c *resourceRPCContext) GetResourceValue(resourceID uint32) (any, error) {
	return c.client.GetResourceValue(resourceID)
}

// GetAttachedResource returns a client-published resource.
func (c *resourceRPCContext) GetAttachedResource(resourceID uint32) (srpc.Client, error) {
	return c.client.GetAttachedResource(resourceID)
}

// _ is a type assertion
var _ ResourceClientContext = (*resourceRPCContext)(nil)
