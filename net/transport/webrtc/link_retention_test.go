//go:build !js

package webrtc

// link_retention_test.go pins the ownership of an established WebRTC link:
// the transport session owns the live PeerConnection and datachannel, not
// the signaling tracker generation. When the signaling directives unwind
// after establishment (the production path where HandleSignalPeer and
// SignalPeer instances are removed), tracker retirement must retain the
// established link and traffic must keep flowing.

import (
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/s4wave/spacewave/net/link"
	signaling "github.com/s4wave/spacewave/net/signaling/rpc"
	signaling_rpc_client "github.com/s4wave/spacewave/net/signaling/rpc/client"
	signaling_server "github.com/s4wave/spacewave/net/signaling/rpc/server"
	"github.com/s4wave/spacewave/net/sim/graph"
	"github.com/s4wave/spacewave/net/sim/simulate"
	"github.com/s4wave/spacewave/net/sim/tests"
	stream_srpc_client "github.com/s4wave/spacewave/net/stream/srpc/client"
	stream_srpc_server "github.com/s4wave/spacewave/net/stream/srpc/server"
	transport_quic "github.com/s4wave/spacewave/net/transport/common/quic"
	transport_controller "github.com/s4wave/spacewave/net/transport/controller"
	"github.com/sirupsen/logrus"
)

// TestLinkSurvivesSignalTrackerRetirement establishes a link between two
// peers over a signaling relay, stops the relay so both peers remove their
// SignalPeer / HandleSignalPeer directive instances, and asserts the
// established link is retained and still carries traffic.
func TestLinkSurvivesSignalTrackerRetirement(t *testing.T) {
	ctx := t.Context()

	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	g := graph.NewGraph()
	addPeer := func() *graph.Peer {
		p, err := graph.GenerateAddPeer(ctx, g)
		if err != nil {
			t.Fatal(err.Error())
		}
		return p
	}

	// p0 <- [webrtc signal via p1] -> p2
	p0 := addPeer()
	p0.AddFactory(func(b bus.Bus) controller.Factory {
		return signaling_rpc_client.NewFactory(b)
	})
	p0.AddFactory(func(b bus.Bus) controller.Factory {
		return NewFactory(b)
	})

	p1 := addPeer()
	p1.AddFactory(func(b bus.Bus) controller.Factory {
		return signaling_server.NewFactory(b)
	})
	p1.AddConfig("signaling-server", &signaling_server.Config{
		Server: &stream_srpc_server.Config{
			PeerIds:     []string{p1.GetPeerID().String()},
			ProtocolIds: []string{string(signaling.ProtocolID)},
		},
	})

	p2 := addPeer()
	p2.AddFactory(func(b bus.Bus) controller.Factory {
		return NewFactory(b)
	})
	p2.AddFactory(func(b bus.Bus) controller.Factory {
		return signaling_rpc_client.NewFactory(b)
	})

	signalingID := "webrtc-signaling"
	signalClientConf := &signaling_rpc_client.Config{
		SignalingId: signalingID,
		Client: &stream_srpc_client.Config{
			ServerPeerIds: []string{p1.GetPeerID().String()},
		},
	}
	p0.AddConfig("signaling-client", signalClientConf)
	p2.AddConfig("signaling-client", signalClientConf)

	webrtcTptConf := &Config{
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

	sim := tests.InitSimulator(t, ctx, le, g)

	px0 := sim.GetPeerByID(p0.GetPeerID())
	px2 := sim.GetPeerByID(p2.GetPeerID())

	// Hold a persistent EstablishLink so the transport keeps dialing while
	// the test inspects tracker state after retirement.
	_, esRef, err := px0.GetTestbed().Bus.AddDirective(
		link.NewEstablishLinkWithPeer(p0.GetPeerID(), p2.GetPeerID()),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer esRef.Release()

	if err := simulate.TestConnectivity(ctx, px0, px2); err != nil {
		t.Fatalf("initial connectivity failed: %v", err)
	}

	getTransport := func(px *simulate.Peer) *WebRTC {
		t.Helper()
		ctrl, _, rel, err := loader.WaitExecControllerRunningTyped[*transport_controller.Controller](
			ctx,
			px.GetTestbed().Bus,
			resolver.NewLoadControllerWithConfig(webrtcTptConf),
			nil,
		)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer rel.Release()
		tp, err := ctrl.GetTransport(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}
		tpt, ok := tp.(*WebRTC)
		if !ok {
			t.Fatalf("transport is %T, want *WebRTC", tp)
		}
		return tpt
	}
	tpt0 := getTransport(px0)
	remoteID := p2.GetPeerID().String()

	// Snapshot the established link this peer is riding.
	var linkDisposed bool
	var established *transport_quic.Link
	tpt0.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		if tkr, ok := tpt0.sessionTrackers.GetKey(remoteID); ok && tkr != nil {
			established = tkr.link
		}
	})
	if established == nil {
		t.Fatal("no established link found after initial connectivity")
	}

	// Force signal-tracker retirement on both peers: drop each peer's
	// signal-ingress lease exactly as HandleSignalPeer directive removal
	// does. With no other references, this retires the tracker generation.
	retireIngress := func(tpt *WebRTC, peerID string) {
		t.Helper()
		var resolver *handleSignalPeerResolver
		tpt.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			ingress := tpt.incomingSessions[peerID]
			if ingress == nil {
				return
			}
			for candidate := range ingress.resolvers {
				resolver = candidate
				break
			}
		})
		if resolver != nil {
			tpt.closeSignalIngress(peerID, resolver)
		}
	}
	retireIngress(tpt0, remoteID)
	retireIngress(getTransport(px2), p0.GetPeerID().String())

	// The established-link reference must survive tracker retirement. Give
	// the retirement cascade a bounded window to observe any disposal: the
	// original link must remain the tracker's published link throughout.
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		var waitCh <-chan struct{}
		tpt0.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			waitCh = getWaitCh()
		})
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-waitCh:
		case <-time.After(250 * time.Millisecond):
		}
		tpt0.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			tkr, ok := tpt0.sessionTrackers.GetKey(remoteID)
			if !ok || tkr == nil || tkr.link != established {
				linkDisposed = true
			}
		})
		if linkDisposed {
			break
		}
	}

	if linkDisposed {
		t.Fatal("established link was disposed when the signal tracker retired")
	}

	// Traffic must still flow over the retained link.
	if err := simulate.TestConnectivity(ctx, px0, px2); err != nil {
		t.Fatalf("post-retirement connectivity failed: %v", err)
	}
}
