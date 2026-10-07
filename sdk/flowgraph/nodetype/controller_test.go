package flowgraph_nodetype

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus/inmem"
	"github.com/aperturerobotics/controllerbus/config"
	directive_controller "github.com/aperturerobotics/controllerbus/directive/controller"
	"github.com/s4wave/spacewave/net/peer"
	stream_forwarding "github.com/s4wave/spacewave/net/stream/forwarding"
	stream_listening "github.com/s4wave/spacewave/net/stream/listening"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
	"github.com/sirupsen/logrus"
)

// TestCompileTCPPortToLocalPort resolves both core node types on a bus and
// compiles a connected TCP Port and Local Port.
func TestCompileTCPPortToLocalPort(t *testing.T) {
	// Serve the core node types on a bus.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	b := inmem.NewBus(directive_controller.NewController(ctx, le))
	release, err := b.AddController(ctx, NewController(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	// Resolve each core node type through the public directive.
	types := make(map[string]s4wave_flowgraph.FlowgraphNodeType)
	for _, typeID := range []string{s4wave_flowgraph.TCPPortNodeTypeID, s4wave_flowgraph.LocalPortNodeTypeID} {
		nodeType, ref, err := s4wave_flowgraph.ExLookupFlowgraphNodeType(ctx, b, typeID)
		if err != nil {
			t.Fatal(err)
		}
		if nodeType == nil {
			t.Fatalf("node type %q not resolved", typeID)
		}
		defer ref.Release()
		types[typeID] = nodeType
	}

	// Leave a type that no controller supplies waiting until the context ends.
	missingCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	if _, _, err := s4wave_flowgraph.ExLookupFlowgraphNodeType(missingCtx, b, "missing"); err == nil {
		t.Fatal("resolved a node type nobody supplies")
	}

	// Place a TCP Port and a Local Port on two Devices and connect them.
	thumper, _, _, err := peer.NewPeerWithGenerateED25519()
	if err != nil {
		t.Fatal(err)
	}
	laptop, _, _, err := peer.NewPeerWithGenerateED25519()
	if err != nil {
		t.Fatal(err)
	}
	connection := &s4wave_flowgraph.FlowgraphConnection{
		OutputNode: "tcp",
		OutputPort: streamPortName,
		InputNode:  "local",
		InputPort:  streamPortName,
	}
	address := map[string]string{addressParameter: "127.0.0.1:8080"}
	tcp := &s4wave_flowgraph.PlacedFlowgraphNode{
		FlowgraphKey: "flowgraph/main",
		NodeID:       "tcp",
		Node:         &s4wave_flowgraph.FlowgraphNode{TypeId: s4wave_flowgraph.TCPPortNodeTypeID, Parameters: address},
		DevicePeerID: thumper.GetPeerID().String(),
		Connections: []*s4wave_flowgraph.PlacedFlowgraphConnection{
			{ID: "forward", Connection: connection, RemoteDevicePeerID: laptop.GetPeerID().String()},
		},
	}
	local := &s4wave_flowgraph.PlacedFlowgraphNode{
		FlowgraphKey: "flowgraph/main",
		NodeID:       "local",
		Node:         &s4wave_flowgraph.FlowgraphNode{TypeId: s4wave_flowgraph.LocalPortNodeTypeID, Parameters: address},
		DevicePeerID: laptop.GetPeerID().String(),
		Connections: []*s4wave_flowgraph.PlacedFlowgraphConnection{
			{ID: "forward", Connection: connection, RemoteDevicePeerID: thumper.GetPeerID().String()},
		},
	}

	// Compile both nodes into configs whose IDs the node types declared.
	compiled := make(map[string]map[string]config.Config)
	for _, node := range []*s4wave_flowgraph.PlacedFlowgraphNode{tcp, local} {
		nodeType := types[node.Node.GetTypeId()]
		entries, err := nodeType.Compile(node)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 {
			t.Fatalf("node %q compiled %d entries, want 1", node.NodeID, len(entries))
		}
		for name, conf := range entries {
			if !slices.Contains(nodeType.GetConfigIDs(), conf.GetConfigID()) {
				t.Fatalf("node %q entry %q has undeclared config ID %q", node.NodeID, name, conf.GetConfigID())
			}
		}
		compiled[node.NodeID] = entries
	}

	// Check that the two ends agree on the protocol and name each other.
	forwarding := compiled["tcp"]["forward"].(*stream_forwarding.Config)
	listening := compiled["local"]["forward"].(*stream_listening.Config)
	if forwarding.GetTargetMultiaddr() != "/ip4/127.0.0.1/tcp/8080" || listening.GetListenMultiaddr() != "/ip4/127.0.0.1/tcp/8080" {
		t.Fatalf("unexpected addresses %q and %q", forwarding.GetTargetMultiaddr(), listening.GetListenMultiaddr())
	}
	if forwarding.GetProtocolId() != listening.GetProtocolId() {
		t.Fatalf("protocols differ: %q and %q", forwarding.GetProtocolId(), listening.GetProtocolId())
	}
	if forwarding.GetPeerId() != thumper.GetPeerID().String() || listening.GetRemotePeerId() != thumper.GetPeerID().String() {
		t.Fatal("the Local Port must dial the Device that runs the TCP Port")
	}

	// Compile no entries once the connection is removed.
	local.Connections = nil
	entries, err := types[s4wave_flowgraph.LocalPortNodeTypeID].Compile(local)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("an unconnected Local Port compiled %d entries", len(entries))
	}
}
