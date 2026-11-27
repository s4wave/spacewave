// Package resource_flowgraph_registry resolves plugin node types through Resource capabilities.
package resource_flowgraph_registry

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/core/resource/registration"
	flowgraph_registry "github.com/s4wave/spacewave/sdk/flowgraph/registry"
)

// RegistryResource retains compilers for the lifetime of their registration
// Resources. A Space plugin registers through its generation scope, so each
// installation of the plugin holds its own node types, selected by the World
// engine ID of the Flowgraph that uses them.
type RegistryResource struct {
	// generations owns visibility of prepared plugin registrations.
	generations *registration.Registry
	// nextID is the next registration ID, under the admission transaction.
	nextID uint32
	// registrations contains each registered compiler by ID, under the admission transaction.
	registrations map[uint32]*nodeTypeRegistration
}

// nodeTypeRegistration binds metadata and its compiler to one Resource lifetime.
type nodeTypeRegistration struct {
	// id is the registration ID.
	id uint32
	// metadata is an immutable copy of the plugin's declaration.
	metadata *flowgraph_registry.NodeType
	// client invokes the compiler attached by the registering Resource client.
	client srpc.Client
	// ctx ends when the registration releases or the registering client disconnects.
	ctx context.Context
}

// NewRegistryResource constructs an empty node type registry that admits
// plugin generations through generations, or through its own boundary when
// generations is nil.
func NewRegistryResource(generations *registration.Registry) *RegistryResource {
	if generations == nil {
		generations = registration.NewRegistry()
	}
	return &RegistryResource{
		generations:   generations,
		nextID:        1,
		registrations: make(map[uint32]*nodeTypeRegistration),
	}
}

// RegisterNodeType retains a caller-attached compiler until its Resource releases.
func (r *RegistryResource) RegisterNodeType(ctx context.Context, req *flowgraph_registry.RegisterNodeTypeRequest) (*flowgraph_registry.RegisterNodeTypeResponse, error) {
	// Require a complete declaration before exposing a compiler.
	metadata := req.GetNodeType()
	if err := metadata.Validate(); err != nil {
		return nil, err
	}
	if req.GetHandlerResourceId() == 0 {
		return nil, errors.New("node type compiler resource is required")
	}
	generation, err := registration.FromContext(ctx, "")
	if err != nil {
		return nil, err
	}

	// Bind the compiler to the caller's Resource generation.
	client, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}
	compiler, err := client.GetAttachedResource(req.GetHandlerResourceId())
	if err != nil {
		return nil, err
	}
	lifetime, cancel := context.WithCancel(client.Context())
	reg := &nodeTypeRegistration{metadata: metadata.CloneVT(), client: compiler, ctx: lifetime}

	// Index the registration unless it is ambiguous in its scope.
	r.generations.Transact(func(broadcast func(), _ func() <-chan struct{}) {
		// Refuse a name another registration in the scope holds.
		if err = r.checkConflictLocked(reg.metadata, generation); err != nil {
			return
		}

		// Bind the registration to its generation, then publish it.
		reg.id = r.nextID
		r.nextID++
		if err = r.generations.BindLocked(reg, generation); err != nil {
			return
		}
		r.registrations[reg.id] = reg
		broadcast()
	})
	if err != nil {
		cancel()
		return nil, err
	}

	// Withdraw the declaration and cancel in-flight compiles and plugin
	// controllers on Resource release.
	release := func() {
		cancel()
		r.generations.Transact(func(broadcast func(), _ func() <-chan struct{}) {
			r.generations.ForgetLocked(reg)
			delete(r.registrations, reg.id)
			broadcast()
		})
	}
	resourceID, err := client.AddResource(srpc.NewMux(), release)
	if err != nil {
		release()
		return nil, err
	}
	return &flowgraph_registry.RegisterNodeTypeResponse{ResourceId: resourceID}, nil
}

// checkConflictLocked refuses a type ID or plugin config ID that another
// registration in the same scope holds. The caller runs in a Registry transaction.
func (r *RegistryResource) checkConflictLocked(metadata *flowgraph_registry.NodeType, generation *registration.Generation) error {
	pluginConfigIDs := idSet(metadata.GetPluginConfigIds())
	for _, other := range r.registrations {
		// Registrations in other scopes may share every name.
		if r.generations.CanShareNameLocked(other, generation) {
			continue
		}

		// Refuse a shared type ID or plugin config ID within one scope.
		if other.metadata.GetTypeId() == metadata.GetTypeId() {
			return errors.New("node type already registered")
		}
		for _, id := range other.metadata.GetPluginConfigIds() {
			if _, ok := pluginConfigIDs[id]; ok {
				return errors.Errorf("plugin config ID %q already registered", id)
			}
		}
	}
	return nil
}

// watchRegistration selects the visible compiler and subscribes to changes.
func (r *RegistryResource) watchRegistration(engineID, typeID string) (*nodeTypeRegistration, <-chan struct{}) {
	var reg *nodeTypeRegistration
	var wait <-chan struct{}
	r.generations.Transact(func(_ func(), nextChange func() <-chan struct{}) {
		reg = r.lookupLocked(engineID, typeID)
		wait = nextChange()
	})
	return reg, wait
}

// watchRegistrationByID retains a compiler selection until its Resource releases.
func (r *RegistryResource) watchRegistrationByID(id uint32) (*nodeTypeRegistration, <-chan struct{}) {
	var reg *nodeTypeRegistration
	var wait <-chan struct{}
	r.generations.Transact(func(_ func(), nextChange func() <-chan struct{}) {
		reg = r.registrations[id]
		wait = nextChange()
	})
	return reg, wait
}

// lookupLocked returns the visible registration of typeID for the World
// engineID: the Space plugin installed in that World, else a global one. The
// caller runs in a Registry transaction.
func (r *RegistryResource) lookupLocked(engineID, typeID string) *nodeTypeRegistration {
	selected := registration.SelectLocked(r.generations, r.registrations, engineID, func(reg *nodeTypeRegistration) string {
		return reg.metadata.GetTypeId()
	})
	for _, reg := range selected {
		if reg.metadata.GetTypeId() == typeID {
			return reg
		}
	}
	return nil
}

// _ is a type assertion.
var _ flowgraph_registry.SRPCNodeTypeRegistryResourceServiceServer = (*RegistryResource)(nil)
