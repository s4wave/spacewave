package resource_worldop_registry

import (
	"context"
	"strings"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	s4wave_worldop_registry "github.com/s4wave/spacewave/sdk/worldop/registry"
)

// WorldOpRegistryResource provides an in-memory world op registry.
// Plugins register world ops via RegisterWorldOp and watch for changes via WatchWorldOps.
type WorldOpRegistryResource struct {
	mux srpc.Invoker

	bcast         broadcast.Broadcast
	nextID        uint32
	registrations map[uint32]*s4wave_worldop_registry.WorldOpRegistration
}

// NewWorldOpRegistryResource creates a new WorldOpRegistryResource.
func NewWorldOpRegistryResource() *WorldOpRegistryResource {
	r := &WorldOpRegistryResource{
		nextID:        1,
		registrations: make(map[uint32]*s4wave_worldop_registry.WorldOpRegistration),
	}
	mux := srpc.NewMux()
	_ = s4wave_worldop_registry.SRPCRegisterWorldOpRegistryResourceService(mux, r)
	r.mux = mux
	return r
}

// GetMux returns the rpc mux.
func (r *WorldOpRegistryResource) GetMux() srpc.Invoker {
	return r.mux
}

// RegisterWorldOp registers a world op from a plugin.
func (r *WorldOpRegistryResource) RegisterWorldOp(
	ctx context.Context,
	req *s4wave_worldop_registry.RegisterWorldOpRequest,
) (*s4wave_worldop_registry.RegisterWorldOpResponse, error) {
	opTypeID := req.GetOperationTypeId()
	pluginID := req.GetPluginId()
	if opTypeID == "" {
		return nil, ErrOperationTypeIdRequired
	}
	if pluginID == "" {
		return nil, ErrPluginIdRequired
	}
	// Require a namespace prefix before the first '/'. The prefix need not match
	// pluginID: a single plugin (e.g. spacewave-app) may serve multiple op
	// namespaces folded in from previously-separate plugins.
	if !strings.Contains(opTypeID, "/") {
		return nil, ErrOpTypeIdMustHavePluginPrefix
	}

	client, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	var regID uint32
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		regID = r.nextID
		r.nextID++
		r.registrations[regID] = &s4wave_worldop_registry.WorldOpRegistration{
			OperationTypeId: opTypeID,
			RegistrationId:  regID,
			PluginId:        pluginID,
		}
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

	return &s4wave_worldop_registry.RegisterWorldOpResponse{ResourceId: resourceID}, nil
}

// WatchWorldOps streams all registered world ops.
func (r *WorldOpRegistryResource) WatchWorldOps(
	req *s4wave_worldop_registry.WatchWorldOpsRequest,
	strm s4wave_worldop_registry.SRPCWorldOpRegistryResourceService_WatchWorldOpsStream,
) error {
	ctx := strm.Context()

	for {
		var regs []*s4wave_worldop_registry.WorldOpRegistration
		var waitCh <-chan struct{}

		r.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			regs = r.getRegistrationsLocked()
			waitCh = getWaitCh()
		})

		if err := strm.Send(&s4wave_worldop_registry.WatchWorldOpsResponse{
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

// LookupRegistrationByOpType finds a registration by operation type ID.
func (r *WorldOpRegistryResource) LookupRegistrationByOpType(
	opTypeID string,
) *s4wave_worldop_registry.WorldOpRegistration {
	var reg *s4wave_worldop_registry.WorldOpRegistration
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for _, v := range r.registrations {
			if v.GetOperationTypeId() == opTypeID {
				reg = v.CloneVT()
				break
			}
		}
	})
	return reg
}

// getRegistrationsLocked returns a snapshot of all registrations.
// Must be called with bcast lock held.
func (r *WorldOpRegistryResource) getRegistrationsLocked() []*s4wave_worldop_registry.WorldOpRegistration {
	regs := make([]*s4wave_worldop_registry.WorldOpRegistration, 0, len(r.registrations))
	for _, reg := range r.registrations {
		regs = append(regs, reg)
	}
	return regs
}

// _ is a type assertion
var _ s4wave_worldop_registry.SRPCWorldOpRegistryResourceServiceServer = (*WorldOpRegistryResource)(nil)
