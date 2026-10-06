//go:build !js

package webrtc_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
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

// restartReconnectBound is how long the surviving peer may take to link with
// a restarted peer. Without session replacement the survivor renegotiates its
// stale PeerConnection or ignores the request for a new one, and the link
// returns only after the ICE failure and QUIC idle timeouts, over 30 s.
const restartReconnectBound = 15 * time.Second

// TestTransportPeerRestartReconnects verifies that two peers link again within
// a few seconds after either one restarts. The restarted transport goes
// silent on its old address, as a crashed process does, and a fresh transport
// with the same peer identity starts on a new address. pion keeps its default
// ICE timeouts so a stale PeerConnection cannot fail over quickly.
//
// In the reconnect cases the surviving peer first replaces its signaling
// client while the link stays up, as a hosted signaling websocket does when
// it reconnects. The survivor must then signal the restarted peer through the
// new client instead of the closed one.
func TestTransportPeerRestartReconnects(t *testing.T) {
	for _, restart := range []int{0, 2} {
		for _, reconnect := range []bool{false, true} {
			name := fmt.Sprintf("restart-p%d", restart)
			if reconnect {
				name += "-after-signaling-reconnect"
			}
			t.Run(name, func(t *testing.T) {
				testTransportPeerRestart(t, restart, reconnect)
			})
		}
	}
}

// testTransportPeerRestart links p0 and p2 over WebRTC, optionally restarts
// the signaling client of the surviving peer, restarts the transport of peer
// restart, and requires the link to return in time.
func testTransportPeerRestart(t *testing.T, restart int, reconnectSignaling bool) {
	// Configure logging for transport negotiation and recovery.
	ctx := t.Context()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Route the ICE data path over a vnet with an address for each transport
	// the test runs: both initial transports and the restarted one.
	iceNet := newICENetwork(t, 3)

	// Construct the endpoint peers and the signaling server between them.
	g := graph.NewGraph()
	addPeer := func() *graph.Peer {
		p, err := graph.GenerateAddPeer(ctx, g)
		if err != nil {
			t.Fatal(err.Error())
		}
		return p
	}
	p0, p1, p2 := addPeer(), addPeer(), addPeer()
	p1.AddFactory(func(b bus.Bus) controller.Factory { return signaling_server.NewFactory(b) })
	p1.AddConfig("signaling-server", &signaling_server.Config{
		Server: &stream_srpc_server.Config{
			PeerIds:     []string{p1.GetPeerID().String()},
			ProtocolIds: []string{string(signaling.ProtocolID)},
		},
	})
	signalingID := "webrtc-signaling"
	signalClientConf := &signaling_rpc_client.Config{
		SignalingId: signalingID,
		Client: &stream_srpc_client.Config{
			ServerPeerIds: []string{p1.GetPeerID().String()},
		},
	}

	// Connect both endpoint peers to the signaling server.
	lan1 := graph.AddLAN(g)
	lan1.AddPeer(g, p0)
	lan1.AddPeer(g, p1)
	lan2 := graph.AddLAN(g)
	lan2.AddPeer(g, p1)
	lan2.AddPeer(g, p2)
	sim := initSimulator(t, ctx, le, g)
	px := []*simulate.Peer{sim.GetPeerByID(p0.GetPeerID()), nil, sim.GetPeerByID(p2.GetPeerID())}

	// Configure the WebRTC transport both endpoint peers run.
	webrtcTptConf := &webrtc.Config{
		SignalingId: signalingID,
		AllPeers:    true,
		BlockPeers:  []string{p1.GetPeerID().String()},
		Verbose:     true,
	}

	// startTransport runs a WebRTC transport for peer i and returns a function
	// that crashes it.
	startTransport := func(i int) func() {
		return iceNet.startTransport(ctx, t, le, px[i].GetTestbed().Bus, webrtcTptConf)
	}
	crash := []func(){startTransport(0), nil, startTransport(2)}

	// startSignaling runs a signaling client for peer i and returns a
	// function that stops it. Stopping removes the client's SignalPeer
	// values, and its sessions never deliver again.
	startSignaling := func(i int) func() {
		// Construct and run the signaling client controller on the peer bus.
		t.Helper()
		b := px[i].GetTestbed().Bus
		ctrl, err := signaling_rpc_client.NewFactory(b).Construct(
			ctx,
			signalClientConf,
			controller.ConstructOpts{Logger: le.WithField("signaling-client", i)},
		)
		if err != nil {
			t.Fatal(err.Error())
		}
		sigCtx, sigCancel := context.WithCancel(ctx)
		go func() { _ = b.ExecuteController(sigCtx, ctrl) }()
		return sigCancel
	}
	stopSignaling := []func(){startSignaling(0), nil, startSignaling(2)}
	defer func() {
		for _, stop := range stopSignaling {
			if stop != nil {
				stop()
			}
		}
	}()

	// Hold a link from each endpoint to the other, as both daemons do.
	for _, ids := range [][2]*graph.Peer{{p0, p2}, {p2, p0}} {
		_, ref, err := sim.GetPeerByID(ids[0].GetPeerID()).GetTestbed().Bus.AddDirective(
			link.NewEstablishLinkWithPeer(ids[0].GetPeerID(), ids[1].GetPeerID()),
			nil,
		)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer ref.Release()
	}

	// waitStage requires the endpoints to link within timeout.
	waitStage := func(stage string, timeout time.Duration) {
		t.Helper()
		waitConnectivity(ctx, t, le, px[0], px[2], stage, timeout)
	}
	waitStage("initial", 30*time.Second)

	// Replace the signaling client of the surviving peer. The established
	// link does not depend on signaling and stays up.
	if reconnectSignaling {
		survivor := 2 - restart
		le.Infof("reconnecting the signaling client of p%d", survivor)
		stopSignaling[survivor]()
		stopSignaling[survivor] = startSignaling(survivor)
		waitStage("after-signaling-reconnect", restartReconnectBound)
	}

	// Crash the transport of the restarted peer and start its successor.
	le.Infof("restarting the transport of p%d", restart)
	crash[restart]()
	crash[restart] = startTransport(restart)
	waitStage("after-restart", restartReconnectBound)
	for _, c := range crash {
		if c != nil {
			c()
		}
	}
}

// waitConnectivity probes the link between a and b until it carries a stream
// or the stage bound passes. Each probe is bounded so a probe over a stale link
// cannot hold the stage past the moment the new link comes up.
func waitConnectivity(
	ctx context.Context,
	t *testing.T,
	le *logrus.Entry,
	a, b *simulate.Peer,
	stage string,
	timeout time.Duration,
) {
	// Bound the stage from now.
	t.Helper()
	start := time.Now()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	var lastErr error
	for {
		probeCtx, cancelProbe := context.WithTimeout(ctx, 2*time.Second)
		err := simulate.TestConnectivity(probeCtx, a, b)
		cancelProbe()
		if err == nil {
			le.Infof("connectivity ok: %s after %v", stage, time.Since(start))
			return
		}
		lastErr = err
		select {
		case <-ctx.Done():
			t.Fatalf("%s: context done: %v", stage, ctx.Err())
		case <-deadline.C:
			t.Fatalf("%s: connectivity not restored in %v: %v", stage, timeout, lastErr)
		case <-time.After(250 * time.Millisecond):
		}
	}
}
