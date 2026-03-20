package resource_server

import (
	"context"

	"github.com/aperturerobotics/bldr/resource"
	"github.com/aperturerobotics/starpc/srpc"
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
	// resources contains the map of resources owned by this client
	resources map[uint32]*trackedResource
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
	var resourceID uint32

	err := func() error {
		var released bool
		c.server.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			if c.released {
				released = true
				return
			}

			c.server.resourceIDCtr++
			resourceID = c.server.resourceIDCtr

			res := &trackedResource{
				mux:           mux,
				ownerClientID: c.clientID,
				releaseFn:     releaseFn,
			}

			c.resources[resourceID] = res
			broadcast()
		})

		if released {
			return resource.ErrClientReleased
		}
		return nil
	}()

	return resourceID, err
}

// ReleaseResource releases a resource that was previously added.
// Calls the releaseFn callback if it was provided to AddResource.
// Sends a ResourceReleasedResponse message to the client.
// Returns true if the resource was found and released, false if not found.
// Safe to call even if the resource has already been released.
func (c *RemoteResourceClient) ReleaseResource(resourceID uint32) bool {
	var released bool
	var releaseFn func()

	c.server.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if c.released {
			return
		}

		res, ok := c.resources[resourceID]
		if !ok {
			return
		}

		delete(c.resources, resourceID)
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

	return released
}
