//go:build !js

package webrtc_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/pion/logging"
	"github.com/pion/transport/v5/vnet"
	"github.com/s4wave/spacewave/net/link"
	signaling "github.com/s4wave/spacewave/net/signaling/rpc"
	signaling_rpc_client "github.com/s4wave/spacewave/net/signaling/rpc/client"
	signaling_server "github.com/s4wave/spacewave/net/signaling/rpc/server"
	"github.com/s4wave/spacewave/net/sim/graph"
	"github.com/s4wave/spacewave/net/sim/simulate"
	stream_srpc_client "github.com/s4wave/spacewave/net/stream/srpc/client"
	stream_srpc_server "github.com/s4wave/spacewave/net/stream/srpc/server"
	webrtc "github.com/s4wave/spacewave/net/transport/webrtc"
	"github.com/sirupsen/logrus"
)

// TestTransportReconnect verifies the webrtc transport recovers connectivity
// after the peer-to-peer ICE data path fails while signaling stays alive.
//
// pion/ice runs over a vnet whose chunk filter can drop all traffic on demand,
// while the signaling RPC continues over the in-process transport. This mirrors
// the browser e2e where the relay signaling survives but ICE consent fails under
// CPU starvation: the connection enters "failed", the session restarts, and the
// renegotiation must re-establish the link once the path returns.
func TestTransportReconnect(t *testing.T) {
	// Use the test lifetime for the simulated peer network.
	ctx := t.Context()

	// Configure logging for transport negotiation and recovery.
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// vnet carrying the pion/ice data path between p0 and p2.
	var severed atomic.Bool
	router, err := vnet.NewRouter(&vnet.RouterConfig{
		CIDR:          "10.0.0.0/24",
		LoggerFactory: logging.NewDefaultLoggerFactory(),
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	router.AddChunkFilter(func(vnet.Chunk) bool { return !severed.Load() })

	// Attach both peer ICE networks to the virtual router.
	iceNet0, err := vnet.NewNet(&vnet.NetConfig{StaticIPs: []string{"10.0.0.10"}})
	if err != nil {
		t.Fatal(err.Error())
	}
	iceNet2, err := vnet.NewNet(&vnet.NetConfig{StaticIPs: []string{"10.0.0.20"}})
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := router.AddNet(iceNet0); err != nil {
		t.Fatal(err.Error())
	}
	if err := router.AddNet(iceNet2); err != nil {
		t.Fatal(err.Error())
	}
	if err := router.Start(); err != nil {
		t.Fatal(err.Error())
	}
	defer func() { _ = router.Stop() }()

	// Short ICE timeouts so a severed path reaches "failed" within a few seconds.
	iceTimeouts := webrtc.WithICETimeouts(time.Second, 2*time.Second, 300*time.Millisecond)

	// Construct the simulated peer graph and peer creation helper.
	g := graph.NewGraph()
	addPeer := func() *graph.Peer {
		p, err := graph.GenerateAddPeer(ctx, g)
		if err != nil {
			t.Fatal(err.Error())
		}
		return p
	}

	// Configure the initiating peer with its virtual ICE network.
	p0 := addPeer()
	p0.AddFactory(func(b bus.Bus) controller.Factory { return signaling_rpc_client.NewFactory(b) })
	p0.AddFactory(func(b bus.Bus) controller.Factory {
		return webrtc.NewFactory(b, webrtc.WithICENet(iceNet0), iceTimeouts)
	})

	// Configure the intermediate peer as the signaling server.
	p1 := addPeer()
	p1.AddFactory(func(b bus.Bus) controller.Factory { return signaling_server.NewFactory(b) })
	p1.AddConfig("signaling-server", &signaling_server.Config{
		Server: &stream_srpc_server.Config{
			PeerIds:     []string{p1.GetPeerID().String()},
			ProtocolIds: []string{string(signaling.ProtocolID)},
		},
	})

	// Configure the remote peer with its virtual ICE network.
	p2 := addPeer()
	p2.AddFactory(func(b bus.Bus) controller.Factory {
		return webrtc.NewFactory(b, webrtc.WithICENet(iceNet2), iceTimeouts)
	})
	p2.AddFactory(func(b bus.Bus) controller.Factory { return signaling_rpc_client.NewFactory(b) })

	// Bind both peer signaling clients to the intermediate server.
	signalingID := "webrtc-signaling"
	signalClientConf := &signaling_rpc_client.Config{
		SignalingId: signalingID,
		Client: &stream_srpc_client.Config{
			ServerPeerIds: []string{p1.GetPeerID().String()},
		},
	}
	p0.AddConfig("signaling-client", signalClientConf)
	p2.AddConfig("signaling-client", signalClientConf)

	// Enable WebRTC dialing between the two endpoint peers.
	webrtcTptConf := &webrtc.Config{
		SignalingId: signalingID,
		AllPeers:    true,
		BlockPeers:  []string{p1.GetPeerID().String()},
		Verbose:     true,
	}
	p0.AddConfig("webrtc-tpt", webrtcTptConf)
	p2.AddConfig("webrtc-tpt", webrtcTptConf)

	// Connect the initiating peer to the signaling server LAN.
	lan1 := graph.AddLAN(g)
	lan1.AddPeer(g, p0)
	lan1.AddPeer(g, p1)

	// Connect the remote peer to the signaling server LAN.
	lan2 := graph.AddLAN(g)
	lan2.AddPeer(g, p1)
	lan2.AddPeer(g, p2)

	// Start the peer simulator for the configured network.
	sim := initSimulator(t, ctx, le, g)

	// Resolve the endpoint peers for connectivity checks.
	px0 := sim.GetPeerByID(p0.GetPeerID())
	px2 := sim.GetPeerByID(p2.GetPeerID())

	// Hold a persistent EstablishLink so the session tracker stays alive across
	// the data-path drop (provides the persistent dial reference).
	_, esRef, err := px0.GetTestbed().Bus.AddDirective(
		link.NewEstablishLinkWithPeer(p0.GetPeerID(), p2.GetPeerID()),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer esRef.Release()

	// connectivityProbeTimeout bounds one TestConnectivity probe. A probe over a
	// not-yet-reestablished link blocks in ExecWaitValue on the establish
	// directive until its context is done; without a per-probe bound the probe
	// inherits the whole-test context and blocks for the entire test timeout, so
	// the retry loop never re-probes to catch the moment recovery completes.
	const connectivityProbeTimeout = 5 * time.Second
	waitConnectivity := func(stage string, timeout time.Duration) {
		// Bound connectivity probes by the stage deadline and report recovery failures.
		t.Helper()
		deadline := time.NewTimer(timeout)
		defer deadline.Stop()
		var lastErr error
		for {
			probeCtx, cancelProbe := context.WithTimeout(ctx, connectivityProbeTimeout)
			err := simulate.TestConnectivity(probeCtx, px0, px2)
			cancelProbe()
			if err == nil {
				le.Infof("connectivity ok: %s", stage)
				return
			}
			lastErr = err
			select {
			case <-ctx.Done():
				t.Fatalf("%s: context done: %v", stage, ctx.Err())
			case <-deadline.C:
				t.Fatalf("%s: connectivity not restored in %v: %v", stage, timeout, lastErr)
			case <-time.After(500 * time.Millisecond):
			}
		}
	}

	// 1. Initial connectivity over the vnet ICE path.
	waitConnectivity("initial", 30*time.Second)

	// 2. Sever the ICE data path; signaling via p1 stays up.
	le.Info("severing ICE data path")
	severed.Store(true)

	// 3. Wait for pion to mark the connection failed and the session to restart.
	select {
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	case <-time.After(8 * time.Second):
	}
	le.Info("restoring ICE data path")

	// 4. Restore the path and assert recovery.
	severed.Store(false)
	waitConnectivity("after-reconnect", 60*time.Second)
	le.Info("reconnect test successful")
}
