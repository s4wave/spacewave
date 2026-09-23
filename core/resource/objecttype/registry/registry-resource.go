package resource_objecttype_registry

import (
	"context"
	"strings"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/core/resource/registration"
	s4wave_objecttype_registry "github.com/s4wave/spacewave/sdk/objecttype/registry"
)

// ObjectTypeRegistryResource provides an in-memory ObjectType registry.
// Plugins register ObjectTypes via RegisterObjectType and watch for changes via WatchObjectTypes.
type ObjectTypeRegistryResource struct {
	mux srpc.Invoker

	// generations owns visibility of prepared plugin registrations.
	generations   *registration.Registry
	bcast         *broadcast.Broadcast
	nextID        uint32
	registrations map[uint32]*objectTypeRegistration
}

// objectTypeRegistration keeps private handler capability state beside the
// public registration. Attached handler IDs never leave the caller generation.
type objectTypeRegistration struct {
	registration *s4wave_objecttype_registry.ObjectTypeRegistration
	attached     *attachedObjectTypeHandler
}

// attachedObjectTypeHandler is a caller-owned ResourceService capability.
type attachedObjectTypeHandler struct {
	client srpc.Client
	ctx    context.Context
}

// NewObjectTypeRegistryResource creates a new ObjectTypeRegistryResource.
func NewObjectTypeRegistryResource(generations *registration.Registry) *ObjectTypeRegistryResource {
	// A standalone registry has its own admission boundary.
	if generations == nil {
		generations = registration.NewRegistry()
	}
	r := &ObjectTypeRegistryResource{
		generations:   generations,
		bcast:         generations.Broadcast(),
		nextID:        1,
		registrations: make(map[uint32]*objectTypeRegistration),
	}
	mux := srpc.NewMux()
	_ = s4wave_objecttype_registry.SRPCRegisterObjectTypeRegistryResourceService(mux, r)
	r.mux = mux
	return r
}

// GetMux returns the rpc mux.
func (r *ObjectTypeRegistryResource) GetMux() srpc.Invoker {
	return r.mux
}

// RegisterObjectType registers an ObjectType from a plugin.
func (r *ObjectTypeRegistryResource) RegisterObjectType(
	ctx context.Context,
	req *s4wave_objecttype_registry.RegisterObjectTypeRequest,
) (*s4wave_objecttype_registry.RegisterObjectTypeResponse, error) {
	typeID := req.GetTypeId()
	pluginID := req.GetPluginId()
	if typeID == "" {
		return nil, ErrTypeIdRequired
	}
	if pluginID == "" {
		return nil, ErrPluginIdRequired
	}
	// Require a namespace prefix before the first '/'. The prefix need not match
	// pluginID: a single plugin (e.g. spacewave-v86) may serve multiple type
	// namespaces (e.g. vm/v86 and vm/image/v86).
	if !strings.Contains(typeID, "/") {
		return nil, ErrTypeIdMustHavePluginPrefix
	}

	generation, err := registration.FromContext(ctx, pluginID)
	if err != nil {
		return nil, err
	}

	client, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	var attached *attachedObjectTypeHandler
	if attachedID := req.GetAttachedHandlerResourceId(); attachedID != 0 {
		attachedClient, err := client.GetAttachedResource(attachedID)
		if err != nil {
			return nil, err
		}
		attached = &attachedObjectTypeHandler{client: attachedClient, ctx: client.Context()}
	}

	var regID uint32
	var duplicate bool
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		for _, registration := range r.registrations {
			if registration.registration.GetTypeId() == typeID && !r.generations.CanShareNameLocked(registration, generation) {
				duplicate = true
				return
			}
		}
		regID = r.nextID
		r.nextID++
		var metadata *s4wave_objecttype_registry.ObjectTypeMetadata
		if req.GetMetadata() != nil {
			metadata = req.GetMetadata().CloneVT()
		}
		r.registrations[regID] = &objectTypeRegistration{registration: &s4wave_objecttype_registry.ObjectTypeRegistration{
			TypeId:         typeID,
			RegistrationId: regID,
			PluginId:       pluginID,
			Metadata:       metadata,
		}, attached: attached}
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
		return nil, ErrTypeIdAlreadyRegistered
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

	return &s4wave_objecttype_registry.RegisterObjectTypeResponse{ResourceId: resourceID}, nil
}

// WatchObjectTypes streams all registered ObjectTypes.
func (r *ObjectTypeRegistryResource) WatchObjectTypes(
	req *s4wave_objecttype_registry.WatchObjectTypesRequest,
	strm s4wave_objecttype_registry.SRPCObjectTypeRegistryResourceService_WatchObjectTypesStream,
) error {
	ctx := strm.Context()

	for {
		var regs []*s4wave_objecttype_registry.ObjectTypeRegistration
		var waitCh <-chan struct{}

		r.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			regs = r.getRegistrationsLocked(req.GetInstanceKey())
			waitCh = getWaitCh()
		})

		if err := strm.Send(&s4wave_objecttype_registry.WatchObjectTypesResponse{
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

// LookupRegistration finds a registration by typeID.
func (r *ObjectTypeRegistryResource) LookupRegistration(
	typeID, instanceKey string,
) *s4wave_objecttype_registry.ObjectTypeRegistration {
	var reg *s4wave_objecttype_registry.ObjectTypeRegistration
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if selected := r.lookupRegistrationLocked(typeID, instanceKey); selected != nil {
			reg = selected.registration.CloneVT()
		}
	})
	return reg
}

// lookupRegistrationLocked finds the visible private handler capability.
// The caller holds bcast.
func (r *ObjectTypeRegistryResource) lookupRegistrationLocked(typeID, instanceKey string) *objectTypeRegistration {
	for _, registration := range r.selectedLocked(instanceKey) {
		if registration.registration.GetTypeId() == typeID {
			return registration
		}
	}
	return nil
}

// getRegistrationsLocked returns a snapshot of all registrations.
// Must be called with bcast lock held.
func (r *ObjectTypeRegistryResource) getRegistrationsLocked(instanceKey string) []*s4wave_objecttype_registry.ObjectTypeRegistration {
	selected := r.selectedLocked(instanceKey)
	regs := make([]*s4wave_objecttype_registry.ObjectTypeRegistration, 0, len(selected))
	for _, reg := range selected {
		regs = append(regs, reg.registration.CloneVT())
	}
	return regs
}

// selectedLocked selects handlers for the consuming World. The caller holds bcast.
func (r *ObjectTypeRegistryResource) selectedLocked(instanceKey string) []*objectTypeRegistration {
	return registration.SelectLocked(r.generations, r.registrations, instanceKey, func(reg *objectTypeRegistration) string {
		return reg.registration.GetTypeId()
	})
}

// _ is a type assertion
var _ s4wave_objecttype_registry.SRPCObjectTypeRegistryResourceServiceServer = (*ObjectTypeRegistryResource)(nil)
