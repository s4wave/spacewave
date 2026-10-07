package flowgraph_nodetype

import (
	"github.com/aperturerobotics/controllerbus/config"
	stream_forwarding "github.com/s4wave/spacewave/net/stream/forwarding"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
)

// tcpPort exposes a local TCP port of its Device to the connected Local Ports.
type tcpPort struct{}

// GetDisplayName returns the name shown for the node type.
func (tcpPort) GetDisplayName() string {
	return "TCP Port"
}

// GetPorts returns the stream output a new TCP Port declares.
func (tcpPort) GetPorts() []*s4wave_flowgraph.FlowgraphPort {
	return []*s4wave_flowgraph.FlowgraphPort{{
		Name:      streamPortName,
		Direction: s4wave_flowgraph.FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_OUTPUT,
		TypeId:    streamPortTypeID,
	}}
}

// GetConfigIDs returns the forwarding controller config ID.
func (tcpPort) GetConfigIDs() []string {
	return []string{stream_forwarding.ConfigID}
}

// Compile forwards each connection's peer stream to the node's TCP address.
func (tcpPort) Compile(node *s4wave_flowgraph.PlacedFlowgraphNode) (map[string]config.Config, error) {
	// Resolve the Device's local address and identity.
	if err := requireDevicePeerID(node); err != nil {
		return nil, err
	}
	target, err := streamMultiaddr(node.Node)
	if err != nil {
		return nil, err
	}

	// Forward the stream of each connection that leaves the stream port.
	entries := make(map[string]config.Config)
	for _, conn := range node.Connections {
		if conn.Connection.GetOutputNode() != node.NodeID || conn.Connection.GetOutputPort() != streamPortName {
			continue
		}
		conf := &stream_forwarding.Config{
			PeerId:          node.DevicePeerID,
			ProtocolId:      streamProtocolID(node.FlowgraphKey, conn.ID),
			TargetMultiaddr: target,
		}
		if err := conf.Validate(); err != nil {
			return nil, err
		}
		entries[conn.ID] = conf
	}
	return entries, nil
}

// _ is a type assertion
var _ s4wave_flowgraph.FlowgraphNodeType = tcpPort{}
