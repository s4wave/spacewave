//go:build !js

package webrtc_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	signaling "github.com/s4wave/spacewave/net/signaling/rpc"
	signaling_rpc_client "github.com/s4wave/spacewave/net/signaling/rpc/client"
	signaling_server "github.com/s4wave/spacewave/net/signaling/rpc/server"
	"github.com/s4wave/spacewave/net/sim/graph"
	stream_srpc_client "github.com/s4wave/spacewave/net/stream/srpc/client"
	stream_srpc_server "github.com/s4wave/spacewave/net/stream/srpc/server"
	webrtc "github.com/s4wave/spacewave/net/transport/webrtc"
	"github.com/sirupsen/logrus"
)

// msgSessionEnded is the message logged when a session ends.
const msgSessionEnded = "webrtc session ended"

// failOnceICEProvider refuses the first credential request for one peer, as a
// failed relay credential fetch does, and serves direct-only config after.
type failOnceICEProvider struct {
	// peerID restricts the provider to the dialing peer.
	peerID peer.ID
	// calls counts credential requests.
	calls atomic.Int32
}

// GetControllerInfo identifies the fixture.
func (p *failOnceICEProvider) GetControllerInfo() *controller.Info {
	return controller.NewInfo("test/ice-fail-once", controller.MustParseVersion("0.0.1"), "ICE fails once")
}

// Execute leaves the fixture attached until cleanup.
func (p *failOnceICEProvider) Execute(context.Context) error { return nil }

// Close releases nothing.
func (p *failOnceICEProvider) Close() error { return nil }

// HandleDirective serves the provider to its peer only.
func (p *failOnceICEProvider) HandleDirective(_ context.Context, inst directive.Instance) ([]directive.Resolver, error) {
	d, ok := inst.GetDirective().(webrtc.LookupICEProvider)
	if !ok || d.ICEPeerID() != p.peerID {
		return nil, nil
	}
	return directive.Resolvers(directive.NewValueResolver([]webrtc.ICEProvider{p})), nil
}

// ICEConfig refuses the first request.
func (p *failOnceICEProvider) ICEConfig(context.Context) (*webrtc.WebRtcConfig, error) {
	if p.calls.Add(1) == 1 {
		return nil, errors.New("could not reach account service; try again")
	}
	return &webrtc.WebRtcConfig{}, nil
}

// TestTransportRetriesFailedConstruction verifies that a session that could
// not be constructed does not spend the presence epoch. p0's first credential
// fetch fails while p2 stays present and does not dial. p0 links after a pause
// instead of waiting for p2 to rejoin.
func TestTransportRetriesFailedConstruction(t *testing.T) {
	// Count transport log messages by transport address.
	ctx := t.Context()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	counter := newLogCounter()
	log.AddHook(counter)
	le := logrus.NewEntry(log)
	iceNet := newICENetwork(t, 2)
	ip0 := iceNet.ip(0)

	// Construct the endpoint peers and the signaling server p1 between them.
	g := graph.NewGraph()
	addPeer := func() *graph.Peer {
		p, err := graph.GenerateAddPeer(ctx, g)
		if err != nil {
			t.Fatal(err.Error())
		}
		return p
	}
	p0, p1, p2 := addPeer(), addPeer(), addPeer()

	// Run the signaling server on p1, reachable from both endpoint peers.
	p1.AddFactory(func(b bus.Bus) controller.Factory { return signaling_server.NewFactory(b) })
	p1.AddConfig("signaling-server", &signaling_server.Config{
		Server: &stream_srpc_server.Config{
			PeerIds:     []string{p1.GetPeerID().String()},
			ProtocolIds: []string{string(signaling.ProtocolID)},
		},
	})

	// Connect both endpoint peers to the signaling server.
	lan1 := graph.AddLAN(g)
	lan1.AddPeer(g, p0)
	lan1.AddPeer(g, p1)
	lan2 := graph.AddLAN(g)
	lan2.AddPeer(g, p1)
	lan2.AddPeer(g, p2)

	// Start the simulated peers and take the endpoint buses.
	sim := initSimulator(t, ctx, le, g)
	b0 := sim.GetPeerByID(p0.GetPeerID()).GetTestbed().Bus
	b2 := sim.GetPeerByID(p2.GetPeerID()).GetTestbed().Bus

	// p0's first credential fetch fails.
	provider := &failOnceICEProvider{peerID: p0.GetPeerID()}
	releaseProvider, err := b0.AddController(ctx, provider, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseProvider)

	// Run both transports and signaling clients.
	signalingID := "webrtc-signaling"
	conf := &webrtc.Config{
		SignalingId: signalingID,
		AllPeers:    true,
		BlockPeers:  []string{p1.GetPeerID().String()},
		Verbose:     true,
	}
	for _, b := range []bus.Bus{b0, b2} {
		crash := iceNet.startTransport(ctx, t, le, b, conf, webrtc.WithICETimeouts(time.Second, 2*time.Second, 300*time.Millisecond))
		t.Cleanup(crash)
		ctrl, err := signaling_rpc_client.NewFactory(b).Construct(ctx, &signaling_rpc_client.Config{
			SignalingId: signalingID,
			Client:      &stream_srpc_client.Config{ServerPeerIds: []string{p1.GetPeerID().String()}},
		}, controller.ConstructOpts{Logger: le})
		if err != nil {
			t.Fatal(err)
		}
		sigCtx, stop := context.WithCancel(ctx)
		go func() { _ = b.ExecuteController(sigCtx, ctrl) }()
		t.Cleanup(stop)
	}

	// Only p0 holds the link. Its first session fails to construct, and a
	// later one in the same presence epoch links.
	_, ref, err := b0.AddDirective(ctx, link.NewEstablishLinkWithPeer(p0.GetPeerID(), p2.GetPeerID()), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ref.Release)
	counter.wait(t, ip0, msgSessionEnded, 1, 30*time.Second)
	counter.wait(t, ip0, msgLinkPublished, 1, 30*time.Second)
	if provider.calls.Load() < 2 {
		t.Fatal("the link came up without retrying the failed construction")
	}
}
