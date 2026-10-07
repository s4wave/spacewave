package resource_client

import (
	"context"
	"slices"
	"sync"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/resource"
)

type resourceLifetime struct {
	// ctx ends every ResourceRpc stream in this generation
	ctx context.Context
	// service opens ResourceRpc streams
	service resource.SRPCResourceServiceClient
	// enqueue sends lifecycle controls in generation order
	enqueue func(*resource.ResourceClientRequest) bool

	// mtx guards the lifetime state below
	mtx              sync.Mutex
	released         bool
	resources        map[uint32]*resourceRefSet
	srpcClients      map[uint32]srpc.Client
	resourceContexts map[uint32]context.CancelFunc
	controlIDCtr     uint32
	controlAcked     uint32
	controlWaiters   map[uint32]chan struct{}
}

type resourceRefSet struct {
	refs map[*resourceRef]struct{}
}

func newResourceLifetime(
	ctx context.Context,
	service resource.SRPCResourceServiceClient,
	enqueue func(*resource.ResourceClientRequest) bool,
) *resourceLifetime {
	return &resourceLifetime{
		ctx:              ctx,
		service:          service,
		enqueue:          enqueue,
		resources:        make(map[uint32]*resourceRefSet),
		srpcClients:      make(map[uint32]srpc.Client),
		resourceContexts: make(map[uint32]context.CancelFunc),
		controlWaiters:   make(map[uint32]chan struct{}),
	}
}

func (l *resourceLifetime) createReference(resourceID uint32) ResourceRef {
	// Return a pre-released reference when the lifetime already retired.
	l.mtx.Lock()
	defer l.mtx.Unlock()
	if l.released {
		return &resourceRef{
			lifetime:   l,
			resourceID: resourceID,
			released:   true,
		}
	}

	// Create the resource's first local reference set before publishing Adopt.
	set := l.resources[resourceID]
	first := set == nil
	if first {
		set = &resourceRefSet{refs: make(map[*resourceRef]struct{})}
		l.resources[resourceID] = set
	}
	ref := &resourceRef{lifetime: l, resourceID: resourceID}
	set.refs[ref] = struct{}{}
	if first {
		_ = l.enqueueControlLocked(&resource.ResourceClientRequest{Body: &resource.ResourceClientRequest_Adopt{
			Adopt: &resource.ResourceClientAdopt{ResourceId: resourceID},
		}})
	}
	return ref
}

func (l *resourceLifetime) releaseAll() {
	// Retire every local reference and ResourceRpc client as one transition.
	l.mtx.Lock()
	if l.released {
		l.mtx.Unlock()
		return
	}
	l.released = true
	ids := make([]uint32, 0, len(l.resources))
	for id, set := range l.resources {
		ids = append(ids, id)
		for ref := range set.refs {
			ref.released = true
		}
	}

	// Cancel each resource's call context and drop the cached clients.
	for _, cancel := range l.resourceContexts {
		cancel()
	}
	clear(l.resources)
	clear(l.srpcClients)
	clear(l.resourceContexts)
	l.mtx.Unlock()

	// Enqueue one Release control per retired resource id.
	// Preserve deterministic control order when the generation closes.
	slices.Sort(ids)
	for _, id := range ids {
		_ = l.enqueueControl(&resource.ResourceClientRequest{Body: &resource.ResourceClientRequest_Release{
			Release: &resource.ResourceClientRelease{ResourceId: id},
		}})
	}
}

func (l *resourceLifetime) releaseFromServer(resourceID uint32) {
	// Retire every local reference to a server-released resource.
	l.mtx.Lock()
	defer l.mtx.Unlock()
	set := l.resources[resourceID]
	if set == nil {
		return
	}
	for ref := range set.refs {
		ref.released = true
	}
	l.clearResourceLocked(resourceID)
}

func (l *resourceLifetime) releaseRef(ref *resourceRef) {
	// Drop one local reference and release the resource with its last one.
	l.mtx.Lock()
	defer l.mtx.Unlock()
	if ref.released {
		return
	}
	ref.released = true
	id, notify := l.releaseRefLocked(ref)
	if notify {
		_ = l.enqueueControlLocked(&resource.ResourceClientRequest{Body: &resource.ResourceClientRequest_Release{
			Release: &resource.ResourceClientRelease{ResourceId: id},
		}})
	}
}

func (l *resourceLifetime) clientForRef(ref *resourceRef) (srpc.Client, error) {
	l.mtx.Lock()
	defer l.mtx.Unlock()
	if ref.released {
		return nil, resource.ErrResourceOrClientReleased
	}
	return l.getOrCreateClientLocked(ref.resourceID)
}

func (l *resourceLifetime) releaseRefLocked(ref *resourceRef) (uint32, bool) {
	// Remove the reference and release the resource when its set empties.
	id := ref.resourceID
	set := l.resources[id]
	if set == nil {
		return 0, false
	}
	delete(set.refs, ref)
	if len(set.refs) != 0 {
		return 0, false
	}
	l.clearResourceLocked(id)
	return id, true
}

func (l *resourceLifetime) clearResourceLocked(id uint32) {
	if cancel := l.resourceContexts[id]; cancel != nil {
		cancel()
		delete(l.resourceContexts, id)
	}
	delete(l.resources, id)
	delete(l.srpcClients, id)
}

func (l *resourceLifetime) getOrCreateClientLocked(resourceID uint32) (srpc.Client, error) {
	// Reuse the resource's cached ResourceRpc client.
	if client := l.srpcClients[resourceID]; client != nil {
		return client, nil
	}

	// Reject an unknown resource id.
	if l.resources[resourceID] == nil {
		return nil, resource.ErrResourceNotFound
	}

	// Build the ResourceRpc client bound to a per-resource context.
	resourceCtx, cancel := context.WithCancel(l.ctx)
	l.resourceContexts[resourceID] = cancel
	client := resource.NewResourceRpcClient(func(ctx context.Context) (resource.SRPCResourceService_ResourceRpcClient, error) {
		callCtx, releaseCallCtx := resourceRPCCallContext(resourceCtx, ctx)
		strm, err := l.service.ResourceRpc(callCtx)
		if err != nil {
			releaseCallCtx()
			return nil, err
		}
		return &resourceRPCClientWithContext{
			SRPCResourceService_ResourceRpcClient: strm,
			ctx:                                   callCtx,
			release:                               releaseCallCtx,
		}, nil
	}, resourceID)

	// Cache the ordered client wrapper for later calls.
	client = &orderedResourceClient{lifetime: l, resourceID: resourceID, client: client}
	l.srpcClients[resourceID] = client
	return client, nil
}

func (l *resourceLifetime) enqueueControl(req *resource.ResourceClientRequest) bool {
	// Assign the control's sequence id under the lock.
	l.mtx.Lock()
	defer l.mtx.Unlock()

	return l.enqueueControlLocked(req)
}

func (l *resourceLifetime) enqueueControlLocked(req *resource.ResourceClientRequest) bool {
	l.controlIDCtr++
	req.ControlId = l.controlIDCtr
	return l.enqueue(req)
}

func (l *resourceLifetime) acknowledgeControl(controlID uint32) error {
	// Accept only the next in-order control acknowledgment.
	l.mtx.Lock()
	defer l.mtx.Unlock()
	if controlID == 0 || controlID != l.controlAcked+1 || controlID > l.controlIDCtr {
		return errors.Errorf("unexpected ResourceClient control acknowledgment %d after %d", controlID, l.controlAcked)
	}
	l.controlAcked = controlID

	// Wake every waiter whose controls are now acknowledged.
	for target, waiter := range l.controlWaiters {
		if target <= controlID {
			close(waiter)
			delete(l.controlWaiters, target)
		}
	}
	return nil
}

func (l *resourceLifetime) waitForControls(ctx context.Context) error {
	// Return immediately when every queued control is already acknowledged.
	l.mtx.Lock()
	target := l.controlIDCtr
	if target <= l.controlAcked {
		l.mtx.Unlock()
		return nil
	}
	waiter := l.controlWaiters[target]
	if waiter == nil {
		waiter = make(chan struct{})
		l.controlWaiters[target] = waiter
	}
	l.mtx.Unlock()

	// Wait for the target control's acknowledgment or context cancellation.
	select {
	case <-waiter:
		return nil
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-l.ctx.Done():
		return context.Cause(l.ctx)
	}
}

// orderedResourceClient orders calls after prior lifecycle controls and
// retires the resource's references when the server refuses its route.
type orderedResourceClient struct {
	// lifetime owns the references to resourceID.
	lifetime *resourceLifetime
	// resourceID identifies the routed resource.
	resourceID uint32
	// client opens ResourceRpc routes to resourceID.
	client srpc.Client
}

// ExecCall invokes a unary method after every queued lifecycle control.
func (c *orderedResourceClient) ExecCall(ctx context.Context, service, method string, in, out srpc.Message) error {
	if err := c.lifetime.waitForControls(ctx); err != nil {
		return err
	}
	return c.retireRefused(c.client.ExecCall(ctx, service, method, in, out))
}

// NewStream opens a streaming method after every queued lifecycle control.
func (c *orderedResourceClient) NewStream(ctx context.Context, service, method string, firstMsg srpc.Message) (srpc.Stream, error) {
	if err := c.lifetime.waitForControls(ctx); err != nil {
		return nil, err
	}
	strm, err := c.client.NewStream(ctx, service, method, firstMsg)
	return strm, c.retireRefused(err)
}

// retireRefused releases every local reference when the route acknowledgement
// reports that the server no longer holds the resource. Handler errors reach
// the caller as diagnostic text and leave the references alive.
func (c *orderedResourceClient) retireRefused(err error) error {
	if errors.Is(err, resource.ErrResourceNotFound) ||
		errors.Is(err, resource.ErrInvalidResourceID) ||
		errors.Is(err, resource.ErrResourceOrClientReleased) {
		c.lifetime.releaseFromServer(c.resourceID)
	}
	return err
}

func resourceRPCCallContext(resourceCtx, callCtx context.Context) (context.Context, func()) {
	if callCtx == nil {
		callCtx = context.Background()
	}
	ctx, cancel := context.WithCancel(callCtx)
	stop := context.AfterFunc(resourceCtx, cancel)
	return ctx, func() {
		stop()
		cancel()
	}
}

type resourceRPCClientWithContext struct {
	resource.SRPCResourceService_ResourceRpcClient
	ctx     context.Context
	release func()
}

func (c *resourceRPCClientWithContext) Context() context.Context { return c.ctx }
func (c *resourceRPCClientWithContext) Close() error {
	err := c.SRPCResourceService_ResourceRpcClient.Close()
	c.release()
	return err
}

type resourceRef struct {
	lifetime   *resourceLifetime
	resourceID uint32
	released   bool
}

func (r *resourceRef) GetResourceID() uint32           { return r.resourceID }
func (r *resourceRef) GetClient() (srpc.Client, error) { return r.lifetime.clientForRef(r) }
func (r *resourceRef) Release()                        { r.lifetime.releaseRef(r) }

// _ is a type assertion
var _ ResourceRef = (*resourceRef)(nil)
