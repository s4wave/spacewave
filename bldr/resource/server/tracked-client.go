package resource_server

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/resource"
	"github.com/tidwall/btree"
)

// RemoteResourceClient tracks one immutable ResourceClient generation.
type RemoteResourceClient struct {
	// server owns the admission and routing indexes.
	server *ResourceServer
	// bcast guards this generation; no server lock is held while acquiring it.
	bcast broadcast.Broadcast
	// clientID identifies this immutable generation.
	clientID uint32
	// rootResourceID identifies the root retained until disconnect.
	rootResourceID uint32
	// ctx ends when the generation retires.
	ctx context.Context

	// txQueue holds outbound notifications and acknowledgements under bcast.
	txQueue []*resource.ResourceClientResponse
	// lastControlID tracks applied FIFO controls on the control receiver.
	lastControlID uint32
	// released rejects further registrations under bcast.
	released bool
	// resources retains owned handles under bcast.
	resources map[uint32]*trackedResource
	// children links parent resource IDs to descendants under bcast.
	children map[uint32]*btree.Map[uint32, struct{}]
	// attachedResources retains client-published handles under bcast.
	attachedResources map[uint32]*attachedResource
}

// Context returns the generation lifecycle context.
func (c *RemoteResourceClient) Context() context.Context { return c.ctx }

// applyControl applies the next generation control and returns its sequence ID.
func (c *RemoteResourceClient) applyControl(req *resource.ResourceClientRequest) (uint32, error) {
	// Reject nil controls and out-of-order control IDs.
	if req == nil {
		return 0, errors.New("nil ResourceClient control")
	}
	controlID := req.GetControlId()
	if controlID == 0 || controlID != c.lastControlID+1 {
		return 0, errors.Errorf("unexpected ResourceClient control ID %d after %d", controlID, c.lastControlID)
	}

	// Apply the Adopt or Release control to the tracked resources.
	switch body := req.GetBody().(type) {
	case *resource.ResourceClientRequest_Adopt:
		if body.Adopt == nil {
			return 0, errors.New("empty adopt control")
		}
		if !c.adoptResource(body.Adopt.ResourceId) {
			return 0, resource.ErrResourceNotFound
		}
	case *resource.ResourceClientRequest_Release:
		if body.Release == nil {
			return 0, errors.New("empty release control")
		}
		if _, err := c.releaseClientControl(body.Release.ResourceId); err != nil {
			return 0, err
		}
	default:
		return 0, errors.New("unexpected ResourceClient init/control packet")
	}

	// Record the control ID as applied and acknowledge it.
	c.lastControlID = controlID
	return controlID, nil
}

// addResource registers a generation resource and its optional publishing invocation.
func (c *RemoteResourceClient) addResource(
	parentID uint32,
	mux srpc.Invoker,
	value any,
	releaseFn func(),
	invocation *resourceRPCContext,
) (uint32, error) {
	// Allocate the resource ID and register it under the generation lock.
	var id uint32
	var err error
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Reject registration once the client generation is released.
		if c.released {
			err = resource.ErrClientReleased
			return
		}

		// Reject registrations retained beyond their publishing handler.
		if invocation != nil && invocation.ended {
			err = context.Canceled
			return
		}

		// Reject a child whose parent was released while the invocation ran.
		// A new child would refer to native state whose cleanup already ran.
		if parentID != 0 && c.resources[parentID] == nil {
			err = resource.ErrResourceNotFound
			return
		}

		// Allocate and index one handle under the bounded server lock.
		c.server.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			// Publish the allocated ID and live-handle count together.
			c.server.resourceIDCtr++
			id = c.server.resourceIDCtr
			c.server.routes[id] = c
			c.server.trackedCount++
			broadcast()
		})

		// Retain the handle and its publication scope under the generation lock.
		c.resources[id] = &trackedResource{
			mux:              mux,
			value:            value,
			ownerClientID:    c.clientID,
			releaseFn:        releaseFn,
			parentResourceID: parentID,
			offer:            invocation,
		}
		if invocation != nil {
			if invocation.unpublished == nil {
				invocation.unpublished = make(map[uint32]struct{})
			}
			invocation.unpublished[id] = struct{}{}
		}

		// Keep provenance solely for child-first generation teardown.
		if parentID != 0 {
			if c.children[parentID] == nil {
				c.children[parentID] = btree.NewMap[uint32, struct{}](0)
			}
			c.children[parentID].Set(id, struct{}{})
		}
		broadcast()
	})
	if err != nil {
		return 0, err
	}
	return id, nil
}

// adoptResource retains a pending resource or acknowledges its release.
func (c *RemoteResourceClient) adoptResource(resourceID uint32) bool {
	var ok bool
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if c.released {
			return
		}
		if _, exists := c.resources[resourceID]; exists {
			res := c.resources[resourceID]
			res.acceptOffer(resourceID)
			broadcast()
			ok = true
			return
		}
		if _, attached := c.attachedResources[resourceID]; attached {
			ok = true
			return
		}
		if c.releasedLocked(resourceID) {
			ok = true
			c.queueReleasedLocked(resourceID, broadcast)
		}
	})
	return ok
}

// releasedLocked reports whether id identifies a resource of this generation
// that was released, so a late Adopt or Release gets the release notification
// instead of an error. One server-wide counter allocates every resource ID
// after the generation's root, and each resource is tracked or attached until
// its release, so an ID in that range held by neither resources nor
// attachedResources was released. An ID of another generation in the range
// reads as released too.
// Treating that ID as released reveals and grants nothing.
func (c *RemoteResourceClient) releasedLocked(id uint32) bool {
	var allocated bool
	c.server.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) { allocated = id <= c.server.resourceIDCtr })
	return id >= c.rootResourceID && allocated && c.resources[id] == nil && c.attachedResources[id] == nil
}

// queueReleasedLocked queues a release notification while holding bcast.
func (c *RemoteResourceClient) queueReleasedLocked(id uint32, broadcast func()) {
	c.txQueue = append(c.txQueue, &resource.ResourceClientResponse{Body: &resource.ResourceClientResponse_ResourceReleased{
		ResourceReleased: &resource.ResourceReleasedResponse{ResourceId: id},
	}})
	broadcast()
}

// queueControlAck acknowledges one applied control to the generation.
func (c *RemoteResourceClient) queueControlAck(controlID uint32) {
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		c.txQueue = append(c.txQueue, &resource.ResourceClientResponse{Body: &resource.ResourceClientResponse_ControlAck{
			ControlAck: &resource.ResourceClientControlAck{ControlId: controlID},
		}})
		broadcast()
	})
}

// releaseLocked removes one resource without revoking its invocation offers.
// It appends callbacks and notifications under the generation lock, or while
// exclusively draining detached maps. Callbacks always run after unlocking.
func (c *RemoteResourceClient) releaseLocked(id uint32, notify bool, keepRoot bool, releaseFns *[]func(), releasedIDs *[]uint32) bool {
	// Look up the resource and reject unknown IDs.
	res := c.resources[id]
	if res == nil {
		return false
	}

	// Keep the root retained until generation teardown.
	if keepRoot && id == c.rootResourceID {
		return true
	}

	// A live handle must never be freed under its holder: an invocation owns
	// its offers independently of this parent, until adoption or abandonment.
	if !c.released {
		res.acceptOffer(id)
	}
	delete(c.resources, id)
	count := 1
	if c.released {
		count = 0
	}
	c.server.removeRoute(id, count)

	// Unlink the handle from the ordered child indexes.
	if kids := c.children[res.parentResourceID]; kids != nil {
		kids.Delete(id)
		if kids.Len() == 0 {
			delete(c.children, res.parentResourceID)
		}
	}
	delete(c.children, id)

	// Queue the release callback and the released notification.
	if res.releaseFn != nil {
		*releaseFns = append(*releaseFns, res.releaseFn)
	}
	if notify && releasedIDs != nil {
		*releasedIDs = append(*releasedIDs, id)
	}
	return true
}

// releaseAllChildrenLocked releases an exclusively owned tree child-first.
// Generation teardown calls it on detached maps without holding any lifecycle lock.
func (c *RemoteResourceClient) releaseAllChildrenLocked(id uint32, releaseFns *[]func()) {
	// Consume each child in ID order before releasing its parent.
	kids := c.children[id]
	if kids != nil {
		for kids.Len() != 0 {
			childID, _, _ := kids.Min()
			c.releaseAllChildrenLocked(childID, releaseFns)
		}
	}
	c.releaseLocked(id, false, false, releaseFns, nil)
}

// finishRelease queues resource-tree notifications while holding bcast.
func (c *RemoteResourceClient) finishRelease(releasedIDs []uint32, broadcast func()) {
	for _, id := range releasedIDs {
		c.queueReleasedLocked(id, broadcast)
	}
}

// AddResource adds a server-created resource to this generation.
func (c *RemoteResourceClient) AddResource(mux srpc.Invoker, releaseFn func()) (uint32, error) {
	return c.AddResourceValue(mux, nil, releaseFn)
}

// AddResourceValue adds a server-created resource with an in-process value.
func (c *RemoteResourceClient) AddResourceValue(mux srpc.Invoker, value any, releaseFn func()) (uint32, error) {
	return c.addResource(0, mux, value, releaseFn, nil)
}

// GetResourceValue returns an in-process resource value.
func (c *RemoteResourceClient) GetResourceValue(resourceID uint32) (any, error) {
	var value any
	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if c.released {
			return
		}
		if res := c.resources[resourceID]; res != nil {
			value = res.value
		}
	})
	if value == nil {
		return nil, resource.ErrResourceNotFound
	}
	return value, nil
}

// ReleaseResource revokes the target and notifies its holders.
func (c *RemoteResourceClient) ReleaseResource(resourceID uint32) bool {
	// Revoke only the target; invocation offers have their own owner.
	var releaseFns []func()
	var releasedIDs []uint32
	var found bool
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if c.released {
			return
		}
		found = c.releaseLocked(resourceID, true, true, &releaseFns, &releasedIDs)
		if found {
			c.finishRelease(releasedIDs, broadcast)
		}
	})

	// Handle client-published resources through their attachment lifetime.
	if !found {
		var attached bool
		c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			attached = c.attachedResources[resourceID] != nil
		})
		if attached {
			c.removeAttachedResource(resourceID, true)
			c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				c.queueReleasedLocked(resourceID, broadcast)
			})
			return true
		}
	}

	// Run resource callbacks after the lifecycle state transition.
	for _, releaseFn := range releaseFns {
		if releaseFn != nil {
			releaseFn()
		}
	}
	return found
}

// releaseClientControl releases a client-owned target before acknowledging cleanup.
func (c *RemoteResourceClient) releaseClientControl(resourceID uint32) (bool, error) {
	// Resolve and remove the target under the generation lock.
	var releaseFns []func()
	var releasedIDs []uint32
	var found bool
	var attached bool
	var err error
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if c.released {
			err = resource.ErrClientReleased
			return
		}
		if _, exists := c.resources[resourceID]; !exists {
			if c.attachedResources[resourceID] != nil {
				attached = true
				return
			}
			if c.releasedLocked(resourceID) {
				return
			}
			err = resource.ErrResourceNotFound
			return
		}
		found = c.releaseLocked(resourceID, false, true, &releaseFns, &releasedIDs)
		if found {
			c.finishRelease(releasedIDs, broadcast)
			broadcast()
		}
	})

	// Delegate attached targets to their transport cleanup.
	if attached {
		c.removeAttachedResource(resourceID, true)
		return true, nil
	}

	// Run resource callbacks after the lifecycle state transition.
	for _, releaseFn := range releaseFns {
		if releaseFn != nil {
			releaseFn()
		}
	}
	return found, err
}

// AddAttachedResource registers a client-published resource with this generation.
func (c *RemoteResourceClient) AddAttachedResource(
	id uint32,
	label string,
	cancel context.CancelFunc,
	srpcClient srpc.Client,
	releaseFn func(),
) error {
	var released bool
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Refuse publication after ownership has transferred to the drain.
		if c.released {
			released = true
			return
		}
		if c.attachedResources == nil {
			c.attachedResources = make(map[uint32]*attachedResource)
		}

		// Publish one attachment route and count before admitting lookups.
		c.server.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			if c.attachedResources[id] == nil {
				c.server.trackedCount++
			}
			c.server.routes[id] = c
			broadcast()
		})
		c.attachedResources[id] = &attachedResource{
			label:      label,
			cancel:     cancel,
			releaseFn:  releaseFn,
			srpcClient: srpcClient,
		}
		broadcast()
	})
	if released {
		return resource.ErrClientReleased
	}
	return nil
}

// RemoveAttachedResource removes a client-published resource.
func (c *RemoteResourceClient) RemoveAttachedResource(id uint32) {
	c.removeAttachedResource(id, true)
}

// removeAttachedResource removes a published handle and ends its transport context.
func (c *RemoteResourceClient) removeAttachedResource(id uint32, notify bool) {
	// Remove the attached resource from the generation under the generation lock.
	var cancel context.CancelFunc
	var releaseFn func()
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Skip resources that are not attached.
		ar := c.attachedResources[id]
		if ar == nil {
			return
		}
		delete(c.attachedResources, id)
		c.server.removeRoute(id, 1)
		cancel = ar.cancel
		if notify {
			releaseFn = ar.releaseFn
		}
		broadcast()
	})
	if releaseFn != nil {
		releaseFn()
	}
	if cancel != nil {
		cancel()
	}
}

// GetAttachedResource returns a client-published ResourceRpc client.
func (c *RemoteResourceClient) GetAttachedResource(id uint32) (srpc.Client, error) {
	var client srpc.Client
	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if ar := c.attachedResources[id]; ar != nil {
			client = ar.srpcClient
		}
	})
	if client == nil {
		return nil, resource.ErrResourceNotFound
	}
	return client, nil
}
