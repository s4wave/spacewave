package flowgraph_nodetype

import (
	"context"

	"github.com/aperturerobotics/controllerbus/config"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
)

// statePortName is the input a new Object Sink declares.
const statePortName = "state"

// objectSink marks a node whose bound object a run writes.
type objectSink struct{}

// GetDisplayName returns the name shown for the node type.
func (objectSink) GetDisplayName() string {
	return "Object Sink"
}

// GetPorts returns the state input a new Object Sink declares. It accepts an
// activation, which every Step output carries, so one Step output may feed
// the sink and the next Step at once.
func (objectSink) GetPorts() []*s4wave_flowgraph.FlowgraphPort {
	return []*s4wave_flowgraph.FlowgraphPort{{
		Name:      statePortName,
		Direction: s4wave_flowgraph.FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_INPUT,
		TypeId:    activationPortTypeID,
	}}
}

// GetConfigIDs returns no config IDs, since the node runs no controller of its
// own.
func (objectSink) GetConfigIDs() []string {
	return nil
}

// Compile returns no entries. The run controller commits each activation
// routed into the sink in the transaction that routes it, so no Device places
// the node.
func (objectSink) Compile(context.Context, *s4wave_flowgraph.PlacedFlowgraphNode) (map[string]config.Config, error) {
	return nil, nil
}

// _ is a type assertion
var _ s4wave_flowgraph.FlowgraphNodeType = objectSink{}
