package resource_server

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/bldr/resource"
)

// RemoteResourceClient holds state for an attached client.
type RemoteResourceClient struct {
	// server is a reference to the parent server
	server *ResourceServer
	// clientID is the ID of this client
	clientID uint32
	// ctx is the client session context, canceled when the client is released.
	ctx context.Context
	// txQueue contains messages to transmit to the client.
	txQueue []*resource.ResourceClientResponse
	// released indicates if the client has been released.
	released bool
	// adoptionAckEnabled selects held ResourceRpc receipt finalization.
	adoptionAckEnabled bool
	// resources contains the map of resources owned by this client.
	resources map[uint32]*trackedResource
	// pendingAdoptions maps invocation-created resources to their owner.
	pendingAdoptions map[uint32]*resourceRPCContext
	// adoptedAdoptions maps acknowledged resources to their invocation until commit.
	adoptedAdoptions map[uint32]*resourceRPCContext
	// attachedResources are resources provided by the client via
	// ResourceAttach. Keyed by server-assigned resource ID.
	attachedResources map[uint32]*attachedResource
}

func (c *RemoteResourceClient) addInvocationResource(
	rpcCtx *resourceRPCContext,
	mux srpc.Invoker,
	value any,
	releaseFn func(),
) (uint32, error) {
	var resourceID uint32
	var released bool
	c.server.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if c.released || rpcCtx.finished {
			released = true
			return
		}
		c.server.resourceIDCtr++
		resourceID = c.server.resourceIDCtr
		c.resources[resourceID] = &trackedResource{
			mux:           mux,
			value:         value,
			ownerClientID: c.clientID,
			releaseFn:     releaseFn,
		}
		if c.pendingAdoptions == nil {
			c.pendingAdoptions = make(map[uint32]*resourceRPCContext)
		}
		c.pendingAdoptions[resourceID] = rpcCtx
	})
	if released {
		return 0, resource.ErrClientReleased
	}
	return resourceID, nil
}

func (c *RemoteResourceClient) adoptResource(resourceID uint32) bool {
	var adopted bool
	c.server.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if !c.adoptionAckEnabled ||
			c.released ||
			(c.resources[resourceID] == nil && c.attachedResources[resourceID] == nil) {
			return
		}
		rpcCtx := c.pendingAdoptions[resourceID]
		if rpcCtx != nil {
			if rpcCtx.finished {
				return
			}
			delete(c.pendingAdoptions, resourceID)
			if c.adoptedAdoptions == nil {
				c.adoptedAdoptions = make(map[uint32]*resourceRPCContext)
			}
			c.adoptedAdoptions[resourceID] = rpcCtx
		}
		adopted = true
	})
	return adopted
}

func (c *RemoteResourceClient) finishInvocation(
	rpcCtx *resourceRPCContext,
	success bool,
	releasePendingOnSuccess bool,
) {
	var releaseFns []func()
	var queuedRelease bool
	c.server.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if rpcCtx.finished {
			return
		}
		rpcCtx.finished = true
		releasePending := !success || releasePendingOnSuccess
		for resourceID, owner := range c.pendingAdoptions {
			if owner != rpcCtx {
				continue
			}
			delete(c.pendingAdoptions, resourceID)
			if !releasePending {
				continue
			}
			res := c.resources[resourceID]
			if res == nil {
				continue
			}
			delete(c.resources, resourceID)
			if res.releaseFn != nil {
				releaseFns = append(releaseFns, res.releaseFn)
			}
			queuedRelease = true
			c.txQueue = append(c.txQueue, &resource.ResourceClientResponse{
				Body: &resource.ResourceClientResponse_ResourceReleased{
					ResourceReleased: &resource.ResourceReleasedResponse{
						ResourceId: resourceID,
					},
				},
			})
		}
		for resourceID, owner := range c.adoptedAdoptions {
			if owner != rpcCtx {
				continue
			}
			delete(c.adoptedAdoptions, resourceID)
			if success {
				continue
			}
			res := c.resources[resourceID]
			if res == nil {
				continue
			}
			delete(c.resources, resourceID)
			if res.releaseFn != nil {
				releaseFns = append(releaseFns, res.releaseFn)
			}
			queuedRelease = true
			c.txQueue = append(c.txQueue, &resource.ResourceClientResponse{
				Body: &resource.ResourceClientResponse_ResourceReleased{
					ResourceReleased: &resource.ResourceReleasedResponse{
						ResourceId: resourceID,
					},
				},
			})
		}
		if queuedRelease {
			broadcast()
		}
	})
	for _, releaseFn := range releaseFns {
		releaseFn()
	}
}

// Context returns the client session context.
// This context lives for the duration of the client session and is
// canceled when the client is released. Use this for sub-resources
// that need to outlive individual RPC calls.
func (c *RemoteResourceClient) Context() context.Context {
	return c.ctx
}

// AddResource adds a new resource with the given mux and returns its unique ID.
// The releaseFn callback will be called when the resource is released (can be nil).
// Returns an error if the client has already been released.
//
// Note: Server-side handlers may send the same resource ID to the client multiple
// times (out-of-band from this API). The client uses reference counting to track
// when all references to a resource ID have been released.
func (c *RemoteResourceClient) AddResource(mux srpc.Invoker, releaseFn func()) (uint32, error) {
	return c.AddResourceValue(mux, nil, releaseFn)
}

// AddResourceValue adds a new resource with an optional typed value.
func (c *RemoteResourceClient) AddResourceValue(mux srpc.Invoker, value any, releaseFn func()) (uint32, error) {
	var resourceID uint32

	err := func() error {
		var released bool
		c.server.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			if c.released {
				released = true
				return
			}

			c.server.resourceIDCtr++
			resourceID = c.server.resourceIDCtr

			res := &trackedResource{
				mux:           mux,
				value:         value,
				ownerClientID: c.clientID,
				releaseFn:     releaseFn,
			}

			// ResourceClient waits on this broadcast only for outbound queue
			// events. Adding a resource mutates the lookup table for later
			// RPCs, but it does not enqueue a client notification.
			c.resources[resourceID] = res
		})

		if released {
			return resource.ErrClientReleased
		}
		return nil
	}()

	return resourceID, err
}

// GetResourceValue returns the typed resource value for a tracked resource.
func (c *RemoteResourceClient) GetResourceValue(resourceID uint32) (any, error) {
	var value any
	c.server.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		res := c.resources[resourceID]
		if res != nil {
			value = res.value
		}
	})
	if value == nil {
		return nil, resource.ErrResourceNotFound
	}
	return value, nil
}

// ReleaseResource releases a resource that was previously added.
// Calls the releaseFn callback if it was provided to AddResource.
// Sends a ResourceReleasedResponse message to the client.
// Returns true if the resource was found and released, false if not found.
// Safe to call even if the resource has already been released.
func (c *RemoteResourceClient) ReleaseResource(resourceID uint32) bool {
	var released bool
	var releaseFn func()
	var attachedCancel func()

	c.server.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if c.released {
			return
		}

		res, ok := c.resources[resourceID]
		if !ok {
			ar := c.attachedResources[resourceID]
			if ar == nil {
				return
			}
			delete(c.attachedResources, resourceID)
			attachedCancel = ar.cancel
			releaseFn = ar.releaseFn
			released = true
			return
		}

		delete(c.resources, resourceID)
		delete(c.pendingAdoptions, resourceID)
		delete(c.adoptedAdoptions, resourceID)
		releaseFn = res.releaseFn
		released = true

		// Queue a message to notify the client
		c.txQueue = append(c.txQueue, &resource.ResourceClientResponse{
			Body: &resource.ResourceClientResponse_ResourceReleased{
				ResourceReleased: &resource.ResourceReleasedResponse{
					ResourceId: resourceID,
				},
			},
		})

		broadcast()
	})

	// Call releaseFn outside of lock
	if releaseFn != nil {
		releaseFn()
	}
	if attachedCancel != nil {
		attachedCancel()
	}

	return released
}

// AddAttachedResource registers a client-provided attached resource.
// Returns an error if the client has been released.
func (c *RemoteResourceClient) AddAttachedResource(
	id uint32,
	label string,
	cancel context.CancelFunc,
	srpcClient srpc.Client,
	releaseFn func(),
) error {
	var released bool
	c.server.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if c.released {
			released = true
			return
		}
		if c.attachedResources == nil {
			c.attachedResources = make(map[uint32]*attachedResource)
		}
		c.attachedResources[id] = &attachedResource{
			label:      label,
			cancel:     cancel,
			releaseFn:  releaseFn,
			srpcClient: srpcClient,
		}
	})
	if released {
		return resource.ErrClientReleased
	}
	return nil
}

// RemoveAttachedResource removes an attached resource and cancels its context.
func (c *RemoteResourceClient) RemoveAttachedResource(id uint32) {
	c.removeAttachedResource(id, true)
}

func (c *RemoteResourceClient) removeAttachedResource(id uint32, notify bool) {
	var cancel context.CancelFunc
	var releaseFn func()
	c.server.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		ar, ok := c.attachedResources[id]
		if !ok {
			return
		}
		cancel = ar.cancel
		if notify {
			releaseFn = ar.releaseFn
		}
		delete(c.attachedResources, id)
	})
	if releaseFn != nil {
		releaseFn()
	}
	if cancel != nil {
		cancel()
	}
}

// GetAttachedResource returns the srpc.Client for an attached resource.
func (c *RemoteResourceClient) GetAttachedResource(id uint32) (srpc.Client, error) {
	var client srpc.Client
	c.server.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		ar := c.attachedResources[id]
		if ar != nil {
			client = ar.srpcClient
		}
	})
	if client == nil {
		return nil, resource.ErrResourceNotFound
	}
	return client, nil
}

// releaseAllAttachedResources cancels and removes all attached resources.
// Called during client cleanup.
func (c *RemoteResourceClient) releaseAllAttachedResources() {
	var cancels []context.CancelFunc
	c.server.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for _, ar := range c.attachedResources {
			cancels = append(cancels, ar.cancel)
		}
		clear(c.attachedResources)
	})
	for _, cancel := range cancels {
		cancel()
	}
}
