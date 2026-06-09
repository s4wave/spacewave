package resource_client

import (
	"context"
	"strconv"
	"sync"

	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/bldr/resource"
)

type resourceLifetime struct {
	ctx            context.Context
	service        resource.SRPCResourceServiceClient
	clientHandleID uint32

	// mtx guards below fields
	mtx sync.Mutex
	// released indicates the owning client is closed.
	released bool
	// resources tracks all references to each resource ID.
	resources map[uint32]*resourceRefSet
	// srpcClients holds lazy-created SRPC clients per resource.
	srpcClients map[uint32]srpc.Client
	// resourceContexts holds per-resource context cancellation.
	resourceContexts map[uint32]context.CancelFunc
}

type resourceRefSet struct {
	// refs contains all active references.
	refs map[*resourceRef]struct{}
}

func newResourceLifetime(
	ctx context.Context,
	service resource.SRPCResourceServiceClient,
	clientHandleID uint32,
) *resourceLifetime {
	return &resourceLifetime{
		ctx:              ctx,
		service:          service,
		clientHandleID:   clientHandleID,
		resources:        make(map[uint32]*resourceRefSet),
		srpcClients:      make(map[uint32]srpc.Client),
		resourceContexts: make(map[uint32]context.CancelFunc),
	}
}

func (l *resourceLifetime) createReference(resourceID uint32) ResourceRef {
	l.mtx.Lock()
	defer l.mtx.Unlock()

	if l.released {
		return &resourceRef{
			lifetime:   l,
			resourceID: resourceID,
			released:   true,
		}
	}

	refSet := l.resources[resourceID]
	if refSet == nil {
		refSet = &resourceRefSet{
			refs: make(map[*resourceRef]struct{}),
		}
		l.resources[resourceID] = refSet
	}

	ref := &resourceRef{
		lifetime:   l,
		resourceID: resourceID,
	}
	refSet.refs[ref] = struct{}{}
	return ref
}

func (l *resourceLifetime) releaseAll() {
	l.mtx.Lock()
	defer l.mtx.Unlock()

	l.released = true
	for _, refSet := range l.resources {
		for ref := range refSet.refs {
			ref.released = true
		}
	}
	for _, cancel := range l.resourceContexts {
		cancel()
	}

	clear(l.resources)
	clear(l.srpcClients)
	clear(l.resourceContexts)
}

func (l *resourceLifetime) releaseFromServer(resourceID uint32) {
	l.mtx.Lock()
	defer l.mtx.Unlock()

	refSet := l.resources[resourceID]
	if refSet == nil {
		return
	}
	for ref := range refSet.refs {
		ref.released = true
	}

	l.clearResourceLocked(resourceID)
}

func (l *resourceLifetime) releaseRef(ref *resourceRef) {
	l.mtx.Lock()

	if ref.released {
		l.mtx.Unlock()
		return
	}
	ref.released = true

	resourceID, notifyServer := l.releaseRefLocked(ref)
	clientHandleID := l.clientHandleID
	service := l.service
	ctx := l.ctx
	l.mtx.Unlock()

	if notifyServer {
		_, _ = service.ResourceRefRelease(ctx, &resource.ResourceRefReleaseRequest{
			ClientHandleId: clientHandleID,
			ResourceId:     resourceID,
		})
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
	resourceID := ref.resourceID

	refSet := l.resources[resourceID]
	if refSet == nil {
		return 0, false
	}

	delete(refSet.refs, ref)
	if len(refSet.refs) != 0 {
		return 0, false
	}

	l.clearResourceLocked(resourceID)
	return resourceID, true
}

func (l *resourceLifetime) clearResourceLocked(resourceID uint32) {
	if cancel := l.resourceContexts[resourceID]; cancel != nil {
		cancel()
		delete(l.resourceContexts, resourceID)
	}
	delete(l.resources, resourceID)
	delete(l.srpcClients, resourceID)
}

func (l *resourceLifetime) getOrCreateClientLocked(resourceID uint32) (srpc.Client, error) {
	if client := l.srpcClients[resourceID]; client != nil {
		return client, nil
	}
	if l.resources[resourceID] == nil {
		return nil, resource.ErrResourceNotFound
	}

	// #nosec G118 -- stored in resourceContexts and canceled on resource release or Client.Release().
	resourceCtx, cancel := context.WithCancel(l.ctx)
	l.resourceContexts[resourceID] = cancel

	resourceIDStr := strconv.FormatUint(uint64(resourceID), 10)
	client := rpcstream.NewRpcStreamClient(
		func(ctx context.Context) (resource.SRPCResourceService_ResourceRpcClient, error) {
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
		},
		resourceIDStr,
		true,
	)
	l.srpcClients[resourceID] = client
	return client, nil
}

func resourceRPCCallContext(resourceCtx, callCtx context.Context) (context.Context, func()) {
	if callCtx == nil {
		callCtx = context.Background()
	}
	ctx, cancel := context.WithCancel(callCtx)
	stopResourceCancel := context.AfterFunc(resourceCtx, cancel)
	return ctx, func() {
		stopResourceCancel()
		cancel()
	}
}

type resourceRPCClientWithContext struct {
	resource.SRPCResourceService_ResourceRpcClient
	ctx     context.Context
	release func()
}

func (c *resourceRPCClientWithContext) Context() context.Context {
	return c.ctx
}

func (c *resourceRPCClientWithContext) Close() error {
	err := c.SRPCResourceService_ResourceRpcClient.Close()
	c.release()
	return err
}

type resourceRef struct {
	lifetime   *resourceLifetime
	resourceID uint32
	// released is protected by resourceLifetime.mtx.
	released bool
}

func (r *resourceRef) GetResourceID() uint32 {
	return r.resourceID
}

func (r *resourceRef) GetClient() (srpc.Client, error) {
	return r.lifetime.clientForRef(r)
}

func (r *resourceRef) Release() {
	r.lifetime.releaseRef(r)
}

// _ is a type assertion
var _ ResourceRef = ((*resourceRef)(nil))
