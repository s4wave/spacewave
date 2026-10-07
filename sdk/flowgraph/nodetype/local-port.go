package flowgraph_nodetype

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
	stream_listening "github.com/s4wave/spacewave/net/stream/listening"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
)

// localPort listens on its Device and forwards to the connected TCP Port.
type localPort struct{}

// GetDisplayName returns the name shown for the node type.
func (localPort) GetDisplayName() string {
	return "Local Port"
}

// GetPorts returns the stream input a new Local Port declares.
func (localPort) GetPorts() []*s4wave_flowgraph.FlowgraphPort {
	return []*s4wave_flowgraph.FlowgraphPort{{
		Name:      streamPortName,
		Direction: s4wave_flowgraph.FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_INPUT,
		TypeId:    streamPortTypeID,
	}}
}

// GetConfigIDs returns the listening controller config ID.
func (localPort) GetConfigIDs() []string {
	return []string{stream_listening.ConfigID}
}

// Compile listens on the node's address and opens a peer stream to the Device
// at the other end of the connection. A Local Port with no connection compiles
// to no entries, because it has no peer to forward to.
func (localPort) Compile(node *s4wave_flowgraph.PlacedFlowgraphNode) (map[string]config.Config, error) {
	// Select the connections that arrive at the stream port.
	var inbound []*s4wave_flowgraph.PlacedFlowgraphConnection
	for _, conn := range node.Connections {
		if conn.Connection.GetInputNode() == node.NodeID && conn.Connection.GetInputPort() == streamPortName {
			inbound = append(inbound, conn)
		}
	}
	if len(inbound) == 0 {
		return nil, nil
	}

	// Require one peer, since one address cannot be listened on twice.
	if len(inbound) > 1 {
		return nil, errors.New("a Local Port accepts one connection")
	}
	conn := inbound[0]

	// Listen on the address and open streams to the connected Device.
	if err := requireDevicePeerID(node); err != nil {
		return nil, err
	}
	listen, err := streamMultiaddr(node.Node)
	if err != nil {
		return nil, err
	}
	conf := &stream_listening.Config{
		LocalPeerId:     node.DevicePeerID,
		RemotePeerId:    conn.RemoteDevicePeerID,
		ProtocolId:      streamProtocolID(node.FlowgraphKey, conn.ID),
		ListenMultiaddr: listen,
	}
	if err := conf.Validate(); err != nil {
		return nil, err
	}
	return map[string]config.Config{conn.ID: conf}, nil
}

// _ is a type assertion
var _ s4wave_flowgraph.FlowgraphNodeType = localPort{}
