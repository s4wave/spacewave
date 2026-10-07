package s4wave_flowgraph

import "github.com/aperturerobotics/controllerbus/config"

// FlowgraphNodeType is the behavior behind a node's type_id. A controller
// supplies it by resolving LookupFlowgraphNodeType.
type FlowgraphNodeType interface {
	// GetDisplayName returns the name shown for the node type.
	GetDisplayName() string
	// GetPorts returns the ports a new node of this type declares.
	GetPorts() []*FlowgraphPort
	// GetConfigIDs returns every config ID that Compile may emit. A Device
	// rejects an entry whose config ID the type did not declare.
	GetConfigIDs() []string
	// Compile returns the controller configs that run the placed node, keyed by
	// entry name. The name is unique among the node's entries.
	Compile(node *PlacedFlowgraphNode) (map[string]config.Config, error)
}

// PlacedFlowgraphNode is a node placed on a Device with the context its node
// type compiles from.
type PlacedFlowgraphNode struct {
	// FlowgraphKey is the object key of the Flowgraph holding the node.
	FlowgraphKey string
	// NodeID is the graph-local ID of the node.
	NodeID string
	// Node is the authored node.
	Node *FlowgraphNode
	// DevicePeerID is the peer ID of the Device running the node.
	DevicePeerID string
	// Connections contains the connections that end at the node.
	Connections []*PlacedFlowgraphConnection
}

// PlacedFlowgraphConnection is a connection that ends at a placed node.
type PlacedFlowgraphConnection struct {
	// ID is the graph-local ID of the connection.
	ID string
	// Connection is the authored connection.
	Connection *FlowgraphConnection
	// RemoteDevicePeerID is the peer ID of the Device running the other end.
	RemoteDevicePeerID string
}
