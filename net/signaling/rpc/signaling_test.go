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
	ctx := t.Context()

	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	g := graph.NewGraph()
	addPeer := func(ctx context.Context, t *testing.T, g *graph.Graph) *graph.Peer {
		p, err := graph.GenerateAddPeer(ctx, g)
		if err != nil {
			t.Fatal(err.Error())
		}
		return p
	}

	// descrip := `p0 <-> p1 <-> p2`

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

	p2 := addPeer(ctx, t, g)

	lan1 := graph.AddLAN(g)
	lan1.AddPeer(g, p0)
	lan1.AddPeer(g, p1)

	lan2 := graph.AddLAN(g)
	lan2.AddPeer(g, p2)
	lan2.AddPeer(g, p1)

	sim := initSimulator(
		t,
		ctx,
		le,
		g,
		// simulate.WithVerbose(),
	)

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

	p0SignalClient.SetContext(p0Sim.GetTestbed().Context)
	p0SignalClient.SetListenHandler(func(ctx context.Context, reset, added bool, pid string) {
		le.Debugf("p0: listen handler called: reset(%v) added(%v) pid(%v)", reset, added, pid)
	})

	// track which remote peers want a session with p2
	p2SignalClient.SetContext(p2Sim.GetTestbed().Context)
	gotMsg := make(chan string)
	p2SignalClient.SetListenHandler(func(ctx context.Context, reset, added bool, pid string) {
		le.Debugf("p2: listen handler called: reset(%v) added(%v) pid(%v)", reset, added, pid)

		// For this simple test assume this is an added event.
		if !added || reset {
			return
		}

		// Add peer ref
		ref := p2SignalClient.AddPeerRef(pid)
		_ = ref

		// Recv
		go func() {
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

			dataStr := string(sm.GetSignedMsg().GetData())
			le.Infof("p2: got message from peer %v: %v", pid.String(), dataStr)
			gotMsg <- dataStr
			ref.Release()
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

	le.Info("tests successful")
}

// TestSignaling_ClientController tests the signaling client controller.
func TestSignaling_ClientController(t *testing.T) {
	ctx := t.Context()

	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	g := graph.NewGraph()
	addPeer := func(ctx context.Context, t *testing.T, g *graph.Graph) *graph.Peer {
		p, err := graph.GenerateAddPeer(ctx, g)
		if err != nil {
			t.Fatal(err.Error())
		}
		return p
	}

	// descrip := `p0 <-> p1 <-> p2`

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

	lan1 := graph.AddLAN(g)
	lan1.AddPeer(g, p0)
	lan1.AddPeer(g, p1)

	lan2 := graph.AddLAN(g)
	lan2.AddPeer(g, p2)
	lan2.AddPeer(g, p1)

	sim := initSimulator(
		t,
		ctx,
		le,
		g,
		// simulate.WithVerbose(),
	)

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

	le.Info("tests successful")
}
