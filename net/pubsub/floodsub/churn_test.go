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
	// Create the FloodSub router for publication during subscription churn.
	m := newTestFloodSub(t)

	// A synchronous wake proves Execute accepted churn before the publication.
	m.wakeCh = make(chan struct{})

	// Register a subscribed receiver to observe forwarded publications.
	tpl := pubsub.PeerLinkTuple{PeerID: peer.ID("receiver"), LinkID: 1}
	receiver := &streamHandler{tpl: tpl, le: m.le, ctx: t.Context(), packetCh: make(chan *Packet, 1)}
	m.peers[tpl] = receiver
	m.peerChannels["topic"] = map[pubsub.PeerLinkTuple]struct{}{tpl: {}}

	// Run the router until test cleanup cancels and joins it.
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- m.Execute(ctx) }()
	t.Cleanup(func() { cancel(); <-done })

	// Require the router to accept subscription churn before publication.
	select {
	case m.wakeCh <- struct{}{}:
	case <-time.After(time.Second):
		t.Fatal("router did not accept subscription churn")
	}

	// Publish to the subscribed receiver while reconciliation is pending.
	started := time.Now()
	m.publishCh <- &publishChMsg{msg: &peer.SignedMsg{}, channelID: "topic"}

	// Require the publication to reach the receiver before the deadline.
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
		// Create a FloodSub router and signing key inside the virtual clock.
		m := newTestFloodSub(t)
		key, _, err := crypto.GenerateEd25519Key(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}

		// Subscribe to the test channel and count local message deliveries.
		sub, err := m.AddSubscription(t.Context(), key, "topic")
		if err != nil {
			t.Fatal(err)
		}
		defer sub.Release()
		deliveries := 0
		sub.AddHandler(func(pubsub.Message) { deliveries++ })

		// Sign a channel message whose deduplication record will expire.
		packet, inner, err := pubmessage.NewPubMessage("topic", key, hash.HashType_HashType_SHA256, []byte("payload"))
		if err != nil {
			t.Fatal(err)
		}

		// Run the router with a cancelable lifecycle in the virtual clock.
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		done := make(chan error, 1)
		go func() { done <- m.Execute(ctx) }()

		// Deliver the same publication twice and let router work settle.
		m.handleValidMessage(ctx, "", packet, inner)
		m.handleValidMessage(ctx, "", packet, inner)
		synctest.Wait()

		// Require the duplicate publication to produce only one delivery.
		if deliveries != 1 {
			t.Fatalf("duplicate delivered: %d deliveries", deliveries)
		}

		// Advance the virtual clock beyond the message record's known expiry.
		time.Sleep(seenMessageTTL + time.Nanosecond)
		synctest.Wait()

		// Read the router's retained message records after expiry processing.
		m.mtx.Lock()
		retained := len(m.seenMessages)
		m.mtx.Unlock()

		// Require expired message records to be removed from FloodSub.
		if retained != 0 {
			t.Fatalf("retained %d expired messages", retained)
		}

		// Deliver the publication again after its deduplication record expires.
		m.handleValidMessage(ctx, "", packet, inner)

		// Require a second delivery after message expiry.
		if deliveries != 2 {
			t.Fatal("expired message was still suppressed")
		}

		// Cancel the router and require its expected shutdown result.
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("unexpected shutdown: %v", err)
		}
	})
}

// TestSlowPeerSubscriptionCoalescing keeps a stalled writer bounded while
// preserving the order of states already handed to that writer.
func TestSlowPeerSubscriptionCoalescing(t *testing.T) {
	// Queue repeated channel churn while the peer writer remains stalled.
	peer := &streamHandler{subWake: make(chan struct{}, 1)}
	for range 1000 {
		peer.queueSubscriptions([]*SubscriptionOpts{{ChannelId: "topic", Subscribe: true}})
		peer.queueSubscriptions([]*SubscriptionOpts{{ChannelId: "topic", Subscribe: false}})
	}

	// Require transient channel states to vanish before reaching the writer.
	if packet := peer.takeSubscriptions(); packet != nil {
		t.Fatal("transient subscriptions reached the stalled writer")
	}

	// Transfer the final subscribe state to the peer writer.
	peer.queueSubscriptions([]*SubscriptionOpts{{ChannelId: "topic", Subscribe: true}})
	first := peer.takeSubscriptions()

	// Require the first packet to announce the channel subscription.
	if len(first.GetSubscriptions()) != 1 || !first.GetSubscriptions()[0].GetSubscribe() {
		t.Fatal("missing initial subscribe")
	}

	// Transfer the unsubscribe state after the in-flight subscribe.
	peer.queueSubscriptions([]*SubscriptionOpts{{ChannelId: "topic", Subscribe: false}})
	last := peer.takeSubscriptions()

	// Require the second packet to remove the channel subscription.
	if len(last.GetSubscriptions()) != 1 || last.GetSubscriptions()[0].GetSubscribe() {
		t.Fatal("unsubscribe overtook the in-flight subscribe")
	}

	// Require the quiescent peer to retain no subscription state.
	if len(peer.pending) != 0 || len(peer.announced) != 0 {
		t.Fatal("quiescent peer retained subscription state")
	}
}
