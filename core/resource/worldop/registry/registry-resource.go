package resource_worldop_registry

import (
	"context"
	"strings"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/core/resource/registration"
	s4wave_worldop_registry "github.com/s4wave/spacewave/sdk/worldop/registry"
)

// WorldOpRegistryResource provides an in-memory world op registry.
// Plugins register world ops via RegisterWorldOp and watch for changes via WatchWorldOps.
type WorldOpRegistryResource struct {
	mux srpc.Invoker

	// generations owns visibility of prepared plugin registrations.
	generations   *registration.Registry
	bcast         *broadcast.Broadcast
	nextID        uint32
	registrations map[uint32]*s4wave_worldop_registry.WorldOpRegistration
}

// NewWorldOpRegistryResource creates a new WorldOpRegistryResource.
func NewWorldOpRegistryResource(generations *registration.Registry) *WorldOpRegistryResource {
	// A standalone registry has its own admission boundary.
	if generations == nil {
		generations = registration.NewRegistry()
	}
	r := &WorldOpRegistryResource{
		generations:   generations,
		bcast:         generations.Broadcast(),
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
	// pluginID: a single plugin (e.g. spacewave-v86) may serve multiple op
	// namespaces.
	if !strings.Contains(opTypeID, "/") {
		return nil, ErrOpTypeIdMustHavePluginPrefix
	}

	generation, err := registration.FromContext(ctx, pluginID)
	if err != nil {
		return nil, err
	}

	client, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	var regID uint32
	duplicate := false
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		for _, v := range r.registrations {
			if v.GetOperationTypeId() == opTypeID && !r.generations.CanShareNameLocked(v, generation) {
				duplicate = true
				return
			}
		}
		regID = r.nextID
		r.nextID++
		r.registrations[regID] = &s4wave_worldop_registry.WorldOpRegistration{
			OperationTypeId: opTypeID,
			RegistrationId:  regID,
			PluginId:        pluginID,
		}
		err = r.generations.BindLocked(r.registrations[regID], generation)
		if err != nil {
			r.generations.ForgetLocked(r.registrations[regID])
			delete(r.registrations, regID)
			return
		}
		broadcast()
	})
	if err != nil {
		return nil, err
	}
	if duplicate {
		return nil, ErrOperationTypeAlreadyRegistered
	}

	emptyMux := srpc.NewMux()
	resourceID, err := client.AddResource(emptyMux, func() {
		r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			if _, ok := r.registrations[regID]; ok {
				r.generations.ForgetLocked(r.registrations[regID])
				delete(r.registrations, regID)
				broadcast()
			}
		})
	})
	if err != nil {
		r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			r.generations.ForgetLocked(r.registrations[regID])
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
			regs = r.getRegistrationsLocked(req.GetInstanceKey())
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
	opTypeID, instanceKey string,
) *s4wave_worldop_registry.WorldOpRegistration {
	var reg *s4wave_worldop_registry.WorldOpRegistration
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for _, v := range r.getRegistrationsLocked(instanceKey) {
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
func (r *WorldOpRegistryResource) getRegistrationsLocked(instanceKey string) []*s4wave_worldop_registry.WorldOpRegistration {
	selected := registration.SelectLocked(r.generations, r.registrations, instanceKey, (*s4wave_worldop_registry.WorldOpRegistration).GetOperationTypeId)
	regs := make([]*s4wave_worldop_registry.WorldOpRegistration, 0, len(selected))
	for _, reg := range selected {
		regs = append(regs, reg.CloneVT())
	}
	return regs
}

// _ is a type assertion
var _ s4wave_worldop_registry.SRPCWorldOpRegistryResourceServiceServer = (*WorldOpRegistryResource)(nil)
