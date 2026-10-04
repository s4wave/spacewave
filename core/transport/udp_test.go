//go:build !tinygo

package transport_test

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
)

// TestSessionTransportUDPAuthorizesPeers checks that a UDP link mounts only
// after the listening Session's authorizer admits the dialing peer.
func TestSessionTransportUDPAuthorizesPeers(t *testing.T) {
	for _, tc := range []struct {
		name  string
		admit bool
	}{
		{name: "refused", admit: false},
		{name: "admitted", admit: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Build a listener that reports each decision and a dialer that
			// admits any peer.
			var dialerID peer.ID
			asked := make(chan peer.ID, 1)
			ctx, _, listener := newTestSessionTransport(t, "", transport.WithPeerAuthorizer(
				func(_ context.Context, remote peer.ID) error {
					select {
					case asked <- remote:
					default:
					}
					if !tc.admit || remote != dialerID {
						return errors.New("not an account session")
					}
					return nil
				},
			))
			_, _, dialer := newTestSessionTransport(t, "", transport.WithPeerAuthorizer(
				func(context.Context, peer.ID) error { return nil },
			))
			dialerID = dialer.GetPeerID()
			for _, st := range []*transport.SessionTransport{listener, dialer} {
				go func() { _ = st.Execute(ctx) }()
				if err := st.AwaitReady(ctx); err != nil {
					t.Fatal(err)
				}
			}

			// Listen on loopback and dial the listener from the other Session.
			addr, err := listener.StartUDP(ctx, "127.0.0.1:0", nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := dialer.StartUDP(ctx, "127.0.0.1:0", map[peer.ID]string{
				listener.GetPeerID(): addr.String(),
			}); err != nil {
				t.Fatal(err)
			}
			_, elRef, err := dialer.GetChildBus().AddDirective(
				link.NewEstablishLinkWithPeer(dialerID, listener.GetPeerID()),
				nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			defer elRef.Release()

			// The listener decides on the dialer before any link mounts.
			select {
			case remote := <-asked:
				if remote != dialerID {
					t.Fatalf("asked about %s, want %s", remote, dialerID)
				}
			case <-ctx.Done():
				t.Fatal("listener never authorized the dialer")
			}
			if !tc.admit {
				linked, _ := listener.GetLinkedPeerIDsSnapshotWithWait([]peer.ID{dialerID})
				if _, ok := linked[dialerID]; ok {
					t.Fatal("refused peer mounted a link")
				}
				return
			}

			// An admitted dialer links.
			for {
				linked, waitChs := listener.GetLinkedPeerIDsSnapshotWithWait([]peer.ID{dialerID})
				if _, ok := linked[dialerID]; ok {
					return
				}
				select {
				case <-waitAny(waitChs):
				case <-ctx.Done():
					t.Fatal("admitted peer did not link")
				}
			}
		})
	}
}
