package space_world_replica

import (
	"context"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
)

// NodeTypeID selects the replica node type. Two replica nodes, each placed on
// one Device and joined by one connection, keep one SharedObject on both
// Devices and move its ownership to the desired owner.
const NodeTypeID = "sobject/replica"

// The ports of a replica node. A pair is joined from one node's output to the
// other's input; which node holds which end does not matter.
const (
	// PortOut is the replica output port.
	PortOut = "replica-out"
	// PortIn is the replica input port.
	PortIn = "replica-in"
	// portTypeID is the data contract of a replica connection.
	portTypeID = "sobject/replica"
)

// The parameters of a replica node.
const (
	// ParamSharedObject is the ID of the replicated SharedObject.
	ParamSharedObject = "shared_object"
	// ParamDesiredOwner is the graph-local ID of the replica node whose Device
	// should own the SharedObject: this node or the one it is joined to.
	ParamDesiredOwner = "desired_owner"
	// ParamProvider is the ID of the local provider holding the SharedObject,
	// "local" when unset.
	ParamProvider = "provider"
)

// defaultProviderID is the provider a node names when it sets none.
const defaultProviderID = "local"

// NodeType is the replica node type.
type NodeType struct{}

// GetDisplayName returns the name shown for the node type.
func (NodeType) GetDisplayName() string {
	return "SharedObject Replica"
}

// GetPorts returns the replica output and input a new node declares.
func (NodeType) GetPorts() []*s4wave_flowgraph.FlowgraphPort {
	return []*s4wave_flowgraph.FlowgraphPort{
		{Name: PortOut, Direction: s4wave_flowgraph.FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_OUTPUT, TypeId: portTypeID},
		{Name: PortIn, Direction: s4wave_flowgraph.FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_INPUT, TypeId: portTypeID},
	}
}

// GetConfigIDs returns the replica controller config ID.
func (NodeType) GetConfigIDs() []string {
	return []string{ConfigID}
}

// Compile returns the replica controller config of the node. A node with no
// replica connection compiles to no entries, because it has no peer.
func (NodeType) Compile(_ context.Context, node *s4wave_flowgraph.PlacedFlowgraphNode) (map[string]config.Config, error) {
	conf, err := NewConfigFromNode(node)
	if err != nil || conf == nil {
		return nil, err
	}
	return map[string]config.Config{"replica": conf}, nil
}

// NewConfigFromNode builds the replica config of a placed node, or nil when
// the node has no replica connection.
func NewConfigFromNode(node *s4wave_flowgraph.PlacedFlowgraphNode) (*Config, error) {
	// Find the one connection on a replica port.
	conn, remoteNodeID, err := replicaConnection(node)
	if err != nil || conn == nil {
		return nil, err
	}
	if node.DevicePeerID == "" || conn.RemoteDevicePeerID == "" {
		return nil, errors.New("both replica nodes must be placed on a Device")
	}

	// Resolve the desired owner's node to its Device.
	params := node.Node.GetParameters()
	var desired string
	switch params[ParamDesiredOwner] {
	case node.NodeID:
		desired = node.DevicePeerID
	case remoteNodeID:
		desired = conn.RemoteDevicePeerID
	default:
		return nil, errors.Errorf("desired owner %q is not a node of the replica pair", params[ParamDesiredOwner])
	}

	// Build the config on the connection's peer stream.
	provider := params[ParamProvider]
	if provider == "" {
		provider = defaultProviderID
	}
	conf := &Config{
		Spec:         &ReplicaSpec{SharedObjectId: params[ParamSharedObject], DesiredOwnerPeerId: desired},
		ProviderId:   provider,
		LocalPeerId:  node.DevicePeerID,
		RemotePeerId: conn.RemoteDevicePeerID,
		ProtocolId:   "spacewave/" + node.FlowgraphKey + "/" + conn.ID,
	}
	if err := conf.Validate(); err != nil {
		return nil, err
	}
	return conf, nil
}

// replicaConnection returns the connection on the node's replica ports and the
// graph-local ID of the node at its other end. It returns nil when there is
// none and an error when there is more than one, since a node replicates to
// one peer.
func replicaConnection(node *s4wave_flowgraph.PlacedFlowgraphNode) (*s4wave_flowgraph.PlacedFlowgraphConnection, string, error) {
	var found *s4wave_flowgraph.PlacedFlowgraphConnection
	var remoteNodeID string
	for _, conn := range node.Connections {
		// Keep a connection that leaves the output or arrives at the input.
		c := conn.Connection
		var other string
		switch {
		case c.GetOutputNode() == node.NodeID && c.GetOutputPort() == PortOut:
			other = c.GetInputNode()
		case c.GetInputNode() == node.NodeID && c.GetInputPort() == PortIn:
			other = c.GetOutputNode()
		default:
			continue
		}
		if found != nil {
			return nil, "", errors.New("a replica node accepts one connection")
		}
		found, remoteNodeID = conn, other
	}
	return found, remoteNodeID, nil
}

// _ is a type assertion
var _ s4wave_flowgraph.FlowgraphNodeType = NodeType{}
