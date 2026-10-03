package signaling_rpc_test

import (
	"bytes"
	"context"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/net/signaling"
	signaling_echo "github.com/s4wave/spacewave/net/signaling/echo"
	signaling_rpc "github.com/s4wave/spacewave/net/signaling/rpc"
	signaling_client "github.com/s4wave/spacewave/net/signaling/rpc/client"
	signaling_server "github.com/s4wave/spacewave/net/signaling/rpc/server"
	"github.com/s4wave/spacewave/net/sim/graph"

	// "github.com/s4wave/spacewave/net/sim/simulate"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/s4wave/spacewave/net/sim/tests"
	stream_srpc_client "github.com/s4wave/spacewave/net/stream/srpc/client"
	stream_srpc_server "github.com/s4wave/spacewave/net/stream/srpc/server"
	"github.com/sirupsen/logrus"
)

var initSimulator = tests.InitSimulator

// TestSignaling tests the signaling server and client end to end.
func TestSignaling(t *testing.T) {
	// Use the test context for the signaling simulation.
	ctx := t.Context()

	// Create a debug logger for the signaling peers.
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Create the peer graph and its peer generator.
	g := graph.NewGraph()
	addPeer := func(ctx context.Context, t *testing.T, g *graph.Graph) *graph.Peer {
		p, err := graph.GenerateAddPeer(ctx, g)
		if err != nil {
			t.Fatal(err.Error())
		}
		return p
	}

	// descrip := `p0 <-> p1 <-> p2`

	// Create the initiating peer in the signaling graph.
	p0 := addPeer(ctx, t, g)

	// Create the signaling server peer.
	p1 := addPeer(ctx, t, g)
	p1.AddFactory(func(b bus.Bus) controller.Factory {
		return signaling_server.NewFactory(b)
	})
	p1.AddConfig("signaling-server", &signaling_server.Config{
		Server: &stream_srpc_server.Config{
			PeerIds:     []string{p1.GetPeerID().String()},
			ProtocolIds: []string{string(signaling_rpc.ProtocolID)},
		},
	})

	// Configure the peers that will contact each other via the signaling server.
	srpcClientConf := &stream_srpc_client.Config{
		ServerPeerIds: []string{p1.GetPeerID().String()},
	}

	// Create the receiving peer in the signaling graph.
	p2 := addPeer(ctx, t, g)

	// Connect the initiating peer to the signaling server on one LAN.
	lan1 := graph.AddLAN(g)
	lan1.AddPeer(g, p0)
	lan1.AddPeer(g, p1)

	// Connect the receiving peer to the signaling server on another LAN.
	lan2 := graph.AddLAN(g)
	lan2.AddPeer(g, p2)
	lan2.AddPeer(g, p1)

	// Start the signaling peer simulation.
	sim := initSimulator(
		t,
		ctx,
		le,
		g,
		// simulate.WithVerbose(),
	)

	// Locate the initiating and receiving simulated peers.
	p0Sim, p2Sim := sim.GetPeerByID(p0.GetPeerID()), sim.GetPeerByID(p2.GetPeerID())

	// Attempt to signal between the two peers.
	p0SignalClient, err := signaling_client.NewClientWithBus(
		le.WithField("sim-peer", "0"),
		p0Sim.GetTestbed().Bus,
		p0Sim.GetPeerPriv(),
		srpcClientConf,
		signaling_rpc.ProtocolID,
		"",
	)
	if err != nil {
		t.Fatal(err)
	}

	// Create the receiving peer signaling client.
	p2SignalClient, err := signaling_client.NewClientWithBus(
		le.WithField("sim-peer", "2"),
		p2Sim.GetTestbed().Bus,
		p2Sim.GetPeerPriv(),
		srpcClientConf,
		signaling_rpc.ProtocolID,
		"",
	)
	if err != nil {
		t.Fatal(err)
	}

	// Attach the initiating client to its simulated peer lifecycle.
	p0SignalClient.SetContext(p0Sim.GetTestbed().Context)
	p0SignalClient.SetListenHandler(func(ctx context.Context, reset, added bool, pid string) {
		le.Debugf("p0: listen handler called: reset(%v) added(%v) pid(%v)", reset, added, pid)
	})

	// track which remote peers want a session with p2
	p2SignalClient.SetContext(p2Sim.GetTestbed().Context)
	gotMsg := make(chan string)
	p2SignalClient.SetListenHandler(func(ctx context.Context, reset, added bool, pid string) {
		// Record the receiving client session notification.
		le.Debugf("p2: listen handler called: reset(%v) added(%v) pid(%v)", reset, added, pid)

		// For this simple test assume this is an added event.
		if !added || reset {
			return
		}

		// Add a reference to the announced peer.
		ref := p2SignalClient.AddPeerRef(pid)

		// Receive the peer's message in the background.
		go func() {
			// Release the peer reference when the receive ends.
			defer ref.Release()

			// Receive and verify the incoming signed signaling message.
			sm, err := ref.Recv(ctx)
			if err != nil {
				le.WithError(err).Error("unable to recv message")
				return
			}
			_, pid, err := sm.ExtractAndVerify()
			if err != nil {
				le.WithError(err).Error("got invalid message")
				return
			}

			// Publish the received payload.
			dataStr := string(sm.GetSignedMsg().GetData())
			le.Infof("p2: got message from peer %v: %v", pid.String(), dataStr)
			gotMsg <- dataStr
		}()
	})

	// Initiate the connection on p0.
	p0Ref := p0SignalClient.AddPeerRef(p2Sim.GetPeerID().String())
	defer p0Ref.Release()

	// Send waits until the remote acks the message before returning.
	_, err = p0Ref.Send(ctx, []byte("hello from p0"))
	if err != nil {
		t.Fatal(err.Error())
	}

	// Wait for the message on p2.
	select {
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	case msg := <-gotMsg:
		t.Logf("transferred message successfully via signaling: %v", msg)
	}

	// Record successful signaling message transfer.
	le.Info("tests successful")
}

// TestSignaling_ClientController tests the signaling client controller.
func TestSignaling_ClientController(t *testing.T) {
	// Use the test context for the signaling controller simulation.
	ctx := t.Context()

	// Create a debug logger for the signaling controllers.
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Create the controller peer graph and its peer generator.
	g := graph.NewGraph()
	addPeer := func(ctx context.Context, t *testing.T, g *graph.Graph) *graph.Peer {
		p, err := graph.GenerateAddPeer(ctx, g)
		if err != nil {
			t.Fatal(err.Error())
		}
		return p
	}

	// descrip := `p0 <-> p1 <-> p2`

	// Create the initiating peer with a signaling client factory.
	p0 := addPeer(ctx, t, g)
	p0.AddFactory(func(b bus.Bus) controller.Factory {
		return signaling_client.NewFactory(b)
	})

	// Create the signaling server peer.
	p1 := addPeer(ctx, t, g)
	p1.AddFactory(func(b bus.Bus) controller.Factory {
		return signaling_server.NewFactory(b)
	})
	p1.AddConfig("signaling-server", &signaling_server.Config{
		Server: &stream_srpc_server.Config{
			PeerIds:     []string{p1.GetPeerID().String()},
			ProtocolIds: []string{string(signaling_rpc.ProtocolID)},
		},
	})

	// Create the receiving peer with signaling and echo factories.
	p2 := addPeer(ctx, t, g)
	p2.AddFactory(func(b bus.Bus) controller.Factory {
		return signaling_client.NewFactory(b)
	})
	p2.AddFactory(func(b bus.Bus) controller.Factory {
		return signaling_echo.NewFactory(b)
	})

	// Configure the signaling clients
	signalingID := "signaling-test"
	signalingClientConf := &signaling_client.Config{
		SignalingId: signalingID,
		Client: &stream_srpc_client.Config{
			ServerPeerIds: []string{p1.GetPeerID().String()},
		},
	}
	p0.AddConfig("signaling-client", signalingClientConf.CloneVT())
	p2.AddConfig("signaling-client", signalingClientConf.CloneVT())
	p2.AddConfig("signaling-echo", &signaling_echo.Config{SignalingId: signalingID})

	// Connect the initiating peer to the signaling server on one LAN.
	lan1 := graph.AddLAN(g)
	lan1.AddPeer(g, p0)
	lan1.AddPeer(g, p1)

	// Connect the receiving peer to the signaling server on another LAN.
	lan2 := graph.AddLAN(g)
	lan2.AddPeer(g, p2)
	lan2.AddPeer(g, p1)

	// Start the signaling controller simulation.
	sim := initSimulator(
		t,
		ctx,
		le,
		g,
		// simulate.WithVerbose(),
	)

	// Locate the initiating and echoing simulated peers.
	p0Sim, p2Sim := sim.GetPeerByID(p0.GetPeerID()), sim.GetPeerByID(p2.GetPeerID())

	// Attempt to signal between the two peers.
	p0Handle, p0HandleRel, err := signaling.ExSignalPeer(
		ctx,
		p0Sim.GetTestbed().Bus,
		signalingID,
		p0Sim.GetPeerID(),
		p2Sim.GetPeerID(),
		false,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer p0HandleRel()

	// Send a message
	txMsg := []byte("hello world from p0")
	if err := p0Handle.Send(ctx, slices.Clone(txMsg)); err != nil {
		t.Fatal(err.Error())
	}

	// Recv the echo
	recvMsg, err := p0Handle.Recv(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !bytes.Equal(txMsg, recvMsg) {
		t.Fatalf("unexpected rx message: %v", string(recvMsg))
	}

	// Record successful signaling echo transfer.
	le.Info("tests successful")
}
