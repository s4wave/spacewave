package sdk_world_engine

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
)

// AttachedResourceClient creates resource refs for resources attached to the current resource request.
type AttachedResourceClient struct {
	resourceCtx resource_server.ResourceClientContext
}

// NewAttachedResourceClient constructs an AttachedResourceClient.
func NewAttachedResourceClient(resourceCtx resource_server.ResourceClientContext) *AttachedResourceClient {
	return &AttachedResourceClient{resourceCtx: resourceCtx}
}

// NewAttachedEngine constructs an SDKEngine from an attached world engine resource id.
func NewAttachedEngine(ctx context.Context, resourceID uint32) (*SDKEngine, error) {
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}
	client := NewAttachedResourceClient(resourceCtx)
	ref := client.CreateResourceReference(resourceID)
	engine, err := NewSDKEngine(client, ref)
	if err != nil {
		ref.Release()
		return nil, err
	}
	return engine, nil
}

// CreateResourceReference creates a reference to an attached resource.
func (c *AttachedResourceClient) CreateResourceReference(resourceID uint32) resource_client.ResourceRef {
	return &attachedResourceRef{resourceCtx: c.resourceCtx, resourceID: resourceID}
}

type attachedResourceRef struct {
	resourceCtx resource_server.ResourceClientContext
	resourceID  uint32
	released    bool
}

func (r *attachedResourceRef) GetResourceID() uint32 {
	return r.resourceID
}

func (r *attachedResourceRef) GetClient() (srpc.Client, error) {
	if r.released {
		return nil, resource.ErrResourceOrClientReleased
	}
	if r.resourceCtx == nil {
		return nil, resource.ErrNoResourceClientContext
	}
	return r.resourceCtx.GetAttachedResource(r.resourceID)
}

func (r *attachedResourceRef) Release() {
	if r.released {
		return
	}
	r.released = true
	if r.resourceCtx == nil {
		return
	}
	r.resourceCtx.ReleaseResource(r.resourceID)
}

// _ is a type assertion.
var _ ResourceClient = ((*AttachedResourceClient)(nil))

// _ is a type assertion.
var _ resource_client.ResourceRef = ((*attachedResourceRef)(nil))
