package resource_configtype_registry

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	s4wave_configtype_registry "github.com/s4wave/spacewave/sdk/configtype/registry"
)

// ConfigTypeRegistryResource provides an in-memory ConfigType registry.
// Plugins register config type editors via RegisterConfigType and watch for changes via WatchConfigTypes.
type ConfigTypeRegistryResource struct {
	mux srpc.Invoker

	bcast         broadcast.Broadcast
	nextID        uint32
	registrations map[uint32]*s4wave_configtype_registry.ConfigTypeRegistration
}

// NewConfigTypeRegistryResource creates a new ConfigTypeRegistryResource.
func NewConfigTypeRegistryResource() *ConfigTypeRegistryResource {
	r := &ConfigTypeRegistryResource{
		nextID:        1,
		registrations: make(map[uint32]*s4wave_configtype_registry.ConfigTypeRegistration),
	}
	mux := srpc.NewMux()
	_ = s4wave_configtype_registry.SRPCRegisterConfigTypeRegistryResourceService(mux, r)
	r.mux = mux
	return r
}

// GetMux returns the rpc mux.
func (r *ConfigTypeRegistryResource) GetMux() srpc.Invoker {
	return r.mux
}

// RegisterConfigType registers a config type editor from a plugin.
func (r *ConfigTypeRegistryResource) RegisterConfigType(
	ctx context.Context,
	req *s4wave_configtype_registry.RegisterConfigTypeRequest,
) (*s4wave_configtype_registry.RegisterConfigTypeResponse, error) {
	configID := req.GetConfigId()
	pluginID := req.GetPluginId()
	scriptPath := req.GetScriptPath()
	if configID == "" {
		return nil, ErrConfigIdRequired
	}
	if pluginID == "" {
		return nil, ErrPluginIdRequired
	}
	if scriptPath == "" {
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
		r.registrations[regID] = &s4wave_configtype_registry.ConfigTypeRegistration{
			ConfigId:       configID,
			RegistrationId: regID,
			PluginId:       pluginID,
			DisplayName:    req.GetDisplayName(),
			Category:       req.GetCategory(),
			ScriptPath:     scriptPath,
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

	return &s4wave_configtype_registry.RegisterConfigTypeResponse{ResourceId: resourceID}, nil
}

// WatchConfigTypes streams all registered config types.
func (r *ConfigTypeRegistryResource) WatchConfigTypes(
	req *s4wave_configtype_registry.WatchConfigTypesRequest,
	strm s4wave_configtype_registry.SRPCConfigTypeRegistryResourceService_WatchConfigTypesStream,
) error {
	ctx := strm.Context()

	for {
		var regs []*s4wave_configtype_registry.ConfigTypeRegistration
		var waitCh <-chan struct{}

		r.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			regs = r.getRegistrationsLocked()
			waitCh = getWaitCh()
		})

		if err := strm.Send(&s4wave_configtype_registry.WatchConfigTypesResponse{
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

// LookupRegistration finds a registration by configID.
func (r *ConfigTypeRegistryResource) LookupRegistration(
	configID string,
) *s4wave_configtype_registry.ConfigTypeRegistration {
	var reg *s4wave_configtype_registry.ConfigTypeRegistration
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for _, v := range r.registrations {
			if v.GetConfigId() == configID {
				reg = v.CloneVT()
				break
			}
		}
	})
	return reg
}

// getRegistrationsLocked returns a snapshot of all registrations.
// Must be called with bcast lock held.
func (r *ConfigTypeRegistryResource) getRegistrationsLocked() []*s4wave_configtype_registry.ConfigTypeRegistration {
	regs := make([]*s4wave_configtype_registry.ConfigTypeRegistration, 0, len(r.registrations))
	for _, reg := range r.registrations {
		regs = append(regs, reg)
	}
	return regs
}

// _ is a type assertion
var _ s4wave_configtype_registry.SRPCConfigTypeRegistryResourceServiceServer = (*ConfigTypeRegistryResource)(nil)
