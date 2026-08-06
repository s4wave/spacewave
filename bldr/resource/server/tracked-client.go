package resource_server

import (
	"context"
	"slices"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/resource"
)

// RemoteResourceClient tracks one immutable ResourceClient generation.
type RemoteResourceClient struct {
	server         *ResourceServer
	clientID       uint32
	rootResourceID uint32
	// ctx ends when the generation retires
	ctx context.Context

	// server.bcast guards the lifecycle state below
	txQueue       []*resource.ResourceClientResponse
	lastControlID uint32
	released      bool
	resources     map[uint32]*trackedResource
	children      map[uint32]map[uint32]struct{}
	// tombstones retain every released ID until this immutable generation ends
	// so a late Adopt cannot revive or miss the matching release notification.
	tombstones        map[uint32]struct{}
	attachedResources map[uint32]*attachedResource
}

// Context returns the generation lifecycle context.
func (c *RemoteResourceClient) Context() context.Context { return c.ctx }

func (c *RemoteResourceClient) applyControl(req *resource.ResourceClientRequest) (uint32, error) {
	if req == nil {
		return 0, errors.New("nil ResourceClient control")
	}
	controlID := req.GetControlId()
	if controlID == 0 || controlID != c.lastControlID+1 {
		return 0, errors.Errorf("unexpected ResourceClient control ID %d after %d", controlID, c.lastControlID)
	}
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
	c.lastControlID = controlID
	return controlID, nil
}

func (c *RemoteResourceClient) addInvocationResource(
	parentID uint32,
	serviceID string,
	methodID string,
	mux srpc.Invoker,
	value any,
	releaseFn func(),
) (uint32, error) {
	return c.addResource(parentID, serviceID, methodID, mux, value, releaseFn, true)
}

func (c *RemoteResourceClient) addResource(
	parentID uint32,
	serviceID string,
	methodID string,
	mux srpc.Invoker,
	value any,
	releaseFn func(),
	pending bool,
) (uint32, error) {
	var id uint32
	var rejected bool
	c.server.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if c.released {
			rejected = true
			return
		}
		c.server.resourceIDCtr++
		id = c.server.resourceIDCtr
		c.resources[id] = &trackedResource{
			mux:              mux,
			value:            value,
			ownerClientID:    c.clientID,
			releaseFn:        releaseFn,
			parentResourceID: parentID,
			serviceID:        serviceID,
			methodID:         methodID,
			createdAt:        c.server.now(),
			pending:          pending,
		}
		if parentID != 0 {
			if c.children[parentID] == nil {
				c.children[parentID] = make(map[uint32]struct{})
			}
			c.children[parentID][id] = struct{}{}
		}
		broadcast()
	})
	if rejected {
		return 0, resource.ErrClientReleased
	}
	return id, nil
}

func (c *RemoteResourceClient) adoptResource(resourceID uint32) bool {
	var ok bool
	c.server.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if c.released {
			return
		}
		if _, exists := c.resources[resourceID]; exists {
			c.resources[resourceID].pending = false
			broadcast()
			ok = true
			return
		}
		if _, attached := c.attachedResources[resourceID]; attached {
			ok = true
			return
		}
		if _, tombstone := c.tombstones[resourceID]; tombstone {
			ok = true
			c.queueReleasedLocked(resourceID, broadcast)
		}
	})
	return ok
}

func (c *RemoteResourceClient) queueReleasedLocked(id uint32, broadcast func()) {
	c.txQueue = append(c.txQueue, &resource.ResourceClientResponse{Body: &resource.ResourceClientResponse_ResourceReleased{
		ResourceReleased: &resource.ResourceReleasedResponse{ResourceId: id},
	}})
	broadcast()
}

func (c *RemoteResourceClient) queueControlAck(controlID uint32) {
	c.server.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		c.txQueue = append(c.txQueue, &resource.ResourceClientResponse{Body: &resource.ResourceClientResponse_ControlAck{
			ControlAck: &resource.ResourceClientControlAck{ControlId: controlID},
		}})
		broadcast()
	})
}

// releaseLocked removes one resource after recursively removing pending
// descendants. It appends callbacks and notifications while under the server
// lock; callbacks are always invoked by the caller after unlocking.
func (c *RemoteResourceClient) releaseLocked(id uint32, notify bool, keepRoot bool, releaseFns *[]func(), releasedIDs *[]uint32) bool {
	res := c.resources[id]
	if res == nil {
		return false
	}
	if keepRoot && id == c.rootResourceID {
		c.releasePendingChildrenLocked(id, releaseFns, releasedIDs)
		return true
	}
	c.releasePendingChildrenLocked(id, releaseFns, releasedIDs)
	delete(c.resources, id)
	if kids := c.children[res.parentResourceID]; kids != nil {
		delete(kids, id)
		if len(kids) == 0 {
			delete(c.children, res.parentResourceID)
		}
	}
	delete(c.children, id)
	c.tombstones[id] = struct{}{}
	if res.releaseFn != nil {
		*releaseFns = append(*releaseFns, res.releaseFn)
	}
	if notify && releasedIDs != nil {
		*releasedIDs = append(*releasedIDs, id)
	}
	return true
}

func (c *RemoteResourceClient) releasePendingChildrenLocked(parentID uint32, releaseFns *[]func(), releasedIDs *[]uint32) {
	kids := c.children[parentID]
	if len(kids) == 0 {
		return
	}
	ids := make([]uint32, 0, len(kids))
	for id := range kids {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		child := c.resources[id]
		if child == nil || !child.pending {
			continue
		}
		c.releaseLocked(id, true, false, releaseFns, releasedIDs)
	}
}

// releaseAllChildrenLocked is used only when a client generation ends. Unlike
// parent-controlled Release, disconnect cleanup releases adopted descendants
// too, in deterministic child-first order.
func (c *RemoteResourceClient) releaseAllChildrenLocked(id uint32, releaseFns *[]func()) {
	kids := c.children[id]
	ids := make([]uint32, 0, len(kids))
	for childID := range kids {
		ids = append(ids, childID)
	}
	slices.Sort(ids)
	for _, childID := range ids {
		c.releaseAllChildrenLocked(childID, releaseFns)
	}
	c.releaseLocked(id, false, false, releaseFns, nil)
}

func (c *RemoteResourceClient) finishRelease(releasedIDs []uint32, broadcast func()) {
	for _, id := range releasedIDs {
		c.queueReleasedLocked(id, broadcast)
	}
}

func (c *RemoteResourceClient) addServerResource(mux srpc.Invoker, value any, releaseFn func()) (uint32, error) {
	return c.addResource(0, "", "", mux, value, releaseFn, false)
}

// AddResource adds a server-created resource to this generation.
func (c *RemoteResourceClient) AddResource(mux srpc.Invoker, releaseFn func()) (uint32, error) {
	return c.AddResourceValue(mux, nil, releaseFn)
}

// AddResourceValue adds a server-created resource with an in-process value.
func (c *RemoteResourceClient) AddResourceValue(mux srpc.Invoker, value any, releaseFn func()) (uint32, error) {
	return c.addServerResource(mux, value, releaseFn)
}

// GetResourceValue returns an in-process resource value.
func (c *RemoteResourceClient) GetResourceValue(resourceID uint32) (any, error) {
	var value any
	c.server.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if res := c.resources[resourceID]; res != nil {
			value = res.value
		}
	})
	if value == nil {
		return nil, resource.ErrResourceNotFound
	}
	return value, nil
}

// ReleaseResource is server initiated and therefore notifies the client of
// the target as well as pending descendants.
func (c *RemoteResourceClient) ReleaseResource(resourceID uint32) bool {
	// Release the server resource tree and queue descendant notifications.
	var releaseFns []func()
	var releasedIDs []uint32
	var found bool
	c.server.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		found = c.releaseLocked(resourceID, true, true, &releaseFns, &releasedIDs)
		if found {
			c.finishRelease(releasedIDs, broadcast)
		}
	})

	// Handle client-published resources through their attachment lifetime.
	if !found {
		var attached bool
		c.server.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			attached = c.attachedResources[resourceID] != nil
		})
		if attached {
			c.removeAttachedResource(resourceID, true)
			c.server.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
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

// releaseClientControl handles a client-owned target release and reports only
// descendants released by the server.
func (c *RemoteResourceClient) releaseClientControl(resourceID uint32) (bool, error) {
	// Resolve the target and release its pending descendants under the lock.
	var releaseFns []func()
	var releasedIDs []uint32
	var found bool
	var attached bool
	var err error
	c.server.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if c.released {
			err = resource.ErrClientReleased
			return
		}
		if _, exists := c.resources[resourceID]; !exists {
			if _, tombstone := c.tombstones[resourceID]; tombstone {
				return
			}
			if c.attachedResources[resourceID] != nil {
				attached = true
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
	c.server.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
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

func (c *RemoteResourceClient) removeAttachedResource(id uint32, notify bool) {
	var cancel context.CancelFunc
	var releaseFn func()
	c.server.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		ar := c.attachedResources[id]
		if ar == nil {
			return
		}
		delete(c.attachedResources, id)
		c.tombstones[id] = struct{}{}
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
	c.server.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if ar := c.attachedResources[id]; ar != nil {
			client = ar.srpcClient
		}
	})
	if client == nil {
		return nil, resource.ErrResourceNotFound
	}
	return client, nil
}

func (c *RemoteResourceClient) releaseAllAttachedResources() {
	var cancels []context.CancelFunc
	c.server.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		for _, ar := range c.attachedResources {
			cancels = append(cancels, ar.cancel)
		}
		clear(c.attachedResources)
		broadcast()
	})
	for _, cancel := range cancels {
		cancel()
	}
}
