package resource_objecttype_registry

import (
	"context"
	"strings"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	s4wave_objecttype_registry "github.com/s4wave/spacewave/sdk/objecttype/registry"
)

// ObjectTypeRegistryResource provides an in-memory ObjectType registry.
// Plugins register ObjectTypes via RegisterObjectType and watch for changes via WatchObjectTypes.
type ObjectTypeRegistryResource struct {
	mux srpc.Invoker

	bcast         broadcast.Broadcast
	nextID        uint32
	registrations map[uint32]*s4wave_objecttype_registry.ObjectTypeRegistration
}

// NewObjectTypeRegistryResource creates a new ObjectTypeRegistryResource.
func NewObjectTypeRegistryResource() *ObjectTypeRegistryResource {
	r := &ObjectTypeRegistryResource{
		nextID:        1,
		registrations: make(map[uint32]*s4wave_objecttype_registry.ObjectTypeRegistration),
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
	// pluginID: a single plugin (e.g. spacewave-app) may serve multiple type
	// namespaces (e.g. spacewave-notes/*, spacewave/vm/*).
	if !strings.Contains(typeID, "/") {
		return nil, ErrTypeIdMustHavePluginPrefix
	}

	client, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	var regID uint32
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		regID = r.nextID
		r.nextID++
		r.registrations[regID] = &s4wave_objecttype_registry.ObjectTypeRegistration{
			TypeId:         typeID,
			RegistrationId: regID,
			PluginId:       pluginID,
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
			regs = r.getRegistrationsLocked()
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
	typeID string,
) *s4wave_objecttype_registry.ObjectTypeRegistration {
	var reg *s4wave_objecttype_registry.ObjectTypeRegistration
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for _, v := range r.registrations {
			if v.GetTypeId() == typeID {
				reg = v.CloneVT()
				break
			}
		}
	})
	return reg
}

// getRegistrationsLocked returns a snapshot of all registrations.
// Must be called with bcast lock held.
func (r *ObjectTypeRegistryResource) getRegistrationsLocked() []*s4wave_objecttype_registry.ObjectTypeRegistration {
	regs := make([]*s4wave_objecttype_registry.ObjectTypeRegistration, 0, len(r.registrations))
	for _, reg := range r.registrations {
		regs = append(regs, reg)
	}
	return regs
}

// _ is a type assertion
var _ s4wave_objecttype_registry.SRPCObjectTypeRegistryResourceServiceServer = (*ObjectTypeRegistryResource)(nil)
