//go:build test_examples

package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/s4wave/spacewave/net/sim/graph"
	"github.com/s4wave/spacewave/net/sim/simulate"
	"github.com/s4wave/spacewave/net/stream"
	stream_echo "github.com/s4wave/spacewave/net/stream/echo"
	"github.com/sirupsen/logrus"
)

// TestDirectChat tests peer-to-peer messaging between directly connected peers.
// This demonstrates the core functionality of opening a stream and sending messages.
func TestDirectChat(t *testing.T) {
	ctx := t.Context()

	log := logrus.New()
	log.SetLevel(logrus.WarnLevel)
	le := logrus.NewEntry(log)

	// Create two peers on the same LAN
	g := graph.NewGraph()

	p0 := addPeerWithChat(t, g)
	p1 := addPeerWithChat(t, g)

	lan := graph.AddLAN(g)
	lan.AddPeer(g, p0)
	lan.AddPeer(g, p1)

	// Start simulator
	sim := initSimulator(t, ctx, le, g)
	defer sim.Close()

	// Verify connectivity
	le.Info("Testing peer-to-peer connectivity...")
	if err := simulate.TestConnectivity(ctx, sim.GetPeerByID(p0.GetPeerID()), sim.GetPeerByID(p1.GetPeerID())); err != nil {
		t.Fatalf("p0-p1 connectivity failed: %v", err)
	}
	le.Info("p0 <-> p1: connected")

	// Test: p0 sends message to p1
	chatProtocol := protocol.ID("demo/chat/v1")
	msg := []byte("Hello from p0 to p1!")

	p0Peer := sim.GetPeerByID(p0.GetPeerID())
	p0tb := p0Peer.GetTestbed()

	// Open stream from p0 to p1
	ms, msRel, err := link.OpenStreamWithPeerEx(
		ctx,
		p0tb.Bus,
		chatProtocol,
		p0.GetPeerID(),
		p1.GetPeerID(),
		0,
		stream.OpenOpts{},
	)
	if err != nil {
		t.Fatalf("failed to open stream p0->p1: %v", err)
	}
	defer msRel()

	// Send message
	if _, err := ms.GetStream().Write(msg); err != nil {
		t.Fatalf("failed to write message: %v", err)
	}
	le.Infof("Message sent: %s", string(msg))

	// Read echoed response (echo protocol echoes back)
	resp := make([]byte, len(msg)+100)
	n, err := ms.GetStream().Read(resp)
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}

	resp = resp[:n]
	if !bytes.Contains(resp, msg) {
		t.Errorf("expected response to contain %q, got %q", msg, resp)
	}

	le.Info("Direct chat test passed - P2P messaging works!")
}

// TestMultiPeerTopology tests communication in a multi-peer network.
func TestMultiPeerTopology(t *testing.T) {
	ctx := t.Context()

	log := logrus.New()
	log.SetLevel(logrus.WarnLevel)
	le := logrus.NewEntry(log)

	// Build a star topology: center connected to 3 peers
	g := graph.NewGraph()

	center := addPeerWithChat(t, g)
	peers := make([]*graph.Peer, 3)
	for i := range peers {
		peers[i] = addPeerWithChat(t, g)
	}

	// All on same LAN
	lan := graph.AddLAN(g)
	lan.AddPeer(g, center)
	for _, p := range peers {
		lan.AddPeer(g, p)
	}

	sim := initSimulator(t, ctx, le, g)
	defer sim.Close()

	// Test connectivity from center to all peers
	centerSim := sim.GetPeerByID(center.GetPeerID())
	for i, p := range peers {
		if err := simulate.TestConnectivity(ctx, centerSim, sim.GetPeerByID(p.GetPeerID())); err != nil {
			t.Fatalf("center-p%d connectivity failed: %v", i, err)
		}
		le.Infof("Center <-> Peer %d: connected", i)
	}

	le.Infof("Multi-peer topology test passed - %d peers connected", len(peers))
}

// TestChatProtocolConfig validates the chat protocol configuration.
func TestChatProtocolConfig(t *testing.T) {
	conf := &stream_echo.Config{
		ProtocolId: "demo/chat/v1",
	}

	if conf.GetProtocolId() != "demo/chat/v1" {
		t.Error("protocol ID mismatch")
	}

	if err := conf.Validate(); err != nil {
		t.Errorf("config validation failed: %v", err)
	}
}

// addPeerWithChat adds a peer with chat protocol handler.
func addPeerWithChat(t *testing.T, g *graph.Graph) *graph.Peer {
	ctx := context.Background()
	p, err := graph.GenerateAddPeer(ctx, g)
	if err != nil {
		t.Fatalf("failed to add peer: %v", err)
	}

	chatProtocol := protocol.ID("demo/chat/v1")

	// Use echo controller as chat handler
	p.AddFactory(func(b bus.Bus) controller.Factory {
		return stream_echo.NewFactory(b)
	})
	p.AddConfig("chat", &stream_echo.Config{
		ProtocolId: string(chatProtocol),
	})

	return p
}

// initSimulator creates a simulator.
func initSimulator(t *testing.T, ctx context.Context, le *logrus.Entry, g *graph.Graph) *simulate.Simulator {
	sim, err := simulate.NewSimulator(ctx, le, g)
	if err != nil {
		t.Fatalf("failed to create simulator: %v", err)
	}
	return sim
}
