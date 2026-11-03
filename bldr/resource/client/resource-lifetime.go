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
		ref := &resourceRef{lifetime: l, resourceID: resourceID, done: make(chan struct{})}
		ref.retireLocked()
		return ref
	}

	// Create the resource's first local reference set before publishing Adopt.
	set := l.resources[resourceID]
	first := set == nil
	if first {
		set = &resourceRefSet{refs: make(map[*resourceRef]struct{})}
		l.resources[resourceID] = set
	}
	ref := &resourceRef{lifetime: l, resourceID: resourceID, done: make(chan struct{})}
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
			ref.retireLocked()
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

// releaseFromServer retires every local reference to a resource the server
// released with a ResourceReleased message, marking each one released by
// the server.
func (l *resourceLifetime) releaseFromServer(resourceID uint32) {
	l.retireResource(resourceID, true)
}

// retireResource retires every local reference to resourceID. byServer marks
// the references released by the server; a refused route leaves it false,
// since a server that is shutting down refuses routes too.
func (l *resourceLifetime) retireResource(resourceID uint32, byServer bool) {
	// Retire all references while their lifetime is locked.
	l.mtx.Lock()
	defer l.mtx.Unlock()
	set := l.resources[resourceID]
	if set == nil {
		return
	}
	for ref := range set.refs {
		if !ref.released {
			ref.serverReleased = byServer
		}
		ref.retireLocked()
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
	ref.retireLocked()
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

	// Bind streams to a per-resource context, which a release cancels, and
	// unary calls to the generation, so a call the server already answered
	// still reads its answer when the release overtakes it.
	resourceCtx, cancel := context.WithCancel(l.ctx)
	l.resourceContexts[resourceID] = cancel
	client := &orderedResourceClient{
		lifetime:   l,
		resourceID: resourceID,
		calls:      l.newResourceRPCClient(l.ctx, resourceID),
		streams:    l.newResourceRPCClient(resourceCtx, resourceID),
	}

	// Cache the ordered client wrapper for later calls.
	l.srpcClients[resourceID] = client
	return client, nil
}

// newResourceRPCClient opens ResourceRpc routes to resourceID whose calls end
// with scope.
func (l *resourceLifetime) newResourceRPCClient(scope context.Context, resourceID uint32) srpc.Client {
	return resource.NewResourceRpcClient(func(ctx context.Context) (resource.SRPCResourceService_ResourceRpcClient, error) {
		callCtx, releaseCallCtx := resourceRPCCallContext(scope, ctx)
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
	// calls opens unary routes to resourceID, ending with the generation.
	calls srpc.Client
	// streams opens streaming routes to resourceID, ending with its release.
	streams srpc.Client
}

// ExecCall invokes a unary method after every queued lifecycle control.
func (c *orderedResourceClient) ExecCall(ctx context.Context, service, method string, in, out srpc.Message) error {
	if err := c.lifetime.waitForControls(ctx); err != nil {
		return err
	}
	return c.retireRefused(c.calls.ExecCall(ctx, service, method, in, out))
}

// NewStream opens a streaming method after every queued lifecycle control.
func (c *orderedResourceClient) NewStream(ctx context.Context, service, method string, firstMsg srpc.Message) (srpc.Stream, error) {
	if err := c.lifetime.waitForControls(ctx); err != nil {
		return nil, err
	}
	strm, err := c.streams.NewStream(ctx, service, method, firstMsg)
	return strm, c.retireRefused(err)
}

// retireRefused releases every local reference when the route acknowledgement
// reports that the server no longer holds the resource. Handler errors reach
// the caller as diagnostic text and leave the references alive.
func (c *orderedResourceClient) retireRefused(err error) error {
	if errors.Is(err, resource.ErrResourceNotFound) ||
		errors.Is(err, resource.ErrInvalidResourceID) ||
		errors.Is(err, resource.ErrResourceOrClientReleased) {
		c.lifetime.retireResource(c.resourceID, false)
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

// Context returns the call context bound to the resource lifetime.
func (c *resourceRPCClientWithContext) Context() context.Context { return c.ctx }

// Close closes the stream client and releases the lifetime binding.
func (c *resourceRPCClientWithContext) Close() error {
	err := c.SRPCResourceService_ResourceRpcClient.Close()
	c.release()
	return err
}

// resourceRef is one local reference to a remote resource. The lifetime's
// mutex guards released and serverReleased.
type resourceRef struct {
	lifetime   *resourceLifetime
	resourceID uint32
	released   bool
	// serverReleased is set when a ResourceReleased message retired the
	// reference, and never changes after released becomes true.
	serverReleased bool
	// done closes when released becomes true.
	done chan struct{}
}

// GetResourceID returns the resource ID the reference holds.
func (r *resourceRef) GetResourceID() uint32 { return r.resourceID }

// GetClient returns the shared client for the resource.
func (r *resourceRef) GetClient() (srpc.Client, error) { return r.lifetime.clientForRef(r) }

// Release drops this reference to the resource.
func (r *resourceRef) Release() { r.lifetime.releaseRef(r) }

// Done returns a channel that closes when the reference is released.
func (r *resourceRef) Done() <-chan struct{} { return r.done }

// ReleasedByServer reports whether the server released the resource.
func (r *resourceRef) ReleasedByServer() bool {
	r.lifetime.mtx.Lock()
	defer r.lifetime.mtx.Unlock()
	return r.serverReleased
}

// retireLocked marks the reference released and closes done once.
func (r *resourceRef) retireLocked() {
	if !r.released {
		r.released = true
		close(r.done)
	}
}

// _ is a type assertion
var _ ResourceRef = (*resourceRef)(nil)
