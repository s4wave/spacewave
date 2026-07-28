package s4wave_viewer_registry

import (
	"context"

	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
)

// ViewerRegistry is the Go SDK wrapper for the ViewerRegistryResourceService.
type ViewerRegistry struct {
	client  *resource_client.Client
	ref     resource_client.ResourceRef
	service SRPCViewerRegistryResourceServiceClient
}

// NewViewerRegistry creates a new ViewerRegistry resource wrapper.
func NewViewerRegistry(client *resource_client.Client, ref resource_client.ResourceRef) (*ViewerRegistry, error) {
	srpcClient, err := ref.GetClient()
	if err != nil {
		return nil, err
	}
	return &ViewerRegistry{
		client:  client,
		ref:     ref,
		service: NewSRPCViewerRegistryResourceServiceClient(srpcClient),
	}, nil
}

// GetResourceRef returns the resource reference.
func (r *ViewerRegistry) GetResourceRef() resource_client.ResourceRef {
	return r.ref
}

// Release releases the resource reference.
func (r *ViewerRegistry) Release() {
	r.ref.Release()
}

// RegisterViewer registers a viewer for an object type and returns the registration resource ID.
func (r *ViewerRegistry) RegisterViewer(ctx context.Context, reg *ViewerRegistration) (uint32, error) {
	resp, err := r.service.RegisterViewer(ctx, &RegisterViewerRequest{Registration: reg})
	if err != nil {
		return 0, err
	}
	return resp.GetResourceId(), nil
}

// ListViewers returns registered viewers for surface.
func (r *ViewerRegistry) ListViewers(ctx context.Context, surface ViewerSurface) ([]*ViewerRegistration, error) {
	resp, err := r.service.ListViewers(ctx, &ListViewersRequest{Surface: surface})
	if err != nil {
		return nil, err
	}
	return resp.GetRegistrations(), nil
}

// WatchViewers streams viewer registration changes for surface.
func (r *ViewerRegistry) WatchViewers(
	ctx context.Context,
	surface ViewerSurface,
) (SRPCViewerRegistryResourceService_WatchViewersClient, error) {
	return r.service.WatchViewers(ctx, &WatchViewersRequest{Surface: surface})
}
