package floodsub

import (
	"context"
	"crypto/rand"
	"errors"
	"net"
	"testing"
	"testing/synctest"
	"time"

	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/pubsub"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
)

// TestSlowPeerReconciliation keeps forwarding while a subscription write stalls.
func TestSlowPeerReconciliation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Connect a peer whose transport cannot write until its remote end reads.
		m := newTestFloodSub(t)
		local, remote := net.Pipe()
		defer remote.Close()
		conn := &observedConn{Conn: local, writes: make(chan struct{}, 1)}
		slowTpl := pubsub.PeerLinkTuple{PeerID: peer.ID("slow"), LinkID: 1}
		slow := &streamHandler{
			m: m, le: m.le, tpl: slowTpl,
			stream:   stream_packet.NewSession(conn, maxMessageSize),
			packetCh: make(chan *Packet, 32), subWake: make(chan struct{}, 1),
		}
		m.peers[slowTpl] = slow
		m.incSessions = append(m.incSessions, slow)

		// Observe the fast peer's publication queue independently of the stalled pipe.
		fastTpl := pubsub.PeerLinkTuple{PeerID: peer.ID("fast"), LinkID: 2}
		fast := &streamHandler{
			tpl: fastTpl, le: m.le, ctx: t.Context(),
			packetCh: make(chan *Packet, 1), subWake: make(chan struct{}, 1),
		}
		m.peers[fastTpl] = fast
		m.peerChannels["topic"] = map[pubsub.PeerLinkTuple]struct{}{slowTpl: {}, fastTpl: {}}

		// Announce a channel subscription before starting the peer writer.
		key, _, err := crypto.GenerateEd25519Key(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		sub, err := m.AddSubscription(t.Context(), key, "churn")
		if err != nil {
			t.Fatal(err)
		}
		defer sub.Release()

		// Start the router and stop the peer inside its first subscription write.
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		done := make(chan error, 1)
		go func() { done <- m.Execute(ctx) }()
		synctest.Wait()
		select {
		case <-conn.writes:
		default:
			t.Fatal("slow peer did not start its subscription write")
		}

		// Forward while subscription churn exceeds the stalled peer's queue capacity.
		for i := range 40 {
			// Alternate the desired subscription at each known reconciliation deadline.
			sub.Release()
			if i%2 != 0 {
				sub, err = m.AddSubscription(ctx, key, "churn")
				if err != nil {
					t.Fatal(err)
				}
			}
			synctest.Wait()
			time.Sleep(100 * time.Millisecond)
			synctest.Wait()

			// Forward to the fast peer while the slow writer remains blocked.
			m.publishCh <- &publishChMsg{msg: &peer.SignedMsg{}, channelID: "topic"}
			synctest.Wait()
			select {
			case <-fast.packetCh:
			default:
				t.Fatal("slow subscription writer suspended central publication")
			}
		}

		// Resume the slow peer and require subscribe followed by final unsubscribe.
		session := stream_packet.NewSession(remote, maxMessageSize)
		first := &Packet{}
		if err := session.RecvMsg(first); err != nil {
			t.Fatal(err)
		}
		if len(first.GetSubscriptions()) != 1 || !first.GetSubscriptions()[0].GetSubscribe() {
			t.Fatalf("missing in-flight subscription: %v", first)
		}

		// Reconcile the final unsubscribe after the initial packet reaches the peer.
		sub.Release()
		synctest.Wait()
		time.Sleep(100 * time.Millisecond)
		synctest.Wait()

		// Drain queued publications until the ordered unsubscribe reaches the peer.
		var last *Packet
		for range 33 {
			packet := &Packet{}
			if err := session.RecvMsg(packet); err != nil {
				t.Fatal(err)
			}
			if len(packet.GetSubscriptions()) != 0 {
				last = packet
				break
			}
		}
		if len(last.GetSubscriptions()) != 1 || last.GetSubscriptions()[0].GetSubscribe() {
			t.Fatalf("unsubscribe overtook subscribe: %v", last)
		}

		// Cancel with queued publications and require blocked transport work to exit.
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("unexpected shutdown: %v", err)
		}
	})
}
