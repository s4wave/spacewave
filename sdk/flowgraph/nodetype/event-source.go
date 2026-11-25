package flowgraph_nodetype

import (
	"context"

	"github.com/aperturerobotics/controllerbus/config"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
)

const (
	// eventPortName is the one port of an event source.
	eventPortName = "event"
	// activationPortTypeID is the data contract of an arrival: an event, or a
	// Step's chosen output, which a Step input or an object sink accepts.
	activationPortTypeID = "activation"
)

// eventSource marks a node whose bound object serves EventSourceService.
type eventSource struct{}

// GetDisplayName returns the name shown for the node type.
func (eventSource) GetDisplayName() string {
	return "Event Source"
}

// GetPorts returns the event output a new Event Source declares.
func (eventSource) GetPorts() []*s4wave_flowgraph.FlowgraphPort {
	return []*s4wave_flowgraph.FlowgraphPort{{
		Name:      eventPortName,
		Direction: s4wave_flowgraph.FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_OUTPUT,
		TypeId:    activationPortTypeID,
	}}
}

// GetConfigIDs returns no config IDs, since the node runs no controller of its
// own.
func (eventSource) GetConfigIDs() []string {
	return nil
}

// Compile returns no entries. The run controller of the Flowgraph's standing
// run reads the node's events, on each Device that approves the run's process
// binding, so no Device places the node.
func (eventSource) Compile(context.Context, *s4wave_flowgraph.PlacedFlowgraphNode) (map[string]config.Config, error) {
	return nil, nil
}

// _ is a type assertion
var _ s4wave_flowgraph.FlowgraphNodeType = eventSource{}
