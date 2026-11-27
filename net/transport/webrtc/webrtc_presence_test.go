//go:build !js

package webrtc_test

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/util/broadcast"
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

// msgLinkPublished, msgSessionStarted and msgLinkLost are the messages the
// presence test counts per transport.
const (
	msgLinkPublished  = "webrtc quic phase: link published"
	msgSessionStarted = "webrtc session starting"
	msgLinkLost       = "link lost/closed"
)

// unrefLinkLifetime outlasts the time an unreferenced link stays open before
// its holder closes it.
const unrefLinkLifetime = 12 * time.Second

// TestTransportLinksOnPresence verifies that a WebRTC link follows the
// presence of the remote peer on the signaling session instead of a redial
// loop.
//
// A link held only by p0 comes up once: p2 closes it after its unreferenced
// hold, and p0 does not dial again while p2 stays present. Once p2 also holds
// it, one more link comes up and stays. When p2 leaves, p0 starts no session,
// and when p2 returns the peers link once.
func TestTransportLinksOnPresence(t *testing.T) {
	// Count transport log messages by transport address.
	ctx := t.Context()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	counter := newLogCounter()
	log.AddHook(counter)
	le := logrus.NewEntry(log)

	// Route the ICE data path over a vnet with addresses for p0, p2 and the
	// restarted p2.
	iceNet := newICENetwork(t, 3)
	ip0, ip2, ip2Restarted := iceNet.ip(0), iceNet.ip(1), iceNet.ip(2)

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

	// Run the signaling server on p1 and point both signaling clients at it.
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

	// startTransport runs a WebRTC transport for peer i with short ICE
	// timeouts and returns a function that crashes it.
	webrtcTptConf := &webrtc.Config{
		SignalingId: signalingID,
		AllPeers:    true,
		BlockPeers:  []string{p1.GetPeerID().String()},
		Verbose:     true,
	}
	startTransport := func(i int) func() {
		return iceNet.startTransport(
			ctx,
			t,
			le,
			px[i].GetTestbed().Bus,
			webrtcTptConf,
			webrtc.WithICETimeouts(time.Second, 2*time.Second, 300*time.Millisecond),
		)
	}

	// startSignaling runs a signaling client for peer i and returns a
	// function that stops it.
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

	// holdLink holds a link from peer a to peer b until the test ends.
	holdLink := func(a, b *graph.Peer) {
		t.Helper()
		_, ref, err := sim.GetPeerByID(a.GetPeerID()).GetTestbed().Bus.AddDirective(
			ctx,
			link.NewEstablishLinkWithPeer(a.GetPeerID(), b.GetPeerID()),
			nil,
		)
		if err != nil {
			t.Fatal(err.Error())
		}
		t.Cleanup(ref.Release)
	}

	// Start the transports and signaling clients of both endpoint peers.
	crash := []func(){startTransport(0), nil, startTransport(2)}
	stopSignaling := []func(){startSignaling(0), nil, startSignaling(2)}
	t.Cleanup(func() {
		for i := range crash {
			if crash[i] != nil {
				crash[i]()
				stopSignaling[i]()
			}
		}
	})

	// Held from one end, the link comes up once and is not redialed after p2
	// closes it.
	holdLink(p0, p2)
	counter.wait(t, ip0, msgLinkPublished, 1, 30*time.Second)
	runFor(t, unrefLinkLifetime)
	counter.require(t, ip0, msgLinkPublished, 1)
	counter.require(t, ip0, msgSessionStarted, 1)

	// Held from both ends, one more link comes up and stays up.
	holdLink(p2, p0)
	counter.wait(t, ip0, msgLinkPublished, 2, 30*time.Second)
	runFor(t, unrefLinkLifetime)
	counter.require(t, ip0, msgLinkPublished, 2)
	counter.require(t, ip2, msgLinkPublished, 2)
	waitConnectivity(ctx, t, le, px[0], px[2], "held", 10*time.Second)

	// While p2 is gone, p0 waits on presence and starts no session.
	lost := counter.count(ip0, msgLinkLost)
	stopSignaling[2]()
	crash[2]()
	crash[2] = nil
	counter.wait(t, ip0, msgLinkLost, lost+1, 30*time.Second)
	sessions := counter.count(ip0, msgSessionStarted)
	runFor(t, 3*time.Second)
	counter.require(t, ip0, msgSessionStarted, sessions)

	// When p2 returns, the peers link once.
	crash[2] = startTransport(2)
	stopSignaling[2] = startSignaling(2)
	counter.wait(t, ip0, msgLinkPublished, 3, 30*time.Second)
	waitConnectivity(ctx, t, le, px[0], px[2], "returned", 10*time.Second)
	runFor(t, 3*time.Second)
	counter.require(t, ip0, msgLinkPublished, 3)
	counter.require(t, ip2Restarted, msgLinkPublished, 1)
}

// logCounter is a logrus hook that counts entries by transport address and
// message.
type logCounter struct {
	// bcast guards counts.
	bcast broadcast.Broadcast
	// counts maps a transport address and message to its entry count.
	counts map[[2]string]int
}

// newLogCounter constructs an empty logCounter.
func newLogCounter() *logCounter {
	return &logCounter{counts: make(map[[2]string]int)}
}

// Levels returns the levels the counter observes.
func (c *logCounter) Levels() []logrus.Level {
	return logrus.AllLevels
}

// Fire counts entry under its transport address.
func (c *logCounter) Fire(entry *logrus.Entry) error {
	ip, _ := entry.Data["transport-ip"].(string)
	if ip == "" {
		return nil
	}
	c.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		c.counts[[2]string{ip, entry.Message}]++
		broadcast()
	})
	return nil
}

// count returns the number of msg entries logged by the transport at ip.
func (c *logCounter) count(ip, msg string) int {
	var n int
	c.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		n = c.counts[[2]string{ip, msg}]
	})
	return n
}

// wait waits until the transport at ip logged msg at least n times.
func (c *logCounter) wait(t *testing.T, ip, msg string, n int, timeout time.Duration) {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		var got int
		var waitCh <-chan struct{}
		c.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			got = c.counts[[2]string{ip, msg}]
			waitCh = getWaitCh()
		})
		if got >= n {
			return
		}
		select {
		case <-t.Context().Done():
			t.Fatal(t.Context().Err())
		case <-deadline.C:
			t.Fatalf("%s: %q logged %d times in %v, want %d", ip, msg, got, timeout, n)
		case <-waitCh:
		}
	}
}

// runFor lets the transports run for d.
func runFor(t *testing.T, d time.Duration) {
	t.Helper()
	select {
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	case <-time.After(d):
	}
}

// require fails the test unless the transport at ip logged msg exactly n
// times.
func (c *logCounter) require(t *testing.T, ip, msg string, n int) {
	t.Helper()
	if got := c.count(ip, msg); got != n {
		t.Fatalf("%s: %q logged %d times, want %d", ip, msg, got, n)
	}
}

// _ is a type assertion
var _ logrus.Hook = (*logCounter)(nil)
