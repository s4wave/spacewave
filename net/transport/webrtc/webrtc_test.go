//go:build !js

package webrtc_test

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	signaling "github.com/s4wave/spacewave/net/signaling/rpc"
	signaling_rpc_client "github.com/s4wave/spacewave/net/signaling/rpc/client"
	signaling_server "github.com/s4wave/spacewave/net/signaling/rpc/server"
	"github.com/s4wave/spacewave/net/sim/graph"
	"github.com/s4wave/spacewave/net/sim/simulate"
	"github.com/s4wave/spacewave/net/sim/tests"
	stream_srpc_client "github.com/s4wave/spacewave/net/stream/srpc/client"
	stream_srpc_server "github.com/s4wave/spacewave/net/stream/srpc/server"
	webrtc "github.com/s4wave/spacewave/net/transport/webrtc"
	"github.com/sirupsen/logrus"
)

var initSimulator = tests.InitSimulator

// TestTransport tests the webrtc transport end to end.
func TestTransport(t *testing.T) {
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

	descrip := `p0 <- [webrtc signal via p1] -> p2`

	// Create p0
	p0 := addPeer(ctx, t, g)
	p0.AddFactory(func(b bus.Bus) controller.Factory {
		return signaling_rpc_client.NewFactory(b)
	})
	p0.AddFactory(func(b bus.Bus) controller.Factory {
		return webrtc.NewFactory(b)
	})

	// Create the signaling server peer.
	p1 := addPeer(ctx, t, g)
	p1.AddFactory(func(b bus.Bus) controller.Factory {
		return signaling_server.NewFactory(b)
	})
	p1.AddConfig("signaling-server", &signaling_server.Config{
		Server: &stream_srpc_server.Config{
			PeerIds:     []string{p1.GetPeerID().String()},
			ProtocolIds: []string{string(signaling.ProtocolID)},
		},
	})

	// Create p2
	p2 := addPeer(ctx, t, g)
	p2.AddFactory(func(b bus.Bus) controller.Factory {
		return webrtc.NewFactory(b)
	})
	p2.AddFactory(func(b bus.Bus) controller.Factory {
		return signaling_rpc_client.NewFactory(b)
	})

	// Configure the peers that will contact each other via the signaling server.
	signalingID := "webrtc-signaling"
	signalClientConf := &signaling_rpc_client.Config{
		SignalingId: signalingID,
		Client: &stream_srpc_client.Config{
			ServerPeerIds: []string{p1.GetPeerID().String()},
		},
	}
	p0.AddConfig("signaling-client", signalClientConf)
	p2.AddConfig("signaling-client", signalClientConf)

	webrtcTptConf := &webrtc.Config{
		SignalingId: signalingID,
		AllPeers:    true,
		BlockPeers:  []string{p1.GetPeerID().String()},
		Verbose:     true,
	}
	p0.AddConfig("webrtc-tpt", webrtcTptConf)
	p2.AddConfig("webrtc-tpt", webrtcTptConf)

	lan1 := graph.AddLAN(g)
	lan1.AddPeer(g, p0)
	lan1.AddPeer(g, p1)

	lan2 := graph.AddLAN(g)
	lan2.AddPeer(g, p1)
	lan2.AddPeer(g, p2)

	sim := initSimulator(t, ctx, le, g)

	le.Infof("attempting to dial %v", descrip)
	if err := simulate.TestConnectivity(
		ctx,
		sim.GetPeerByID(p0.GetPeerID()),
		sim.GetPeerByID(p2.GetPeerID()),
	); err != nil {
		t.Fatal(err.Error())
	}

	le.Info("tests successful")
}
