package s4wave_flowgraph

import (
	"context"
	"strings"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/s4wave/spacewave/db/world"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
)

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

// FlowgraphNodeCapability is implemented by a node type whose placed node shows
// a Device capability of its own, in place of the default flowgraph-node
// capability. A Device keeps the first node, in Flowgraph key and node ID order,
// that shows each kind and checkout root name, and rejects the rest.
type FlowgraphNodeCapability interface {
	// GetCapability returns the capability the node shows: its kind, label,
	// link, policy and checkout root. The Device sets the ID and the state. It
	// returns an error to reject the node, such as when the object the link
	// names is not in ws.
	GetCapability(ctx context.Context, ws world.WorldState, node *PlacedFlowgraphNode) (*s4wave_device.DeviceCapability, error)
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

// CapabilityID returns the ID of the Device capability that shows the node.
func (n *PlacedFlowgraphNode) CapabilityID() string {
	return s4wave_device.DeviceCapabilityKindFlowgraphNode + "/" + n.FlowgraphKey + "/" + n.NodeID
}

// IsNodeCapabilityID reports whether a Device capability ID belongs to a
// placed node. The Device's Flowgraph reconciler owns these capabilities, even
// when a node type gives one another kind.
func IsNodeCapabilityID(id string) bool {
	return strings.HasPrefix(id, s4wave_device.DeviceCapabilityKindFlowgraphNode+"/")
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
