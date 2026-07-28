package resource_viewer_registry

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	s4wave_viewer_registry "github.com/s4wave/spacewave/sdk/viewer/registry"
)

// ViewerRegistryResource provides an in-memory viewer registry.
// Plugins register viewers via RegisterViewer and watch for changes via WatchViewers.
type ViewerRegistryResource struct {
	mux srpc.Invoker

	bcast         broadcast.Broadcast
	nextID        uint32
	registrations map[uint32]*s4wave_viewer_registry.ViewerRegistration
}

// NewViewerRegistryResource creates a new ViewerRegistryResource.
func NewViewerRegistryResource() *ViewerRegistryResource {
	r := &ViewerRegistryResource{
		nextID:        1,
		registrations: make(map[uint32]*s4wave_viewer_registry.ViewerRegistration),
	}
	mux := srpc.NewMux()
	_ = s4wave_viewer_registry.SRPCRegisterViewerRegistryResourceService(mux, r)
	r.mux = mux
	return r
}

// GetMux returns the rpc mux.
func (r *ViewerRegistryResource) GetMux() srpc.Invoker {
	return r.mux
}

// RegisterViewer registers a viewer for an object type.
func (r *ViewerRegistryResource) RegisterViewer(
	ctx context.Context,
	req *s4wave_viewer_registry.RegisterViewerRequest,
) (*s4wave_viewer_registry.RegisterViewerResponse, error) {
	reg := req.GetRegistration()
	if reg == nil {
		return nil, ErrRegistrationRequired
	}
	if reg.GetSurface() == s4wave_viewer_registry.ViewerSurface_VIEWER_SURFACE_UNSPECIFIED {
		return nil, ErrSurfaceRequired
	}
	if reg.GetTypeId() == "" {
		return nil, ErrTypeIdRequired
	}
	if reg.GetScriptPath() == "" {
		return nil, ErrScriptPathRequired
	}
	if reg.GetComponentId() == "" {
		return nil, ErrComponentIdRequired
	}

	client, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	var regID uint32
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		regID = r.nextID
		r.nextID++
		r.registrations[regID] = reg.CloneVT()
		broadcast()
	})

	emptyMux := srpc.NewMux()
	resourceID, err := client.AddResource(emptyMux, func() {
		r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			if _, ok := r.registrations[regID]; ok {
				delete(r.registrations, regID)
				broadcast()
			}
		})
	})
	if err != nil {
		r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			delete(r.registrations, regID)
			broadcast()
		})
		return nil, err
	}

	return &s4wave_viewer_registry.RegisterViewerResponse{ResourceId: resourceID}, nil
}

// ListViewers returns registered viewers for the requested surface.
func (r *ViewerRegistryResource) ListViewers(
	ctx context.Context,
	req *s4wave_viewer_registry.ListViewersRequest,
) (*s4wave_viewer_registry.ListViewersResponse, error) {
	if req.GetSurface() == s4wave_viewer_registry.ViewerSurface_VIEWER_SURFACE_UNSPECIFIED {
		return nil, ErrSurfaceRequired
	}
	var regs []*s4wave_viewer_registry.ViewerRegistration
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		regs = r.getRegistrationsLocked(req.GetSurface())
	})
	return &s4wave_viewer_registry.ListViewersResponse{Registrations: regs}, nil
}

// WatchViewers streams registration changes for the requested surface.
func (r *ViewerRegistryResource) WatchViewers(
	req *s4wave_viewer_registry.WatchViewersRequest,
	strm s4wave_viewer_registry.SRPCViewerRegistryResourceService_WatchViewersStream,
) error {
	if req.GetSurface() == s4wave_viewer_registry.ViewerSurface_VIEWER_SURFACE_UNSPECIFIED {
		return ErrSurfaceRequired
	}
	ctx := strm.Context()

	for {
		var regs []*s4wave_viewer_registry.ViewerRegistration
		var waitCh <-chan struct{}

		r.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			regs = r.getRegistrationsLocked(req.GetSurface())
			waitCh = getWaitCh()
		})

		if err := strm.Send(&s4wave_viewer_registry.WatchViewersResponse{
			Registrations: regs,
		}); err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-waitCh:
		}
	}
}

// getRegistrationsLocked returns a snapshot of registrations for surface.
// Must be called with bcast lock held.
func (r *ViewerRegistryResource) getRegistrationsLocked(
	surface s4wave_viewer_registry.ViewerSurface,
) []*s4wave_viewer_registry.ViewerRegistration {
	regs := make([]*s4wave_viewer_registry.ViewerRegistration, 0, len(r.registrations))
	for _, reg := range r.registrations {
		if reg.GetSurface() != surface {
			continue
		}
		regs = append(regs, reg)
	}
	return regs
}

// _ is a type assertion
var _ s4wave_viewer_registry.SRPCViewerRegistryResourceServiceServer = (*ViewerRegistryResource)(nil)
