package floodsub

import (
	"context"
	"testing"
	"time"

	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/pubsub"
	"github.com/sirupsen/logrus"
)

// newTestFloodSub builds a FloodSub for unit tests.
func newTestFloodSub(t *testing.T) *FloodSub {
	ps, err := NewFloodSub(context.Background(), logrus.NewEntry(logrus.New()), nil, &Config{})
	if err != nil {
		t.Fatal(err)
	}
	return ps.(*FloodSub)
}

// TestExecPublishSkipsPendingAndSlowPeers tests that publishing neither
// panics on a not-yet-started session nor blocks on a full peer queue.
func TestExecPublishSkipsPendingAndSlowPeers(t *testing.T) {
	m := newTestFloodSub(t)
	le := logrus.NewEntry(logrus.New())
	pending := pubsub.PeerLinkTuple{PeerID: peer.ID("pending"), LinkID: 1}
	slow := pubsub.PeerLinkTuple{PeerID: peer.ID("slow"), LinkID: 2}
	m.peers[pending] = &streamHandler{tpl: pending, le: le, packetCh: make(chan *Packet, 1)}
	slowSh := &streamHandler{
		tpl:       slow,
		le:        le,
		packetCh:  make(chan *Packet), // unbuffered: always full
		ctx:       context.Background(),
		ctxCancel: func() {},
	}
	m.peers[slow] = slowSh
	m.peerChannels["ch"] = map[pubsub.PeerLinkTuple]struct{}{pending: {}, slow: {}}

	done := make(chan struct{})
	go func() {
		defer close(done)
		m.execPublish("", &publishChMsg{msg: &peer.SignedMsg{}, channelID: "ch"})
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("execPublish blocked on slow peer")
	}
	if len(m.peers[pending].packetCh) != 0 {
		t.Fatal("expected no packet queued to pending session")
	}
}

// TestRemovePeerChannels tests that a peer is removed from peerChannels.
func TestRemovePeerChannels(t *testing.T) {
	m := newTestFloodSub(t)
	a := pubsub.PeerLinkTuple{PeerID: peer.ID("a"), LinkID: 1}
	b := pubsub.PeerLinkTuple{PeerID: peer.ID("b"), LinkID: 2}
	m.peerChannels["x"] = map[pubsub.PeerLinkTuple]struct{}{a: {}}
	m.peerChannels["y"] = map[pubsub.PeerLinkTuple]struct{}{a: {}, b: {}}
	m.removePeerChannelsLocked(a)
	if _, ok := m.peerChannels["x"]; ok {
		t.Fatal("expected empty channel x to be removed")
	}
	if len(m.peerChannels["y"]) != 1 {
		t.Fatalf("expected only b in y, got %v", m.peerChannels["y"])
	}
}
