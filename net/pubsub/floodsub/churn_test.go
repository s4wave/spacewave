package floodsub

import (
	"context"
	"crypto/rand"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/pubsub"
	"github.com/s4wave/spacewave/net/pubsub/util/pubmessage"
)

// TestPublishDuringPendingReconciliation measures the router's publication path.
func TestPublishDuringPendingReconciliation(t *testing.T) {
	m := newTestFloodSub(t)
	// A synchronous wake proves Execute accepted churn before the publication.
	m.wakeCh = make(chan struct{})
	tpl := pubsub.PeerLinkTuple{PeerID: peer.ID("receiver"), LinkID: 1}
	receiver := &streamHandler{tpl: tpl, le: m.le, ctx: t.Context(), packetCh: make(chan *Packet, 1)}
	m.peers[tpl] = receiver
	m.peerChannels["topic"] = map[pubsub.PeerLinkTuple]struct{}{tpl: {}}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- m.Execute(ctx) }()
	t.Cleanup(func() { cancel(); <-done })
	select {
	case m.wakeCh <- struct{}{}:
	case <-time.After(time.Second):
		t.Fatal("router did not accept subscription churn")
	}
	started := time.Now()
	m.publishCh <- &publishChMsg{msg: &peer.SignedMsg{}, channelID: "topic"}
	select {
	case <-receiver.packetCh:
		t.Logf("publication during churn: %s", time.Since(started))
	case <-time.After(time.Second):
		t.Fatal("pending reconciliation suspended publication")
	}
}

// TestMessageExpiryAndCancellation checks deduplication and cleanup at its known
// deadline without waiting for wall-clock time or a periodic topology scan.
func TestMessageExpiryAndCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := newTestFloodSub(t)
		key, _, err := crypto.GenerateEd25519Key(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		sub, err := m.AddSubscription(t.Context(), key, "topic")
		if err != nil {
			t.Fatal(err)
		}
		defer sub.Release()
		deliveries := 0
		sub.AddHandler(func(pubsub.Message) { deliveries++ })
		packet, inner, err := pubmessage.NewPubMessage("topic", key, hash.HashType_HashType_SHA256, []byte("payload"))
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		done := make(chan error, 1)
		go func() { done <- m.Execute(ctx) }()
		m.handleValidMessage(ctx, "", packet, inner)
		m.handleValidMessage(ctx, "", packet, inner)
		synctest.Wait()
		if deliveries != 1 {
			t.Fatalf("duplicate delivered: %d deliveries", deliveries)
		}
		time.Sleep(seenMessageTTL + time.Nanosecond)
		synctest.Wait()
		m.mtx.Lock()
		retained := len(m.seenMessages)
		m.mtx.Unlock()
		if retained != 0 {
			t.Fatalf("retained %d expired messages", retained)
		}
		m.handleValidMessage(ctx, "", packet, inner)
		if deliveries != 2 {
			t.Fatal("expired message was still suppressed")
		}
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("unexpected shutdown: %v", err)
		}
	})
}

// TestSlowPeerSubscriptionCoalescing keeps a stalled writer bounded while
// preserving the order of states already handed to that writer.
func TestSlowPeerSubscriptionCoalescing(t *testing.T) {
	peer := &streamHandler{subWake: make(chan struct{}, 1)}
	for range 1000 {
		peer.queueSubscriptions([]*SubscriptionOpts{{ChannelId: "topic", Subscribe: true}})
		peer.queueSubscriptions([]*SubscriptionOpts{{ChannelId: "topic", Subscribe: false}})
	}
	if packet := peer.takeSubscriptions(); packet != nil {
		t.Fatal("transient subscriptions reached the stalled writer")
	}
	peer.queueSubscriptions([]*SubscriptionOpts{{ChannelId: "topic", Subscribe: true}})
	first := peer.takeSubscriptions()
	if len(first.GetSubscriptions()) != 1 || !first.GetSubscriptions()[0].GetSubscribe() {
		t.Fatal("missing initial subscribe")
	}
	peer.queueSubscriptions([]*SubscriptionOpts{{ChannelId: "topic", Subscribe: false}})
	last := peer.takeSubscriptions()
	if len(last.GetSubscriptions()) != 1 || last.GetSubscriptions()[0].GetSubscribe() {
		t.Fatal("unsubscribe overtook the in-flight subscribe")
	}
	if len(peer.pending) != 0 || len(peer.announced) != 0 {
		t.Fatal("quiescent peer retained subscription state")
	}
}
