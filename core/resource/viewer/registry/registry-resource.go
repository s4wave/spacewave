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
	if reg.GetTypeId() == "" {
		return nil, ErrTypeIdRequired
	}
	if reg.GetScriptPath() == "" {
		return nil, ErrScriptPathRequired
	}

	client, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	var regID uint32
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		regID = r.nextID
		r.nextID++
		r.registrations[regID] = reg
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

// ListViewers returns all registered viewers.
func (r *ViewerRegistryResource) ListViewers(
	ctx context.Context,
	req *s4wave_viewer_registry.ListViewersRequest,
) (*s4wave_viewer_registry.ListViewersResponse, error) {
	var regs []*s4wave_viewer_registry.ViewerRegistration
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		regs = r.getRegistrationsLocked()
	})
	return &s4wave_viewer_registry.ListViewersResponse{Registrations: regs}, nil
}

// WatchViewers streams viewer registration changes.
func (r *ViewerRegistryResource) WatchViewers(
	req *s4wave_viewer_registry.WatchViewersRequest,
	strm s4wave_viewer_registry.SRPCViewerRegistryResourceService_WatchViewersStream,
) error {
	ctx := strm.Context()

	for {
		var regs []*s4wave_viewer_registry.ViewerRegistration
		var waitCh <-chan struct{}

		r.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			regs = r.getRegistrationsLocked()
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

// getRegistrationsLocked returns a snapshot of all registrations.
// Must be called with bcast lock held.
func (r *ViewerRegistryResource) getRegistrationsLocked() []*s4wave_viewer_registry.ViewerRegistration {
	regs := make([]*s4wave_viewer_registry.ViewerRegistration, 0, len(r.registrations))
	for _, reg := range r.registrations {
		regs = append(regs, reg)
	}
	return regs
}

// _ is a type assertion
var _ s4wave_viewer_registry.SRPCViewerRegistryResourceServiceServer = (*ViewerRegistryResource)(nil)
