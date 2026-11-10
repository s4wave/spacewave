package s4wave_flowgraph

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_objecttype_registry "github.com/s4wave/spacewave/sdk/objecttype/registry"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
)

// ReadBindings derives the binding of each node that names a Space object.
// A node with no object key has no entry. The state is read from the World and
// the object type registry on b; a nil bus has no registry, so every existing
// object reads UNREGISTERED. engineID scopes the registry lookup to the plugins
// demanded by the World engine, as an object type factory's context does.
func ReadBindings(ctx context.Context, ws world.WorldState, b bus.Bus, engineID string, graph *Flowgraph) (map[string]*FlowgraphBinding, error) {
	var bindings map[string]*FlowgraphBinding
	for id, node := range graph.GetNodes() {
		if node.GetObjectKey() == "" {
			continue
		}
		binding, err := readBinding(ctx, ws, b, engineID, node.GetObjectKey())
		if err != nil {
			return nil, err
		}
		if bindings == nil {
			bindings = make(map[string]*FlowgraphBinding)
		}
		bindings[id] = binding
	}
	return bindings, nil
}

// readBinding derives the state of one bound object.
func readBinding(ctx context.Context, ws world.WorldState, b bus.Bus, engineID, objectKey string) (*FlowgraphBinding, error) {
	// An absent object leaves the node unavailable and keeps its connections.
	binding := &FlowgraphBinding{ObjectKey: objectKey}
	found, err := ws.HasObject(ctx, objectKey)
	if err != nil {
		return nil, err
	}
	if !found {
		binding.State = FlowgraphBindingState_FLOWGRAPH_BINDING_STATE_UNAVAILABLE
		return binding, nil
	}

	// An existing object whose type no plugin registers declares no ports.
	binding.State = FlowgraphBindingState_FLOWGRAPH_BINDING_STATE_UNREGISTERED
	binding.TypeId, err = world_types.GetObjectType(ctx, ws, objectKey)
	if err != nil {
		return nil, err
	}
	if binding.GetTypeId() == "" || b == nil {
		return binding, nil
	}
	metadata, err := s4wave_objecttype_registry.ExLookupObjectTypeMetadata(
		objecttype.WithEngineID(ctx, engineID), b, binding.GetTypeId(),
	)
	if err != nil || metadata == nil {
		return binding, err
	}

	// A registered type is bound, with the ports its registration declares.
	binding.State = FlowgraphBindingState_FLOWGRAPH_BINDING_STATE_BOUND
	for _, port := range metadata.GetPorts() {
		binding.Ports = append(binding.Ports, &FlowgraphPort{
			Name:      port.GetName(),
			Direction: flowgraphPortDirection(port.GetDirection()),
			TypeId:    port.GetTypeId(),
		})
	}
	return binding, nil
}

// flowgraphPortDirection maps an object type port's direction to a node port's.
func flowgraphPortDirection(direction s4wave_objecttype_registry.ObjectTypePortDirection) FlowgraphPortDirection {
	switch direction {
	case s4wave_objecttype_registry.ObjectTypePortDirection_OBJECT_TYPE_PORT_DIRECTION_INPUT:
		return FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_INPUT
	case s4wave_objecttype_registry.ObjectTypePortDirection_OBJECT_TYPE_PORT_DIRECTION_OUTPUT:
		return FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_OUTPUT
	default:
		return FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_UNKNOWN
	}
}

// FindPort returns a port the bound object's type declares, or nil when the
// binding declares none by that name.
func (b *FlowgraphBinding) FindPort(name string) *FlowgraphPort {
	for _, port := range b.GetPorts() {
		if port.GetName() == name {
			return port
		}
	}
	return nil
}
